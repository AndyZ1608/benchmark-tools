package presentation

import (
	"bytes"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"

	automatic "vmbench/internal/benchmark/auto"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/statistics"
)

// Synthetic display fixtures; no engine or stress workload runs in these tests.
func compactDiskFixture() automatic.Result {
	options := automatic.Defaults("max")
	options.Component = "disk"
	makePeak := func(name, unit string, value, low, high float64) automatic.Peak {
		trial := automatic.Trial{
			Setting:  automatic.Setting{Depth: 4, Jobs: 2},
			Stats:    statistics.Summary{Count: 3, Median: value, CV: .01},
			Duration: 5, Stable: true,
			Samples: []automatic.Sample{{Value: value, MiBps: 95.61, DiskValidated: true, Repetition: 1,
				MeanMS: .1, MeanLowMS: .05, P99LowMS: low, P99HighMS: high,
				Evidence: "libaio; requested QD4; job 1: bucket 4 99.0%",
				Raw: disk.Result{Jobs: []disk.Job{{Name: "private-job", Read: disk.Direction{
					IOPS: value, BWBytes: 95.61 * (1 << 20), TotalIOs: 100, IOBytes: 409600,
					Completion: disk.Latency{Mean: 100000, Percentile: map[string]float64{"95.000000": low * 1e6 / 2, "99.000000": low * 1e6}},
				}}}},
			}},
		}
		if name == "randread-4k" {
			trial.Samples[0].MiBps = value / 256
		}
		for len(trial.Samples) < trial.Stats.Count {
			sample := trial.Samples[0]
			sample.Repetition = len(trial.Samples) + 1
			trial.Samples = append(trial.Samples, sample)
		}
		return automatic.Peak{Name: name, Unit: unit, Best: &trial, Confirmed: true, Trials: []automatic.Trial{trial}}
	}
	return automatic.Result{Mode: "max", Status: "COMPLETED", Limits: options,
		States:       map[string]string{"DISK": "COMPLETED"},
		DiskInfo:     automatic.DiskInfo{Path: "/data", Filesystem: "ext4", Mount: "/data", Engine: "libaio", Prepared: true, PreparedJobs: 2, FileBytes: 64 << 20},
		DatasetBytes: 128 << 20, DiskCleaned: true,
		Disk: []automatic.Peak{makePeak("randread-4k", "IOPS", 15083.3, 18.5, 25.6), makePeak("seqread-1m", "MiB/s", 95.61, 2020, 2840)},
	}
}

func renderAutomaticTest(t *testing.T, result automatic.Result, details bool) string {
	t.Helper()
	var out bytes.Buffer
	if err := RenderAutomatic(&out, result, details); err != nil {
		t.Fatal(err)
	}
	for _, char := range out.String() {
		if char > 127 {
			t.Fatal("non-ASCII output", out.String())
		}
	}
	if strings.Contains(out.String(), `{"`) || strings.Contains(out.String(), `"jobs"`) {
		t.Fatal("raw JSON printed", out.String())
	}
	return out.String()
}

func TestAutomaticCompactDiskLayout(t *testing.T) {
	result := compactDiskFixture()
	output := renderAutomaticTest(t, result, false)
	expected := `========================================
       VMBENCH - DISK MAX
========================================
Status: COMPLETED
Results: Maximum observed within tested limits
Target: /data

RANDOM READ 4K
----------------------------------------
IOPS           : 15,083
Throughput     : 58.92 MiB/s
P99 Latency    : 18.5 - 25.6 ms (job/run range)
Confidence     : CONFIRMED

SEQUENTIAL READ
----------------------------------------
Throughput     : 95.61 MiB/s
P99 Latency    : 2.02 - 2.84 s (job/run range)
Confidence     : CONFIRMED

----------------------------------------
Temporary Files: Cleaned
========================================
`
	if output != expected {
		t.Fatalf("unexpected summary:\n%s", output)
	}
	for _, hidden := range []string{"libaio", "ext4", "dataset", "Configuration", "Search:", "bucket", "Average", "ceilings", "CV", "P95", "Duration:"} {
		if strings.Contains(output, hidden) {
			t.Fatalf("technical noise %q: %s", hidden, output)
		}
	}
}

