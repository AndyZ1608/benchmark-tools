package presentation

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/benchmark/network"
	"vmbench/internal/config"
	"vmbench/internal/report"
	"vmbench/internal/runner"
	"vmbench/internal/statistics"
)

func fixture(command string, n int) report.Report {
	c, _ := config.Defaults("quick")
	c.Lang = "en"
	c.Workers = 4
	c.Repetitions = n
	r := report.New(command, c)
	r.Status = "completed"
	r.DurationSeconds = 12
	r.Results["execution_status"] = map[string]string{command: "completed"}
	r.System.Raw = map[string]string{"/sys/devices/system/cpu/cpu0/cache/index3/level": "3", "/sys/devices/system/cpu/cpu0/cache/index3/size": "1024K"}
	return r
}
func nativeFixture(command string, n int) report.Report {
	r := fixture(command, n)
	raw := []json.RawMessage{}
	for _, w := range []int{1, 4} {
		for i := 0; i < n; i++ {
			measurements := []cpu.Measurement{{Workload: "integer-xorshift64", Unit: "defined_iterations/s", Value: 100, Elapsed: 2}, {Workload: "fp-multiply-add", Unit: "defined_iterations/s", Value: 50, Elapsed: 2}}
			if w == 4 {
				measurements[0].Value = 360
				measurements[1].Value = 190
			}
			if command == "ram test" {
				measurements = []cpu.Measurement{{Workload: "triad", Unit: "GB/s", Value: 28.6, Elapsed: 2}, {Workload: "dependent-pointer-chase", Unit: "ns/access mean per worker", Value: 85, Elapsed: 2}}
			}
			b, _ := json.Marshal(cpu.NativeResult{Kernel: "compute-bench-v1", Workers: w, Dataset: 256 << 20, Measurements: measurements})
			raw = append(raw, b)
		}
	}
	r.Results[command] = raw
	return r
}
func output(t *testing.T, r report.Report) string {
	t.Helper()
	var b bytes.Buffer
	if e := Render(&b, r, "results/fixture.json", false); e != nil {
		t.Fatal(e)
	}
	return b.String()
}
func TestCPUSummaryAndMissingSteal(t *testing.T) {
	r := nativeFixture("cpu test", 1)
	s := output(t, r)
	for _, want := range []string{"Completed", "90.0 %", "95.0 %", "Integer", "Floating Point", "Insufficient data", "Not measured", "Reported integrity errors", "results/fixture.json"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q: %s", want, s)
		}
	}
	if strings.Contains(s, "0.00%") || strings.Contains(s, "\"cpu_telemetry\"") {
		t.Fatal(s)
	}
	m := Modules(r)[0]
	if len(m.Metrics) != 5 {
		t.Fatal(m.Metrics)
	}
}
func TestStabilityEvidence(t *testing.T) {
	for _, c := range []struct {
		n    int
		cv   float64
		want string
	}{{1, 0, "Insufficient data"}, {2, 0, "Insufficient data"}, {3, 0, "Stable"}, {3, .20, "Variable"}} {
		label, _ := stability("en", []statistics.Summary{{Count: c.n, Mean: 100, CV: c.cv}}, 5)
		if label != c.want {
			t.Fatal(c, label)
		}
	}
	if cvText("en", statistics.Summary{Count: 1, CV: 0}) != "N/A" {
		t.Fatal("single CV was displayed")
	}
	if !strings.Contains(cvText("en", statistics.Summary{Count: 2, CV: .1}), "limited evidence") {
		t.Fatal("two samples lacked caution")
	}
	label, _ := stability("en", []statistics.Summary{{Count: 3, Mean: 100, CV: .1}}, 15)
	if label != "Stable" {
		t.Fatal("threshold not honored")
	}
}
func TestLocalizationAndFormatting(t *testing.T) {
	r := nativeFixture("ram test", 3)
	r.Settings.Lang = "vi"
	s := output(t, r)
	for _, want := range []string{"Hoàn thành", "Băng thông TRIAD", "Độ trễ truy cập phụ thuộc", "Ổn định", "28.60 GB/s", "85.00 ns"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q: %s", want, s)
		}
	}
	if number(85000, 0) != "85,000" || unit(1.24, "ms", 2) != "1.24 ms" {
		t.Fatal("number formatting")
	}
}
func TestCacheWarningProminentAndPersistable(t *testing.T) {
	r := nativeFixture("ram test", 3)
	r.Settings.Size = "12MiB"
	s := output(t, r)
	if !strings.Contains(s, "cache effects may dominate") {
		t.Fatal(s)
	}
	if len(GeneratedWarnings(r)) != 1 {
		t.Fatal(GeneratedWarnings(r))
	}
	r.System.Raw = nil
	if !strings.Contains(output(t, r), "LLC size is unavailable") {
		t.Fatal("missing cache evidence was hidden")
	}
}
func diskFixture(profile string, n int) report.Report {
	r := fixture("disk test", n)
	r.Settings.Profile = profile
	run := disk.Result{Jobs: []disk.Job{{Read: disk.Direction{IOPS: 70, BWBytes: 286720, Runtime: 1000, Completion: disk.Latency{Percentile: map[string]float64{"99.000000": 20000}}}, Write: disk.Direction{IOPS: 30, BWBytes: 122880, Runtime: 1000, Completion: disk.Latency{Percentile: map[string]float64{"99.000000": 40000}}}}}}
	runs := []disk.Result{}
	for i := 0; i < n; i++ {
		runs = append(runs, run)
	}
	r.Results["disk test"] = runs
	return r
}
func TestMixedDiskSeparatesPercentilesAndUnits(t *testing.T) {
	r := diskFixture("mixed-70-30", 3)
	s := output(t, r)
	for _, want := range []string{"Read 70 IOPS / Write 30 IOPS", "Read 0.27 MiB/s / Write 0.12 MiB/s", "Read 0.020 ms / Write 0.040 ms", "Stable"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q: %s", want, s)
		}
	}
	if strings.Contains(s, "0.030 ms") {
		t.Fatal("percentiles averaged")
	}
	r.Settings.Profile = "randwrite-4k"
	s = output(t, r)
	if !strings.Contains(s, "30 IOPS") || strings.Contains(s, "70 IOPS") {
		t.Fatal("write profile used read results", s)
	}
}
func TestDiskPercentileRangeAcrossJobsAndRuns(t *testing.T) {
	r := diskFixture("randread-4k", 2)
	runs := r.Results["disk test"].([]disk.Result)
	runs[1] = disk.Result{Jobs: []disk.Job{{Read: disk.Direction{IOPS: 75, BWBytes: 300000, Completion: disk.Latency{Percentile: map[string]float64{"99.000000": 80000}}}}}}
	r.Results["disk test"] = runs
	s := output(t, r)
	if !strings.Contains(s, "0.020 - 0.080 ms") || strings.Contains(s, "0.050 ms") {
		t.Fatal(s)
	}
}
func TestQueueDepthScaling(t *testing.T) {
	r := diskFixture("qd-scaling", 1)
	base := r.Results["disk test"].([]disk.Result)[0]
	last := disk.Result{Jobs: []disk.Job{{Read: disk.Direction{IOPS: 140, BWBytes: 573440, Completion: disk.Latency{Percentile: map[string]float64{"99.000000": 80000}}}}}}
	r.Results["disk test"] = []disk.Result{base, base, base, base, last}
	s := output(t, r)
	for _, want := range []string{"QD1", "QD64", "2.00x", "0.080 ms"} {
		if !strings.Contains(s, want) {
			t.Fatal(want, s)
		}
	}
}
func TestNetworkReceiverOnlyAndNoInventedRTT(t *testing.T) {
	r := fixture("network test", 1)
	r.Results["network test"] = []network.Result{{End: json.RawMessage(`{"sum_sent":{"bits_per_second":9990000000},"sum_received":{"bits_per_second":8420000000,"seconds":30}}`)}}
	s := output(t, r)
	if !strings.Contains(s, "8.42 Gbps") || strings.Contains(s, "9.99 Gbps") || strings.Contains(s, "RTT") {
		t.Fatal(s)
	}
	r.Settings.Details = true
	s = output(t, r)
	if !strings.Contains(s, "RTT was not measured") {
		t.Fatal(s)
	}
}
func TestUDPAndBidirectionalPreserveDirections(t *testing.T) {
	r := fixture("network test", 1)
	r.Settings.UDP = true
	r.Settings.Bidir = true
	r.Results["network test"] = []network.Result{{End: json.RawMessage(`{"sum_received":{"bits_per_second":100000000,"seconds":10,"jitter_ms":0.3,"lost_percent":2},"sum_received_bidir_reverse":{"bits_per_second":90000000,"seconds":10,"jitter_ms":0.4,"lost_percent":5}}`)}}
	s := output(t, r)
	for _, want := range []string{"0.10 Gbps", "0.09 Gbps", "2.00 %", "5.00 %", "0.300 ms", "0.400 ms"} {
		if !strings.Contains(s, want) {
			t.Fatal(want, s)
		}
	}
	if len(Modules(r)[0].Metrics) > 5 {
		t.Fatal("too many metrics")
	}
}
func TestBidirectionalDurationsAndDetailedDirections(t *testing.T) {
	r := fixture("network test", 1)
	r.Settings.Bidir = true
	r.Settings.Details = true
	r.Results["network test"] = []network.Result{{End: json.RawMessage(`{"sum_received":{"bits_per_second":100000000,"seconds":10},"sum_received_bidir_reverse":{"bits_per_second":90000000,"seconds":9.5}}`)}}
	s := output(t, r)
	if !strings.Contains(s, "Client -> Server 10.00 s / Server -> Client 9.50 s") {
		t.Fatal("directional durations lost", s)
	}
	rows := Modules(r)[0].Tables[0].Rows
	if rows[0][1] != "Client -> Server" || rows[1][1] != "Server -> Client" {
		t.Fatal("detail directions mislabeled", rows)
	}
}
func TestMissingMeasurementsAndDetails(t *testing.T) {
	r := fixture("cpu test", 1)
	s := output(t, r)
	if !strings.Contains(s, "No validated measurements") || strings.Contains(s, "Stable") {
		t.Fatal(s)
	}
	r = nativeFixture("cpu test", 1)
	r.Settings.Details = true
	s = output(t, r)
	if !strings.Contains(s, "integer-xorshift64") || !strings.Contains(s, "N/A") || strings.Contains(s, "\"Measurements\"") {
		t.Fatal(s)
	}
}
func TestJSONOnlyStdoutAndRoundTrip(t *testing.T) {
	r := nativeFixture("cpu test", 1)
	r.Settings.JSON = true
	var b bytes.Buffer
	if e := Render(&b, r, "results/x.json", true); e != nil {
		t.Fatal(e)
	}
	var restored report.Report
	if e := json.Unmarshal(b.Bytes(), &restored); e != nil {
		t.Fatal(e, b.String())
	}
	if restored.ID != r.ID || restored.Results["cpu test"] == nil || strings.Contains(b.String(), "\x1b[") {
		t.Fatal("report changed")
	}
}
func TestNoColorModeAndFailureWarnings(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm")
	if ColorEnabled(os.Stdout) {
		t.Fatal("NO_COLOR ignored")
	}
	r := nativeFixture("cpu test", 1)
	r.Status = "failed"
	r.Results["execution_status"] = map[string]string{"cpu test": "failed"}
	r.Errors = []string{"engine verification failed"}
	r.Warnings = []string{"same warning", "same warning"}
	s := output(t, r)
	if !strings.Contains(s, "Failed") || !strings.Contains(s, "engine verification failed") || strings.Contains(s, "RUNNING") || strings.Count(s, "same warning") != 1 {
		t.Fatal(s)
	}
}
func TestFullSummaryRespectsUnrunAndSkipped(t *testing.T) {
	r := nativeFixture("cpu test", 1)
	r.Command = "full test"
	r.Results["execution_status"] = map[string]string{"cpu test": "completed", "ram test": "not_run", "disk test": "failed", "network test": "skipped"}
	r.Status = "failed"
	s := output(t, r)
	for _, want := range []string{"FULL BENCHMARK RESULTS", "CPU", "RAM", "DISK", "NETWORK", "Not run", "Skipped", "Failed"} {
		if !strings.Contains(s, want) {
			t.Fatal(want, s)
		}
	}
	if strings.Contains(s, "\"Kernel\"") {
		t.Fatal(s)
	}
}
func TestProgressBoundariesOnly(t *testing.T) {
	var b bytes.Buffer
	p := Progress{Writer: &b, Lang: "en"}
	p.Start("cpu test")
	time.Sleep(time.Millisecond)
	p.Finish("completed")
	s := b.String()
	if !strings.Contains(s, "Preparing workload") || !strings.Contains(s, "Completed") || strings.Contains(s, "100%") || strings.Contains(s, "\r") {
		t.Fatal(s)
	}
}
func TestJSONReportContentsUnaffectedByRendering(t *testing.T) {
	r := nativeFixture("ram test", 3)
	before, _ := json.Marshal(r)
	_ = output(t, r)
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("rendering mutated report")
	}
}

