package presentation

import (
	"fmt"
	"strings"

	automatic "vmbench/internal/benchmark/auto"
)

// Only known informational messages move to details. Unknown warnings always
// remain visible, including future engine and filesystem safety failures.
func automaticInformational(message string) bool {
	switch message {
	case "Preliminary result; stability and maximum throughput are not established",
		"Near-saturation search heuristic triggered; physical saturation is not established",
		"Aggregate last-level cache size unavailable", "NUMA topology unavailable",
		"CPU instruction capabilities unavailable":
		return true
	}
	return false
}

func automaticWarnings(r automatic.Result, details bool) []string {
	var warnings []string
	for _, message := range r.Warnings {
		if details || !automaticInformational(message) {
			warnings = append(warnings, message)
		}
	}
	if r.Limits.Selects("DISK") && r.DiskInfo.System {
		warnings = append(warnings, "Disk target is on the system filesystem; other applications may be affected")
	}
	if r.Limits.Selects("DISK") && r.DiskInfo.Prepared && !r.DiskCleaned {
		warnings = append(warnings, "Temporary file cleanup incomplete; inspect errors and retained files")
	}
	for _, module := range []struct {
		name  string
		peaks []automatic.Peak
	}{{"CPU", r.CPU}, {"MEMORY", r.Memory}, {"DISK", r.Disk}, {"NETWORK", r.Network}} {
		if !r.Limits.Selects(module.name) {
			continue
		}
		if r.States[module.name] == "COMPLETED" && len(module.peaks) == 0 {
			warnings = append(warnings, module.name+": Incomplete benchmark; no measured results available")
		}
		for _, peak := range module.peaks {
			name := automaticPeakName(peak.Name)
			for _, finding := range automaticInvalidEvidence(peak) {
				warnings = append(warnings, name+": "+finding)
			}
			if module.name == "NETWORK" {
				for _, finding := range automaticNetworkObservations(peak).warnings {
					warnings = append(warnings, name+": "+finding)
				}
			}
			if !displayablePeak(peak) {
				warnings = append(warnings, name+": Incomplete benchmark; no validated result available")
			} else if r.Mode != "quick" && !peak.Confirmed {
				warnings = append(warnings, name+": Unstable or unconfirmed measurements")
			}
			if peak.Boundary {
				warnings = append(warnings, name+": Search limit reached; higher performance may be possible")
			}
			for _, note := range peak.Notes {
				if automaticInformational(note) && !details {
					continue
				}
				if !details {
					switch note {
					case "Dataset is smaller than available RAM; backend/cache effects remain possible despite direct I/O":
						// One shared caveat rather than repeating it under both workloads.
						warnings = append(warnings, "Disk dataset fits in available RAM; cache effects may affect results")
						continue
					case "Search limit reached; higher performance may be possible.":
						if peak.Boundary {
							continue
						}
					}
					if strings.HasPrefix(note, "TCP traffic paced to at most ") {
						note = strings.TrimSuffix(note, "; no UDP or RTT test")
					}
				}
				warnings = append(warnings, name+": "+note)
			}
			if peak.Best == nil {
				continue
			}
			if module.name == "DISK" && r.Limits.DiskP99WarningMS > 0 {
				values := automaticP99(*peak.Best)
				high := 0.0
				for _, value := range values {
					high = max(high, value)
				}
				if high > r.Limits.DiskP99WarningMS {
					warnings = append(warnings, fmt.Sprintf("%s: High P99 latency under load (%s); exceeds configured %s threshold", name, latencyRange(values), latencyRange([]float64{r.Limits.DiskP99WarningMS})))
				}
			}
			if module.name == "CPU" {
				if _, confirmed, ok := automaticEfficiency(peak); ok && !confirmed && r.Mode != "quick" {
					warnings = append(warnings, name+": MT efficiency baseline is exploratory; confirmation is limited")
				}
			}
			for _, sample := range peak.Best.Samples {
				if module.name == "CPU" {
					if sample.Extras["CPU steal percent"] >= 1 {
						warnings = append(warnings, "CPU steal observed during confirmation; host contention may affect results")
					}
					if sample.Extras["Throttled periods"] > 0 {
						warnings = append(warnings, "CPU cgroup throttling observed during confirmation")
					}
				}
				if module.name == "MEMORY" && (sample.Extras["Swap pages in"] > 0 || sample.Extras["Swap pages out"] > 0) {
					warnings = append(warnings, "Guest swap activity occurred during RAM confirmation; main-memory bandwidth interpretation is limited")
				}
			}
		}
	}
	return unique(warnings)
}
