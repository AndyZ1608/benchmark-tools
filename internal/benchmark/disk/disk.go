package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"vmbench/internal/config"
)

type Latency struct {
	Mean       float64            `json:"mean"`
	Percentile map[string]float64 `json:"percentile"`
	Bins       map[string]uint64  `json:"bins"`
}
type Direction struct {
	IOPS       float64 `json:"iops"`
	BWBytes    float64 `json:"bw_bytes"`
	Runtime    float64 `json:"runtime"`
	Completion Latency `json:"clat_ns"`
	Total      Latency `json:"lat_ns"`
}
type Job struct {
	Options map[string]string  `json:"job options"`
	Error   int                `json:"error"`
	Read    Direction          `json:"read"`
	Write   Direction          `json:"write"`
	Depth   map[string]float64 `json:"iodepth_level"`
}
type Result struct {
	Options map[string]string `json:"global options"`
	Jobs    []Job             `json:"jobs"`
}

func Parse(b []byte) (Result, error) {
	var r Result
	e := json.Unmarshal(b, &r)
	if e != nil {
		return r, e
	}
	if len(r.Jobs) == 0 {
		return r, fmt.Errorf("fio returned no jobs")
	}
	for _, j := range r.Jobs {
		if j.Error != 0 {
			return r, fmt.Errorf("fio I/O error %d", j.Error)
		}
	}
	return r, nil
}
func Run(ctx context.Context, c config.Config, id string) ([]Result, []json.RawMessage, error) {
	out, err := RunManaged(ctx, c, id)
	return out.Results, out.Raw, err
}
