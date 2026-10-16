package presentation

import (
	"fmt"
	"io"
	"strings"

	"vmbench/internal/benchmark/disk"
	"vmbench/internal/benchmark/network"
	"vmbench/internal/report"
)

// Advanced test commands share the same concise layout. Their existing
// summaries, workload selection and validation findings remain authoritative.
func renderCompactReport(w io.Writer, r report.Report, color bool) error {
	modules := Modules(r)
	if len(modules) == 0 {
		return nil
	}
	m := modules[0]
	var b strings.Builder
	component := "UNKNOWN"
	if fields := strings.Fields(r.Command); len(fields) > 0 {
		component = strings.ToUpper(fields[0])
	}
	mode := strings.ToUpper(r.Settings.Preset)
	if strings.HasSuffix(r.Command, "stress") {
		mode += " STRESS"
	}
	if r.Command == "network server" {
		mode = "SERVER"
	}
	status := m.Status
	if r.Status == "failed" || r.Status == "partial" {
		status = r.Status
	}
	if len(r.Errors) > 0 && status == "completed" {
		status = "failed"
	}
	var extra []string
	var directions []reportNetworkDirection
	if r.Command == "network test" {
		directions = reportNetworkDirections(r)
		for _, direction := range directions {
			for _, warning := range direction.observations.warnings {
				extra = append(extra, direction.name+": "+warning)
			}
		}
		if len(extra) > 0 && status == "completed" {
			status = "partial"
		}
	}
	fmt.Fprintf(&b, "%s\n       VMBENCH - %s %s\n%s\nStatus: %s\n", automaticBorder, component, mode, automaticBorder, statusText(status, color))
	if r.Settings.Preset == "quick" && mode != "SERVER" {
		fmt.Fprintln(&b, "Results: Preliminary")
	}
	fmt.Fprintln(&b)
	switch r.Command {
	case "cpu test":
		for _, item := range []struct{ name, label string }{{"integer-xorshift64", "Integer Compute"}, {"fp-multiply-add", "Float Compute"}} {
			if summary, ok := selected(nativeSummaries(r, r.Command), item.name, "defined_iterations/s"); ok {
				compactMetric(&b, item.label, nativeComputeValue(summary.Statistics.Median, summary.Unit, summary.ScalingEfficiency*100, summary.Workers > 1 && summary.ScalingFactor > 0 && finite(summary.ScalingEfficiency)))
			}
		}
		if value, ok := telemetry(r, "steal_percent"); ok {
			compactMetric(&b, "CPU Steal", percentRange([]float64{value}))
		}
		renderCompactStability(&b, m)
	case "cpu stress":
		if value, ok := telemetry(r, "busy_percent"); ok {
			compactMetric(&b, "CPU Busy", percentRange([]float64{value}))
		}
		if value, ok := telemetry(r, "steal_percent"); ok {
			compactMetric(&b, "CPU Steal", percentRange([]float64{value}))
		}
		compactMetric(&b, "Duration", unit(r.DurationSeconds, "s", 1))
	case "ram test":
		summaries := nativeSummaries(r, r.Command)
		if summary, ok := selected(summaries, "triad", "GB/s"); ok {
			compactMetricWidth(&b, "Memory Bandwidth", unit(summary.Statistics.Median, "GB/s", 2)+" (TRIAD)", 16)
		}
		if summary, ok := selected(summaries, "dependent-pointer-chase", "ns/access mean per worker"); ok {
			compactMetricWidth(&b, "Memory Latency", unit(summary.Statistics.Median, "ns/access", 2), 16)
		}
		renderCompactStability(&b, m)
	case "ram stress":
		compactMetric(&b, "Duration", unit(r.DurationSeconds, "s", 1))
	case "disk test":
		renderCompactDiskReport(&b, r, m)
	case "network test":
		for _, direction := range directions {
			renderNetworkDirection(&b, direction, r.Settings.UDP, len(directions) > 1)
		}
	case "network server":
		for _, metric := range m.Metrics {
			compactMetric(&b, metric.Label, SafeText(metric.Value))
		}
	}
	warnings, _ := visibleWarnings(r, modules)
	warnings = unique(append(warnings, extra...))
	if len(warnings) > 0 {
		fmt.Fprintln(&b, "\nWARNINGS")
		for _, warning := range warnings {
			fmt.Fprintln(&b, "- "+SafeText(warning))
		}
	}
	for _, message := range unique(r.Errors) {
		fmt.Fprintln(&b, "ERROR: "+SafeLines(message))
	}
	if r.Command == "disk test" {
		if life, ok := decode[disk.Lifecycle](r.Results["disk_lifecycle"]); ok && life.Workspace != "" {
			state := "Cleanup incomplete"
			if life.CleanupCompleted {
				state = "Cleaned"
			}
			fmt.Fprintf(&b, "\n%s\nTemporary Files: %s\n", automaticRule, state)
		}
	}
	fmt.Fprintln(&b, automaticBorder)
	_, err := io.WriteString(w, b.String())
	return err
}

