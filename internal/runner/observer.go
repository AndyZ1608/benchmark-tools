package runner

import (
	"context"
	"time"
)

type Event struct {
	Name           string
	Args           []string
	Starting       bool
	Error          error
	Phase, Message string
}
type observerKey struct{}

// WithObserver adds phase notices outside engine execution and measurement timers.
func WithObserver(ctx context.Context, fn func(Event)) context.Context {
	return context.WithValue(ctx, observerKey{}, fn)
}
func observedRun(ctx context.Context, name string, args []string, timeout time.Duration, combined bool) ([]byte, error) {
	observer, _ := ctx.Value(observerKey{}).(func(Event))
	if observer != nil {
		observer(Event{Name: name, Args: append([]string(nil), args...), Starting: true})
	}
	out, err := run(ctx, name, args, timeout, combined)
	if observer != nil {
		observer(Event{Name: name, Args: append([]string(nil), args...), Error: err})
	}
	return out, err
}

// Notice reports lifecycle changes outside measured engine execution.
func Notice(ctx context.Context, phase, message string) {
	if observer, _ := ctx.Value(observerKey{}).(func(Event)); observer != nil {
		observer(Event{Name: "disk", Phase: phase, Message: message})
	}
}
