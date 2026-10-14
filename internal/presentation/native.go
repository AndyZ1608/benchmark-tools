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
func errorMetric(r report.Report, key, status, lang string) string {
	n, ok := integrity(r, key)
	if !ok {
		return text(lang, "unavailable")
	}
	value := number(float64(n), 0)
	if status == "failed" {
		value += " (" + tr(lang, "partial results", "dữ liệu một phần") + ")"
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
	if !present || json.Unmarshal(b, &v) != nil || !finite(v) {
		return 0, false
	}
	return v, true
}
func telemetryMetric(r report.Report, key, lang string) string {
	v, ok := telemetry(r, key)
	if !ok {
		return text(lang, "unavailable")
	}
	return unit(v, "%", 2)
}
func nativeTable(lang string, s []cpu.Summary) Table {
	t := Table{Title: tr(lang, "Workload repetitions", "Các lần đo theo tác vụ"), Headers: []string{text(lang, "workload"), text(lang, "workers"), text(lang, "count"), text(lang, "median"), text(lang, "min"), text(lang, "max"), text(lang, "cv"), tr(lang, "Scaling", "Mức mở rộng"), tr(lang, "Efficiency", "Hiệu suất")}}
	for _, v := range s {
		factor, efficiency := "N/A", "N/A"
		if v.Workers > 1 && v.ScalingFactor > 0 && finite(v.ScalingEfficiency) {
			factor = number(v.ScalingFactor, 2) + "x"
			efficiency = unit(v.ScalingEfficiency*100, "%", 1)
		}
		t.Rows = append(t.Rows, []string{v.Workload, strconv.Itoa(v.Workers), strconv.Itoa(v.Statistics.Count), unit(v.Statistics.Median, v.Unit, 3), number(v.Statistics.Min, 3), number(v.Statistics.Max, 3), cvText(lang, v.Statistics), factor, efficiency})
	}
	return t
}
func cpuModule(r report.Report, m Module, lang string, threshold float64) Module {
	if m.Command == "cpu stress" {
		m.Metrics = []Metric{{text(lang, "busy"), telemetryMetric(r, "busy_percent", lang)}, {text(lang, "steal"), telemetryMetric(r, "steal_percent", lang)}, {text(lang, "elapsed"), unit(r.DurationSeconds, "s", 1)}}
		m.Assessment = text(lang, "stress_done")
		m.Notes = []string{text(lang, "stress_note")}
		return m
	}
	s := nativeSummaries(r, m.Command)
	var stats []statistics.Summary
	for _, k := range []struct{ workload, label string }{{"integer-xorshift64", "integer_scaling"}, {"fp-multiply-add", "float_scaling"}} {
		v, ok := selected(s, k.workload, "defined_iterations/s")
		value := text(lang, "unavailable")
		if ok {
			stats = append(stats, v.Statistics)
			if v.Workers > 1 && v.ScalingFactor > 0 && finite(v.ScalingEfficiency) {
				value = fmt.Sprintf("%s (%d %s)", unit(v.ScalingEfficiency*100, "%", 1), v.Workers, text(lang, "workers"))
			} else {
				m.Notes = append(m.Notes, text(lang, "single_only"))
			}
		}
		m.Metrics = append(m.Metrics, Metric{text(lang, k.label), value})
	}
	label, assessment := stability(lang, stats, threshold)
	if len(stats) < 2 {
		label = text(lang, "insufficient")
		assessment = text(lang, "missing_note")
	}
	m.Metrics = append(m.Metrics, Metric{text(lang, "steal"), telemetryMetric(r, "steal_percent", lang)}, Metric{text(lang, "integrity"), errorMetric(r, m.Command, m.Status, lang)}, Metric{text(lang, "stability"), label})
	m.Assessment = assessment
	m.Notes = append(m.Notes, text(lang, "cpu_note"), text(lang, "baseline"))
	if v, ok := telemetry(r, "steal_percent"); ok && v > 0 {
		m.Warnings = append(m.Warnings, text(lang, "steal_note"))
	}
	if n, ok := integrity(r, m.Command); ok && n > 0 {
		m.Assessment = text(lang, "investigate")
	}
	m.Tables = append(m.Tables, nativeTable(lang, s))
	return m
}
func cacheWarning(r report.Report, workers int, lang string) string {
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
		return text(lang, "cache_unknown")
	}
	dataset, e := config.Bytes(r.Settings.Size)
	if e != nil || workers < 1 {
		return text(lang, "cache_unknown")
	}
	if dataset/int64(workers)/3 < 4*largest {
		return text(lang, "cache_small")
	}
	return ""
}
func memoryModule(r report.Report, m Module, lang string, threshold float64) Module {
	s := nativeSummaries(r, m.Command)
	if m.Command == "ram stress" {
		raw, _ := decode[[]json.RawMessage](r.Results[m.Command])
		sets := text(lang, "unavailable")
		if len(raw) > 0 {
			sets = number(float64(len(raw)), 0)
		}
		m.Metrics = []Metric{{text(lang, "integrity"), errorMetric(r, m.Command, m.Status, lang)}, {tr(lang, "Recorded result sets", "Bộ kết quả đã ghi"), sets}, {text(lang, "elapsed"), unit(r.DurationSeconds, "s", 1)}}
		m.Assessment = tr(lang, "Integrity checks finished for the recorded samples.", "Đã kiểm tra dữ liệu trong các mẫu được ghi nhận.")
		m.Notes = []string{tr(lang, "Completion does not prove hardware reliability.", "Hoàn thành không chứng minh phần cứng đáng tin cậy.")}
		m.Tables = append(m.Tables, nativeTable(lang, s))
		return m
	}
	bw, bwOK := selected(s, "triad", "GB/s")
	lat, latOK := selected(s, "dependent-pointer-chase", "ns/access mean per worker")
	bandwidth, latency := text(lang, "unavailable"), text(lang, "unavailable")
	var stats []statistics.Summary
	workers := r.Settings.Workers
	if bwOK {
		workers = bw.Workers
		bandwidth = fmt.Sprintf("%s (%d %s)", unit(bw.Statistics.Median, "GB/s", 2), bw.Workers, text(lang, "workers"))
		stats = append(stats, bw.Statistics)
	}
	if latOK {
		latency = unit(lat.Statistics.Median, "ns", 2)
		stats = append(stats, lat.Statistics)
	}
	label, assessment := stability(lang, stats, threshold)
	if !bwOK || !latOK {
		label = text(lang, "insufficient")
		assessment = text(lang, "missing_note")
	}
	m.Metrics = []Metric{{text(lang, "bandwidth"), bandwidth}, {text(lang, "latency"), latency}, {text(lang, "stability"), label}, {text(lang, "integrity"), errorMetric(r, m.Command, m.Status, lang)}}
	m.Assessment = assessment
	m.Notes = []string{text(lang, "ram_note"), text(lang, "baseline")}
	if bwOK {
		if w := cacheWarning(r, workers, lang); w != "" {
			m.Warnings = append(m.Warnings, w)
		}
	}
	if n, ok := integrity(r, m.Command); ok && n > 0 {
		m.Assessment = text(lang, "investigate")
	}
	m.Tables = append(m.Tables, nativeTable(lang, s))
	return m
}
