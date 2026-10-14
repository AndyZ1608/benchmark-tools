package runner

import (
	"context"
	"time"
)

type Event struct {
	Name     string
	Args     []string
	Starting bool
	Error    error
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
