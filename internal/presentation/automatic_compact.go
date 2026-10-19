package presentation

import (
	"fmt"
	"io"
	"strings"

	automatic "vmbench/internal/benchmark/auto"
	"vmbench/internal/benchmark/cpu"
)

const automaticRule = "----------------------------------------"
const automaticBorder = "========================================"

func renderAutomaticCompact(w io.Writer, r automatic.Result) error {
	var b strings.Builder
	title := strings.ToUpper(SafeText(strings.TrimSpace(r.Limits.Component + " " + r.Mode)))
	fmt.Fprintf(&b, "%s\n       VMBENCH - %s\n%s\nStatus: %s\n", automaticBorder, title, automaticBorder, SafeText(automaticDisplayStatus(r)))
	switch r.Mode {
	case "quick":
		fmt.Fprintln(&b, "Results: Preliminary")
	case "max":
		label := "Maximum observed within tested limits"
		for _, peak := range r.Disk {
			if r.Limits.Selects("DISK") && !peak.Confirmed {
				label = "Observed measurements within tested limits"
			}
		}
		fmt.Fprintln(&b, "Results: "+label)
	}
	if r.Limits.Selects("DISK") && r.DiskInfo.Path != "" {
		fmt.Fprintln(&b, "Target: "+SafeText(r.DiskInfo.Path))
	}
	for _, module := range []struct {
		name  string
		peaks []automatic.Peak
	}{{"CPU", r.CPU}, {"MEMORY", r.Memory}, {"DISK", r.Disk}, {"NETWORK", r.Network}} {
		if !r.Limits.Selects(module.name) {
			continue
		}
		state := r.States[module.name]
		if state != "" && state != "COMPLETED" {
			fmt.Fprintf(&b, "%s: %s\n", module.name, SafeText(state))
		}
		if module.name != "DISK" && len(module.peaks) > 0 {
			fmt.Fprintln(&b)
			if r.Limits.Component == "" {
				fmt.Fprintln(&b, module.name+"\n"+automaticRule)
			}
		}
		for _, peak := range module.peaks {
			if module.name == "DISK" {
				renderCompactAutomaticDisk(&b, peak)
				continue
			}
			if !visibleAutomaticPeak(peak, r.Mode) {
				continue
			}
			best := peak.Best
			switch module.name {
			case "CPU":
				value := unit(best.Stats.Median, SafeText(peak.Unit), 0)
				if efficiency, _, ok := automaticEfficiency(peak); ok {
					value += " (scaling " + number(efficiency, 1) + " %)"
				}
				label := "Integer Compute"
				if peak.Name == "fp-multiply-add" {
					label = "Float Compute"
				}
				compactMetric(&b, label, value)
			case "MEMORY":
				compactMetricWidth(&b, "Memory Bandwidth", unit(best.Stats.Median, SafeText(peak.Unit), 2)+" ("+SafeText(strings.ToUpper(peak.Name))+")", 16)
				var latency []float64
				for _, sample := range best.Samples {
					if finite(sample.LatencyNS) && sample.LatencyNS > 0 {
						latency = append(latency, sample.LatencyNS)
					}
				}
				if len(latency) > 0 {
					compactMetricWidth(&b, "Memory Latency", medianText(latency, "ns/access", 2), 16)
				}
			case "NETWORK":
				renderAutomaticNetwork(&b, peak, len(module.peaks) > 1)
			}
		}
		if module.name == "CPU" {
			if steal := automaticSteal(module.peaks, r.Mode); len(steal) > 0 {
				compactMetric(&b, "CPU Steal", percentRange(steal))
			}
		}
		if module.name != "DISK" && (r.Mode == "standard" || module.name == "CPU") && len(module.peaks) > 0 {
			state := "Confirmed"
			for _, peak := range module.peaks {
				if !peak.Confirmed {
					state = "Unconfirmed"
				}
			}
			if r.Mode == "quick" {
				state = "Preliminary"
			}
			width := 15
			if module.name == "MEMORY" {
				width = 16
			}
			compactMetricWidth(&b, "Stability", state, width)
		}

	}

	if warnings := automaticWarnings(r, false); len(warnings) > 0 {
		fmt.Fprintln(&b, "\nWARNINGS")
		for _, warning := range warnings {
			fmt.Fprintln(&b, "- "+SafeText(warning))
		}
	}
	for _, message := range r.Errors {
		fmt.Fprintln(&b, "ERROR: "+SafeLines(message))
	}
	if r.Limits.Selects("DISK") && r.DiskInfo.Path != "" {
		state := "Not prepared"
		if r.DiskCleaned {
			state = "Cleaned"
		} else if r.DiskInfo.Prepared {
			state = "Cleanup incomplete"
		}
		fmt.Fprintf(&b, "\n%s\nTemporary Files: %s\n", automaticRule, state)
	}
	fmt.Fprintln(&b, automaticBorder)
	_, err := io.WriteString(w, b.String())
	return err
}

