package disk

import (
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"strconv"
	"strings"
	"vmbench/internal/config"
)

// CounterObservation preserves the independently reported counters. A positive
// difference is not used to recalculate either fio IOPS or fio bandwidth.
type CounterObservation struct {
	Job, Direction, ByteDifference, Explanation string
	IOBytes, TotalIOs, BlockBytes               uint64
}

func option(r Result, j Job, key string) string {
	if value, ok := j.Options[key]; ok {
		return value
	}
	return r.Options[key]
}

func fieldMissing(fields map[string]bool, key string) bool {
	return fields != nil && !fields[key]
}

func counterObservation(job, direction string, v Direction, block uint64) CounterObservation {
	expected := new(big.Int).Mul(new(big.Int).SetUint64(v.TotalIOs), new(big.Int).SetUint64(block))
	difference := new(big.Int).Sub(new(big.Int).SetUint64(v.IOBytes), expected)
	text := difference.String()
	if difference.Sign() > 0 {
		text = "+" + text
	}
	return CounterObservation{Job: job, Direction: direction, IOBytes: v.IOBytes, TotalIOs: v.TotalIOs, BlockBytes: block, ByteDifference: text}
}

// These releases were checked against upstream ioengines.c, io_u.c, time.c,
// libfio.c and stat.c. Unknown releases retain strict counter validation.
func auditedAccounting(version string) bool {
	for _, minor := range []string{"35", "36", "37", "38", "39"} {
		base := "fio-3." + minor
		if version == base {
			return true
		}
	}
	return false
}

func workloadBlock(c config.Config) uint64 {
	_, bs, _ := profile(c)
	if bs == "1m" {
		return 1 << 20
	}
	return 4096
}

func blockOption(value string) uint64 {
	switch strings.ToLower(value) {
	case "4k", "4kb", "4kib":
		return 4096
	case "1m", "1mb", "1mib":
		return 1 << 20
	}
	n, _ := strconv.ParseUint(value, 10, 64)
	return n
}

func activeDirections(rw string) []string {
	switch rw {
	case "read", "randread":
		return []string{"read"}
	case "write", "randwrite":
		return []string{"write"}
	case "randrw":
		return []string{"read", "write"}
	}
	return nil
}

func jobName(job Job, index int) string {
	if job.Name != "" {
		quoted := strconv.QuoteToASCII(job.Name)
		return quoted[1 : len(quoted)-1]
	}
	return fmt.Sprintf("job-%03d", index+1)
}

// fio prints iops as %f (six fractional digits). bw_bytes is integer division
// of io_bytes*1000/runtime. These checks use those exact serialization rules,
// not a throughput percentage or a count inferred from bandwidth.
func validateRates(v Direction) error {
	if v.Runtime != math.Trunc(v.Runtime) || v.Runtime > 1<<53 || v.TotalIOs > 1<<53 {
		return fmt.Errorf("runtime/count exceeds exact supported numeric range")
	}
	expectedIOPS := float64(v.TotalIOs) * 1000 / v.Runtime
	ulp := func(n float64) float64 { return math.Nextafter(n, math.Inf(1)) - n }
	if math.Abs(v.IOPS-expectedIOPS) > 0.0000005+2*ulp(expectedIOPS) {
		return fmt.Errorf("reported IOPS is inconsistent with total_ios and runtime")
	}
	hi, lo := bits.Mul64(v.IOBytes, 1000)
	if hi >= uint64(v.Runtime) {
		return fmt.Errorf("bandwidth arithmetic overflow")
	}
	bw, _ := bits.Div64(hi, lo, uint64(v.Runtime))
	if bw > 1<<53 || v.BWBytes != float64(bw) {
		return fmt.Errorf("reported bw_bytes is inconsistent with io_bytes and runtime")
	}
	return nil
}

func validateMeasurement(r Result, c config.Config, rw string) error {
	_, err := validateMeasured(r, c, rw, c.Depth, false)
	return err
}

