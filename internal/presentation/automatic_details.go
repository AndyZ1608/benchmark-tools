package presentation

import (
	"fmt"
	"io"
	"text/tabwriter"

	automatic "vmbench/internal/benchmark/auto"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/benchmark/disk"
)

// Raw validated observations stay in memory. A table preserves every active
// direction's separate job/run distributions, including P95 when recorded.
func renderAutomaticDiskDetails(w io.Writer, trial automatic.Trial) {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	started := false
	for i, sample := range trial.Samples {
		raw, ok := decode[disk.Result](sample.Raw)
		if !ok {
			continue
		}
		for j, job := range raw.Jobs {
			for _, direction := range []struct {
				name  string
				value disk.Direction
			}{{"read", job.Read}, {"write", job.Write}} {
				if direction.value.IOPS <= 0 {
					continue
				}
				if !started {
					fmt.Fprintln(table, "Run\tJob\tDirection\tMean\tP95\tP99 (separate distributions)")
					started = true
				}
				mean, p95, p99 := "-", "-", "-"
				if value, ok := meanLatency(direction.value); ok {
					mean = latencyRange([]float64{value})
				}
				if value, ok := percentile(direction.value.Completion, 95); ok {
					p95 = latencyRange([]float64{value})
				}
				if value, ok := percentile(direction.value.Completion, 99); ok {
					p99 = latencyRange([]float64{value})
				}
				fmt.Fprintf(table, "%d\t%d\t%s\t%s\t%s\t%s\n", i+1, j+1, direction.name, mean, p95, p99)
			}
		}
	}
	table.Flush()
	fmt.Fprintln(w, "Observed depth / validation:")
	for i, sample := range trial.Samples {
		fmt.Fprintf(w, "Run %d: original repetition %d; execution/validation PASSED\n", i+1, sample.Repetition)
		if sample.Evidence != "" {
			fmt.Fprintf(w, "Run %d depth/validation: %s\n", i+1, SafeText(sample.Evidence))
		}
		if raw, ok := decode[disk.Result](sample.Raw); ok {
			for jobIndex, job := range raw.Jobs {
				for _, direction := range []struct {
					name  string
					value disk.Direction
				}{{"read", job.Read}, {"write", job.Write}} {
					if direction.value.IOPS > 0 {
						fmt.Fprintf(w, "  Job %d %s: %s; %s; runtime %s; completed I/Os %s; completed bytes %s; fio error %d\n", jobIndex+1, direction.name, unit(direction.value.IOPS, "IOPS", 2), unit(direction.value.BWBytes/(1<<20), "MiB/s", 2), unit(direction.value.Runtime/1000, "s", 3), groupedCount(direction.value.TotalIOs), groupedCount(direction.value.IOBytes), job.Error)
					}
				}
			}
			for _, observation := range raw.Accounting {
				if observation.ByteDifference != "0" {
					fmt.Fprintf(w, "Run %d counter validation: %s %s; completed bytes %s; queued I/Os %s; byte difference %s; %s\n", i+1, SafeText(observation.Job), SafeText(observation.Direction), groupedCount(observation.IOBytes), groupedCount(observation.TotalIOs), SafeText(observation.ByteDifference), SafeText(observation.Explanation))
				}
			}
		}
	}
}

func renderAutomaticNativeDetails(w io.Writer, trial automatic.Trial) {
	rows := Table{Title: "Recorded native measurements", Headers: []string{"Run", "Workload", "Value", "Workers", "Dataset bytes", "Seconds", "Integrity errors"}}
	for run, sample := range trial.Samples {
		if raw, ok := decode[cpu.NativeResult](sample.Raw); ok {
			appendNativeMeasurements(&rows, run+1, raw)
		}
	}
	table(w, rows)
}

func appendNativeMeasurements(rows *Table, run int, raw cpu.NativeResult) {
	for _, measurement := range raw.Measurements {
		rows.Rows = append(rows.Rows, []string{fmt.Sprint(run), SafeText(measurement.Workload), unit(measurement.Value, SafeText(measurement.Unit), 3), fmt.Sprint(raw.Workers), groupedCount(raw.Dataset), number(measurement.Elapsed, 3), groupedCount(measurement.Errors)})
	}
}

func renderAutomaticTrials(w io.Writer, peak automatic.Peak) {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "Configuration\tMeasured throughput\tCV\tRepetitions\tSeconds/run\tPhase\tCompletion")
	for _, trial := range peak.Trials {
		value, cv := unit(trial.Stats.Median, SafeText(peak.Unit), 2), number(100*trial.Stats.CV, 1)+"%"
		if len(trial.Samples) == 0 {
			value, cv = "N/A", "-"
		}
		if trial.Stats.Count < 2 {
			cv = "-"
		}
		phase, completion := "exploration", "complete"
		if trial.Confirmation {
			phase = "confirmation"
		}
		if trial.Failure != "" {
			completion = "incomplete: " + SafeText(trial.Failure)
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%d\t%d\t%s\t%s\n", trial.Setting.String(), value, cv, trial.Stats.Count, trial.Duration, phase, completion)
	}
	table.Flush()
}
