package presentation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/report"
	"vmbench/internal/system"
)

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
	commands := []string{r.Command}
	if r.Command == "full test" {
		commands = []string{"cpu test", "ram test", "disk test", "network test"}
	}
	out := []Module{}
	for _, command := range commands {
		m := Module{Command: command, Status: moduleState(r, command)}
		if m.Status == "skipped" || m.Status == "not_run" {
			if m.Status == "skipped" {
				m.Assessment = text("peer_missing")
			} else {
				m.Assessment = text("notrun_note")
			}
			out = append(out, m)
			continue
		}
		switch {
		case strings.HasPrefix(command, "cpu "):
			m = cpuModule(r, m, r.Settings.StabilityCV)
		case strings.HasPrefix(command, "ram "):
			m = memoryModule(r, m, r.Settings.StabilityCV)
		case command == "disk test":
			m = diskModule(r, m, r.Settings.StabilityCV)
		case strings.HasPrefix(command, "network "):
			m = networkModule(r, m)
		}
		if m.Status == "completed" && strings.HasPrefix(m.Assessment, text("missing_note")) {
			m.Status = "failed"
			m.Warnings = append(m.Warnings, text("missing_note"))
		}
		switch m.Status {
		case "failed":
			m.Assessment = text("failure_note")
			m.Warnings = append(m.Warnings, "Test failed; resolve the errors before rerunning.")
		case "skipped":
			m.Assessment = text("peer_missing")
			m.Metrics = nil
			m.Notes = nil
		case "not_run":
			m.Assessment = text("notrun_note")
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
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
func statusText(status string, color bool) string {
	value := text(status)
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

const divider = "================================================"
const separator = "------------------------------------------------"

func table(w io.Writer, t Table) {
	if len(t.Rows) == 0 {
		return
	}
	if t.Title != "" {
		fmt.Fprintln(w, clean(t.Title))
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if len(t.Headers) > 0 {
		headers := append([]string{}, t.Headers...)
		for i := range headers {
			headers[i] = clean(headers[i])
		}
		fmt.Fprintln(tw, strings.Join(headers, "\t"))
	}
	for _, row := range t.Rows {
		safe := append([]string{}, row...)
		for i := range safe {
			safe[i] = clean(safe[i])
		}
		fmt.Fprintln(tw, strings.Join(safe, "\t"))
	}
	_ = tw.Flush()
}
func metricValue(m Module, label string) string {
	for _, v := range m.Metrics {
		if v.Label == label {
			return v.Value
		}
	}
	return "N/A"
}
func brief(m Module) string {
	if m.Status == "skipped" || m.Status == "not_run" {
		return m.Assessment
	}
	if len(m.Metrics) == 0 {
		return "N/A"
	}
	switch m.Command {
	case "cpu test":
		return "Integer: " + metricValue(m, text("integer_scaling")) + "; FP: " + metricValue(m, text("float_scaling"))
	case "disk test":
		return metricValue(m, text("iops")) + "; p99 " + metricValue(m, text("p99"))
	case "ram test":
		return metricValue(m, text("bandwidth"))
	case "network test":
		if len(m.Metrics) > 1 {
			return m.Metrics[1].Label + ": " + m.Metrics[1].Value
		}
	}
	return m.Metrics[0].Label + ": " + m.Metrics[0].Value
}

// Render displays observations only. It never writes report files or emits JSON.
func Render(w io.Writer, r report.Report, color bool) error {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		color = false
	}
	if !r.Settings.Details && r.Command != "full test" {
		return renderCompactReport(w, r, color)
	}
	var b bytes.Buffer
	modules := Modules(r)
	if r.Command == "full test" {
		overall := r.Status
		if len(r.Errors) > 0 {
			overall = "failed"
		}
		counts := map[string]int{}
		for _, m := range modules {
			counts[m.Status]++
			if m.Status == "failed" {
				overall = "failed"
			}
			if m.Status == "partial" && overall == "completed" {
				overall = "partial"
			}
		}
		fmt.Fprintln(&b, divider+"\nVMBENCH FULL TEST\n"+divider)
		fmt.Fprintln(&b, "Status: "+statusText(overall, color))
		fmt.Fprintf(&b, "Duration: %s seconds\n\n", number(r.DurationSeconds, 2))
		t := Table{Headers: []string{"Module", "Status", "Result"}}
		for _, m := range modules {
			value := brief(m)
			if m.Status == "failed" || m.Status == "partial" {
				value = "Partial observations: " + value
			}
			t.Rows = append(t.Rows, []string{strings.ToUpper(strings.Fields(m.Command)[0]), text(m.Status), value})
		}
		table(&b, t)
		if counts["partial"] > 0 {
			fmt.Fprintf(&b, "\nOverall: %d completed, %d skipped, %d failed, %d partial, %d not run\n", counts["completed"], counts["skipped"], counts["failed"], counts["partial"], counts["not_run"])
		} else {
			fmt.Fprintf(&b, "\nOverall: %d completed, %d skipped, %d failed, %d not run\n", counts["completed"], counts["skipped"], counts["failed"], counts["not_run"])
		}
		if r.Settings.Details {
			for _, m := range modules {
				fmt.Fprintln(&b)
				renderModule(&b, r, m, color)
			}
		}
	} else {
		for _, m := range modules {
			renderModule(&b, r, m, color)
		}
	}
	warnings, notes := visibleWarnings(r, modules)
	renderMessages(&b, "WARNINGS", warnings)
	if r.Settings.Details {
		renderMessages(&b, "MEASUREMENT NOTES", notes)
	}
	renderMessages(&b, "ERRORS", unique(r.Errors))
	fmt.Fprintln(&b, divider)
	_, err := io.Copy(w, &b)
	return err
}
func renderMessages(w io.Writer, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintln(w, "\n"+title+"\n"+separator)
	for _, value := range values {
		fmt.Fprintln(w, "- "+clean(value))
	}
}
func renderModule(w io.Writer, r report.Report, m Module, color bool) {
	state := m.Status
	if r.Command == m.Command && (r.Status == "failed" || r.Status == "partial") {
		state = r.Status
	}
	title := strings.ToUpper(m.Command)
	if strings.HasPrefix(title, "RAM ") {
		title = "MEMORY " + strings.TrimPrefix(title, "RAM ")
	}
	fmt.Fprintln(w, divider+"\nVMBENCH "+title+"\n"+divider)
	fmt.Fprintln(w, "Status: "+statusText(state, color))
	if r.Command != "full test" {
		fmt.Fprintf(w, "Duration: %s seconds\n", number(r.DurationSeconds, 2))
	}
	if len(m.Context) > 0 {
		t := Table{}
		for _, v := range m.Context {
			t.Rows = append(t.Rows, []string{v.Label, " : " + v.Value})
		}
		table(w, t)
	}
	if len(m.Metrics) > 0 {
		fmt.Fprintln(w, "\nPERFORMANCE\n"+separator)
		t := Table{}
		for _, metric := range m.Metrics {
			t.Rows = append(t.Rows, []string{metric.Label, " : " + metric.Value})
		}
		table(w, t)
	}
	fmt.Fprintln(w, "\nASSESSMENT\n"+separator)
	fmt.Fprintln(w, clean(m.Assessment))
	if !r.Settings.Details && m.Status == "completed" {
		if m.Command == "cpu test" {
			fmt.Fprintln(w, text("cpu_note"))
		}
		if m.Command == "cpu stress" {
			fmt.Fprintln(w, "High CPU utilization is expected during stress testing.")
		}
		if m.Command == "network test" {
			fmt.Fprintln(w, "Endpoint throughput does not measure forwarding capacity.")
		}
	}
	if r.Settings.Details {
		fmt.Fprintln(w, "\nDETAILS\n"+separator)
		fmt.Fprintln(w, statsNote(r.Settings.StabilityCV))
		for _, t := range m.Tables {
			table(w, t)
		}
		if m.Command == "network test" {
			for _, direction := range reportNetworkDirections(r) {
				renderNetworkDirection(w, direction, r.Settings.UDP, r.Settings.Bidir)
			}
		}
		if m.Command == "cpu test" || m.Command == "ram test" {
			runs, _ := decode[[]json.RawMessage](r.Results[m.Command])
			t := Table{Title: "Recorded native measurements", Headers: []string{"Run", "Workload", "Value", "Workers", "Dataset bytes", "Seconds", "Integrity errors"}}
			for i, run := range runs {
				var native cpu.NativeResult
				if json.Unmarshal(run, &native) == nil {
					appendNativeMeasurements(&t, i+1, native)
				}
			}
			table(w, t)
		}
		if strings.HasPrefix(m.Command, "cpu ") {
			table(w, Table{Title: "Whole-run guest telemetry (includes setup)", Rows: [][]string{{"CPU busy", telemetryMetric(r, "busy_percent")}, {"CPU steal", telemetryMetric(r, "steal_percent")}, {"Cgroup throttle events", throttleDelta(r, "nr_throttled", 1, "")}, {"Cgroup throttled time", throttleDelta(r, "throttled_usec", 1000, "ms")}}})
		}
		for _, note := range m.Notes {
			fmt.Fprintln(w, clean(note))
		}
	}
}

// RenderSystemInfo keeps the metadata command human-readable, with no raw objects.
func RenderSystemInfo(w io.Writer, s system.Info) error {
	osName := "unknown"
	for _, line := range strings.Split(s.OS, "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			osName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			break
		}
	}
	memory, limit := "N/A", "N/A"
	if s.AvailableMemory > 0 {
		memory = unit(float64(s.AvailableMemory)/(1<<30), "GiB", 2)
	}
	if s.MemoryLimit > 0 {
		limit = unit(float64(s.MemoryLimit)/(1<<30), "GiB", 2)
	}
	var b bytes.Buffer
	fmt.Fprintln(&b, divider+"\nVMBENCH SYSTEM INFO\n"+divider)
	table(&b, Table{Rows: [][]string{{"Hostname", s.Hostname}, {"OS", osName}, {"Kernel", s.Kernel}, {"Architecture", s.Architecture}, {"CPU", s.CPUModel}, {"Logical CPUs", strconv.Itoa(s.LogicalCPUs)}, {"Available CPU capacity", number(s.Capacity, 2)}, {"CPU affinity", s.Affinity}, {"CPU quota", s.CPUQuota}, {"Available memory", memory}, {"Memory limit", limit}, {"Hypervisor", s.Hypervisor}}})
	fmt.Fprintln(&b, divider)
	_, err := io.Copy(w, &b)
	return err
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
