package cpu

import (
	"encoding/json"
	"fmt"
	"vmbench/internal/statistics"
)

type Measurement struct {
	Workload string  `json:"workload"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit"`
	Elapsed  float64 `json:"elapsed_seconds"`
	Errors   uint64  `json:"integrity_errors"`
}
type NativeResult struct {
	Kernel       string        `json:"kernel"`
	Workers      int           `json:"workers"`
	Dataset      uint64        `json:"dataset_bytes"`
	Measurements []Measurement `json:"measurements"`
}
type Summary struct {
	Workload, Unit                   string
	Workers                          int
	Statistics                       statistics.Summary
	ScalingFactor, ScalingEfficiency float64
	Warnings                         []string
}

func Parse(b []byte) (NativeResult, error) {
	var r NativeResult
	if e := json.Unmarshal(b, &r); e != nil {
		return r, e
	}
	if r.Kernel != "compute-bench-v1" || r.Workers < 1 || len(r.Measurements) == 0 {
		return r, fmt.Errorf("incompatible native benchmark result")
	}
	for _, m := range r.Measurements {
		if m.Errors != 0 || m.Elapsed <= 0 || m.Value <= 0 {
			return r, fmt.Errorf("native workload failed validation: %s", m.Workload)
		}
	}
	return r, nil
}
func Summarize(raw []json.RawMessage) ([]Summary, error) {
	groups := map[string][]float64{}
	meta := map[string]Summary{}
	single := map[string]float64{}
	for _, b := range raw {
		r, e := Parse(b)
		if e != nil {
			return nil, e
		}
		for _, m := range r.Measurements {
			k := fmt.Sprintf("%s/%d", m.Workload, r.Workers)
			groups[k] = append(groups[k], m.Value)
			meta[k] = Summary{Workload: m.Workload, Unit: m.Unit, Workers: r.Workers}
		}
	}
	var out []Summary
	for k, v := range groups {
		s := meta[k]
		s.Statistics, _ = statistics.Describe(v)
		if s.Workers == 1 {
			single[s.Workload] = s.Statistics.Median
		}
		meta[k] = s
	}
	for k := range groups {
		s := meta[k]
		if base := single[s.Workload]; base > 0 && s.Unit != "ns/access mean per worker" {
			s.ScalingFactor = s.Statistics.Median / base
			s.ScalingEfficiency = s.ScalingFactor / float64(s.Workers)
		}
		if s.Statistics.CV > .05 {
			s.Warnings = append(s.Warnings, "Run-to-run coefficient of variation exceeds 5%")
		}
		if s.Statistics.Count < 3 {
			s.Warnings = append(s.Warnings, "Fewer than three repetitions; limited stability evidence")
		}
		out = append(out, s)
	}
	return out, nil
}