func TestAutomaticDetailsAndMeasurementsRemainAvailable(t *testing.T) {
	result, unchanged := compactDiskFixture(), compactDiskFixture()
	for _, details := range []bool{false, true} {
		output := renderAutomaticTest(t, result, details)
		if !reflect.DeepEqual(result, unchanged) {
			t.Fatal("rendering mutated source measurements")
		}
		if !details {
			continue
		}
		for _, want := range []string{"Selected I/O engine: libaio", "QD4 / 2 jobs", "Observed depth / validation:", "bucket 4", "Prepared dataset:", "Search: 1 configurations", "Average completion latency", "P95", "P99", "9.25 ms", "3 repetitions of 5s, CV 1.0%", "Disk ceilings:", "Searched configurations", "Run 1 depth/validation"} {
			if !strings.Contains(output, want) {
				t.Fatalf("missing detail %q:\n%s", want, output)
			}
		}
	}
}

func TestLatencyRangeFormatting(t *testing.T) {
	for _, test := range []struct {
		values []float64
		want   string
	}{
		{[]float64{18.5, 25.6}, "18.5 - 25.6 ms"},
		{[]float64{2020, 2840}, "2.02 - 2.84 s"},
		{[]float64{.02, .08}, "20.00 - 80.00 us"},
		{[]float64{2.02, 2.84}, "2.02 - 2.84 ms"},
		{[]float64{25.6, 18.5, 32}, "18.5 - 32.0 ms"},
		{[]float64{500, 2000}, "500.0 - 2,000.0 ms"},
		{[]float64{3}, "3.00 ms"},
	} {
		if got := latencyRange(test.values); got != test.want {
			t.Errorf("%v: %s, want %s", test.values, got, test.want)
		}
	}
	result := compactDiskFixture()
	sample := result.Disk[0].Best.Samples[0]
	sample.P99LowMS, sample.P99HighMS = 12, 35
	result.Disk[0].Best.Samples = append(result.Disk[0].Best.Samples, sample)
	result.Disk[0].Best.Stats.Count = len(result.Disk[0].Best.Samples)
	output := renderAutomaticTest(t, result, false)
	if !strings.Contains(output, "12.0 - 35.0 ms (job/run range)") {
		t.Fatal(output)
	}
}

func TestAutomaticMissingAndUnconfirmedMetrics(t *testing.T) {
	result := compactDiskFixture()
	result.Disk[0].Best = nil
	result.Disk[0].Confirmed = false
	out := renderAutomaticTest(t, result, false)
	if !strings.Contains(out, "IOPS           : 15,083") || !strings.Contains(out, "Confidence     : UNCONFIRMED") {
		t.Fatal("usable history was hidden", out)
	}
	result.Disk[0].Trials = nil
	result.Disk[0].Failure = "fio validation failed"
	out = renderAutomaticTest(t, result, false)
	if !strings.Contains(out, "IOPS           : N/A") || !strings.Contains(out, "NO VALID MEASUREMENT") || !strings.Contains(out, "fio validation failed") {
		t.Fatal(out)
	}
	result.Disk[1].Best.Stats.Median = math.NaN()
	if strings.Contains(renderAutomaticTest(t, result, false), "NaN") {
		t.Fatal("invalid statistic printed")
	}
}

