package auto

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"vmbench/internal/benchmark/disk"
	"vmbench/internal/config"
	"vmbench/internal/statistics"
)

type retentionBackend struct {
	fakeBackend
	change func(context.Context, config.Config, *disk.Result) error
}

func (f *retentionBackend) OpenDisk(ctx context.Context, c config.Config, reserve uint64) (DiskSession, error) {
	if _, err := f.fakeBackend.OpenDisk(ctx, c, reserve); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *retentionBackend) Measure(ctx context.Context, c config.Config) (disk.Outcome, error) {
	out, err := f.fakeBackend.Measure(ctx, c)
	if err == nil && f.change != nil {
		err = f.change(ctx, c, &out.Results[0])
	}
	return out, err
}

// Change only synthetic counters/rates; all fixtures still pass production
// counter, block-size, runtime and depth validators before they are retained.
func setRetentionRate(r *disk.Result, c config.Config, iops float64) {
	block := uint64(4096)
	if strings.Contains(c.Profile, "1m") {
		block = 1 << 20
	}
	for i := range r.Jobs {
		v := &r.Jobs[i].Read
		if strings.Contains(c.Profile, "write") {
			v = &r.Jobs[i].Write
		}
		v.TotalIOs = uint64(math.Round(iops * float64(c.Duration) / float64(c.Jobs)))
		v.IOBytes = v.TotalIOs * block
		v.IOPS = math.Round(float64(v.TotalIOs)/float64(c.Duration)*1e6) / 1e6
		v.BWBytes = math.Floor(float64(v.IOBytes) / float64(c.Duration))
		v.Completion.Mean = float64(c.Depth) * 1e6
		v.Completion.Percentile = map[string]float64{"95.000000": float64(c.Depth) * 1e6, "99.000000": float64(c.Depth) * 2e6}
	}
}

func retentionOptions(mode string) Options {
	o := Defaults(mode)
	o.Component, o.DiskPath, o.AckMax = "disk", "/fixture", true
	o.MaxDepth, o.MaxJobs, o.MaxOutstanding = 4, 1, 4
	return o
}

func assertDiskCleanup(t *testing.T, r Result, f *retentionBackend) {
	t.Helper()
	if f.opened != 1 || f.closed != 1 || !r.DiskCleaned {
		t.Fatal("private dataset lifecycle lost", f.opened, f.closed, r.DiskCleaned)
	}
}

func TestDiskRetainsUnconfirmedRandomAndConfirmedSequential(t *testing.T) {
	for _, mode := range []string{"standard", "max"} {
		t.Run(mode, func(t *testing.T) {
			o := retentionOptions(mode)
			f := &retentionBackend{}
			calls := 0
			f.change = func(_ context.Context, c config.Config, r *disk.Result) error {
				if c.Profile == "randread-4k" {
					calls++
					value := 100.0
					if calls%2 == 0 {
						value = 1000
					}
					setRetentionRate(r, c, value)
				}
				return nil
			}
			r, err := Run(context.Background(), o, f, nil)
			if err != nil || len(r.Disk) != 2 || r.Status != "COMPLETED WITH LIMITATIONS" || r.States["DISK"] != "UNCONFIRMED" {
				t.Fatal(r, err)
			}
			if !UsableDiskTrial(r.Disk[0].Best) || r.Disk[0].Confirmed || !r.Disk[1].Confirmed {
				t.Fatal(r.Disk)
			}
			if r.Disk[0].Best.Stats.Count < 3 || !strings.Contains(r.Disk[0].Selection, "observed median") {
				t.Fatal(r.Disk[0])
			}
			assertDiskCleanup(t, r, f)
		})
	}
}

