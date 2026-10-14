package presentation

import (
	"fmt"
	"vmbench/internal/statistics"
)

func cvText(s statistics.Summary) string {
	if s.Count < 2 {
		return "N/A"
	}
	v := unit(s.CV*100, "%", 2)
	if s.Count == 2 {
		v += " (" + "limited evidence" + ")"
	}
	return v
}
func stability(stats []statistics.Summary, threshold float64) (string, string) {
	if len(stats) == 0 {
		return text("insufficient"), text("missing_note")
	}
	maxCV := 0.0
	for _, s := range stats {
		if s.Count < 3 || !finite(s.CV) || s.CV < 0 || !finite(s.Mean) || s.Mean <= 0 {
			return text("insufficient"), text("limited")
		}
		if s.CV > maxCV {
			maxCV = s.CV
		}
	}
	if maxCV*100 > threshold {
		return text("variable"), text("varied")
	}
	return text("stable"), text("consistent")
}
func statsNote(threshold float64) string {
	return fmt.Sprintf("%s: CV <= %s; %s", "Consistency rule", unit(threshold, "%", 2), "at least 3 independent repetitions; this is a heuristic, not a confidence guarantee")
}