func TestAutomaticCriticalWarningsAndErrors(t *testing.T) {
	result := compactDiskFixture()
	result.DiskInfo.System = true
	result.DiskCleaned = false
	result.Status = "FAILED"
	result.Warnings = []string{"Insufficient free disk space", "Unknown validation warning", "CPU instruction capabilities unavailable"}
	result.Errors = []string{"DISK: cleanup failed"}
	result.Disk[0].Boundary = true
	result.Disk[0].Notes = []string{"Search limit reached; higher performance may be possible.", "Near-saturation search heuristic triggered; physical saturation is not established", "Dataset is smaller than available RAM; backend/cache effects remain possible despite direct I/O"}
	result.Disk[1].Confirmed = false
	for _, details := range []bool{false, true} {
		output := renderAutomaticTest(t, result, details)
		for _, want := range []string{"system filesystem", "Insufficient free disk space", "Unknown validation warning", "Search limit reached", "Stability not confirmed", "cleanup incomplete", "ERROR: DISK: cleanup failed"} {
			if !strings.Contains(output, want) {
				t.Fatalf("warning lost: %s\n%s", want, output)
			}
		}
		if !details && (strings.Contains(output, "Near-saturation") || strings.Contains(output, "instruction capabilities")) {
			t.Fatal(output)
		}
		if details && !strings.Contains(output, "Near-saturation") {
			t.Fatal("saturation evidence lost", output)
		}
		if !strings.Contains(output, "cache effects") {
			t.Fatal("material limitation hidden", output)
		}
	}
}

func TestOptionalLatencyWarningsAreWorkloadSpecific(t *testing.T) {
	result := compactDiskFixture()
	if strings.Contains(renderAutomaticTest(t, result, false), "High P99") {
		t.Fatal("unconfigured latency warning")
	}
	result.Limits.DiskP99WarningMS = 1000
	for _, details := range []bool{false, true} {
		output := renderAutomaticTest(t, result, details)
		if !strings.Contains(output, "Sequential read: High P99 latency under load") || strings.Contains(output, "Random Read 4K: High P99") || !strings.Contains(output, "configured 1.00 s threshold") {
			t.Fatal(output)
		}
	}
	result.Limits.DiskP99WarningMS = 2840
	if strings.Contains(renderAutomaticTest(t, result, false), "High P99") {
		t.Fatal("threshold equality should not warn")
	}
}

func TestAutomaticModesAndModuleScope(t *testing.T) {
	for _, mode := range []string{"quick", "standard", "max"} {
		result := compactDiskFixture()
		result.Mode = mode
		result.Limits.Mode = mode
		if mode == "quick" {
			for i := range result.Disk {
				result.Disk[i].Confirmed = false
			}
		}
		output := renderAutomaticTest(t, result, false)
		if !strings.Contains(output, "Confidence") {
			t.Fatal(output)
		}
		if mode == "quick" && !strings.Contains(output, "Results: Preliminary") {
			t.Fatal(output)
		}
	}
	for _, component := range []string{"cpu", "ram", "disk", "network"} {
		result := compactDiskFixture()
		result.Limits.Component = component
		output := renderAutomaticTest(t, result, false)
		if strings.Contains(output, "RANDOM READ") != (component == "disk") || strings.Contains(output, "ceilings:") {
			t.Fatal(output)
		}
		detailed := renderAutomaticTest(t, result, true)
		for _, ceiling := range []struct{ component, label string }{{"ram", "RAM allocation ceiling:"}, {"disk", "Disk ceilings:"}, {"network", "Network ceilings:"}} {
			if strings.Contains(detailed, ceiling.label) != (component == ceiling.component) {
				t.Fatal(detailed)
			}
		}
	}
}

