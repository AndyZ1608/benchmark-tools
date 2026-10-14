package presentation

import (
	"fmt"
	"vmbench/internal/statistics"
)

func cvText(lang string, s statistics.Summary) string {
	if s.Count < 2 {
		return "N/A"
	}
	v := unit(s.CV*100, "%", 2)
	if s.Count == 2 {
		v += " (" + tr(lang, "limited evidence", "ít dữ liệu") + ")"
	}
	return v
}
func stability(lang string, stats []statistics.Summary, threshold float64) (string, string) {
	if len(stats) == 0 {
		return text(lang, "insufficient"), text(lang, "missing_note")
	}
	maxCV := 0.0
	for _, s := range stats {
		if s.Count < 3 || !finite(s.CV) || s.CV < 0 || !finite(s.Mean) || s.Mean <= 0 {
			return text(lang, "insufficient"), text(lang, "limited")
		}
		if s.CV > maxCV {
			maxCV = s.CV
		}
	}
	if maxCV*100 > threshold {
		return text(lang, "variable"), text(lang, "varied")
	}
	return text(lang, "stable"), text(lang, "consistent")
}
func statsNote(lang string, threshold float64) string {
	return fmt.Sprintf("%s: CV <= %s; %s", tr(lang, "Consistency rule", "Quy tắc ổn định"), unit(threshold, "%", 2), tr(lang, "at least 3 independent repetitions; this is a heuristic, not a confidence guarantee", "ít nhất 3 lần đo độc lập; đây là quy tắc tham khảo, không bảo đảm độ tin cậy"))
}
