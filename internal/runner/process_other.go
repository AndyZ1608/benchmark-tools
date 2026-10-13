//go:build !linux

package runner

import "os/exec"

func configure(c *exec.Cmd) {}