func renderCompactStability(w io.Writer, m Module) {
	if value := metricValue(m, text("stability")); value != "N/A" {
		width := 15
		if m.Command == "ram test" {
			width = 16
		}
		compactMetricWidth(w, "Stability", value, width)
	}
}

func renderCompactDiskReport(w io.Writer, r report.Report, m Module) {
	if r.Settings.Path != "" {
		fmt.Fprintln(w, "Target: "+SafeText(r.Settings.Path))
	}
	groups, _ := diskGroups(r)
	if len(groups) == 0 {
		return
	}
	group := groups[len(groups)-1]
	fmt.Fprintln(w, strings.ToUpper(diskProfile(r.Settings.Profile))+"\n"+automaticRule)
	write := r.Settings.Profile == "randwrite-4k" || r.Settings.Profile == "seqwrite-1m"
	iops, bandwidth, p99 := group.ReadIOPS, group.ReadBW, group.ReadP99
	if write {
		iops, bandwidth, p99 = group.WriteIOPS, group.WriteBW, group.WriteP99
	}
	if r.Settings.Profile == "mixed-70-30" {
		if len(group.ReadIOPS) > 0 {
			compactMetric(w, "Read IOPS", strings.TrimSpace(medianText(group.ReadIOPS, "", 0)))
		}
		if len(group.ReadP99) > 0 {
			compactMetric(w, "Read P99", latencyRange(group.ReadP99)+" (job/run range)")
		}
		if len(group.WriteIOPS) > 0 {
			compactMetric(w, "Write IOPS", strings.TrimSpace(medianText(group.WriteIOPS, "", 0)))
		}
		if len(group.WriteP99) > 0 {
			compactMetric(w, "Write P99", latencyRange(group.WriteP99)+" (job/run range)")
		}
	} else {
		if strings.Contains(r.Settings.Profile, "1m") {
			if len(bandwidth) > 0 {
				compactMetric(w, "Throughput", medianText(bandwidth, "MiB/s", 2))
			}
		} else if len(iops) > 0 {
			compactMetric(w, "IOPS", strings.TrimSpace(medianText(iops, "", 0)))
		}
		if len(p99) > 0 {
			compactMetric(w, "P99 Latency", latencyRange(p99)+" (job/run range)")
		}
	}
	if r.Settings.Profile == "qd-scaling" {
		fmt.Fprintln(w, "Observed at highest tested depth; scaling in --details")
	}
	renderCompactStability(w, m)
}

type reportNetworkDirection struct {
	name         string
	observations networkObservations
}

func reportNetworkDirections(r report.Report) []reportNetworkDirection {
	name := "Upload"
	if r.Settings.Reverse && !r.Settings.Bidir {
		name = "Download"
	}
	out := []reportNetworkDirection{{name: name}}
	if r.Settings.Bidir {
		out = append(out, reportNetworkDirection{name: "Download"})
	}
	runs, _ := decode[[]network.Result](r.Results["network test"])
	for _, run := range runs {
		out[0].observations.add(run, "sum_received", "sum_sent", r.Settings.UDP)
		if r.Settings.Bidir {
			out[1].observations.add(run, "sum_received_bidir_reverse", "sum_sent_bidir_reverse", r.Settings.UDP)
		}
	}
	return out
}

func renderNetworkDirection(w io.Writer, direction reportNetworkDirection, udp, multiple bool) {
	width := 15
	if multiple {
		width = 20
	}
	metric := func(label, value string) { compactMetricWidth(w, label, value, width) }
	o := direction.observations
	if len(o.speed) == 0 {
		return
	}
	protocol := "TCP"
	if udp {
		protocol = "UDP"
	}
	label := protocol + " Throughput"
	if multiple {
		label = protocol + " " + direction.name
	}
	metric(label, networkRate(description(o.speed).Median))
	prefix := protocol
	if multiple {
		prefix = direction.name
	}
	if !udp && len(o.retransmits) > 0 {
		value := countRange(o.retransmits)
		if len(o.retransmits) < len(o.speed) {
			value += " (recorded runs only)"
		}
		metric(prefix+" Retransmits", value)
	}
	if udp {
		if len(o.loss) > 0 {
			metric(prefix+" Loss", percentRange(o.loss)+" (per-run range)")
		}
		if len(o.jitter) > 0 {
			metric(prefix+" Jitter", rangeText(o.jitter, "ms", 3)+" (per-run range)")
		}
	}
	if !multiple {
		metric("Direction", direction.name)
	}
}
