package presentation

import (
	"fmt"
	"io"
	"strings"

	automatic "vmbench/internal/benchmark/auto"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/benchmark/network"
)

func visibleAutomaticPeak(peak automatic.Peak, mode string) bool {
	return displayablePeak(peak) && (mode != "max" || peak.Confirmed) && len(automaticInvalidEvidence(peak)) == 0
}

// Findings only affect presentation. Engine validation and exit codes remain
// authoritative; retained invalid observations must never look like success.
func automaticInvalidEvidence(peak automatic.Peak) []string {
	var findings []string
	trials := append([]automatic.Trial(nil), peak.Trials...)
	if peak.Best != nil {
		trials = append(trials, *peak.Best)
	}
	for _, trial := range trials {
		for _, sample := range trial.Samples {
			native, nativeOK := sample.Raw.(cpu.NativeResult)
			fio, fioOK := sample.Raw.(disk.Result)
			if _, ok := sample.Raw.(network.Result); ok {
				continue
			}
			if !nativeOK && !fioOK {
				native, nativeOK = decode[cpu.NativeResult](sample.Raw)
				fio, fioOK = decode[disk.Result](sample.Raw)
			}
			if nativeOK {
				for _, measurement := range native.Measurements {
					if measurement.Errors > 0 {
						findings = append(findings, "Data integrity errors detected; performance results are invalid")
					}
				}
			}
			if fioOK {
				for _, job := range fio.Jobs {
					if job.Error != 0 || job.TotalErrors > 0 || job.FirstError != 0 || job.Read.ShortIOs > 0 || job.Write.ShortIOs > 0 || job.Read.DropIOs > 0 || job.Write.DropIOs > 0 {
						findings = append(findings, "Invalid fio I/O evidence; performance results are invalid")
					}
				}
			}
		}
	}
	return unique(findings)
}

func automaticDisplayStatus(r automatic.Result) string {
	status := r.Status
	if status == "" {
		status = "NOT RUN"
	}
	if len(r.Errors) > 0 && status == "COMPLETED" {
		status = "FAILED"
	}
	for _, module := range []struct {
		name  string
		peaks []automatic.Peak
	}{{"CPU", r.CPU}, {"MEMORY", r.Memory}, {"DISK", r.Disk}, {"NETWORK", r.Network}} {
		if !r.Limits.Selects(module.name) {
			continue
		}
		state := r.States[module.name]
		if state == "FAILED" {
			return "FAILED"
		}
		if state == "COMPLETED" && len(module.peaks) == 0 && status == "COMPLETED" {
			status = "PARTIAL"
		}
		if (state == "PARTIAL" || state == "NOT RUN" || state == "UNCONFIRMED") && status == "COMPLETED" {
			status = "PARTIAL"
		}
		for _, peak := range module.peaks {
			if len(automaticInvalidEvidence(peak)) > 0 {
				return "FAILED"
			}
			if module.name == "NETWORK" && len(automaticNetworkObservations(peak).warnings) > 0 && status == "COMPLETED" {
				status = "PARTIAL"
			}
			if (!displayablePeak(peak) || r.Mode != "quick" && !peak.Confirmed) && status == "COMPLETED" {
				status = "PARTIAL"
			}
		}
	}
	return status
}

func automaticSteal(peaks []automatic.Peak, mode string) []float64 {
	var values []float64
	for _, peak := range peaks {
		if !visibleAutomaticPeak(peak, mode) {
			continue
		}
		for _, sample := range peak.Best.Samples {
			if value, ok := sample.Extras["CPU steal percent"]; ok && finite(value) && value >= 0 && value <= 100 {
				values = append(values, value)
			}
		}
	}
	return values
}

func percentRange(values []float64) string {
	return strings.TrimSpace(strings.TrimSuffix(rangeText(values, "%", 2), "%")) + " %"
}

func renderAutomaticNetwork(w io.Writer, peak automatic.Peak, multiple bool) {
	width := 15
	if multiple {
		width = 20
	}
	metric := func(label, value string) { compactMetricWidth(w, label, value, width) }
	direction := "Upload"
	if peak.Name == "TCP RX" {
		direction = "Download"
	}
	label := "TCP Throughput"
	if multiple {
		label = "TCP " + direction
	}
	metric(label, networkRate(peak.Best.Stats.Median*1e9))
	observations := automaticNetworkObservations(peak)
	if len(observations.retransmits) > 0 {
		label = "TCP Retransmits"
		if multiple {
			label = direction + " Retransmits"
		}
		value := countRange(observations.retransmits)
		if len(observations.retransmits) < len(peak.Best.Samples) {
			value += " (recorded runs only)"
		}
		metric(label, value)
	}
	if !multiple {
		metric("Direction", direction)
	}
}

func nativeComputeValue(throughput float64, unitName string, efficiency float64, scaling bool) string {
	value := unit(throughput, SafeText(strings.ReplaceAll(unitName, "_", " ")), 0)
	if scaling {
		value += fmt.Sprintf(" (scaling %s %%)", number(efficiency, 1))
	}
	return value
}
