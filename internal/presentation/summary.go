package presentation

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"vmbench/internal/report"
)

func settings(r report.Report) (string, float64) {
	lang := r.Settings.Lang
	if lang != "en" {
		lang = "vi"
	}
	threshold := r.Settings.StabilityCV
	if r.Settings.Lang == "" {
		threshold = 5
	}
	return lang, threshold
}
func moduleState(r report.Report, key string) string {
	states, ok := decode[map[string]string](r.Results["execution_status"])
	if ok {
		if state, found := states[key]; found {
			return state
		}
	}
	if r.Command == key {
		return r.Status
	}
	if key == "network test" && r.Settings.Target == "" {
		return "skipped"
	}
	if r.Results[key] != nil && r.Status == "completed" {
		return "completed"
	}
	return "not_run"
}
func Modules(r report.Report) []Module {
	lang, threshold := settings(r)
	commands := []string{r.Command}
	if r.Command == "full test" {
		commands = []string{"cpu test", "ram test", "disk test", "network test"}
	}
	out := []Module{}
	for _, cmd := range commands {
		m := Module{Command: cmd, Status: moduleState(r, cmd)}
		switch {
		case strings.HasPrefix(cmd, "cpu "):
			m = cpuModule(r, m, lang, threshold)
		case strings.HasPrefix(cmd, "ram "):
			m = memoryModule(r, m, lang, threshold)
		case cmd == "disk test":
			m = diskModule(r, m, lang, threshold)
		case strings.HasPrefix(cmd, "network "):
			m = networkModule(r, m, lang)
		}
		switch m.Status {
		case "failed":
			m.Assessment = text(lang, "failure_note")
		case "skipped":
			m.Assessment = text(lang, "peer_missing")
			m.Metrics = nil
			m.Notes = nil
		case "not_run":
			m.Assessment = text(lang, "notrun_note")
			m.Metrics = nil
			m.Notes = nil
		}
		m.Notes = unique(m.Notes)
		m.Warnings = unique(m.Warnings)
		out = append(out, m)
	}
	return out
}
func ColorEnabled(f *os.File) bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	info, e := f.Stat()
	return e == nil && info.Mode()&os.ModeCharDevice != 0
}
func statusText(lang, status string, color bool) string {
	value := text(lang, status)
	if !color {
		return value
	}
	code := "33"
	if status == "completed" {
		code = "32"
	}
	if status == "failed" {
		code = "31"
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}
func table(w io.Writer, t Table) {
	if len(t.Rows) == 0 {
		return
	}
	if t.Title != "" {
		fmt.Fprintln(w, t.Title)
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(t.Headers, "\t"))
	for _, row := range t.Rows {
		safe := append([]string{}, row...)
		for i := range safe {
			safe[i] = clean(safe[i])
		}
		fmt.Fprintln(tw, strings.Join(safe, "\t"))
	}
	_ = tw.Flush()
}
func brief(m Module) string {
	if m.Status == "skipped" || m.Status == "not_run" {
		return m.Assessment
	}
	if len(m.Metrics) == 0 {
		return "N/A"
	}
	if m.Command == "disk test" && len(m.Metrics) > 1 {
		return m.Metrics[0].Value + "; " + m.Metrics[1].Value
	}
	if m.Command == "cpu test" && len(m.Metrics) > 1 {
		return "Integer: " + m.Metrics[0].Value + "; Floating Point: " + m.Metrics[1].Value
	}
	return m.Metrics[0].Label + ": " + m.Metrics[0].Value
}
func Render(w io.Writer, r report.Report, path string, color bool) error {
	if r.Settings.JSON {
		return json.NewEncoder(w).Encode(r)
	}
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		color = false
	}
	lang, threshold := settings(r)
	modules := Modules(r)
	fmt.Fprintln(w)
	if r.Command == "full test" {
		overall := r.Status
		for _, m := range modules {
			if m.Status == "failed" {
				overall = "failed"
			}
		}
		fmt.Fprintf(w, "VMBENCH - %s | %s\n", text(lang, "full"), statusText(lang, overall, color))
		t := Table{Headers: []string{text(lang, "workload"), text(lang, "status"), text(lang, "assessment")}}
		for _, m := range modules {
			value := brief(m)
			if m.Status == "failed" {
				value = tr(lang, "Partial observations: ", "Dữ liệu một phần: ") + value
			}
			t.Rows = append(t.Rows, []string{strings.ToUpper(strings.Fields(m.Command)[0]), text(lang, m.Status), value})
		}
		table(w, t)
		fmt.Fprintln(w, text(lang, "baseline"))
		if r.Settings.Details {
			for _, m := range modules {
				renderModule(w, r, m, lang, threshold, color)
			}
		}
	} else {
		for _, m := range modules {
			renderModule(w, r, m, lang, threshold, color)
		}
	}
	warnings := []string{}
	for _, m := range modules {
		warnings = append(warnings, m.Warnings...)
	}
	for _, warn := range r.Warnings {
		if strings.HasPrefix(warn, "Run-to-run coefficient of variation exceeds") {
			if r.Settings.Details {
				warnings = append(warnings, warn+" ("+tr(lang, "legacy report rule: 5%; display rule", "ngưỡng báo cáo cũ: 5%; ngưỡng hiển thị")+" "+number(threshold, 2)+"%)")
			}
			continue
		}
		warnings = append(warnings, warningText(lang, warn))
	}
	warnings = unique(warnings)
	sort.SliceStable(warnings, func(i, j int) bool { return warningPriority(warnings[i]) < warningPriority(warnings[j]) })
	if len(warnings) > 0 {
		fmt.Fprintln(w, "\n"+text(lang, "warnings")+":")
		limit := len(warnings)
		if !r.Settings.Details && limit > 3 {
			limit = 3
		}
		for _, warning := range warnings[:limit] {
			fmt.Fprintln(w, "  - "+clean(warning))
		}
		if limit < len(warnings) {
			fmt.Fprintf(w, "  +%d: %s\n", len(warnings)-limit, text(lang, "more_warnings"))
		}
	}
	if len(r.Errors) > 0 {
		fmt.Fprintln(w, "\n"+text(lang, "errors")+":")
		for _, e := range unique(r.Errors) {
			fmt.Fprintln(w, "  - "+clean(e))
		}
	}
	fmt.Fprintf(w, "\n%s: %s\n", text(lang, "elapsed"), unit(r.DurationSeconds, "s", 2))
	if path != "" {
		fmt.Fprintf(w, "%s: %s\n", text(lang, "report"), clean(path))
	} else {
		fmt.Fprintln(w, text(lang, "no_report"))
	}
	return nil
}
func warningPriority(w string) int {
	for _, s := range []string{"LLC", "cache effects", "bộ nhớ đệm", "system filesystem", "hệ điều hành", "integrity", "CPU steal", "CPU steal:"} {
		if strings.Contains(w, s) {
			return 0
		}
	}
	return 1
}
func renderModule(w io.Writer, r report.Report, m Module, lang string, threshold float64, color bool) {
	headerState := m.Status
	if r.Command == m.Command && r.Status == "failed" {
		headerState = "failed"
	}
	fmt.Fprintf(w, "VMBENCH %s | %s\n", strings.ToUpper(m.Command), statusText(lang, headerState, color))
	metrics := Table{}
	for _, v := range m.Metrics {
		metrics.Rows = append(metrics.Rows, []string{v.Label, v.Value})
	}
	table(w, metrics)
	fmt.Fprintf(w, "\n%s: %s\n", text(lang, "assessment"), m.Assessment)
	notes := m.Notes
	if !r.Settings.Details {
		switch m.Command {
		case "cpu test":
			notes = []string{tr(lang, "Scaling is not absolute CPU speed; no comparable baseline is available for speed ranking.", "Hiệu suất đa luồng không phải tốc độ CPU tuyệt đối; chưa có dữ liệu đối chiếu để xếp hạng tốc độ.")}
		case "ram test":
			notes = []string{tr(lang, "Application measurements include cache/loop effects; no comparable baseline is available for speed ranking.", "Số đo ứng dụng chịu ảnh hưởng của bộ nhớ đệm/vòng lặp; chưa có dữ liệu đối chiếu để xếp hạng tốc độ.")}
		case "disk test":
			notes = []string{tr(lang, "IOPS counts operations/second; p99 ranges retain each job/run distribution, and speed ranking needs a comparable baseline.", "IOPS là số thao tác/giây; khoảng p99 giữ riêng phân bố từng job/lần đo, và xếp hạng tốc độ cần dữ liệu đối chiếu.")}
		}
		if m.Status == "skipped" || m.Status == "not_run" {
			notes = nil
		}
	}
	for _, n := range notes {
		fmt.Fprintln(w, n)
	}
	if r.Settings.Details {
		fmt.Fprintln(w, "\n"+text(lang, "details")+":")
		fmt.Fprintln(w, statsNote(lang, threshold))
		for _, t := range m.Tables {
			table(w, t)
		}
		if strings.HasPrefix(m.Command, "cpu ") {
			table(w, Table{Title: tr(lang, "Whole-run guest telemetry (includes setup)", "Thông tin VM toàn lần chạy (bao gồm chuẩn bị)"), Rows: [][]string{{text(lang, "busy"), telemetryMetric(r, "busy_percent", lang)}, {text(lang, "steal"), telemetryMetric(r, "steal_percent", lang)}, {tr(lang, "Recorded cgroup throttle events", "Số lần cgroup bị giới hạn đã ghi"), throttleDelta(r, "nr_throttled", 1, "")}, {tr(lang, "Recorded cgroup throttled time", "Thời gian cgroup bị giới hạn đã ghi"), throttleDelta(r, "throttled_usec", 1000, "ms")}}})
		}
	}
}

