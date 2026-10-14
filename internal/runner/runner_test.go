package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestMissingEngine(t *testing.T) {
	_, e := Run(context.Background(), "not-an-engine", nil, time.Second)
	if e == nil {
		t.Fatal("missing engine accepted")
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("VMBENCH_HELPER") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}
func TestTimeoutAndCancel(t *testing.T) {
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(filepath.Dir(exe), "engines")
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	name := "helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	b, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(dir, name)
	if e = os.WriteFile(p, b, 0700); e != nil {
		t.Fatal(e)
	}
	defer os.Remove(p)
	t.Setenv("VMBENCH_HELPER", "1")
	start := time.Now()
	_, e = Run(context.Background(), name, []string{"-test.run=TestHelperProcess"}, 50*time.Millisecond)
	if !errors.Is(e, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = Run(ctx, name, nil, time.Second)
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func TestObserverBoundaryAndArgumentIsolation(t *testing.T) {
	args := []string{"--version"}
	events := []Event{}
	ctx := WithObserver(context.Background(), func(event Event) {
		events = append(events, event)
		if len(event.Args) > 0 {
			event.Args[0] = "modified by callback"
		}
	})
	_, err := Run(ctx, "not-an-installed-engine", args, time.Second)
	if err == nil || len(events) != 2 || !events[0].Starting || events[1].Starting || events[1].Error == nil || args[0] != "--version" {
		t.Fatal(events, args, err)
	}
}
