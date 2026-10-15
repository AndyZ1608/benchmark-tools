package presentation

import (
	"fmt"
	"io"
	"strings"
	automatic "vmbench/internal/benchmark/auto"
)

func RenderAutomatic(w io.Writer, r automatic.Result, details bool) error {
	var b strings.Builder
	title := strings.ToUpper(SafeText(r.Mode))
	if r.Limits.Component != "" {
		title = strings.ToUpper(SafeText(r.Limits.Component)) + " - " + title
	}
	fmt.Fprintf(&b, "================================================\nVMBENCH - %s PERFORMANCE\n================================================\n", title)
	fmt.Fprintf(&b, "Status: %s\nDuration: %.1f seconds\n", SafeText(r.Status), r.Duration.Seconds())
	for _, m := range []struct {
		name  string
		peaks []automatic.Peak
	}{{"CPU", r.CPU}, {"MEMORY", r.Memory}, {"DISK", r.Disk}, {"NETWORK", r.Network}} {
		if !r.Limits.Selects(m.name) {
			continue
		}
		label := m.name
		if r.Limits.Component == "ram" && label == "MEMORY" {
			label = "RAM"
		}
		fmt.Fprintf(&b, "\n%s - %s\n------------------------------------------------\n", label, SafeText(r.States[m.name]))
		if m.name == "DISK" && r.DiskInfo.Engine != "" {
			fmt.Fprintln(&b, "Selected I/O engine: "+SafeText(r.DiskInfo.Engine))
		}
		if m.name == "DISK" && r.DiskInfo.Path != "" {
			fmt.Fprintf(&b, "Target: %s (%s, mount %s)\n", SafeText(r.DiskInfo.Path), SafeText(r.DiskInfo.Filesystem), SafeText(r.DiskInfo.Mount))
			if r.DiskInfo.PreparedJobs > 0 {
				label := "Planned"
				if r.DiskInfo.Prepared {
					label = "Prepared"
				}
				fmt.Fprintf(&b, "%s dataset: %s per job x %d files = %s total\n", label, unit(float64(r.DiskInfo.FileBytes)/(1<<20), "MiB", 0), r.DiskInfo.PreparedJobs, unit(float64(r.DatasetBytes)/(1<<20), "MiB", 0))
			}
			if r.DiskCleaned {
				fmt.Fprintln(&b, "Temporary files: removed")
			} else {
				fmt.Fprintln(&b, "WARNING: temporary cleanup incomplete or preparation failed; inspect errors")
			}
		}
		if m.name == "MEMORY" && r.Plan.MemoryBytes > 0 {
			fmt.Fprintf(&b, "Dataset: %s\n", unit(float64(r.Plan.MemoryBytes)/(1<<20), "MiB", 0))
		}
		for _, peak := range m.peaks {
			name := SafeText(peak.Name)
			switch name {
			case "integer-xorshift64":
				name = "Integer throughput"
			case "fp-multiply-add":
				name = "FP throughput"
			case "triad":
				name = "TRIAD bandwidth"
			case "randread-4k":
				name = "Random Read 4K"
			case "seqread-1m":
				name = "Sequential Read"
			case "randwrite-4k":
				name = "Random Write 4K"
			case "seqwrite-1m":
				name = "Sequential Write"
			case "mixed-70-30":
				name = "Mixed 70/30 (total IOPS)"
			}
			if peak.Best == nil {
				fmt.Fprintf(&b, "%s: N/A\n", name)
			} else {
				best := peak.Best
				prefix := ""
				if r.Mode == "max" && peak.Confirmed {
					prefix = "Maximum Observed "
					if peak.Name == "randread-4k" {
						name = "IOPS (Random Read 4K)"
					}
				}
				fmt.Fprintf(&b, "%s%s: %s\nConfiguration: %s\n", prefix, name, unit(best.Stats.Median, SafeText(peak.Unit), 2), best.Setting.String())
				if best.Setting.Depth > 0 {
					var bandwidth, means, percentiles []float64
					for _, s := range best.Samples {
						bandwidth = append(bandwidth, s.MiBps)
						means = append(means, s.MeanMS)
						if s.MeanLowMS > 0 {
							means = append(means, s.MeanLowMS)
						}
						percentiles = append(percentiles, s.P99LowMS, s.P99HighMS)
					}
					fmt.Fprintf(&b, "Throughput: %s\nAverage completion latency (job/run range): %s\nP99 completion latency (job/run range): %s\n", medianText(bandwidth, "MiB/s", 2), rangeText(means, "ms", 3), rangeText(percentiles, "ms", 3))
					if len(best.Samples) > 0 {
						fmt.Fprintln(&b, "Observed depth: "+SafeText(best.Samples[0].Evidence))
					}
					if peak.Name == "mixed-70-30" {
						for _, key := range []string{"Read IOPS", "Write IOPS", "Read MiB/s", "Write MiB/s"} {
							var values []float64
							for _, s := range best.Samples {
								if v, ok := s.Extras[key]; ok {
									values = append(values, v)
								}
							}
							fmt.Fprintf(&b, "%s: %s\n", key, medianText(values, "", 2))
						}
						fmt.Fprintln(&b, "Latency ranges above contain separate read/write job values; percentiles are not pooled")
					}
				}
				if m.name == "CPU" {
					baseline := 0.0
					baselineConfirmed := false
					for _, t := range peak.Trials {
						if t.Setting.Workers == 1 {
							baseline = t.Stats.Median
							baselineConfirmed = t.Stable && t.Duration == best.Duration
						}
					}
					if baseline > 0 {
						fmt.Fprintf(&b, "Scaling efficiency: %s\n", unit(100*best.Stats.Median/baseline/float64(best.Setting.Workers), "%", 1))
						if !baselineConfirmed && r.Mode != "quick" {
							fmt.Fprintln(&b, "Scaling baseline is exploratory; efficiency has limited confirmation")
						}
					}
					fmt.Fprintln(&b, "Computation integrity errors: 0")
				}
				if m.name == "MEMORY" {
					latency := []float64{}
					for _, s := range best.Samples {
						if s.LatencyNS > 0 {
							latency = append(latency, s.LatencyNS)
						}
					}
					fmt.Fprintf(&b, "Memory latency: %s\nIntegrity errors: 0\n", medianText(latency, "ns/access", 2))
				}
				if peak.Confirmed {
					fmt.Fprintf(&b, "Stability: confirmed (%d repetitions of %ds, CV %.1f%%)\n", best.Stats.Count, best.Duration, 100*best.Stats.CV)
				} else {
					fmt.Fprintln(&b, "Stability: preliminary; insufficient confirmation")
				}
			}
			for _, note := range peak.Notes {
				fmt.Fprintln(&b, "Note: "+SafeText(note))
			}
			if len(peak.Trials) > 0 {
				seen := map[automatic.Setting]bool{}
				lo, hi := peak.Trials[0].Setting, peak.Trials[0].Setting
				for _, t := range peak.Trials {
					seen[t.Setting] = true
					lo.Workers = min(lo.Workers, t.Setting.Workers)
					hi.Workers = max(hi.Workers, t.Setting.Workers)
					lo.Depth = min(lo.Depth, t.Setting.Depth)
					hi.Depth = max(hi.Depth, t.Setting.Depth)
					lo.Jobs = min(lo.Jobs, t.Setting.Jobs)
					hi.Jobs = max(hi.Jobs, t.Setting.Jobs)
					lo.Streams = min(lo.Streams, t.Setting.Streams)
					hi.Streams = max(hi.Streams, t.Setting.Streams)
				}
				fmt.Fprintf(&b, "Search: %d configurations (%s through %s)\n", len(seen), lo.String(), hi.String())
			}
			if details {
				fmt.Fprintln(&b, "Searched configurations (intermediate measurements):")
				for _, t := range peak.Trials {
					fmt.Fprintf(&b, "  %s: %s, CV %.1f%%, n=%d, %ds measurement\n", t.Setting.String(), unit(t.Stats.Median, SafeText(peak.Unit), 2), 100*t.Stats.CV, t.Stats.Count, t.Duration)
				}
				if peak.Best != nil && len(peak.Best.Samples) > 0 {
					for _, key := range []string{"copy GB/s", "scale GB/s", "add GB/s", "Read IOPS", "Write IOPS", "CPU steal percent", "Throttled periods"} {
						var v []float64
						for _, s := range peak.Best.Samples {
							if n, ok := s.Extras[key]; ok {
								v = append(v, n)
							}
						}
						if len(v) > 0 {
							fmt.Fprintf(&b, "  %s: %s\n", key, rangeText(v, "", 2))
						}
					}
				}
			}
		}
	}
	if r.Limits.Component == "" {
		fmt.Fprintf(&b, "\nSearch limits: %d effective workers; RAM dataset %s. MAX means observed performance within configured limits.\n", r.Plan.Workers, unit(float64(r.Plan.MemoryBytes)/(1<<20), "MiB", 0))
	} else {
		fmt.Fprintln(&b, "\nMAX means observed performance within configured limits.")
	}
	if r.Limits.MaxDepth > 0 {
		o := r.Limits
		if o.Selects("CPU") || o.Selects("MEMORY") {
			fmt.Fprintf(&b, "Effective worker ceiling: %d\n", r.Plan.Workers)
		}
		if o.Selects("MEMORY") {
			fmt.Fprintf(&b, "RAM allocation ceiling: %s\n", o.MaxRAM)
		}
		if o.Selects("DISK") {
			fmt.Fprintf(&b, "Disk ceilings: data %s; free reserve %s; QD%d / %d jobs / %d outstanding I/Os\n", o.MaxDisk, o.MinFree, o.MaxDepth, o.MaxJobs, o.MaxOutstanding)
		}
		if o.Selects("NETWORK") {
			fmt.Fprintf(&b, "Network ceilings: %d streams, %s aggregate offered bits/s\n", o.MaxStreams, o.NetworkBitrate)
		}
		fmt.Fprintf(&b, "Total time ceiling: %s\n", o.MaxDuration)
	}
	warnings := append([]string{}, r.Warnings...)
	if r.DiskInfo.System {
		warnings = append(warnings, "Disk target is on the system filesystem; other applications may be affected")
	}
	for _, peak := range r.CPU {
		if peak.Best != nil {
			for _, s := range peak.Best.Samples {
				if n, ok := s.Extras["CPU steal percent"]; ok && n >= 1 {
					warnings = append(warnings, "CPU steal observed during confirmation; host contention may affect results")
				}
				if s.Extras["Throttled periods"] > 0 {
					warnings = append(warnings, "CPU cgroup throttling observed during confirmation")
				}
			}
		}
	}
	for _, peak := range r.Memory {
		if peak.Best != nil {
			for _, s := range peak.Best.Samples {
				if s.Extras["Swap pages in"] > 0 || s.Extras["Swap pages out"] > 0 {
					warnings = append(warnings, "Guest swap activity occurred during RAM confirmation; main-memory bandwidth interpretation is limited")
				}
			}
		}
	}
	for _, note := range unique(warnings) {
		fmt.Fprintln(&b, "WARNING: "+SafeText(note))
	}
	for _, message := range r.Errors {
		fmt.Fprintln(&b, "ERROR: "+SafeText(message))
	}
	if details {
		s := r.Resources
		fmt.Fprintf(&b, "\nDetected resources: logical CPUs %d, affinity %s, effective CPU capacity %.2f, quota %s\nArchitecture: %s; instructions: %s\nMemory total %d, available headroom %d, cgroup limit %d; LLC %d; NUMA nodes %d\n", s.LogicalCPUs, SafeText(s.Affinity), s.Capacity, SafeText(s.CPUQuota), SafeText(s.Architecture), SafeText(s.CPUFlags), s.TotalMemory, s.AvailableMemory, s.MemoryLimit, s.LastLevelCache, s.NUMANodes)
		for _, n := range s.Interfaces {
			fmt.Fprintf(&b, "Interface: %s, state %s, reported speed %d Mbps (0 = unavailable)\n", SafeText(n.Name), SafeText(n.State), n.SpeedMbps)
		}
	}
	fmt.Fprintln(&b, "================================================")
	_, err := io.WriteString(w, b.String())
	return err
}
