package disk

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"vmbench/internal/config"
)

func accountingFixture(t *testing.T) (Result, config.Config) {
	t.Helper()
	b, err := os.ReadFile("../../../tests/fixtures/fio-3.36-qd2-accounting.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := config.Defaults("quick")
	c.Engine, c.Profile, c.Depth, c.Jobs, c.Warmup, c.Duration = "io_uring", "randread-4k", 2, 1, 1, 5
	return r, c
}

func setDifference(v *Direction, blocks int64) {
	v.IOBytes = uint64(int64(v.TotalIOs*4096) + blocks*4096)
	v.BWBytes = math.Floor(float64(v.IOBytes) * 1000 / v.Runtime)
}

func TestFio335336CounterAccounting(t *testing.T) {
	for _, version := range []string{"fio-3.35", "fio-3.36"} {
		for _, tc := range []struct {
			name   string
			change func(*Result, *config.Config)
			valid  bool
		}{
			{"reported QD2 one block", func(*Result, *config.Config) {}, true},
			{"exact", func(r *Result, c *config.Config) { setDifference(&r.Jobs[0].Read, 0) }, true},
			{"two outstanding at reset", func(r *Result, c *config.Config) { setDifference(&r.Jobs[0].Read, 2) }, true},
			{"QD8 multiple outstanding", func(r *Result, c *config.Config) {
				c.Depth = 8
				r.Options["iodepth"] = "8"
				r.Jobs[0].Depth = map[string]float64{"8": 100}
				setDifference(&r.Jobs[0].Read, 8)
			}, true},
			{"job options override global", func(r *Result, c *config.Config) { r.Jobs[0].Options = r.Options; r.Options = nil }, true},
			{"no ramp", func(r *Result, c *config.Config) { c.Warmup = 0; r.Options["ramp_time"] = "0" }, false},
			{"time based alone no allowance", func(r *Result, c *config.Config) {
				c.Warmup = 0
				r.Options["ramp_time"] = "0"
				setDifference(&r.Jobs[0].Read, 1)
			}, false},
			{"not time based", func(r *Result, c *config.Config) { r.Options["time_based"] = "0" }, false},
			{"large unexplained", func(r *Result, c *config.Config) { setDifference(&r.Jobs[0].Read, 1000) }, false},
			{"one beyond queue bound", func(r *Result, c *config.Config) { setDifference(&r.Jobs[0].Read, 3) }, false},
			{"negative discrepancy", func(r *Result, c *config.Config) { setDifference(&r.Jobs[0].Read, -1) }, false},
			{"partial block", func(r *Result, c *config.Config) {
				r.Jobs[0].Read.IOBytes++
				r.Jobs[0].Read.BWBytes = math.Floor(float64(r.Jobs[0].Read.IOBytes) * 1000 / r.Jobs[0].Read.Runtime)
			}, false},
			{"short IO", func(r *Result, c *config.Config) { r.Jobs[0].Read.ShortIOs = 1 }, false},
			{"dropped IO", func(r *Result, c *config.Config) { r.Jobs[0].Read.DropIOs = 1 }, false},
			{"job error", func(r *Result, c *config.Config) { r.Jobs[0].Error = 5 }, false},
			{"continued IO error", func(r *Result, c *config.Config) { r.Jobs[0].TotalErrors = 1; r.Jobs[0].FirstError = 5 }, false},
			{"zero IO", func(r *Result, c *config.Config) { r.Jobs[0].Read.TotalIOs = 0 }, false},
			{"wrong block size", func(r *Result, c *config.Config) { r.Options["bs"] = "8k" }, false},
			{"missing fixed block evidence", func(r *Result, c *config.Config) { delete(r.Options, "bs") }, false},
			{"wrong ramp evidence", func(r *Result, c *config.Config) { r.Options["ramp_time"] = "2" }, false},
			{"wrong requested depth", func(r *Result, c *config.Config) { r.Options["iodepth"] = "64" }, false},
			{"unsupported engine", func(r *Result, c *config.Config) { c.Engine = "sync"; r.Options["ioengine"] = "sync" }, false},
			{"insufficient achieved depth", func(r *Result, c *config.Config) { r.Jobs[0].Depth = map[string]float64{"1": 100} }, false},
			{"unknown version discrepancy", func(r *Result, c *config.Config) { r.Version = "fio-unknown" }, false},
			{"unknown version exact", func(r *Result, c *config.Config) { r.Version = "fio-unknown"; setDifference(&r.Jobs[0].Read, 0) }, true},
			{"forged IOPS", func(r *Result, c *config.Config) { r.Jobs[0].Read.IOPS++ }, false},
			{"forged bandwidth", func(r *Result, c *config.Config) { r.Jobs[0].Read.BWBytes++ }, false},
			{"infinite IOPS", func(r *Result, c *config.Config) { r.Jobs[0].Read.IOPS = math.Inf(1) }, false},
			{"huge finite forged IOPS", func(r *Result, c *config.Config) { r.Jobs[0].Read.IOPS = math.MaxFloat64 }, false},
			{"NaN latency", func(r *Result, c *config.Config) { r.Jobs[0].Read.Completion.Mean = math.NaN() }, false},
			{"missing percentile", func(r *Result, c *config.Config) { r.Jobs[0].Read.Completion.Percentile = nil }, false},
		} {
			t.Run(version+"/"+tc.name, func(t *testing.T) {
				r, c := accountingFixture(t)
				r.Version = version
				tc.change(&r, &c)
				observations, err := validateMeasured(r, c, "randread", c.Depth, false)
				if err == nil {
					r.Accounting = observations
					_, err = ExecutionEvidence(r, c.Engine, c.Depth)
				}
				if (err == nil) != tc.valid {
					t.Fatalf("valid=%t: %v", tc.valid, err)
				}
				if tc.name == "reported QD2 one block" {
					if len(observations) != 1 || observations[0].ByteDifference != "+4096" || !strings.Contains(observations[0].Explanation, "ramp reset") {
						t.Fatal(observations)
					}
					if r.Jobs[0].Read.IOPS != 6066.786643 || r.Jobs[0].Read.IOBytes != 124276736 || r.Jobs[0].Read.Completion.Percentile["99.000000"] != 1000000 {
						t.Fatal("reported metrics changed")
					}
				}
			})
		}
	}
}

func TestCounterBudgetsArePerJobAndSharedAcrossDirections(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		for _, excessive := range []bool{false, true} {
			r, c := accountingFixture(t)
			c.Jobs = 2
			setDifference(&r.Jobs[0].Read, 2)
			rw := "randread"
			if mixed {
				rw = "randrw"
				c.Profile = "mixed-70-30"
				r.Options["rw"] = rw
				setDifference(&r.Jobs[0].Read, 1)
				r.Jobs[0].Write = r.Jobs[0].Read
			}
			r.Jobs = append(r.Jobs, r.Jobs[0])
			r.Jobs[1].Name = "job-002"
			if excessive {
				setDifference(&r.Jobs[1].Read, 3)
			}
			obs, err := validateMeasured(r, c, rw, c.Depth, false)
			if (err == nil) == excessive {
				t.Fatal(mixed, excessive, obs, err)
			}
			if !excessive && len(obs) != c.Jobs*len(activeDirections(rw)) {
				t.Fatal(obs)
			}
		}
	}
	r, c := accountingFixture(t)
	if _, err := validateMeasured(r, c, "randread", 2, true); err == nil {
		t.Fatal("finite capability probe accepted a counter discrepancy")
	}
	setDifference(&r.Jobs[0].Read, 0)
	c.Warmup = 0
	r.Options["ramp_time"] = "0"
	r.Options["time_based"] = "0"
	if _, err := validateMeasured(r, c, "randread", 2, true); err != nil {
		t.Fatal(err)
	}
}

