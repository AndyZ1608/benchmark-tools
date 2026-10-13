package safety

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type Target struct {
	Dir      string
	Files    []string
	fd       int
	parent   int
	name     string
	identity os.FileInfo
}

func Prepare(path, id string, size int64, jobs int) (*Target, error) {
	if path == "" {
		return nil, fmt.Errorf("explicit --path directory required")
	}
	a, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	for _, prefix := range []string{"/dev", "/sys", "/proc"} {
		if a == prefix || strings.HasPrefix(a, prefix+"/") {
			return nil, fmt.Errorf("pseudo-filesystem target rejected")
		}
	}
	resolved, e := filepath.EvalSymlinks(a)
	if e != nil || resolved != a {
		return nil, fmt.Errorf("symlink or invalid target directory")
	}
	fd, e := syscall.Open(a, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	t := &Target{Dir: a, fd: fd, parent: -1}
	var st syscall.Statfs_t
	if e = syscall.Fstatfs(fd, &st); e != nil {
		t.Close()
		return nil, e
	}
	free := uint64(st.Bavail) * uint64(st.Bsize)
	need := uint64(size) * uint64(jobs)
	if need > free || free-need < 1<<30 {
		t.Close()
		return nil, fmt.Errorf("insufficient space: require dataset plus 1GiB headroom")
	}
	name := "vmbench-" + id
	if strings.ContainsAny(id, "/\\") || id == "" {
		t.Close()
		return nil, fmt.Errorf("invalid run ID")
	}
	if e = syscall.Mkdirat(fd, name, 0700); e != nil {
		t.Close()
		return nil, e
	}
	child, e := syscall.Openat(fd, name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		t.Close()
		return nil, e
	}
	t.parent = fd
	t.fd = child
	t.name = name
	t.Dir = filepath.Join(a, name)
	t.identity, _ = os.Stat(fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), child))
	fd = child
	for i := 0; i < jobs; i++ {
		n := fmt.Sprintf("vmbench-%s-%d.dat", id, i)
		f, e := syscall.Openat(fd, n, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY|syscall.O_NOFOLLOW, 0600)
		if e != nil {
			t.Close()
			return nil, e
		}
		syscall.Close(f)
		t.Files = append(t.Files, n)
	}
	return t, nil
}
func (t *Target) Paths() []string {
	a := []string{}
	for _, n := range t.Files {
		a = append(a, fmt.Sprintf("/proc/%d/fd/%d/%s", os.Getpid(), t.fd, n))
	}
	return a
}
func (t *Target) Close() {
	if t.fd < 0 {
		return
	}
	for _, n := range t.Files {
		_ = syscall.Unlinkat(t.fd, n)
	}
	if t.fd >= 0 {
		syscall.Close(t.fd)
		t.fd = -1
	}
	if t.parent >= 0 {
		p := fmt.Sprintf("/proc/%d/fd/%d/%s", os.Getpid(), t.parent, t.name)
		if info, e := os.Stat(p); e == nil && t.identity != nil && os.SameFile(info, t.identity) {
			_ = os.Remove(p)
		}
		syscall.Close(t.parent)
		t.parent = -1
	}
}
