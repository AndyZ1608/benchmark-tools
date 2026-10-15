package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/runner"
)

// The helper exercises process execution, inherited files and lifecycle control;
// its synthetic metrics are never treated as real fio measurements.
func TestMain(m *testing.M) {
	if os.Getenv("VMBENCH_FIO_HELPER") == "1" {
		fakeFio()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fakeFio() {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("fio-3.39-fixture")
		return
	}
	if len(args) == 1 && args[0] == "--enghelp" {
		fmt.Println("io_uring libaio")
		return
	}
	get := func(key string) string {
		for _, arg := range args {
			if strings.HasPrefix(arg, "--"+key+"=") {
				return strings.TrimPrefix(arg, "--"+key+"=")
			}
		}
		return ""
	}
	if get("thread") != "1" || get("allow_file_create") != "0" || get("unlink") != "0" || get("fallocate") != "none" || get("time_based") != "1" {
		fmt.Fprintln(os.Stderr, "unsafe or untimed fio fixture invocation")
		os.Exit(91)
	}
	size, _ := strconv.ParseInt(get("size"), 10, 64)
	names := []string{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--filename=") {
			names = append(names, strings.TrimPrefix(arg, "--filename="))
		}
	}
	if len(names) == 0 {
		os.Exit(92)
	}
	for i, name := range names {
		if name != fmt.Sprintf("/proc/self/fd/%d", 3+i) {
			fmt.Fprintln(os.Stderr, "unexpected filename", name)
			os.Exit(93)
		}
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(94)
		}
		st, _ := f.Stat()
		data := make([]byte, 4096)
		n, err := f.ReadAt(data, 0)
		f.Close()
		if err != nil || n != 4096 || st.Size() != size || !st.Mode().IsRegular() {
			fmt.Fprintln(os.Stderr, "file not fully prepared")
			os.Exit(95)
		}
		zero := true
		for _, b := range data {
			if b != 0 {
				zero = false
				break
			}
		}
		if zero {
			fmt.Fprintln(os.Stderr, "trivial preparation data")
			os.Exit(96)
		}
	}
	marker := os.Getenv("VMBENCH_FIO_MARKER")
	if marker != "" {
		os.WriteFile(marker, []byte("started"), 0600)
	}
	mode := os.Getenv("VMBENCH_FIO_MODE")
	if mode == "hang" {
		time.Sleep(60 * time.Second)
		return
	}
	if mode == "cleanup-fail" {
		dir, err := os.Readlink("/proc/self/fd/3")
		if err != nil {
			os.Exit(97)
		}
		os.WriteFile(filepath.Join(filepath.Dir(dir), "untracked-user-file"), []byte("keep"), 0600)
	}
	if mode == "failure" || mode == "cleanup-fail" {
		fmt.Fprintln(os.Stderr, "fio fixture I/O failure")
		os.Exit(9)
	}
	if mode == "invalid" {
		fmt.Println(`{"jobs":[]}`)
		return
	}
	runTime, _ := strconv.ParseFloat(get("runtime"), 64)
	value := Direction{IOPS: 100, BWBytes: 409600, Runtime: runTime * 1000, Completion: Latency{Mean: 10000, Percentile: map[string]float64{"99.000000": 20000}}}
	result := Result{Options: map[string]string{"ioengine": get("ioengine")}}
	for range names {
		job := Job{Depth: map[string]float64{get("iodepth"): 100}}
		switch get("rw") {
		case "randrw":
			job.Read, job.Write = value, value
		case "write", "randwrite":
			job.Write = value
		default:
			job.Read = value
		}
		result.Jobs = append(result.Jobs, job)
	}
	json.NewEncoder(os.Stdout).Encode(result)
}
func fixtureEngine(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(exe), "engines", "fio")
	os.MkdirAll(filepath.Dir(path), 0700)
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("fixture engine path already exists: %s (%v)", path, err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })
	t.Setenv("VMBENCH_FIO_HELPER", "1")
}
func diskConfig(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Defaults("quick")
	if err != nil {
		t.Fatal(err)
	}
	c.Path = t.TempDir()
	c.FileSize = "64KiB"
	c.DiskSizeExplicit = true
	c.Duration = 1
	c.Warmup = 0
	c.Jobs = 4
	c.DataSeed = "42"
	return c
}
func TestFourJobReadLifecycleAndPreparationNotTimed(t *testing.T) {
	fixtureEngine(t)
	c := diskConfig(t)
	sentinel := filepath.Join(c.Path, "existing-data.img")
	os.WriteFile(sentinel, []byte("important"), 0600)
	phases := []string{}
	prepared := false
	ctx := runner.WithObserver(context.Background(), func(event runner.Event) {
		if event.Phase != "" {
			phases = append(phases, event.Phase)
			if event.Phase == "prepared" {
				prepared = true
			}
		}
		if event.Name == "fio" && event.Starting {
			for _, arg := range event.Args {
				if arg == "--time_based=1" && !prepared {
					t.Error("measurement began before preparation")
				}
			}
		}
	})
	out, err := RunManaged(ctx, c, "ignored")
	if err != nil || len(out.Results) != 1 || len(out.Results[0].Jobs) != 4 || !out.Lifecycle.Prepared || !out.Lifecycle.CleanupCompleted || out.Lifecycle.TotalBytes != 4*65536 {
		t.Fatal(out, err)
	}
	for _, job := range out.Results[0].Jobs {
		if job.Read.Runtime != 1000 {
			t.Fatal("preparation affected reported runtime", job)
		}
	}
	if !prepared || len(phases) < 5 {
		t.Fatal(phases)
	}
	entries, err := os.ReadDir(c.Path)
	if err != nil || len(entries) != 1 || entries[0].Name() != "existing-data.img" {
		t.Fatal(entries, err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "important" {
		t.Fatal("existing data modified", err)
	}
}
func TestWriteAndMixedProfilesRetainConfirmationAndMetrics(t *testing.T) {
	fixtureEngine(t)
	for _, profile := range []string{"randwrite-4k", "seqwrite-1m", "mixed-70-30", "seqread-1m", "latency", "qd-scaling"} {
		t.Run(profile, func(t *testing.T) {
			c := diskConfig(t)
			c.Profile = profile
			if strings.Contains(profile, "1m") {
				c.FileSize = "1MiB"
			}
			write := strings.Contains(profile, "write") || profile == "mixed-70-30"
			if write {
				if _, err := RunManaged(context.Background(), c, ""); err == nil {
					t.Fatal("confirmation not enforced")
				}
				c.ConfirmWrites = true
			}
			out, err := RunManaged(context.Background(), c, "")
			if err != nil || !out.Lifecycle.CleanupCompleted {
				t.Fatal(err)
			}
			if write && out.Results[0].Jobs[0].Write.IOPS != 100 {
				t.Fatal(out.Results)
			}
			if profile == "mixed-70-30" && out.Results[0].Jobs[0].Read.IOPS != 100 {
				t.Fatal("mixed read metrics lost")
			}
			if profile == "qd-scaling" && len(out.Results) != 5 {
				t.Fatal(out.Results)
			}
			entries, _ := os.ReadDir(c.Path)
			if len(entries) != 0 {
				t.Fatal(entries)
			}
		})
	}
}
func TestFioFailureAndInvalidResultsCleanup(t *testing.T) {
	fixtureEngine(t)
	for _, mode := range []string{"failure", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("VMBENCH_FIO_MODE", mode)
			c := diskConfig(t)
			out, err := RunManaged(context.Background(), c, "")
			if err == nil || !out.Lifecycle.CleanupCompleted {
				t.Fatal(out, err)
			}
			entries, _ := os.ReadDir(c.Path)
			if len(entries) != 0 {
				t.Fatal("failed benchmark leaked files", entries)
			}
		})
	}
}
func TestCancellationAndTimeoutCleanupAfterEngineStops(t *testing.T) {
	fixtureEngine(t)
	t.Setenv("VMBENCH_FIO_MODE", "hang")
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			c := diskConfig(t)
			marker := filepath.Join(t.TempDir(), "engine-started")
			t.Setenv("VMBENCH_FIO_MARKER", marker)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if timeout {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 2*time.Second)
				defer stop()
			}
			if !timeout {
				go func() {
					for ctx.Err() == nil {
						if _, err := os.Stat(marker); err == nil {
							cancel()
							return
						}
						time.Sleep(10 * time.Millisecond)
					}
				}()
			}
			out, err := RunManaged(ctx, c, "")
			expected := context.Canceled
			if timeout {
				expected = context.DeadlineExceeded
			}
			if !errors.Is(err, expected) || !out.Lifecycle.CleanupCompleted {
				t.Fatal(out, err)
			}
			if _, err = os.Stat(marker); err != nil {
				t.Fatal("engine was not started", err)
			}
			entries, _ := os.ReadDir(c.Path)
			if len(entries) != 0 {
				t.Fatal("cancellation leaked files", entries)
			}
		})
	}
}
func TestCleanupFailurePreservesOriginalEngineErrorAndMetadata(t *testing.T) {
	fixtureEngine(t)
	t.Setenv("VMBENCH_FIO_MODE", "cleanup-fail")
	c := diskConfig(t)
	out, err := RunManaged(context.Background(), c, "")
	if err == nil || !strings.Contains(err.Error(), "fio fixture I/O failure") || !strings.Contains(err.Error(), "cleanup incomplete") || out.Lifecycle.CleanupCompleted {
		t.Fatal(out, err)
	}
	if data, err := os.ReadFile(filepath.Join(out.Lifecycle.Workspace, "untracked-user-file")); err != nil || string(data) != "keep" {
		t.Fatal("untracked file changed", err)
	}
	if _, err = os.Stat(filepath.Join(out.Lifecycle.Workspace, ".vmbench-owner")); err != nil {
		t.Fatal("orphan metadata missing", err)
	}
}
func TestMissingEngineAndLoaderHavePathContext(t *testing.T) {
	c := diskConfig(t)
	if _, err := RunManaged(context.Background(), c, ""); err == nil || !strings.Contains(err.Error(), "engines/fio") {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	path := filepath.Join(filepath.Dir(exe), "engines", "fio")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("#!/missing-vmbench-fixture-interpreter\n"), 0700)
	defer os.Remove(path)
	out, err := RunManaged(context.Background(), c, "")
	if err == nil || !strings.Contains(err.Error(), "cannot execute engine fio") || !strings.Contains(err.Error(), "ELF interpreter") {
		t.Fatal(err)
	}
	if out.Lifecycle.Workspace != "" {
		t.Fatal("prepared before engine dependency check")
	}
}

func TestMissingSharedLibraryErrorBeforePreparation(t *testing.T) {
	c := diskConfig(t)
	exe, _ := os.Executable()
	path := filepath.Join(filepath.Dir(exe), "engines", "fio")
	os.MkdirAll(filepath.Dir(path), 0700)
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'error while loading shared libraries: libfixture-missing.so: cannot open shared object file' >&2\nexit 127\n"), 0700); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	out, err := RunManaged(context.Background(), c, "")
	if err == nil || !strings.Contains(err.Error(), "libfixture-missing.so") || !strings.Contains(err.Error(), "engines/fio") || out.Lifecycle.Workspace != "" {
		t.Fatal(out, err)
	}
}
