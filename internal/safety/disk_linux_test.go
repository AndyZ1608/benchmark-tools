package safety

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestRejectUnsafe(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "link")
	if err := os.Symlink(d, p); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p, "", filepath.Join(d, "absent"), "/dev", "/proc", "/sys"} {
		if _, err := Prepare(path, "test", 4096, 1); err == nil {
			t.Fatal("accepted unsafe target", path)
		}
	}
	child := filepath.Join(d, "child")
	os.Mkdir(child, 0700)
	if _, err := Prepare(filepath.Join(p, "child"), "ancestor", 4096, 1); err == nil {
		t.Fatal("accepted ancestor symlink")
	}
	file := filepath.Join(d, "regular")
	os.WriteFile(file, []byte("keep"), 0600)
	if _, err := Prepare(file, "not-directory", 4096, 1); err == nil {
		t.Fatal("regular file accepted as target")
	}
}
func createTarget(t *testing.T, jobs int) *Target {
	t.Helper()
	target, err := Prepare(t.TempDir(), "exclusive", 2<<20, jobs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { target.Close() })
	return target
}
func TestOwnFilesAndCleanup(t *testing.T) {
	d := t.TempDir()
	sentinel := filepath.Join(d, "keep.txt")
	os.WriteFile(sentinel, []byte("keep"), 0600)
	target, err := Prepare(d, "exclusive", 2<<20, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Prepare(d, "exclusive", 4096, 1); err == nil {
		t.Fatal("run collision accepted")
	}
	if target.Plan.TotalBytes != 4<<20 {
		t.Fatal(target.Plan)
	}
	info, _ := os.Stat(target.Dir)
	if info.Mode().Perm() != 0700 {
		t.Fatal(info.Mode())
	}
	if err = target.Fill(context.Background(), 42, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range target.Paths() {
		info, err := os.Stat(path)
		if err != nil || info.Size() != 2<<20 || info.Mode().Perm() != 0600 {
			t.Fatal(info, err)
		}
		if st := info.Sys().(*syscall.Stat_t); st.Blocks*512 < info.Size() {
			t.Fatal("sparse file", st.Blocks)
		}
	}
	owner, err := os.ReadFile(filepath.Join(target.Dir, ownerName))
	if err != nil || !strings.Contains(string(owner), "seed=42") || !strings.Contains(string(owner), "sha256=") {
		t.Fatal(string(owner), err)
	}
	if err = target.Close(); err != nil {
		t.Fatal(err)
	}
	if err = target.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(d, ".vmbench-exclusive")); !os.IsNotExist(err) {
		t.Fatal("workspace not cleaned", err)
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "keep" {
		t.Fatal("existing file changed", err)
	}
}
func TestPermissionAndSpaceFailuresBeforeWriting(t *testing.T) {
	newDirectory := func(t *testing.T) string {
		t.Helper()
		d := t.TempDir()
		// Run before TempDir's cleanup, including when an assertion calls Fatal.
		t.Cleanup(func() {
			if err := os.Chmod(d, 0700); err != nil {
				t.Errorf("restore fixture permissions: %v", err)
			}
		})
		if err := os.Chmod(d, 0700); err != nil {
			t.Fatal(err)
		}
		return d
	}
	assertEmpty := func(t *testing.T, d string) {
		t.Helper()
		entries, err := os.ReadDir(d)
		if err != nil || len(entries) != 0 {
			t.Fatalf("rejection created files: entries=%v err=%v", entries, err)
		}
	}
	for _, fixture := range []struct {
		name string
		mode os.FileMode
	}{{"readonly", 0500}, {"no-search", 0400}, {"no-access", 0000}} {
		t.Run(fixture.name, func(t *testing.T) {
			d := newDirectory(t)
			if err := os.Chmod(d, fixture.mode); err != nil {
				t.Fatal(err)
			}
			if info, err := os.Stat(d); err != nil || info.Mode().Perm() != fixture.mode {
				t.Fatalf("fixture mode was not applied: info=%v err=%v", info, err)
			}
			target, err := Prepare(d, "permission", 4096, 1)
			// Restore access before inspecting contents or closing an unexpected target.
			if restoreErr := os.Chmod(d, 0700); restoreErr != nil {
				t.Fatal(restoreErr)
			}
			if target != nil {
				if closeErr := target.Close(); closeErr != nil {
					t.Errorf("close unexpected target: %v", closeErr)
				}
				t.Fatalf("permission-restricted target accepted: uid=%d err=%v", os.Geteuid(), err)
			}
			// Non-root normally fails openat/faccessat with EACCES. Root can bypass
			// DAC, then hits the separate fail-closed check for absent write bits.
			if !errors.Is(err, os.ErrPermission) && (os.Geteuid() != 0 || err == nil || !strings.Contains(err.Error(), "no write permission bits")) {
				t.Fatalf("expected permission rejection: uid=%d mode=%04o err=%v", os.Geteuid(), fixture.mode, err)
			}
			assertEmpty(t, d)
		})
	}
	t.Run("space", func(t *testing.T) {
		d := newDirectory(t)
		var fs syscall.Statfs_t
		if err := syscall.Statfs(d, &fs); err != nil {
			t.Fatal(err)
		}
		if fs.Bsize <= 0 || fs.Blocks > (^uint64(0)-metadataReserve)/uint64(fs.Bsize) {
			t.Fatal("invalid fixture filesystem capacity")
		}
		// Reserving the entire filesystem guarantees insufficient headroom even
		// if other processes free space. The requested file itself stays tiny.
		reserve := max(ReserveBytes, fs.Blocks*uint64(fs.Bsize))
		target, err := PrepareReserved(d, "space", 4096, 1, false, reserve)
		if target != nil {
			if closeErr := target.Close(); closeErr != nil {
				t.Errorf("close unexpected target: %v", closeErr)
			}
			t.Fatal("accepted dataset without reserved headroom")
		}
		if err == nil || !strings.Contains(err.Error(), "insufficient disk space") {
			t.Fatal(err)
		}
		assertEmpty(t, d)
	})
}
func TestPreparationCancellationCleansOwnedFiles(t *testing.T) {
	target := createTarget(t, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := target.Fill(ctx, 1, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	dir := target.Dir
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
func TestCleanupRejectsReplacedFileAndSymlink(t *testing.T) {
	for _, kind := range []string{"regular", "symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			target := createTarget(t, 1)
			if err := target.Fill(context.Background(), 1, nil); err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(target.Dir, target.Files[0])
			os.Rename(name, filepath.Join(target.Dir, "moved-owned-file"))
			victim := filepath.Join(t.TempDir(), "user-data")
			os.WriteFile(victim, []byte("important"), 0600)
			switch kind {
			case "regular":
				os.WriteFile(name, []byte("unrelated"), 0600)
			case "symlink":
				os.Symlink(victim, name)
			case "hardlink":
				os.Link(victim, name)
			}
			if err := target.Close(); err == nil || !strings.Contains(err.Error(), "identity changed") {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(victim); err != nil || string(b) != "important" {
				t.Fatal("victim changed", err)
			}
			if _, err := os.Lstat(name); err != nil {
				t.Fatal("replacement removed", err)
			}
			if _, err := os.Stat(filepath.Join(target.Dir, ownerName)); err != nil {
				t.Fatal("orphan metadata removed", err)
			}
		})
	}
}
func TestCleanupRejectsReplacedWorkspaceAndUntrackedContents(t *testing.T) {
	t.Run("replaced", func(t *testing.T) {
		target := createTarget(t, 1)
		original := target.Dir
		os.Rename(original, original+"-moved")
		os.Mkdir(original, 0700)
		sentinel := filepath.Join(original, "keep")
		os.WriteFile(sentinel, []byte("keep"), 0600)
		if err := target.Close(); err == nil {
			t.Fatal("replacement workspace accepted")
		}
		if b, err := os.ReadFile(sentinel); err != nil || string(b) != "keep" {
			t.Fatal("replacement data changed", err)
		}
		if _, err := os.Stat(filepath.Join(original+"-moved", target.Files[0])); err != nil {
			t.Fatal("moved file removed", err)
		}
	})
	t.Run("untracked", func(t *testing.T) {
		target := createTarget(t, 1)
		other := filepath.Join(target.Dir, "unrelated")
		os.WriteFile(other, []byte("keep"), 0600)
		if err := target.Close(); err == nil || !strings.Contains(err.Error(), "untracked entry") {
			t.Fatal(err)
		}
		if b, err := os.ReadFile(other); err != nil || string(b) != "keep" {
			t.Fatal("unrelated file changed", err)
		}
		if _, err := os.Stat(filepath.Join(target.Dir, ownerName)); err != nil {
			t.Fatal("ownership metadata lost", err)
		}
	})
}
func TestRenamedTargetHandleCannotEscapeCleanup(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "target")
	os.Mkdir(path, 0700)
	target, err := Prepare(path, "anchored", 4096, 1)
	if err != nil {
		t.Fatal(err)
	}
	moved := path + "-moved"
	os.Rename(path, moved)
	os.Mkdir(path, 0700)
	sentinel := filepath.Join(path, "important")
	os.WriteFile(sentinel, []byte("keep"), 0600)
	if err = target.Close(); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "keep" {
		t.Fatal("new target was touched", err)
	}
	if entries, err := os.ReadDir(moved); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
}
func TestReadonlyFilesystemRejected(t *testing.T) {
	mount := t.TempDir()
	if err := syscall.Mount("tmpfs", mount, "tmpfs", syscall.MS_RDONLY, "size=32m"); err != nil {
		t.Skipf("mount capability unavailable: %v", err)
	}
	defer syscall.Unmount(mount, 0)
	if _, err := Prepare(mount, "ro", 4096, 1); err == nil || !strings.Contains(err.Error(), "read-only filesystem") {
		t.Fatal(err)
	}
}

func TestPreparationFailureAndPartialCancellationCleanup(t *testing.T) {
	t.Run("partial-cancel", func(t *testing.T) {
		target := createTarget(t, 2)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := target.Fill(ctx, 1, func(done, total int) {
			if done == 1 {
				cancel()
			}
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if err = target.Close(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("write-limit", func(t *testing.T) {
		target := createTarget(t, 1)
		var prior syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &prior); err != nil {
			t.Fatal(err)
		}
		limit := prior
		limit.Cur = 4096
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		defer syscall.Setrlimit(syscall.RLIMIT_FSIZE, &prior)
		err := target.Fill(context.Background(), 42, nil)
		if !errors.Is(err, syscall.EFBIG) || !strings.Contains(err.Error(), "job-001.dat") {
			t.Fatal(err)
		}
		if err = target.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestFileDescriptorExhaustionStillCleansOwnedWorkspace(t *testing.T) {
	directory := t.TempDir()
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &old); err != nil {
		t.Fatal(err)
	}
	limit := old
	limit.Cur = 32
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	defer syscall.Setrlimit(syscall.RLIMIT_NOFILE, &old)
	_, err := Prepare(directory, "fd-limit", 4096, 64)
	if !errors.Is(err, syscall.EMFILE) {
		t.Fatal(err)
	}
	syscall.Setrlimit(syscall.RLIMIT_NOFILE, &old)
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("descriptor failure leaked owned files", entries, err)
	}
}
