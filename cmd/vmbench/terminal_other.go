//go:build !linux

package main

// Benchmark execution is Linux-only. Help and menu navigation remain portable.
func interactiveStdin() bool { return false }