func TestAutomaticOtherModulesAndUnavailableMetrics(t *testing.T) {
	result := compactDiskFixture()
	result.Limits.Component = ""
	best := automatic.Trial{Setting: automatic.Setting{Workers: 4}, Stats: statistics.Summary{Median: 3600}, Stable: true, Duration: 5}
	baseline := automatic.Trial{Setting: automatic.Setting{Workers: 1}, Stats: statistics.Summary{Median: 1000}, Stable: true, Duration: 5}
	for _, name := range []string{"integer-xorshift64", "fp-multiply-add"} {
		result.CPU = append(result.CPU, automatic.Peak{Name: name, Unit: "defined iterations/s", Best: &best, Confirmed: true, Trials: []automatic.Trial{baseline}})
	}
	memory := best
	memory.Stats.Median = 12.345
	memory.Samples = []automatic.Sample{{LatencyNS: 25.6, Raw: cpu.NativeResult{Measurements: []cpu.Measurement{{Workload: "triad", Errors: 0}}}}}
	result.Memory = []automatic.Peak{{Name: "triad", Unit: "GB/s", Best: &memory, Confirmed: true}}
	network := best
	network.Stats.Median = .95
	result.Network = []automatic.Peak{{Name: "TCP TX", Unit: "Gbps", Best: &network, Confirmed: true}, {Name: "TCP RX", Unit: "Gbps", Best: &network, Confirmed: true}}
	output := renderAutomaticTest(t, result, false)
	for _, want := range []string{"Integer Compute", "Float Compute", "90.0 %", "Memory Bandwidth", "12.35 GB/s", "25.60 ns/access", "TCP Upload", "TCP Download", "950.00 Mbps"} {
		if !strings.Contains(output, want) {
			t.Fatalf("%s missing:\n%s", want, output)
		}
	}
	for _, absent := range []string{"loss", "jitter", "RTT"} {
		if strings.Contains(output, absent) {
			t.Fatal(output)
		}
	}
	result.Memory[0].Best = &best
	output = renderAutomaticTest(t, result, false)
	if strings.Contains(output, "Integrity errors") || strings.Contains(output, "ns/access") {
		t.Fatal("fabricated RAM values", output)
	}
}

func TestOptionalMixedOutputKeepsDirectionAndTotalMeaning(t *testing.T) {
	result := compactDiskFixture()
	result.Disk[0].Name = "mixed-70-30"
	output := renderAutomaticTest(t, result, false)
	for _, want := range []string{"Total IOPS", "read/write job/run range"} {
		if !strings.Contains(output, want) {
			t.Fatal(output)
		}
	}
}

func TestAutomaticContentionWarningsRemainVisible(t *testing.T) {
	result := compactDiskFixture()
	result.Limits.Component = ""
	trial := automatic.Trial{Stats: statistics.Summary{Median: 100}, Samples: []automatic.Sample{{Extras: map[string]float64{"CPU steal percent": 2, "Throttled periods": 1, "Swap pages in": 1}}}}
	result.CPU = []automatic.Peak{{Name: "integer-xorshift64", Best: &trial, Confirmed: true}}
	result.Memory = []automatic.Peak{{Name: "triad", Best: &trial, Confirmed: true}}
	for _, details := range []bool{false, true} {
		output := renderAutomaticTest(t, result, details)
		for _, want := range []string{"CPU steal observed", "cgroup throttling", "Guest swap activity"} {
			if !strings.Contains(output, want) {
				t.Fatal(output)
			}
		}
	}
}

func TestAutomaticProgressKeepsTechnicalObservationsInDetails(t *testing.T) {
	observations := []string{"[OK] System detected", "[DISK] Preparing 1048576 bytes across 2 private job files (read preparation writes data)", "[DISK] Prepared dataset; finding observed IOPS and sequential throughput", "randread-4k: QD64 / 2 jobs completed", "seqread-1m: best configuration confirmed", "[OK] libaio: READY - selected I/O engine: libaio", "[INFO] io_uring: FAILED on selected target: unavailable", "[FAILED] DISK benchmark", "WARNING: unknown safety notice"}
	for _, details := range []bool{false, true} {
		var output bytes.Buffer
		progress := AutomaticProgress(&output, details)
		for _, observation := range observations {
			progress(observation)
		}
		text := output.String()
		if details {
			for _, observation := range observations {
				if !strings.Contains(text, observation) {
					t.Fatal(text)
				}
			}
		} else {
			for _, hidden := range []string{"1048576", "QD64", "libaio", "io_uring", "configuration"} {
				if strings.Contains(text, hidden) {
					t.Fatal(text)
				}
			}
			for _, want := range []string{"Benchmark running", "Preparing temporary files", "Measuring", "[FAILED] DISK", "unknown safety notice"} {
				if !strings.Contains(text, want) {
					t.Fatal(text)
				}
			}
		}
	}
}

type automaticErrorWriter struct{}

func (automaticErrorWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestAutomaticWriterErrorsPropagate(t *testing.T) {
	for _, details := range []bool{false, true} {
		if err := RenderAutomatic(automaticErrorWriter{}, compactDiskFixture(), details); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	}
}
