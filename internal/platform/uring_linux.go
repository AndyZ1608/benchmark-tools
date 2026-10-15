//go:build linux && (amd64 || arm64)

package platform

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// IOUring probes setup permission only; no I/O or benchmark is submitted.
func IOUring() (bool, string) {
	var params [120]byte // Linux UAPI struct io_uring_params, zero flags.
	fd, _, errno := syscall.Syscall(425, 1, uintptr(unsafe.Pointer(&params[0])), 0)
	if errno != 0 {
		return false, errno.Error() + uringPolicy()
	}
	syscall.Close(int(fd))
	return true, "setup succeeded; no I/O submitted" + uringPolicy()
}

// LinuxAIO checks setup/destruction permissions without submitting any I/O.
func LinuxAIO() (bool, string) {
	var ctx uint64
	_, _, errno := syscall.Syscall(syscall.SYS_IO_SETUP, 1, uintptr(unsafe.Pointer(&ctx)), 0)
	if errno != 0 {
		return false, errno.Error()
	}
	_, _, errno = syscall.Syscall(syscall.SYS_IO_DESTROY, uintptr(ctx), 0, 0)
	if errno != 0 {
		return false, "context destruction denied: " + errno.Error()
	}
	return true, "setup/destruction succeeded; no I/O submitted"
}

func uringPolicy() string {
	disabled, err := os.ReadFile("/proc/sys/kernel/io_uring_disabled")
	if err != nil {
		return "; io_uring policy sysctl unavailable (kernel/security policy may still restrict setup)"
	}
	group, _ := os.ReadFile("/proc/sys/kernel/io_uring_group")
	return "; " + DescribeUringPolicy(strings.TrimSpace(string(disabled)), strings.TrimSpace(string(group)))
}