func TestMissingAndNullFioFieldsFail(t *testing.T) {
	for _, key := range []string{"io_bytes", "total_ios", "short_ios", "drop_ios", "iops", "bw_bytes", "runtime", "clat_ns", "error"} {
		for _, null := range []bool{false, true} {
			r, c := accountingFixture(t)
			raw, _ := json.Marshal(r)
			var root map[string]any
			json.Unmarshal(raw, &root)
			job := root["jobs"].([]any)[0].(map[string]any)
			fields := job["read"].(map[string]any)
			if key == "error" {
				fields = job
			}
			if null {
				fields[key] = nil
			} else {
				delete(fields, key)
			}
			raw, _ = json.Marshal(root)
			parsed, err := Parse(raw)
			if err == nil {
				_, err = validateMeasured(parsed, c, "randread", 2, false)
			}
			if err == nil {
				t.Fatal("missing/null field accepted", key, null)
			}
		}
	}
}

func TestSequentialAccountingUsesTheActualOneMiBBlock(t *testing.T) {
	r, c := accountingFixture(t)
	c.Profile = "seqread-1m"
	r.Options["rw"] = "read"
	r.Options["bs"] = "1m"
	v := &r.Jobs[0].Read
	v.TotalIOs = 100
	v.IOBytes = 102 * (1 << 20)
	v.IOPS = float64(v.TotalIOs) * 1000 / v.Runtime
	v.BWBytes = math.Floor(float64(v.IOBytes) * 1000 / v.Runtime)
	obs, err := validateMeasured(r, c, "read", 2, false)
	if err != nil || len(obs) != 1 || obs[0].ByteDifference != "+2097152" || obs[0].BlockBytes != 1<<20 {
		t.Fatal(obs, err)
	}
	r.Options["bs"] = "4k"
	if _, err := validateMeasured(r, c, "read", 2, false); err == nil {
		t.Fatal("incorrect effective block accepted")
	}
}

func TestRejectionDiagnosticsPreserveCountersAndCause(t *testing.T) {
	r, c := accountingFixture(t)
	setDifference(&r.Jobs[0].Read, 3)
	raw, _ := json.Marshal(r)
	r, _ = Parse(raw)
	_, err := validateMeasured(r, c, "randread", 2, false)
	err = measurementFailure(c, 2, 1, r, raw, err)
	for _, text := range []string{"Workload        : randread-4k", "I/O Engine      : io_uring", "Requested QD    : 2", "Job             : job-001", "Read Bytes      : 124284928", "Total I/Os      : 30340", "Byte Difference : +12288", "Short I/Os      : 0", "Dropped I/Os    : 0", "fio Error       : 0", "exceeds shared ramp-boundary budget"} {
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("missing %q: %v", text, err)
		}
	}
}
