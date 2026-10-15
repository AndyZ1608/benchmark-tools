package main

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestInteractiveProcessInterruptAtIdlePrompt(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	reached := make(chan struct{}, 1)
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLIProcess$", "--")
	cmd.Env = append(os.Environ(), "VMBENCH_TEST_CLI_PROCESS=1")
	cmd.Stdin = input
	cmd.Stdout = &promptObserver{stage: "main", reached: reached}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("main menu did not appear")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	// Keep stdin open: exit must be caused by the signal, never by EOF.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil || stderr.Len() != 0 {
			t.Fatalf("interrupt exit=%v stderr=%q", err, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Ctrl+C did not terminate the idle CLI")
	}
}
