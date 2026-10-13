package system

import (
	"fmt"
	"os"
	"syscall"
)

func Filesystem(path string) (map[string]any, error) {
	var s syscall.Statfs_t
	if e := syscall.Statfs(path, &s); e != nil {
		return nil, e
	}
	a, e := os.Stat(path)
	if e != nil {
		return nil, e
	}
	root, e := os.Stat("/")
	same := false
	if e == nil {
		same = a.Sys().(*syscall.Stat_t).Dev == root.Sys().(*syscall.Stat_t).Dev
	}
	return map[string]any{"path": path, "filesystem_magic": fmt.Sprintf("0x%x", s.Type), "available_bytes": uint64(s.Bavail) * uint64(s.Bsize), "system_filesystem": same}, nil
}
