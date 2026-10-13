//go:build !linux

package system

import "fmt"

func Filesystem(path string) (map[string]any, error) {
	return nil, fmt.Errorf("Linux filesystem inspection unavailable")
}
