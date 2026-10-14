package presentation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/config"
	"vmbench/internal/report"
	"vmbench/internal/statistics"
)

func nativeSummaries(r report.Report, key string) []cpu.Summary {
	if s, ok := decode[[]cpu.Summary](r.Results[key+"_statistics"]); ok {
		return s
	}
	raw, ok := decode[[]json.RawMessage](r.Results[key])
	if !ok {
		return nil
	}
	s, e := cpu.Summarize(raw)
	if e != nil {
		return nil
	}
	return s
}
func selected(s []cpu.Summary, workload, unitName string) (cpu.Summary, bool) {
	var best cpu.Summary
	found := false
	for _, v := range s {
		if v.Workload == workload && v.Unit == unitName && v.Statistics.Count > 0 && v.Statistics.Median > 0 && finite(v.Statistics.Median) && (!found || v.Workers > best.Workers) {
			best = v
			found = true
		}
	}
	return best, found
}
func integrity(r report.Report, key string) (uint64, bool) {
	type entry struct {
		Measurements []struct {
			Errors *uint64 `json:"integrity_errors"`
		} `json:"measurements"`
	}
	runs, ok := decode[[]entry](r.Results[key])
	if !ok || len(runs) == 0 {
		return 0, false
	}
	var total uint64
	for _, run := range runs {
		if len(run.Measurements) == 0 {
			return total, false
		}
		for _, m := range run.Measurements {
			if m.Errors == nil {
				return total, false
			}
			total += *m.Errors
		}
	}
	return total, true
}
func errorMetric(r report.Report, key, status string) string {
	n, ok := integrity(r, key)
	if !ok {
		return text("unavailable")
	}
	value := number(float64(n), 0)
	if status == "failed" {
		value += " (partial results)"
	}
	return value
}
func telemetry(r report.Report, key string) (float64, bool) {
	m, ok := decode[map[string]json.RawMessage](r.Results["cpu_telemetry"])
	if !ok {
		return 0, false
	}
	b, present := m[key]
	var v float64
	if !present || json.Unmarshal(b, &v) != nil || (!finite(v) || v < 0 || v > 100) {
		return 0, false
	}
	return v, true
}
func telemetryMetric(r report.Report, key string) string {
	v, ok := telemetry(r, key)
	if !ok {
		return text("unavailable")
	}
	return unit(v, "%", 2)
}
func nativeTable(s []cpu.Summary) Table {
	t := Table{Title: "Workload repetitions", Headers: []string{text("workload"), text("workers"), text("count"), text("median"), text("min"), text("max"), text("cv"), "Scaling", "Efficiency"}}
	for _, v := range s {
		factor, efficiency := "N/A", "N/A"
		if v.Workers > 1 && v.ScalingFactor > 0 && finite(v.ScalingEfficiency) {
			factor = number(v.ScalingFactor, 2) + "x"
			efficiency = unit(v.ScalingEfficiency*100, "%", 1)
		}
		t.Rows = append(t.Rows, []string{v.Workload, strconv.Itoa(v.Workers), strconv.Itoa(v.Statistics.Count), unit(v.Statistics.Median, v.Unit, 3), number(v.Statistics.Min, 3), number(v.Statistics.Max, 3), cvText(v.Statistics), factor, efficiency})
	}
	return t
}
func cpuModule(r report.Report, m Module, threshold float64) Module {
	if warning := cpuStealWarning(r); warning != "" {
		m.Warnings = append(m.Warnings, warning)
	}
	if m.Command == "cpu stress" {
		m.Metrics = []Metric{{text("busy"), telemetryMetric(r, "busy_percent")}, {text("steal"), telemetryMetric(r, "steal_percent")}, {text("elapsed"), unit(r.DurationSeconds, "s", 1)}}
		m.Assessment = text("stress_done")
		m.Notes = []string{text("stress_note")}
		return m
	}
	s := nativeSummaries(r, m.Command)
	var stats []statistics.Summary
	for _, k := range []struct{ workload, label string }{{"integer-xorshift64", "integer_scaling"}, {"fp-multiply-add", "float_scaling"}} {
		v, ok := selected(s, k.workload, "defined_iterations/s")
		value := text("unavailable")
		if ok {
			stats = append(stats, v.Statistics)
			if v.Workers > 1 && v.ScalingFactor > 0 && finite(v.ScalingEfficiency) {
				value = fmt.Sprintf("%s (%d %s)", unit(v.ScalingEfficiency*100, "%", 1), v.Workers, text("workers"))
			} else {
				m.Notes = append(m.Notes, text("single_only"))
			}
		}
		m.Metrics = append(m.Metrics, Metric{text(k.label), value})
	}
	label, assessment := stability(stats, threshold)
	if len(stats) < 2 {
		label = text("insufficient")
		assessment = text("missing_note")
	}
	m.Metrics = append(m.Metrics, Metric{text("steal"), telemetryMetric(r, "steal_percent")}, Metric{"Computation errors", errorMetric(r, m.Command, m.Status)}, Metric{text("stability"), label})
	m.Assessment = assessment
	m.Notes = append(m.Notes, text("cpu_note"), text("baseline"))
	m = integrityAssessment(r, m)
	m.Tables = append(m.Tables, nativeTable(s))
	return m
}
func cacheWarning(r report.Report, workers int) string {
	largest := int64(0)
	for path, level := range r.System.Raw {
		if strings.HasSuffix(path, "/level") && strings.TrimSpace(level) == "3" {
			size := r.System.Raw[strings.TrimSuffix(path, "level")+"size"]
			var n int64
			var e error
			if strings.HasSuffix(size, "K") {
				n, e = strconv.ParseInt(strings.TrimSuffix(size, "K"), 10, 64)
				n *= 1024
			} else if strings.HasSuffix(size, "M") {
				n, e = strconv.ParseInt(strings.TrimSuffix(size, "M"), 10, 64)
				n *= 1024 * 1024
			}
			if e == nil && n > largest {
				largest = n
			}
		}
	}
	if largest == 0 {
		return text("cache_unknown")
	}
	dataset, e := config.Bytes(r.Settings.Size)
	if e != nil || workers < 1 {
		return text("cache_unknown")
	}
	if dataset/int64(workers)/3 < 4*largest {
		return text("cache_small")
	}
	return ""
}
func memoryModule(r report.Report, m Module, threshold float64) Module {
	s := nativeSummaries(r, m.Command)
	if m.Command == "ram stress" {
		raw, _ := decode[[]json.RawMessage](r.Results[m.Command])
		sets := text("unavailable")
		if len(raw) > 0 {
			sets = number(float64(len(raw)), 0)
		}
		m.Metrics = []Metric{{text("integrity"), errorMetric(r, m.Command, m.Status)}, {"Recorded result sets", sets}, {text("elapsed"), unit(r.DurationSeconds, "s", 1)}}
		m.Assessment = "Integrity checks finished for the recorded samples."
		m = integrityAssessment(r, m)
		m.Notes = []string{"Completion does not prove hardware reliability."}
		m.Tables = append(m.Tables, nativeTable(s))
		return m
	}
	bw, bwOK := selected(s, "triad", "GB/s")
	lat, latOK := selected(s, "dependent-pointer-chase", "ns/access mean per worker")
	bandwidth, latency := text("unavailable"), text("unavailable")
	var stats []statistics.Summary
	workers := r.Settings.Workers
	if bwOK {
		workers = bw.Workers
		bandwidth = unit(bw.Statistics.Median, "GB/s", 2)
		stats = append(stats, bw.Statistics)
	}
	if latOK {
		latency = unit(lat.Statistics.Median, "ns", 2)
		stats = append(stats, lat.Statistics)
	}
	label, assessment := stability(stats, threshold)
	if !bwOK || !latOK {
		label = text("insufficient")
		assessment = text("missing_note")
	}
	m.Metrics = []Metric{{text("bandwidth"), bandwidth}, {text("latency"), latency}, {text("stability"), label}, {text("integrity"), errorMetric(r, m.Command, m.Status)}}
	m.Assessment = assessment
	m.Notes = []string{text("ram_note"), text("baseline")}
	if bwOK {
		if w := cacheWarning(r, workers); w != "" {
			m.Warnings = append(m.Warnings, w)
		}
	}
	m = integrityAssessment(r, m)
	m.Tables = append(m.Tables, nativeTable(s))
	return m
}

func integrityAssessment(r report.Report, m Module) Module {
	count, ok := integrity(r, m.Command)
	if !ok {
		m.Warnings = append(m.Warnings, "Integrity results are unavailable; correctness cannot be confirmed.")
		return m
	}
	if count > 0 {
		m.Status = "failed"
		m.Assessment = text("investigate")
		m.Warnings = append(m.Warnings, "Integrity errors were detected; investigate before relying on these results.")
		return m
	}
	if m.Command == "cpu test" {
		m.Assessment += " No computation errors were reported."
	} else {
		m.Assessment += " No memory integrity errors were reported."
	}
	return m
}
