package main

import (
	"os"
	"syscall"
	"unsafe"
)

func interactiveStdin() bool {
	// A character device may be /dev/null. Only a terminal may grant an
	// interactive acknowledgment; redirected unattended commands must fail.
	var state syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&state)), 0, 0, 0)
	return errno == 0
}
