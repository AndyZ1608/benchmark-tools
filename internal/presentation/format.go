package presentation

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Metric struct{ Label, Value string }
type Detail struct{ Cells []string }
type Module struct {
	Command, Status, Assessment string
	Metrics                     []Metric
	Notes, Warnings             []string
	Tables                      []Table
}
type Table struct {
	Title   string
	Headers []string
	Rows    [][]string
}

func decode[T any](v any) (T, bool) {
	var out T
	if typed, ok := v.(T); ok {
		return typed, true
	}
	if v == nil {
		return out, false
	}
	b, e := json.Marshal(v)
	if e != nil {
		return out, false
	}
	e = json.Unmarshal(b, &out)
	return out, e == nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func number(v float64, digits int) string {
	if !finite(v) {
		return "N/A"
	}
	if v != 0 && math.Abs(v) < math.Pow10(-digits)/2 {
		return strconv.FormatFloat(v, 'g', 3, 64)
	}
	s := strconv.FormatFloat(v, 'f', digits, 64)
	parts := strings.SplitN(s, ".", 2)
	prefix := ""
	if strings.HasPrefix(parts[0], "-") {
		prefix = "-"
		parts[0] = parts[0][1:]
	}
	whole := parts[0]
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	if len(parts) == 2 {
		whole += "." + parts[1]
	}
	return prefix + whole
}
func unit(v float64, u string, d int) string { return number(v, d) + " " + u }
func rangeText(v []float64, u string, d int) string {
	if len(v) == 0 {
		return "N/A"
	}
	lo, hi := v[0], v[0]
	for _, x := range v {
		lo = math.Min(lo, x)
		hi = math.Max(hi, x)
	}
	if lo == hi {
		return unit(lo, u, d)
	}
	return fmt.Sprintf("%s - %s %s", number(lo, d), number(hi, d), u)
}
func unique(v []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range v {
		if s != "" && !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	return out
}
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 127 {
			return ' '
		}
		return r
	}, s)
}
