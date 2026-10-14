package presentation

import (
	"fmt"
	"sort"
	"strings"
	"vmbench/internal/report"
)

// cpuStealWarning is a configurable screening rule, not a hardware speed grade.
func cpuStealWarning(r report.Report) string {
	value, ok := telemetry(r, "steal_percent")
	if !ok || value <= 0 || value < r.Settings.StealWarningPercent {
		return ""
	}
	return fmt.Sprintf("CPU steal reached %s (screening threshold %s); check host CPU contention.", unit(value, "%", 2), unit(r.Settings.StealWarningPercent, "%", 2))
}
func warningClass(r report.Report, value string) (string, bool) {
	normalized := strings.ToLower(strings.Join(strings.Fields(value), " "))
	switch {
	case strings.Contains(normalized, "fewer than three repetitions"), strings.Contains(normalized, "insufficient repetitions"):
		// The stability metric is the sole insufficient-data notice.
		return "", true
	case strings.HasPrefix(normalized, "run-to-run coefficient of variation exceeds"):
		// Replaced by the selected consistency rule, which may differ from 5%.
		return "", true
	case strings.Contains(normalized, "preliminary workload"), strings.Contains(normalized, "quick test is preliminary"):
		return "Quick measurements are preliminary; use repeated runs for comparisons.", true
	case strings.Contains(normalized, "virtualization, backend contention"), strings.Contains(normalized, "host contention and caches are uncontrolled"):
		return "Host contention and cache effects are uncontrolled; additional repetitions do not guarantee accuracy.", true
	case strings.Contains(normalized, "network skipped: no peer"), normalized == "no test peer configured.":
		return "", true
	case strings.HasPrefix(normalized, "cpu steal time was detected"), strings.HasPrefix(normalized, "cpu steal reached"):
		return cpuStealWarning(r), false
	case strings.Contains(normalized, "read tests also write dataset initialization"), strings.Contains(normalized, "read tests write preparation files"):
		return "Read tests write temporary preparation files; direct I/O does not bypass every storage cache.", false
	case strings.Contains(normalized, "system filesystem"):
		return "Disk target is on the system filesystem; other workloads may be affected.", false
	case strings.Contains(normalized, "per-worker array is smaller"), strings.Contains(normalized, "working set is small relative to llc"):
		return text("cache_small"), false
	case strings.Contains(normalized, "llc size is unavailable"):
		return text("cache_unknown"), false
	}
	// Unknown warnings remain visible so new engine/safety failures cannot disappear.
	return value, false
}
func warningKey(value string) string {
	return strings.TrimRight(strings.ToLower(strings.Join(strings.Fields(value), " ")), ".! ")
}
func warningPriority(value string) int {
	s := strings.ToLower(value)
	for _, word := range []string{"fail", "error", "invalid", "integrity", "missing", "incomplete"} {
		if strings.Contains(s, word) {
			return 0
		}
	}
	for _, word := range []string{"system filesystem", "preparation files", "safety"} {
		if strings.Contains(s, word) {
			return 1
		}
	}
	return 2
}
func visibleWarnings(r report.Report, modules []Module) ([]string, []string) {
	values := append([]string{}, r.Warnings...)
	for _, m := range modules {
		values = append(values, m.Warnings...)
	}
	warnings, notes := []string{}, []string{}
	seen := map[string]bool{}
	for _, value := range values {
		message, info := warningClass(r, value)
		key := warningKey(message)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if info {
			if r.Settings.Details {
				notes = append(notes, message)
			}
		} else {
			warnings = append(warnings, message)
		}
	}
	sort.SliceStable(warnings, func(i, j int) bool { return warningPriority(warnings[i]) < warningPriority(warnings[j]) })
	return warnings, notes
}
