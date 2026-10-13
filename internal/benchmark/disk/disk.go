package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/runner"
	"vmbench/internal/safety"
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
	Error int                `json:"error"`
	Read  Direction          `json:"read"`
	Write Direction          `json:"write"`
	Depth map[string]float64 `json:"iodepth_level"`
}
type Result struct {
	Jobs []Job `json:"jobs"`
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
	size, _ := config.Bytes(c.FileSize)
	if (strings.Contains(c.Profile, "write") || c.Profile == "mixed-70-30" || c.Precondition) && !c.ConfirmWrites {
		return nil, nil, fmt.Errorf("write-intensive test requires --confirm-writes")
	}
	t, e := safety.Prepare(c.Path, id, size, c.Jobs)
	if e != nil {
		return nil, nil, e
	}
	defer t.Close()
	rw, bs := "randread", "4k"
	switch c.Profile {
	case "randwrite-4k":
		rw = "randwrite"
	case "mixed-70-30":
		rw = "randrw"
	case "seqread-1m":
		rw = "read"
		bs = "1m"
	case "seqwrite-1m":
		rw = "write"
		bs = "1m"
	}
	depths := []int{c.Depth}
	if c.Profile == "latency" {
		depths = []int{1}
	}
	if c.Profile == "qd-scaling" {
		depths = []int{1, 4, 16, 32, 64}
	}
	base := []string{"--output-format=json+", "--ioengine=" + c.Engine, "--direct=" + strconv.Itoa(boolInt(c.Direct)), "--size=" + strconv.FormatInt(size, 10), "--refill_buffers=1", "--scramble_buffers=1", "--randrepeat=1", "--group_reporting=0"}
	withJobs := func(args []string) []string {
		for i, p := range t.Paths() {
			args = append(args, fmt.Sprintf("--name=job%d", i), "--filename="+p)
		}
		return args
	} // all workload options precede named jobs, so every job inherits them
	init := append(append([]string{}, base...), "--rw=write", "--bs=1m", "--iodepth=1")
	initial, e := runner.Run(ctx, "fio", withJobs(init), time.Hour)
	if e != nil {
		return nil, nil, fmt.Errorf("dataset initialization: %w", e)
	}
	if _, e = Parse(initial); e != nil {
		return nil, nil, fmt.Errorf("dataset initialization: %w", e)
	}
	var results []Result
	var raw []json.RawMessage
	for _, d := range depths {
		for i := 0; i < c.Repetitions; i++ {
			args := append(append([]string{}, base...), "--rw="+rw, "--bs="+bs, "--rwmixread=70", "--iodepth="+strconv.Itoa(d), "--time_based=1", "--runtime="+strconv.Itoa(c.Duration), "--ramp_time="+strconv.Itoa(c.Warmup))
			b, e := runner.Run(ctx, "fio", withJobs(args), time.Duration(c.Duration+c.Warmup+30)*time.Second)
			if e != nil {
				return results, raw, e
			}
			r, e := Parse(b)
			if e != nil {
				return results, raw, e
			}
			results = append(results, r)
			raw = append(raw, json.RawMessage(b))
		}
	}
	return results, raw, nil
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
