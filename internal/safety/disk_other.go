//go:build !linux

package safety

import (
	"context"
	"fmt"
	"os"
)

type Target struct {
	Dir  string
	Plan Plan
}

func ValidateDirectory(path string) (map[string]any, error) {
	return nil, fmt.Errorf("disk target %q requires Linux", path)
}
func Prepare(path, id string, size int64, jobs int) (*Target, error) {
	return nil, fmt.Errorf("disk benchmarking requires Linux")
}
func PrepareSized(path, id string, size int64, jobs int, automatic bool) (*Target, error) {
	return nil, fmt.Errorf("disk benchmarking requires Linux")
}
func (t *Target) Paths() []string         { return nil }
func (t *Target) EngineFiles() []*os.File { return nil }
func (t *Target) Fill(ctx context.Context, seed uint64, progress func(int, int)) error {
	return fmt.Errorf("disk benchmarking requires Linux")
}
func (t *Target) Validate() error { return fmt.Errorf("disk benchmarking requires Linux") }
func (t *Target) Close() error    { return nil }
