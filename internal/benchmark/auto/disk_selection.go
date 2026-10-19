package auto

import (
	"fmt"

	"vmbench/internal/benchmark/disk"
	"vmbench/internal/statistics"
)

// UsableDiskTrial describes measurement availability, never statistical confidence.
// DiskValidated is set only after successful execution and mandatory fio checks.
func UsableDiskTrial(t *Trial) bool {
	if t == nil || len(t.Samples) == 0 || t.Stats.Count != len(t.Samples) || !finite(t.Stats.Median) || t.Stats.Median <= 0 {
		return false
	}
	for _, s := range t.Samples {
		if !s.DiskValidated {
			return false
		}
		for _, v := range []float64{s.Value, s.MiBps, s.MeanMS, s.MeanLowMS, s.P99LowMS, s.P99HighMS} {
			if !finite(v) || v <= 0 {
				return false
			}
		}
		if s.P99LowMS > s.P99HighMS {
			return false
		}
		raw, ok := s.Raw.(disk.Result)
		if !ok || len(raw.Jobs) == 0 {
			return false
		}
		for _, job := range raw.Jobs {
			if job.Error != 0 || job.TotalErrors != 0 || job.FirstError != 0 {
				return false
			}
			active := false
			for _, direction := range []disk.Direction{job.Read, job.Write} {
				if direction.ShortIOs != 0 || direction.DropIOs != 0 {
					return false
				}
				if direction.IOPS > 0 {
					active = true
					if direction.TotalIOs == 0 || direction.IOBytes == 0 {
						return false
					}
				}
			}
			if !active {
				return false
			}
		}
	}
	return true
}

// SelectDiskResult returns a view over this run's retained observations. Neither
// stored trials nor sample data are changed, so details can be viewed repeatedly.
func SelectDiskResult(p Peak) Peak {
	selectDiskObservation(&p)
	return p
}

// selectDiskObservation never changes the established confirmed winner. Without
// one, prefer the highest median from a valid trial with >=3 repetitions. Ties
// prefer more repetitions, then lower CV, then the earlier retained trial.
// If no such aggregate exists, select the highest individual valid observation,
// retaining its own configuration, latency, runtime and original repetition.
func selectDiskObservation(p *Peak) {
	if p.Confirmed && UsableDiskTrial(p.Best) && p.Best.Stable && p.Best.Failure == "" && p.Best.Stats.Count >= 3 {
		p.Selection = "Highest stability-confirmed median under the existing search policy"
		return
	}
	p.Confirmed = false
	var repeated *Trial
	consider := func(t *Trial) {
		if !UsableDiskTrial(t) || len(t.Samples) < 3 || !finite(t.Stats.CV) {
			return
		}
		if repeated == nil || t.Stats.Median > repeated.Stats.Median || t.Stats.Median == repeated.Stats.Median && (t.Stats.Count > repeated.Stats.Count || t.Stats.Count == repeated.Stats.Count && t.Stats.CV < repeated.Stats.CV) {
			repeated = t
		}
	}
	for i := range p.Trials {
		consider(&p.Trials[i])
	}
	consider(p.Best)
	if repeated != nil {
		selected := *repeated
		p.Best = &selected
		p.Selection = fmt.Sprintf("Highest usable observed median among trials with at least 3 repetitions; selected n=%d, CV %.1f%%; stability not confirmed", selected.Stats.Count, 100*selected.Stats.CV)
		return
	}
	var sample *Sample
	var source *Trial
	considerSample := func(t *Trial) {
		if t == nil {
			return
		}
		for i := range t.Samples {
			single := *t
			single.Samples = t.Samples[i : i+1]
			single.Stats = statistics.Summary{Count: 1, Median: single.Samples[0].Value}
			if UsableDiskTrial(&single) && (sample == nil || single.Samples[0].Value > sample.Value) {
				sample = &t.Samples[i]
				source = t
			}
		}
	}
	for i := range p.Trials {
		considerSample(&p.Trials[i])
	}
	considerSample(p.Best)
	p.Best = nil
	if sample != nil {
		selected := *source
		selected.Samples = []Sample{*sample}
		selected.Stats, _ = statistics.Describe([]float64{sample.Value})
		selected.Stable = false
		p.Best = &selected
		p.Selection = fmt.Sprintf("Highest valid individual observation (original repetition %d); no usable trial with 3 repetitions; not a stable aggregate or confirmed maximum", sample.Repetition)
	} else {
		p.Selection = "No measurement passed mandatory fio validity checks"
	}
}

// Keep compact parsed counters, percentiles, options and depth evidence. fio
// JSON+ histogram bins are not needed by selection or presentation.
func compactDiskObservation(r disk.Result) disk.Result {
	r.Jobs = append([]disk.Job(nil), r.Jobs...)
	for i := range r.Jobs {
		r.Jobs[i].Read.Completion.Bins = nil
		r.Jobs[i].Read.Total.Bins = nil
		r.Jobs[i].Write.Completion.Bins = nil
		r.Jobs[i].Write.Total.Bins = nil
	}
	return r
}