func validateMeasured(r Result, c config.Config, rw string, depth int, probe bool) (observations []CounterObservation, err error) {
	if c.Engine != "io_uring" && c.Engine != "libaio" {
		return nil, fmt.Errorf("unsupported fio engine %q", c.Engine)
	}
	if len(r.Jobs) != c.Jobs {
		return nil, fmt.Errorf("expected %d independent jobs; received %d", c.Jobs, len(r.Jobs))
	}
	if depth < 1 {
		return nil, fmt.Errorf("invalid requested I/O depth")
	}
	directions := activeDirections(rw)
	if len(directions) == 0 {
		return nil, fmt.Errorf("unsupported fio workload %q", rw)
	}
	block := workloadBlock(c)
	for i, job := range r.Jobs {
		name := jobName(job, i)
		if actual := option(r, job, "ioengine"); actual != c.Engine {
			return observations, fmt.Errorf("%s effective fio engine %q differs from selected %q", name, actual, c.Engine)
		}
		if actual := option(r, job, "rw"); actual != "" && actual != rw {
			return observations, fmt.Errorf("%s effective fio workload %q differs from requested %q", name, actual, rw)
		}
		if actual := option(r, job, "iodepth"); actual != "" && actual != strconv.Itoa(depth) {
			return observations, fmt.Errorf("%s effective fio depth %q differs from requested %d", name, actual, depth)
		}
		for key, want := range map[string]string{"time_based": strconv.Itoa(boolInt(!probe)), "ramp_time": strconv.Itoa(c.Warmup), "direct": strconv.Itoa(boolInt(c.Direct)), "group_reporting": "0"} {
			if actual := option(r, job, key); actual != "" && actual != want {
				return observations, fmt.Errorf("%s effective fio option %s=%q differs from requested %q", name, key, actual, want)
			}
		}
		if fieldMissing(job.present, "error") {
			return observations, fmt.Errorf("%s fio job error field is missing or null", name)
		}
		if job.Error != 0 || job.TotalErrors != 0 || job.FirstError != 0 {
			return observations, fmt.Errorf("%s fio reported I/O errors: job=%d, total=%d, first=%d", name, job.Error, job.TotalErrors, job.FirstError)
		}
		for _, v := range []Direction{job.Read, job.Write} {
			if v.ShortIOs != 0 || v.DropIOs != 0 {
				return observations, fmt.Errorf("%s has short or dropped I/O", name)
			}
		}
		if rw != "randrw" && ((directions[0] == "read" && (job.Write.TotalIOs != 0 || job.Write.IOBytes != 0)) || (directions[0] == "write" && (job.Read.TotalIOs != 0 || job.Read.IOBytes != 0))) {
			return observations, fmt.Errorf("%s completed unexpected I/O in the inactive direction", name)
		}
		if bs := option(r, job, "bs"); bs != "" && blockOption(bs) != block {
			return observations, fmt.Errorf("%s effective fio block size %q differs from requested %d", name, bs, block)
		}
		pending := uint64(0) // Read/write share this job's one outstanding queue.
		for _, direction := range directions {
			v := job.Read
			if direction == "write" {
				v = job.Write
			}
			observation := counterObservation(name, direction, v, block)
			observations = append(observations, observation)
			for _, key := range []string{"total_ios", "io_bytes", "short_ios", "drop_ios", "iops", "bw_bytes", "runtime", "clat_ns"} {
				if fieldMissing(v.present, key) {
					return observations, fmt.Errorf("%s %s required field %s is missing or null", name, direction, key)
				}
			}
			if v.TotalIOs == 0 || v.IOBytes == 0 {
				return observations, fmt.Errorf("%s %s completed no successful measured I/O", name, direction)
			}
			for _, metric := range []struct {
				name  string
				value float64
			}{{"IOPS", v.IOPS}, {"bandwidth", v.BWBytes}, {"runtime", v.Runtime}, {"average completion latency", v.Completion.Mean}} {
				if metric.value <= 0 || math.IsNaN(metric.value) || math.IsInf(metric.value, 0) {
					return observations, fmt.Errorf("%s %s %s is missing or invalid", name, direction, metric.name)
				}
			}
			p99 := false
			for key, value := range v.Completion.Percentile {
				p, e := strconv.ParseFloat(key, 64)
				if e != nil || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 100 || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
					return observations, fmt.Errorf("%s %s invalid completion percentile", name, direction)
				}
				if p == 99 && value > 0 {
					p99 = true
				}
			}
			if !p99 {
				return observations, fmt.Errorf("%s %s p99 completion latency is missing or invalid", name, direction)
			}
			if err := validateRates(v); err != nil {
				return observations, fmt.Errorf("%s %s: %w", name, direction, err)
			}
			if v.TotalIOs > math.MaxUint64/block {
				return observations, fmt.Errorf("%s %s expected byte count overflows", name, direction)
			}
			expected := v.TotalIOs * block
			if v.IOBytes == expected {
				continue
			}
			if v.IOBytes < expected || v.IOBytes%block != 0 {
				return observations, fmt.Errorf("%s %s byte/count discrepancy %s cannot be explained by ramp accounting", name, direction, observation.ByteDifference)
			}
			// At ramp reset, already queued requests lose their submission count
			// but can complete afterwards. The reset happens once, with at most
			// iodepth outstanding requests across BOTH directions. Successful
			// time-based termination drains the queue; it adds no second budget.
			if probe || c.Warmup <= 0 || !c.Direct || !auditedAccounting(r.Version) || (c.Engine != "io_uring" && c.Engine != "libaio") {
				return observations, fmt.Errorf("%s %s byte/count discrepancy %s lacks a verified ramp-boundary explanation (fio %q, warmup %d)", name, direction, observation.ByteDifference, r.Version, c.Warmup)
			}
			for key, want := range map[string]string{"ioengine": c.Engine, "rw": rw, "direct": "1", "time_based": "1", "ramp_time": strconv.Itoa(c.Warmup), "iodepth": strconv.Itoa(depth), "group_reporting": "0"} {
				if option(r, job, key) != want {
					return observations, fmt.Errorf("%s %s cannot verify ramp accounting: fio option %s=%q, expected %q", name, direction, key, option(r, job, key), want)
				}
			}
			if blockOption(option(r, job, "bs")) != block {
				return observations, fmt.Errorf("%s %s cannot verify fixed-block workload", name, direction)
			}
			if mode := option(r, job, "io_submit_mode"); mode != "" && mode != "inline" {
				return observations, fmt.Errorf("%s unsupported fio submission mode %q", name, mode)
			}
			delta := (v.IOBytes - expected) / block
			if delta > uint64(depth)-pending {
				return observations, fmt.Errorf("%s %s byte/count discrepancy %s exceeds shared ramp-boundary budget of %d blocks", name, direction, observation.ByteDifference, depth)
			}
			pending += delta
			observations[len(observations)-1].Explanation = fmt.Sprintf("fio ramp reset: queued-before-reset I/O completed afterwards; shared job bound %d blocks", depth)
		}
	}
	return observations, nil
}
