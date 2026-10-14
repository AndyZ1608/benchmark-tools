package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func Engine(name string) (string, error) {
	exe, e := os.Executable()
	if e != nil {
		return "", e
	}
	p := filepath.Join(filepath.Dir(exe), "engines", name)
	s, e := os.Stat(p)
	if e != nil || s.IsDir() {
		return "", fmt.Errorf("bundled engine %s missing: rebuild complete offline release", name)
	}
	return p, nil
}
func Run(ctx context.Context, name string, args []string, timeout time.Duration) ([]byte, error) {
	return observedRun(ctx, name, args, timeout, false)
}
func RunText(ctx context.Context, name string, args []string, timeout time.Duration) ([]byte, error) {
	return observedRun(ctx, name, args, timeout, true)
}
func run(ctx context.Context, name string, args []string, timeout time.Duration, combined bool) ([]byte, error) {
	p, e := Engine(name)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p, args...)
	configure(cmd)
	var out []byte
	if combined {
		out, e = cmd.CombinedOutput()
	} else {
		out, e = cmd.Output()
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if e != nil {
		if x, ok := e.(*exec.ExitError); ok {
			return out, fmt.Errorf("%s failed: %s: %w", name, string(x.Stderr), e)
		}
	}
	return out, e
}
