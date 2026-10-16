package presentation

import (
	"fmt"
	"io"
	"strings"
	automatic "vmbench/internal/benchmark/auto"
)

func RenderAutomatic(w io.Writer, r automatic.Result, details bool) error {
	if !details {
		return renderAutomaticCompact(w, r)
	}
	var b strings.Builder
	title := strings.ToUpper(SafeText(r.Mode))
	if r.Limits.Component != "" {
		title = strings.ToUpper(SafeText(r.Limits.Component)) + " - " + title
	}
	fmt.Fprintf(&b, "================================================\nVMBENCH - %s PERFORMANCE\n================================================\n", title)
	fmt.Fprintf(&b, "Status: %s\nDuration: %.1f seconds\n", SafeText(automaticDisplayStatus(r)), r.Duration.Seconds())
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
			if !displayablePeak(peak) {
				fmt.Fprintf(&b, "%s: No validated result\n", name)
			} else {
				best := peak.Best
				prefix := ""
				if len(automaticInvalidEvidence(peak)) > 0 {
					prefix = "Invalid retained observation - "
				}
				if r.Mode == "max" && peak.Confirmed && len(automaticInvalidEvidence(peak)) == 0 {
					prefix = "Maximum Observed "
					if peak.Name == "randread-4k" {
						name = "IOPS (Random Read 4K)"
					}
				}
				fmt.Fprintf(&b, "%s%s: %s\nConfiguration: %s\n", prefix, name, unit(best.Stats.Median, SafeText(peak.Unit), 2), best.Setting.String())
				if best.Setting.Depth > 0 {
					var bandwidth, means []float64
					for _, sample := range best.Samples {
						if finite(sample.MiBps) && sample.MiBps > 0 {
							bandwidth = append(bandwidth, sample.MiBps)
						}
						for _, mean := range []float64{sample.MeanMS, sample.MeanLowMS} {
							if finite(mean) && mean > 0 {
								means = append(means, mean)
							}
						}
					}
					if len(bandwidth) > 0 {
						fmt.Fprintf(&b, "Throughput: %s\n", medianText(bandwidth, "MiB/s", 2))
					}
					if len(means) > 0 {
						fmt.Fprintf(&b, "Average completion latency (job/run range): %s\n", latencyRange(means))
					}
					if values := automaticP99(*best); len(values) > 0 {
						fmt.Fprintf(&b, "P99 completion latency (job/run range): %s\n", latencyRange(values))
					}
					if peak.Name == "mixed-70-30" {
						for _, key := range []string{"Read IOPS", "Write IOPS", "Read MiB/s", "Write MiB/s"} {
							var values []float64
							for _, s := range best.Samples {
								if v, ok := s.Extras[key]; ok {
									values = append(values, v)
								}
							}
							if len(values) > 0 {
								fmt.Fprintf(&b, "%s: %s\n", key, medianText(values, "", 2))
							}
						}
						fmt.Fprintln(&b, "Latency ranges above contain separate read/write job values; percentiles are not pooled")
					}
					renderAutomaticDiskDetails(&b, *best)
				}
				if m.name == "NETWORK" {
					renderAutomaticNetwork(&b, peak, len(m.peaks) > 1)
				}
				if m.name == "CPU" {
					if efficiency, _, ok := automaticEfficiency(peak); ok {
						fmt.Fprintf(&b, "Scaling efficiency: %s\n", unit(efficiency, "%", 1))
					}
					if errors, ok := automaticIntegrity(*best); ok {
						fmt.Fprintf(&b, "Computation integrity errors: %d\n", errors)
					}
				}
				if m.name == "MEMORY" {
					latency := []float64{}
					for _, s := range best.Samples {
						if s.LatencyNS > 0 {
							latency = append(latency, s.LatencyNS)
						}
					}
					if len(latency) > 0 {
						fmt.Fprintf(&b, "Memory latency: %s\n", medianText(latency, "ns/access", 2))
					}
					if errors, ok := automaticIntegrity(*best); ok {
						fmt.Fprintf(&b, "Integrity errors: %d\n", errors)
					}
				}
				if m.name == "CPU" || m.name == "MEMORY" {
					renderAutomaticNativeDetails(&b, *best)
				}
				if peak.Confirmed {
					fmt.Fprintf(&b, "Stability: confirmed (%d repetitions of %ds, CV %.1f%%)\n", best.Stats.Count, best.Duration, 100*best.Stats.CV)
				} else {
					fmt.Fprintln(&b, "Stability: preliminary; insufficient confirmation")
				}
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
				renderAutomaticTrials(&b, peak)
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
	for _, note := range automaticWarnings(r, true) {
		fmt.Fprintln(&b, "WARNING: "+SafeText(note))
	}
	for _, message := range r.Errors {
		fmt.Fprintln(&b, "ERROR: "+SafeLines(message))
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