// GeneratedWarnings adds measurement caveats to persisted reports in English.
// Presentation translations never replace the original raw observations.
func GeneratedWarnings(r report.Report) []string {
	r.Settings.Lang = "en"
	r.Settings.Details = false
	out := []string{}
	for _, m := range Modules(r) {
		out = append(out, m.Warnings...)
	}
	return unique(out)
}

func throttleDelta(r report.Report, key string, div float64, units string) string {
	snapshots, ok := decode[map[string]json.RawMessage](r.Results["cpu_telemetry"])
	if !ok {
		return "N/A"
	}
	values := []uint64{}
	for _, name := range []string{"throttling_before", "throttling_after"} {
		var raw string
		if json.Unmarshal(snapshots[name], &raw) != nil {
			return "N/A"
		}
		found := false
		for _, line := range strings.Split(raw, "\n") {
			parts := strings.Fields(line)
			if len(parts) == 2 && parts[0] == key {
				v, e := strconv.ParseUint(parts[1], 10, 64)
				if e == nil {
					values = append(values, v)
					found = true
				}
				break
			}
		}
		if !found {
			return "N/A"
		}
	}
	if values[1] < values[0] {
		return "N/A"
	}
	v := float64(values[1]-values[0]) / div
	if units == "" {
		return number(v, 0)
	}
	return unit(v, units, 2)
}
