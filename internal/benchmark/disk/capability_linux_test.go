package disk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"vmbench/internal/safety"
)

func TestFunctionalProbeFallbackErrorsAndCleanup(t *testing.T) {
	fixtureEngine(t)
	for _, test := range []struct{ mode, request, want string }{
		{"", "auto", "io_uring"}, {"uring-denied", "auto", "libaio"}, {"uring-denied", "io_uring", ""},
		{"aio-fails", "libaio", ""}, {"depth-one", "auto", ""},
	} {
		t.Run(test.mode+test.request, func(t *testing.T) {
			t.Setenv("VMBENCH_FIO_PROBE_MODE", test.mode)
			c := diskConfig(t)
			c.Engine = test.request
			sentinel := filepath.Join(c.Path, "sentinel")
			if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			engine, err := SelectEngine(context.Background(), c, safety.ReserveBytes)
			if engine != test.want || (err == nil) != (test.want != "") {
				t.Fatal(engine, err)
			}
			if test.mode == "aio-fails" && !strings.Contains(err.Error(), "io_submit: Input/output error") {
				t.Fatal(err)
			}
			if test.mode == "depth-one" && !strings.Contains(err.Error(), "measurement limitation") {
				t.Fatal(err)
			}
			entries, e := os.ReadDir(c.Path)
			if e != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
				t.Fatal(entries, e)
			}
			data, e := os.ReadFile(sentinel)
			if e != nil || string(data) != "keep" {
				t.Fatal("unrelated file changed", e)
			}
		})
	}
}
func TestRealAsyncCapabilityDesignatedDirectory(t *testing.T) {
	binary, target := os.Getenv("VMBENCH_REAL_FIO"), os.Getenv("VMBENCH_REAL_DISK_DIR")
	if binary == "" || target == "" {
		t.Skip("requires explicitly designated disposable directory and real fio binary")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	enginePath := filepath.Join(filepath.Dir(exe), "engines", "fio")
	if _, err = os.Lstat(enginePath); !os.IsNotExist(err) {
		t.Fatal("engine test path already occupied", err)
	}
	if err = os.MkdirAll(filepath.Dir(enginePath), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(enginePath, data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(enginePath) })
	dir, err := os.MkdirTemp(target, "designated-async-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(dir); err != nil {
			t.Error(err)
		}
	})
	for _, request := range []string{"auto", "libaio"} {
		c := diskConfig(t)
		c.Path = dir
		c.Engine = request
		selected, err := SelectEngine(context.Background(), c, safety.ReserveBytes)
		if err != nil {
			t.Fatal(request, err)
		}
		if selected != "io_uring" && selected != "libaio" || request == "libaio" && selected != "libaio" {
			t.Fatal(selected)
		}
		t.Log("Verified bounded real direct I/O:", request, "->", selected)
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatal("probe cleanup failed", entries, err)
		}
	}
}

func TestProbeCleanupFailureNeverClaimsReadiness(t *testing.T) {
	fixtureEngine(t)
	t.Setenv("VMBENCH_FIO_PROBE_MODE", "cleanup-fail")
	c := diskConfig(t)
	c.Engine = "auto"
	selected, err := SelectEngine(context.Background(), c, safety.ReserveBytes)
	if selected != "" || err == nil || !strings.Contains(err.Error(), "cleanup incomplete") || !strings.Contains(err.Error(), "fio fixture I/O failure") {
		t.Fatal(selected, err)
	}
	entries, e := os.ReadDir(c.Path)
	if e != nil || len(entries) != 1 {
		t.Fatal(entries, e)
	}
	data, e := os.ReadFile(filepath.Join(c.Path, entries[0].Name(), "untracked-user-file"))
	if e != nil || string(data) != "keep" {
		t.Fatal("unrelated file was removed", e)
	}
}
