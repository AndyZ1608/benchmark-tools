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
	if e != nil {
		return "", fmt.Errorf("engine %s executable unavailable at %q: %w; rerun initialization", name, p, e)
	}
	if !s.Mode().IsRegular() {
		return "", fmt.Errorf("engine %s at %q is not a regular executable", name, p)
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
	return runWithFiles(ctx, name, args, timeout, combined, nil)
}

// RunFiles passes only application-owned handles into the child as fd 3, 4, ... .
// The child's /proc/self/fd paths cannot be redirected by a replaced workspace name.
func RunFiles(ctx context.Context, name string, args []string, timeout time.Duration, files []*os.File) ([]byte, error) {
	observer, _ := ctx.Value(observerKey{}).(func(Event))
	if observer != nil {
		observer(Event{Name: name, Args: append([]string(nil), args...), Starting: true})
	}
	out, err := runWithFiles(ctx, name, args, timeout, false, files)
	if observer != nil {
		observer(Event{Name: name, Args: append([]string(nil), args...), Error: err})
	}
	return out, err
}
func runWithFiles(ctx context.Context, name string, args []string, timeout time.Duration, combined bool, files []*os.File) ([]byte, error) {
	p, e := Engine(name)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p, args...)
	configure(cmd)
	cmd.ExtraFiles = files
	var out []byte
	if combined {
		out, e = cmd.CombinedOutput()
	} else {
		out, e = cmd.Output()
	}
	if ctx.Err() != nil {
		return out, fmt.Errorf("engine %s at %q stopped: %w", name, p, ctx.Err())
	}
	if e != nil {
		if x, ok := e.(*exec.ExitError); ok {
			return out, fmt.Errorf("engine %s at %q failed: %s: %w", name, p, string(x.Stderr), e)
		}
	}
	if e != nil {
		return out, fmt.Errorf("cannot execute engine %s at %q: %w (if the file exists, check its ELF interpreter and shared libraries)", name, p, e)
	}
	return out, nil
}
