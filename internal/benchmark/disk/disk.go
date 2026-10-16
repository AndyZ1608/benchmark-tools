package disk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"vmbench/internal/config"
)

type Latency struct {
	Samples    uint64             `json:"N"`
	Mean       float64            `json:"mean"`
	Percentile map[string]float64 `json:"percentile"`
	Bins       map[string]uint64  `json:"bins"`
}
type Direction struct {
	TotalIOs   uint64  `json:"total_ios"`
	IOBytes    uint64  `json:"io_bytes"`
	ShortIOs   uint64  `json:"short_ios"`
	DropIOs    uint64  `json:"drop_ios"`
	IOPS       float64 `json:"iops"`
	BWBytes    float64 `json:"bw_bytes"`
	Runtime    float64 `json:"runtime"`
	Completion Latency `json:"clat_ns"`
	Total      Latency `json:"lat_ns"`
	present    map[string]bool
}
type Job struct {
	Name        string             `json:"jobname"`
	Options     map[string]string  `json:"job options"`
	Error       int                `json:"error"`
	TotalErrors uint64             `json:"total_err,omitempty"`
	FirstError  int                `json:"first_error,omitempty"`
	Read        Direction          `json:"read"`
	Write       Direction          `json:"write"`
	Depth       map[string]float64 `json:"iodepth_level"`
	present     map[string]bool
}
type Result struct {
	Version    string               `json:"fio version"`
	Options    map[string]string    `json:"global options"`
	Jobs       []Job                `json:"jobs"`
	Accounting []CounterObservation `json:"-"`
}

// Keep field presence separate from zero: absent/null error counters are not
// evidence that fio completed without short, dropped, or failed I/O.
func (d *Direction) UnmarshalJSON(data []byte) error {
	type fields Direction
	var value fields
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*d = Direction(value)
	var err error
	d.present, err = jsonPresence(data)
	return err
}
func (j *Job) UnmarshalJSON(data []byte) error {
	type fields Job
	var value fields
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*j = Job(value)
	var err error
	j.present, err = jsonPresence(data)
	return err
}

func jsonPresence(data []byte) (map[string]bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	present := make(map[string]bool, len(raw))
	for key, value := range raw {
		present[key] = !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
	}
	return present, nil
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
		if fieldMissing(j.present, "error") {
			return r, fmt.Errorf("fio job error field is missing or null")
		}
		if j.Error != 0 || j.TotalErrors != 0 || j.FirstError != 0 {
			return r, fmt.Errorf("fio reported I/O errors: job=%d, total=%d, first=%d", j.Error, j.TotalErrors, j.FirstError)
		}
	}
	return r, nil
}
func Run(ctx context.Context, c config.Config, id string) ([]Result, []json.RawMessage, error) {
	out, err := RunManaged(ctx, c, id)
	return out.Results, out.Raw, err
}
