package cpu

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/runner"
)

func Run(ctx context.Context, c config.Config, mode string) ([]json.RawMessage, error) {
	var out []json.RawMessage
	workers := []int{1}
	if c.Workers != 1 {
		workers = append(workers, c.Workers)
	}
	for _, w := range workers {
		for i := 0; i < c.Repetitions; i++ {
			b, e := runner.Run(ctx, "compute-bench", []string{mode, strconv.Itoa(w), strconv.Itoa(c.Duration), strconv.Itoa(c.Warmup), c.Size}, time.Duration(6*(c.Duration+c.Warmup)+60)*time.Second)
			if e != nil {
				return out, e
			}
			if _, e := Parse(b); e != nil {
				return out, e
			}
			out = append(out, json.RawMessage(b))
		}
	}
	return out, nil
}
func Stress(ctx context.Context, c config.Config) (json.RawMessage, error) {
	b, e := runner.RunText(ctx, "stress-ng", []string{"--cpu", strconv.Itoa(c.Workers), "--cpu-load", strconv.Itoa(c.Load), "--cpu-method", c.Method, "--verify", "--timeout", strconv.Itoa(c.Duration) + "s", "--metrics-brief"}, time.Duration(c.Duration+30)*time.Second)
	j, _ := json.Marshal(string(b))
	return j, e
}
