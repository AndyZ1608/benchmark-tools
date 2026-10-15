package disk

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"testing"
	"vmbench/internal/config"
)

func TestAutomaticAndExplicitAsyncSelection(t *testing.T) {
	for _, test := range []struct {
		name, requested, want string
		uring, aio            error
		compiled              []string
	}{
		{"Ubuntu functional", "auto", "io_uring", nil, nil, []string{"libaio", "io_uring"}},
		{"RHEL EPERM", "auto", "libaio", syscall.EPERM, nil, []string{"io_uring", "libaio"}},
		{"both unavailable", "auto", "", syscall.EPERM, syscall.ENOSYS, []string{"io_uring", "libaio"}},
		{"setup succeeds but IO fails", "auto", "", syscall.EPERM, syscall.EIO, []string{"io_uring", "libaio"}},
		{"no compiled async", "auto", "", nil, nil, []string{"sync"}},
		{"explicit uring never substitutes", "io_uring", "", syscall.EPERM, nil, []string{"io_uring", "libaio"}},
		{"explicit libaio", "libaio", "libaio", nil, nil, []string{"io_uring", "libaio"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			engine, err := selectAsync(context.Background(), test.compiled, test.requested, func(name string) error {
				calls = append(calls, name)
				if name == "io_uring" {
					return test.uring
				}
				return test.aio
			})
			if engine != test.want || (err == nil) != (test.want != "") {
				t.Fatal(engine, err, calls)
			}
			if test.want == "io_uring" && len(calls) != 1 || test.requested == "io_uring" && len(calls) > 1 {
				t.Fatal(calls)
			}
			if err != nil && test.uring != nil && !strings.Contains(err.Error(), "io_uring") {
				t.Fatal(err)
			}
			if test.aio == syscall.EIO && !errors.Is(err, syscall.EIO) {
				t.Fatal("actual IO error lost", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err := selectAsync(ctx, []string{"io_uring", "libaio"}, "auto", func(string) error { cancel(); return ctx.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestDepthAndCompletedIOCannotBeFabricated(t *testing.T) {
	for _, depth := range []int{2, 4, 8, 32, 64} {
		r := Result{Options: map[string]string{"ioengine": "libaio"}, Jobs: []Job{{Read: Direction{IOPS: 1, TotalIOs: 1, IOBytes: 4096}, Depth: map[string]float64{"1": 100}}}}
		if _, err := ExecutionEvidence(r, "libaio", depth); err == nil {
			t.Fatal("QD1 accepted", depth)
		}
	}
}

func TestCompletedIOCounterValidation(t *testing.T) {
	c, _ := config.Defaults("quick")
	c.Jobs = 1
	c.Engine = "libaio"
	c.Profile = "randread-4k"
	base := Direction{IOPS: 1, BWBytes: 4096, Runtime: 1000, TotalIOs: 1, IOBytes: 4096, Completion: Latency{Mean: 1, Percentile: map[string]float64{"99.000000": 2}}}
	for _, change := range []string{"valid", "missing-count", "missing-bytes", "short", "dropped", "inconsistent"} {
		v := base
		switch change {
		case "missing-count":
			v.TotalIOs = 0
		case "missing-bytes":
			v.IOBytes = 0
		case "short":
			v.ShortIOs = 1
		case "dropped":
			v.DropIOs = 1
		case "inconsistent":
			v.IOBytes = 1
		}
		r := Result{Options: map[string]string{"ioengine": "libaio"}, Jobs: []Job{{Read: v, Depth: map[string]float64{"4": 100}}}}
		err := validateMeasurement(r, c, "randread")
		if (err == nil) != (change == "valid") {
			t.Fatal(change, err)
		}
	}
}