func TestDiskRetainsPartialRepetitionsAndLaterConfigurationFailures(t *testing.T) {
	for _, mode := range []string{"quick", "standard", "max"} {
		for _, failAt := range []int{2, 4} {
			t.Run(fmt.Sprintf("%s/call-%d", mode, failAt), func(t *testing.T) {
				f := &retentionBackend{}
				calls := 0
				f.change = func(_ context.Context, _ config.Config, _ *disk.Result) error {
					calls++
					if calls == failAt {
						return fmt.Errorf("later fio failure")
					}
					return nil
				}
				r, err := Run(context.Background(), retentionOptions(mode), f, nil)
				if mode == "quick" && failAt == 4 {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				if err == nil || r.Status != "FAILED" {
					t.Fatal(r, err)
				}
				var kept bool
				for _, peak := range r.Disk {
					if UsableDiskTrial(peak.Best) {
						kept = true
					}
				}
				if !kept {
					t.Fatal("successful observations discarded", r.Disk)
				}
				assertDiskCleanup(t, r, f)
			})
		}
	}
}

func TestDiskTimeLimitKeepsEarlierSamplesAndCleans(t *testing.T) {
	o := retentionOptions("max")
	o.MaxDuration = time.Second
	f := &retentionBackend{}
	calls := 0
	f.change = func(ctx context.Context, _ config.Config, _ *disk.Result) error {
		calls++
		if calls == 3 {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	r, err := Run(context.Background(), o, f, nil)
	if !errors.Is(err, context.DeadlineExceeded) || !UsableDiskTrial(r.Disk[0].Best) || r.Disk[0].Confirmed || !r.Disk[0].SearchIncomplete {
		t.Fatal(r, err)
	}
	if calls != 3 || !strings.Contains(r.Disk[0].Failure, "deadline exceeded") {
		t.Fatal(calls, r.Disk[0])
	}
	assertDiskCleanup(t, r, f)
}

func TestDiskConfirmedCandidateSurvivesLaterFailure(t *testing.T) {
	o := retentionOptions("max")
	f := &retentionBackend{}
	confirmationCalls := 0
	f.change = func(_ context.Context, c config.Config, _ *disk.Result) error {
		if c.Profile == "randread-4k" && c.Duration == o.Policy().ConfirmSeconds {
			confirmationCalls++
			if confirmationCalls == 4 {
				return fmt.Errorf("second confirmation failed")
			}
		}
		return nil
	}
	r, err := Run(context.Background(), o, f, nil)
	if err == nil || !r.Disk[0].Confirmed || !UsableDiskTrial(r.Disk[0].Best) || !r.Disk[0].SearchIncomplete || r.Disk[1].Best != nil {
		t.Fatal(r, err)
	}
	assertDiskCleanup(t, r, f)
}

func TestDiskQDCeilingRetainsConfirmedMeasurements(t *testing.T) {
	o := retentionOptions("max")
	f := &retentionBackend{}
	f.change = func(_ context.Context, c config.Config, r *disk.Result) error {
		setRetentionRate(r, c, float64(c.Depth*100))
		return nil
	}
	r, err := Run(context.Background(), o, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, peak := range r.Disk {
		if !peak.Confirmed || !peak.Boundary || peak.Best.Setting.Depth != o.MaxDepth {
			t.Fatal(peak)
		}
	}
	assertDiskCleanup(t, r, f)
}

func TestDiskInvalidObservationsNeverBecomeUnconfirmed(t *testing.T) {
	for _, invalid := range []struct {
		name   string
		change func(*disk.Result)
	}{
		{"fio errors", func(r *disk.Result) { r.Jobs[0].Error = 5 }},
		{"bad count", func(r *disk.Result) { r.Jobs[0].Read.TotalIOs++ }},
		{"bad bytes", func(r *disk.Result) { r.Jobs[0].Read.IOBytes++ }},
		{"unverified completion", func(r *disk.Result) { r.Jobs[0].Read.TotalIOs = 0 }},
		{"short I/O", func(r *disk.Result) { r.Jobs[0].Read.ShortIOs = 1 }},
		{"synchronous engine", func(r *disk.Result) { r.Options["ioengine"] = "sync" }},
		{"direct I/O changed", func(r *disk.Result) { r.Options["direct"] = "0" }},
	} {
		for _, earlyValid := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/early-valid-%t", invalid.name, earlyValid), func(t *testing.T) {
				f := &retentionBackend{}
				calls := 0
				f.change = func(_ context.Context, _ config.Config, r *disk.Result) error {
					calls++
					if !earlyValid || calls > 1 {
						invalid.change(r)
					}
					return nil
				}
				r, err := Run(context.Background(), retentionOptions("max"), f, nil)
				if err == nil || r.Status != "FAILED" || r.Disk[0].Confirmed {
					t.Fatal(r, err)
				}
				if UsableDiskTrial(r.Disk[0].Best) != earlyValid {
					t.Fatal("invalid result promoted or valid predecessor lost", r.Disk[0])
				}
				if calls > 2 {
					t.Fatal("unexpected retry/fallback", calls)
				}
				assertDiskCleanup(t, r, f)
			})
		}
	}
}

func TestDiskSingleObservationKeepsItsOwnConfigurationAndLatency(t *testing.T) {
	o := retentionOptions("max")
	f := &retentionBackend{}
	f.change = func(_ context.Context, c config.Config, r *disk.Result) error {
		if c.Depth == 4 {
			return fmt.Errorf("later configuration failed")
		}
		setRetentionRate(r, c, float64(c.Depth*100))
		return nil
	}
	r, err := Run(context.Background(), o, f, nil)
	best := r.Disk[0].Best
	if err == nil || !UsableDiskTrial(best) || best.Setting.Depth != 2 || best.Stats.Count != 1 || best.Samples[0].P99HighMS != 4 || best.Samples[0].Value != 200 || best.Samples[0].MiBps != 200.0/256 || best.Samples[0].Repetition != 1 {
		t.Fatal(r, err)
	}
	assertDiskCleanup(t, r, f)
}

func TestDiskRetainsCompactEvidenceAndDoesNotReusePreviousRun(t *testing.T) {
	f := &retentionBackend{}
	var original disk.Result
	f.change = func(_ context.Context, _ config.Config, r *disk.Result) error {
		r.Jobs[0].Read.Completion.Bins = map[string]uint64{"1024": 1}
		original = *r
		return nil
	}
	r, err := Run(context.Background(), retentionOptions("quick"), f, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := r.Disk[0].Best.Samples[0].Raw.(disk.Result)
	if len(raw.Jobs[0].Read.Completion.Bins) != 0 || len(original.Jobs[0].Read.Completion.Bins) != 1 || raw.Jobs[0].Read.TotalIOs == 0 || raw.Options["bs"] != "4k" {
		t.Fatal(raw, original)
	}
	f.change = func(context.Context, config.Config, *disk.Result) error {
		return fmt.Errorf("no valid baseline this run")
	}
	r, err = Run(context.Background(), retentionOptions("quick"), f, nil)
	if err == nil || r.Disk[0].Best != nil || r.Disk[1].Best != nil {
		t.Fatal("previous-run measurements reused", r, err)
	}
}

func TestDiskSelectionPrefersConfirmedThenRepeatedThenSingle(t *testing.T) {
	o := retentionOptions("quick")
	f := &retentionBackend{}
	r, err := Run(context.Background(), o, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	sample := r.Disk[0].Best.Samples[0]
	repeated := Trial{Setting: Setting{Depth: 1, Jobs: 1}, Samples: []Sample{sample, sample, sample}, Stable: true}
	repeated.Stats, _ = statistics.Describe([]float64{sample.Value, sample.Value, sample.Value})
	large := sample
	large.Value *= 100
	single := Trial{Setting: Setting{Depth: 2, Jobs: 1}, Samples: []Sample{large}, Stats: statistics.Summary{Count: 1, Median: large.Value}}
	p := SelectDiskResult(Peak{Name: "randread-4k", Best: &repeated, Confirmed: true, Trials: []Trial{single, repeated}})
	if !p.Confirmed || p.Best.Setting.Depth != 1 {
		t.Fatal(p)
	}
	p = SelectDiskResult(Peak{Name: "randread-4k", Trials: []Trial{single, repeated}})
	if p.Confirmed || p.Best.Setting.Depth != 1 || p.Best.Stats.Count != 3 {
		t.Fatal(p)
	}
	single.Samples[0].DiskValidated = false
	p = SelectDiskResult(Peak{Name: "randread-4k", Trials: []Trial{single}})
	if p.Best != nil || p.Confirmed {
		t.Fatal("unverified sample promoted", p)
	}
}