func compactMetric(w io.Writer, label, value string) {
	compactMetricWidth(w, label, value, 15)
}

func compactMetricWidth(w io.Writer, label, value string, width int) {
	fmt.Fprintf(w, "%-*s: %s\n", width, label, value)
}

func automaticPeakName(name string) string {
	switch name {
	case "integer-xorshift64":
		return "Integer throughput"
	case "fp-multiply-add":
		return "Floating point throughput"
	case "triad":
		return "Memory bandwidth"
	case "seqread-1m":
		return "Sequential read"
	case "seqwrite-1m":
		return "Sequential write"
	case "TCP TX":
		return "TCP upload"
	case "TCP RX":
		return "TCP download"
	default:
		return SafeText(diskProfile(name))
	}
}

func displayablePeak(peak automatic.Peak) bool {
	return peak.Best != nil && finite(peak.Best.Stats.Median) && peak.Best.Stats.Median > 0
}

func automaticP99(trial automatic.Trial) []float64 {
	var values []float64
	for _, sample := range trial.Samples {
		for _, value := range []float64{sample.P99LowMS, sample.P99HighMS} {
			if finite(value) && value > 0 {
				values = append(values, value)
			}
		}
	}
	return values
}

// Select one unit for the entire range; never average job/run percentiles.
func latencyRange(milliseconds []float64) string {
	if len(milliseconds) == 0 {
		return ""
	}
	lo, hi := milliseconds[0], milliseconds[0]
	for _, value := range milliseconds[1:] {
		lo, hi = min(lo, value), max(hi, value)
	}
	divisor, suffix, digits := 1.0, "ms", 2
	if lo >= 1000 {
		divisor, suffix = 1000, "s"
	} else if hi < 1 {
		divisor, suffix = .001, "us"
	} else if hi >= 10 {
		digits = 1
	}
	return rangeText([]float64{lo / divisor, hi / divisor}, suffix, digits)
}

// Keep the existing same-workload scaling calculation and baseline selection.
func automaticEfficiency(peak automatic.Peak) (value float64, confirmed, ok bool) {
	if !displayablePeak(peak) || peak.Best.Setting.Workers <= 1 {
		return 0, false, false
	}
	baseline := 0.0
	for _, trial := range peak.Trials {
		if trial.Setting.Workers == 1 {
			baseline = trial.Stats.Median
			confirmed = trial.Stable && trial.Duration == peak.Best.Duration
		}
	}
	if !finite(baseline) || baseline <= 0 {
		return 0, false, false
	}
	return 100 * peak.Best.Stats.Median / baseline / float64(peak.Best.Setting.Workers), confirmed, true
}

// Only recorded native measurements establish integrity; absence is not zero.
func automaticIntegrity(trial automatic.Trial) (uint64, bool) {
	count := 0
	var errors uint64
	for _, sample := range trial.Samples {
		raw, ok := decode[cpu.NativeResult](sample.Raw)
		if !ok || len(raw.Measurements) == 0 {
			return 0, false
		}
		for _, measurement := range raw.Measurements {
			count++
			errors += measurement.Errors
		}
	}
	return errors, count > 0
}
