//go:build !linux

package safety

import "fmt"

type Target struct{ Dir string }

func Prepare(path, id string, size int64, jobs int) (*Target, error) {
	return nil, fmt.Errorf("disk benchmarking requires Linux")
}
func (t *Target) Paths() []string { return nil }
func (t *Target) Close()          {}
