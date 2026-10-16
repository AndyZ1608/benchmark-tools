package disk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailedFioOutputOptInAndCleanup(t *testing.T) {
	fixtureEngine(t)
	for _, mode := range []string{"accounting", "large-mismatch", "short", "job-error"} {
		for _, retain := range []bool{false, true} {
			t.Run(mode+"/"+map[bool]string{false: "normal", true: "diagnostic"}[retain], func(t *testing.T) {
				t.Setenv("VMBENCH_FIO_MODE", mode)
				c := diskConfig(t)
				c.Jobs = 1
				c.Depth = 2
				c.Warmup = 1
				directory := filepath.Join(t.TempDir(), "failed-fio")
				if retain {
					c.FIODiagnostics = directory
				}
				sentinel := filepath.Join(c.Path, "user-data")
				os.WriteFile(sentinel, []byte("keep"), 0600)
				out, err := RunManaged(context.Background(), c, "")
				if (err == nil) != (mode == "accounting") || !out.Lifecycle.CleanupCompleted {
					t.Fatal(out, err)
				}
				if mode == "accounting" {
					if out.Results[0].Accounting[0].ByteDifference != "+4096" {
						t.Fatal(out.Results)
					}
					if _, e := os.Stat(directory); !os.IsNotExist(e) {
						t.Fatal("successful sample wrote diagnostics", e)
					}
				} else if retain {
					files, e := os.ReadDir(directory)
					if e != nil || len(files) != 1 {
						t.Fatal(files, e)
					}
					path := filepath.Join(directory, files[0].Name())
					b, e := os.ReadFile(path)
					if e != nil || !strings.Contains(string(b), `"jobname":"job-001"`) {
						t.Fatal(string(b), e)
					}
					info, _ := os.Stat(path)
					if info.Mode().Perm() != 0600 {
						t.Fatal(info.Mode())
					}
					if !strings.Contains(err.Error(), "Raw fio output") {
						t.Fatal(err)
					}
				} else if _, e := os.Stat(directory); !os.IsNotExist(e) {
					t.Fatal("normal run retained output", e)
				}
				entries, _ := os.ReadDir(c.Path)
				if len(entries) != 1 || entries[0].Name() != "user-data" {
					t.Fatal("workspace cleanup failed", entries)
				}
				if b, _ := os.ReadFile(sentinel); string(b) != "keep" {
					t.Fatal("user data changed")
				}
			})
		}
	}
}

func TestDiagnosticWriteFailurePreservesOriginalErrorAndUserFile(t *testing.T) {
	fixtureEngine(t)
	t.Setenv("VMBENCH_FIO_MODE", "short")
	c := diskConfig(t)
	file := filepath.Join(t.TempDir(), "user-config")
	os.WriteFile(file, []byte("keep"), 0600)
	c.FIODiagnostics = file
	out, err := RunManaged(context.Background(), c, "")
	if err == nil || !strings.Contains(err.Error(), "short or dropped") || !strings.Contains(err.Error(), "retain failed fio output") || !out.Lifecycle.CleanupCompleted {
		t.Fatal(out, err)
	}
	if b, _ := os.ReadFile(file); string(b) != "keep" {
		t.Fatal("diagnostics overwrote user file")
	}
}
