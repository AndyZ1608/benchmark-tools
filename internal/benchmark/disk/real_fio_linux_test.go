//go:build linux && integration

package disk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/safety"
)

// Replay explicit research captures through the final validator, independently
// of the old executable that collected them. Expected settings come from the
// integration case names, not from trusting the fio options being checked.
func TestReplayRealFioEvidence(t *testing.T) {
	directory := os.Getenv("VMBENCH_FIO_EVIDENCE")
	if directory == "" {
		t.Skip("requires captured integration evidence")
	}
	files, err := filepath.Glob(filepath.Join(directory, "binary*-*.json"))
	if err != nil || len(files) == 0 {
		t.Fatal("missing research captures", err)
	}
	differences := 0
	for _, path := range files {
		parts := strings.SplitN(strings.TrimSuffix(filepath.Base(path), ".json"), "-", 3)
		if len(parts) != 3 {
			t.Fatal(path)
		}
		c, _ := config.Defaults("quick")
		c.Engine = parts[1]
		c.Duration = 1
		c.Warmup = 1
		c.Jobs = 1
		c.Depth = 2
		label := parts[2]
		rw := "randread"
		switch {
		case label == "original-qd2":
			c.Duration = 5
		case label == "no-ramp":
			c.Warmup = 0
		case label == "two-jobs":
			c.Depth = 8
			c.Jobs = 2
		case label == "mixed":
			c.Depth = 8
			c.Jobs = 2
			c.Profile = "mixed-70-30"
			c.ConfirmWrites = true
			rw = "randrw"
		case strings.HasPrefix(label, "qd"):
			c.Depth, err = strconv.Atoi(strings.TrimPrefix(label, "qd"))
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("unknown captured workload", label)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := Parse(data)
		if err != nil {
			t.Fatal(path, err)
		}
		r.Accounting, err = validateMeasured(r, c, rw, c.Depth, false)
		if err != nil {
			t.Fatal(path, err)
		}
		if _, err := ExecutionEvidence(r, c.Engine, c.Depth); err != nil {
			t.Fatal(path, err)
		}
		for _, obs := range r.Accounting {
			if obs.ByteDifference != "0" {
				differences++
			}
		}
	}
	t.Logf("validated %d real JSON captures; %d bounded ramp-accounting observations", len(files), differences)
}

// Explicit opt-in only. This runs real bounded reads and one confirmed mixed
// workload inside unique owned directories, never against devices/user files.
func TestRealFioAsyncAccounting(t *testing.T) {
	if os.Getenv("VMBENCH_REAL_FIO") != "1" {
		t.Skip("requires explicitly authorized disposable integration environment")
	}
	if os.Geteuid() == 0 {
		t.Fatal("run this integration test as a regular user")
	}
	binaries := filepath.SplitList(os.Getenv("VMBENCH_FIO_BINARIES"))
	if len(binaries) == 0 {
		t.Fatal("set VMBENCH_FIO_BINARIES to trusted absolute fio executable paths")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	enginePath := filepath.Join(filepath.Dir(exe), "engines", "fio")
	if err := os.MkdirAll(filepath.Dir(enginePath), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(enginePath); !os.IsNotExist(err) {
		t.Fatal("integration engine path already exists", enginePath, err)
	}
	t.Cleanup(func() { os.Remove(enginePath) })
	for index, binary := range binaries {
		if !filepath.IsAbs(binary) {
			t.Fatal("fio path must be absolute", binary)
		}
		if err := os.Symlink(binary, enginePath); err != nil {
			t.Fatal(err)
		}
		for _, engine := range []string{"io_uring", "libaio"} {
			t.Run(fmt.Sprintf("binary%d/%s", index, engine), func(t *testing.T) {
				c, _ := config.Defaults("quick")
				c.Engine = engine
				c.Path = t.TempDir()
				c.FileSize = "8MiB"
				c.Jobs = 2
				c.DiskSizeExplicit = true
				c.Repetitions = 1
				c.Warmup = 1
				c.Duration = 1
				sentinel := filepath.Join(c.Path, "user-data")
				if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				selected, err := SelectEngine(ctx, c, safety.ReserveBytes)
				if err != nil || selected != engine {
					t.Fatal("functional direct-I/O probe", selected, err)
				}
				session, err := OpenSession(ctx, c, safety.ReserveBytes)
				if err != nil {
					t.Fatal(err)
				}
				defer session.Close()
				measure := func(label string, current config.Config) {
					t.Helper()
					out, err := session.Measure(ctx, current)
					if err != nil {
						t.Fatal(label, err)
					}
					r := out.Results[0]
					evidence, err := ExecutionEvidence(r, current.Engine, current.Depth)
					if err != nil {
						t.Fatal(err)
					}
					t.Log(label, r.Version, evidence)
					for _, obs := range r.Accounting {
						t.Logf("%s %s io_bytes=%d total_ios=%d difference=%s", obs.Job, obs.Direction, obs.IOBytes, obs.TotalIOs, obs.ByteDifference)
					}
					if directory := os.Getenv("VMBENCH_FIO_EVIDENCE"); directory != "" {
						if err := os.MkdirAll(directory, 0700); err != nil {
							t.Fatal(err)
						}
						name := fmt.Sprintf("binary%d-%s-%s.json", index, engine, label)
						if err := os.WriteFile(filepath.Join(directory, name), out.Raw[0], 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
				current := c
				current.Jobs = 1
				current.Depth = 2
				current.Duration = 5
				measure("original-qd2", current)
				current.Duration = 1
				for _, depth := range []int{1, 2, 4, 8, 16, 32, 64} {
					current.Depth = depth
					measure(fmt.Sprintf("qd%d", depth), current)
				}
				current.Depth = 2
				current.Warmup = 0
				measure("no-ramp", current)
				current = c
				current.Depth = 8
				measure("two-jobs", current)
				current.Profile = "mixed-70-30"
				current.ConfirmWrites = true
				measure("mixed", current)
				if err := session.Close(); err != nil {
					t.Fatal(err)
				}
				entries, _ := os.ReadDir(c.Path)
				if len(entries) != 1 || entries[0].Name() != "user-data" {
					t.Fatal("unsafe cleanup", entries)
				}
				if b, _ := os.ReadFile(sentinel); string(b) != "keep" {
					t.Fatal("user sentinel modified")
				}
			})
		}
		// Inject a real EINVAL from a one-byte direct read in the owned file.
		// The wrapper changes this failure test only, never production parameters.
		if err := os.Remove(enginePath); err != nil {
			t.Fatal(err)
		}
		quoted := "'" + strings.ReplaceAll(binary, "'", "'\"'\"'") + "'"
		wrapper := "#!/bin/sh\ncase \"$1\" in --version|--enghelp) exec " + quoted + " \"$@\";; esac\nexec " + quoted + " \"$@\" --bs=1\n"
		if err := os.WriteFile(enginePath, []byte(wrapper), 0700); err != nil {
			t.Fatal(err)
		}
		c, _ := config.Defaults("quick")
		c.Engine = "io_uring"
		c.Path = t.TempDir()
		c.FileSize = "1MiB"
		c.Jobs = 1
		c.Depth = 2
		c.Warmup = 0
		c.Repetitions = 1
		c.Duration = 1
		session, err := OpenSession(context.Background(), c, safety.ReserveBytes)
		if err != nil {
			t.Fatal(err)
		}
		out, err := session.Measure(context.Background(), c)
		cleanup := session.Close()
		if err == nil || cleanup != nil || !session.Lifecycle.CleanupCompleted {
			t.Fatal("real direct-I/O failure accepted or cleanup failed", out, err, cleanup)
		}
		t.Log("real EINVAL rejected:", err)
		if entries, _ := os.ReadDir(c.Path); len(entries) != 0 {
			t.Fatal("real failure left data", entries)
		}
		if err := os.Remove(enginePath); err != nil {
			t.Fatal(err)
		}
	}
}
