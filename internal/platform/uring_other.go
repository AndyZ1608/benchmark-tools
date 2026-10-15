//go:build !linux || (!amd64 && !arm64)

package platform

func IOUring() (bool, string)  { return false, "probe unavailable on this architecture" }
func LinuxAIO() (bool, string) { return false, "probe unavailable on this architecture" }
