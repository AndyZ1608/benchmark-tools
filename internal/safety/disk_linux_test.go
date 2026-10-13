package safety

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectUnsafe(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "link")
	if e := os.Symlink(d, p); e != nil {
		t.Fatal(e)
	}
	if _, e := Prepare(p, "test", 1024, 1); e == nil {
		t.Fatal("accepted symlink")
	}
	if _, e := Prepare("", "test", 1024, 1); e == nil {
		t.Fatal("accepted empty path")
	}
}

func TestOwnFilesAndCleanup(t *testing.T) {
	d := t.TempDir()
	sentinel := filepath.Join(d, "keep.txt")
	if e := os.WriteFile(sentinel, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	x, e := Prepare(d, "exclusive", 4096, 2)
	if e != nil {
		t.Skipf("storage prerequisite: %v", e)
	}
	if _, e = Prepare(d, "exclusive", 4096, 1); e == nil {
		t.Fatal("run collision accepted")
	}
	paths := x.Paths()
	if len(paths) != 2 {
		t.Fatal(paths)
	}
	for _, p := range paths {
		if _, e = os.Stat(p); e != nil {
			t.Fatal(e)
		}
	}
	x.Close()
	x.Close()
	if _, e = os.Stat(filepath.Join(d, "vmbench-exclusive")); !os.IsNotExist(e) {
		t.Fatal("run directory not cleaned", e)
	}
	if b, e := os.ReadFile(sentinel); e != nil || string(b) != "keep" {
		t.Fatal("existing file changed")
	}
}
