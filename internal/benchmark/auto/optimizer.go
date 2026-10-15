package auto

import (
	"context"
	"fmt"
	"math"
	"sort"
	"vmbench/internal/statistics"
)

type Setting struct{ Workers, Depth, Jobs, Streams int }

func (s Setting) String() string {
	if s.Depth > 0 {
		return fmt.Sprintf("QD%d / %d jobs", s.Depth, s.Jobs)
	}
	if s.Streams > 0 {
		return fmt.Sprintf("%d streams", s.Streams)
	}
	return fmt.Sprintf("%d workers", s.Workers)
}

type Sample struct {
	Value, MiBps, MeanMS, MeanLowMS, P99LowMS, P99HighMS, LatencyNS float64
	Extras                                                          map[string]float64
	Evidence                                                        string
	Raw                                                             any
}
type Trial struct {
	Setting  Setting
	Stats    statistics.Summary
	Samples  []Sample
	Duration int
	Stable   bool
}
type Peak struct {
	Name, Unit          string
	Best                *Trial
	Trials              []Trial
	Confirmed, Boundary bool
	Notes               []string
}
type measure func(context.Context, Setting, int, int) (Sample, error)

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func evaluate(ctx context.Context, s Setting, seconds, reps, warm int, cv float64, fn measure) (Trial, error) {
	t := Trial{Setting: s, Duration: seconds}
	values := []float64{}
	for i := 0; i < reps; i++ {
		if e := ctx.Err(); e != nil {
			return t, e
		}
		v, e := fn(ctx, s, seconds, warm)
		if e != nil {
			return t, e
		}
		if !finite(v.Value) || v.Value <= 0 {
			return t, fmt.Errorf("invalid performance measurement for %s", s)
		}
		for _, n := range []float64{v.MiBps, v.MeanMS, v.MeanLowMS, v.P99LowMS, v.P99HighMS, v.LatencyNS} {
			if !finite(n) || n < 0 {
				return t, fmt.Errorf("invalid secondary metric for %s", s)
			}
		}
		t.Samples = append(t.Samples, v)
		values = append(values, v.Value)
	}
	var e error
	t.Stats, e = statistics.Describe(values)
	t.Stable = e == nil && t.Stats.Count >= 3 && t.Stats.CV <= cv
	return t, e
}

type explorer struct {
	peak     Peak
	options  Options
	policy   Policy
	fn       measure
	progress func(string)
	seen     map[Setting]bool
}

func newExplorer(name, unit string, o Options, fn measure, progress func(string)) *explorer {
	return &explorer{peak: Peak{Name: name, Unit: unit}, options: o, policy: o.Policy(), fn: fn, progress: progress, seen: map[Setting]bool{}}
}

// NearPlateau is an explicit search heuristic, not proof of physical saturation.
func NearPlateau(a, b Trial, threshold, maxCV float64) bool {
	if a.Stats.Median <= 0 || a.Stats.CV > maxCV || b.Stats.CV > maxCV {
		return false
	}
	improvement := (b.Stats.Median - a.Stats.Median) / a.Stats.Median
	uncertainty := math.Max(threshold, 2*math.Max(a.Stats.CV, b.Stats.CV))
	latency := func(t Trial) float64 {
		n := 0.0
		for _, s := range t.Samples {
			n = math.Max(n, s.P99HighMS)
		}
		return n
	}
	return improvement < uncertainty && (latency(a) == 0 || latency(b) >= latency(a))
}
func (e *explorer) survey(ctx context.Context, settings []Setting, adaptive bool) error {
	consecutive := 0
	var previous *Trial
	for _, s := range settings {
		if e.seen[s] {
			continue
		}
		e.seen[s] = true
		t, err := evaluate(ctx, s, e.policy.ExploreSeconds, e.policy.ExploreReps, e.policy.Warmup, e.options.MaxCV, e.fn)
		if err != nil {
			return err
		}
		e.peak.Trials = append(e.peak.Trials, t)
		if e.progress != nil {
			e.progress(e.peak.Name + ": " + s.String() + " completed")
		}
		if previous != nil && NearPlateau(*previous, t, e.options.Improvement, e.options.MaxCV) {
			consecutive++
		} else {
			consecutive = 0
		}
		copy := t
		previous = &copy
		if adaptive && consecutive >= 2 {
			e.peak.Notes = append(e.peak.Notes, "Near-saturation search heuristic triggered; physical saturation is not established")
			break
		}
	}
	return nil
}
func (e *explorer) promising() *Trial {
	var best *Trial
	for _, t := range e.peak.Trials {
		if best == nil || (t.Stats.CV <= e.options.MaxCV && best.Stats.CV > e.options.MaxCV) || (t.Stats.CV <= e.options.MaxCV) == (best.Stats.CV <= e.options.MaxCV) && t.Stats.Median > best.Stats.Median {
			copy := t
			best = &copy
		}
	}
	return best
}
func (e *explorer) finish(ctx context.Context, boundary func(Setting) bool) (Peak, error) {
	if len(e.peak.Trials) == 0 {
		return e.peak, fmt.Errorf("no valid configurations measured")
	}
	if e.options.Mode == "quick" {
		e.peak.Best = e.promising()
		e.peak.Notes = append(e.peak.Notes, "Preliminary result; stability and maximum throughput are not established")
		return e.peak, nil
	}
	candidates := append([]Trial(nil), e.peak.Trials...)
	sort.SliceStable(candidates, func(i, j int) bool {
		stableI, stableJ := candidates[i].Stats.CV <= e.options.MaxCV, candidates[j].Stats.CV <= e.options.MaxCV
		if stableI != stableJ {
			return stableI
		}
		return candidates[i].Stats.Median > candidates[j].Stats.Median
	})
	seen := map[Setting]bool{}
	// Confirm up to three promising configurations. Selection uses fresh repeated
	// medians, never a single exploratory spike; every confirmation is retained.
	for _, candidate := range candidates {
		if len(seen) >= 3 {
			break
		}
		if seen[candidate.Setting] {
			continue
		}
		seen[candidate.Setting] = true
		t, err := evaluate(ctx, candidate.Setting, e.policy.ConfirmSeconds, e.policy.ConfirmReps, e.policy.Warmup, e.options.MaxCV, e.fn)
		if err != nil {
			return e.peak, err
		}
		e.peak.Trials = append(e.peak.Trials, t)
		if t.Stable && (e.peak.Best == nil || t.Stats.Median > e.peak.Best.Stats.Median) {
			copy := t
			e.peak.Best = &copy
			e.peak.Confirmed = true
		}
	}
	if e.peak.Best == nil {
		e.peak.Notes = append(e.peak.Notes, "Maximum performance could not be confirmed reliably.")
		return e.peak, nil
	}
	e.peak.Boundary = boundary(e.peak.Best.Setting)
	if e.peak.Boundary {
		e.peak.Notes = append(e.peak.Notes, "Search limit reached; higher performance may be possible.")
	}
	if e.progress != nil {
		e.progress(e.peak.Name + ": best configuration confirmed")
	}
	return e.peak, nil
}
