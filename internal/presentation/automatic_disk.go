package presentation

import (
	"fmt"
	"io"
	"strings"

	automatic "vmbench/internal/benchmark/auto"
)

func isDiskPeak(p automatic.Peak) bool {
	switch p.Name {
	case "randread-4k", "seqread-1m", "randwrite-4k", "seqwrite-1m", "mixed-70-30":
		return true
	}
	return false
}

// Always include the two read workloads for a selected Disk run. Other modules
// and intentionally skipped legacy Disk endpoints retain their existing views.
func diskResultView(r automatic.Result) automatic.Result {
	if !r.Limits.Selects("DISK") || r.States["DISK"] == "SKIPPED" {
		return r
	}
	peaks := make([]automatic.Peak, 0, max(2, len(r.Disk)))
	for _, name := range []string{"randread-4k", "seqread-1m"} {
		peak := automatic.Peak{Name: name, Unit: "IOPS"}
		if name == "seqread-1m" {
			peak.Unit = "MiB/s"
		}
		for _, stored := range r.Disk {
			if stored.Name == name {
				peak = stored
				break
			}
		}
		peaks = append(peaks, peak)
	}
	for _, peak := range r.Disk {
		if peak.Name != "randread-4k" && peak.Name != "seqread-1m" {
			peaks = append(peaks, peak)
		}
	}
	for i := range peaks {
		peaks[i] = automatic.SelectDiskResult(peaks[i])
		if !automatic.UsableDiskTrial(peaks[i].Best) && peaks[i].Failure == "" {
			if len(r.Errors) > 0 {
				peaks[i].Failure = r.Errors[0]
			} else {
				peaks[i].Failure = "No successful validated fio measurement was recorded"
			}
		}
	}
	r.Disk = peaks
	return r
}

func diskConfidence(p automatic.Peak) string {
	if !automatic.UsableDiskTrial(p.Best) {
		return "NO VALID MEASUREMENT"
	}
	if p.Confirmed {
		return "CONFIRMED"
	}
	return "UNCONFIRMED"
}

func renderCompactAutomaticDisk(w io.Writer, p automatic.Peak) {
	fmt.Fprintf(w, "\n%s\n%s\n", strings.ToUpper(automaticPeakName(p.Name)), automaticRule)
	valid := automatic.UsableDiskTrial(p.Best)
	if p.Unit != "MiB/s" {
		label, value := "IOPS", "N/A"
		if p.Name == "mixed-70-30" {
			label = "Total IOPS"
		}
		if valid {
			value = number(p.Best.Stats.Median, 0)
		}
		compactMetric(w, label, value)
	}
	bandwidth, latency := "N/A", "N/A"
	if valid {
		var values []float64
		for _, sample := range p.Best.Samples {
			values = append(values, sample.MiBps)
		}
		bandwidth = medianText(values, "MiB/s", 2)
		latency = latencyRange(automaticP99(*p.Best)) + " (job/run range)"
		if p.Name == "mixed-70-30" {
			latency = latencyRange(automaticP99(*p.Best)) + " (read/write job/run range)"
		}
	}
	compactMetric(w, "Throughput", bandwidth)
	compactMetric(w, "P99 Latency", latency)
	compactMetric(w, "Confidence", diskConfidence(p))
	if !valid {
		compactMetric(w, "Reason", SafeText(strings.SplitN(p.Failure, "\n", 2)[0]))
	} else if !p.Confirmed && p.Best.Stats.Count < 3 {
		compactMetric(w, "Observation", "Single sample")
	}
}

func diskConfidenceWarning(p automatic.Peak) string {
	if p.Confirmed || !automatic.UsableDiskTrial(p.Best) {
		return ""
	}
	if p.SearchIncomplete {
		return "Stability not confirmed; search incomplete"
	}
	if p.Best.Stats.Count < 3 {
		return "Stability not confirmed; insufficient repetitions"
	}
	for _, trial := range p.Trials {
		if trial.Confirmation && trial.Failure == "" && trial.Stats.Count >= 3 && !trial.Stable {
			return "Stability not confirmed; confirmation measurements were variable"
		}
	}
	if !p.Best.Stable {
		return "Stability not confirmed; high measurement variability"
	}
	return "Stability not confirmed; no stability-confirmed candidate"
}