func TestPhaseObserverSkipVersionAndJSONWriterSeparation(t *testing.T) {
	var b bytes.Buffer
	p := Progress{Writer: &b, Lang: "en"}
	p.Observe(runner.Event{Name: "compute-bench", Args: []string{"--version"}, Starting: true})
	if b.Len() != 0 {
		t.Fatal("version probe became workload progress")
	}
	p.Start("cpu test")
	p.Observe(runner.Event{Name: "compute-bench", Args: []string{"cpu", "4", "1", "0", "16MiB"}, Starting: true})
	p.Finish("failed")
	if !strings.Contains(b.String(), "Launching cpu; 4 Workers") || !strings.Contains(b.String(), "Failed") {
		t.Fatal(b.String())
	}
}
func TestSmallNonzeroValuesAreNotRoundedToZero(t *testing.T) {
	if number(.0001, 2) == "0.00" || number(.4, 0) == "0" {
		t.Fatal("small measured value erased")
	}
}
func TestNOCOLOREnforcedByRenderer(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	r := nativeFixture("cpu test", 1)
	var b bytes.Buffer
	if err := Render(&b, r, "report.json", true); err != nil || strings.Contains(b.String(), "\x1b[") {
		t.Fatal(err, b.String())
	}
}
func TestSerializedReportRendersSameMetrics(t *testing.T) {
	r := diskFixture("mixed-70-30", 3)
	b, _ := json.Marshal(r)
	var restored report.Report
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if output(t, r) != output(t, restored) {
		t.Fatal("typed and serialized results differ")
	}
}
func TestInvalidDiskJobExcludedFromSummary(t *testing.T) {
	r := diskFixture("randread-4k", 1)
	runs := r.Results["disk test"].([]disk.Result)
	runs[0].Jobs[0].Error = 5
	r.Results["disk test"] = runs
	r.Status = "failed"
	r.Results["execution_status"] = map[string]string{"disk test": "failed"}
	s := output(t, r)
	if strings.Contains(s, "70 IOPS") || strings.Contains(s, "0.020 ms") || strings.Contains(s, "Stable") {
		t.Fatal(s)
	}
}

func TestJobFailureCannotAppearCompleted(t *testing.T) {
	r := diskFixture("randread-4k", 1)
	runs := r.Results["disk test"].([]disk.Result)
	runs[0].Jobs[0].Error = 5
	r.Results["disk test"] = runs
	s := output(t, r)
	if !strings.Contains(s, "VMBENCH DISK TEST | Failed") || !strings.Contains(s, "fio reported an I/O error") {
		t.Fatal(s)
	}
}

func TestDetailedThrottlingUsesOnlyRecordedCounters(t *testing.T) {
	r := nativeFixture("cpu test", 3)
	r.Settings.Details = true
	r.Results["cpu_telemetry"] = map[string]any{"throttling_before": "nr_throttled 4\nthrottled_usec 1000", "throttling_after": "nr_throttled 7\nthrottled_usec 3500"}
	if throttleDelta(r, "nr_throttled", 1, "") != "3" || throttleDelta(r, "throttled_usec", 1000, "ms") != "2.50 ms" {
		t.Fatal("counter conversion")
	}
	s := output(t, r)
	if !strings.Contains(s, "Recorded cgroup throttled time") || !strings.Contains(s, "2.50 ms") {
		t.Fatal(s)
	}
}
