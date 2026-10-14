package presentation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/benchmark/network"
	"vmbench/internal/system"
)

func TestStealWarningUsesConfiguredScreeningThreshold(t *testing.T) {
	for _, test := range []struct {
		value, threshold float64
		warning          bool
	}{{.02, 1, false}, {.99, 1, false}, {1, 1, true}, {1.5, 2, false}, {2, 2, true}, {.02, 0, true}} {
		r := nativeFixture("cpu test", 3)
		r.Settings.StealWarningPercent = test.threshold
		r.Results["cpu_telemetry"] = map[string]any{"steal_percent": test.value}
		// A historical equivalent warning must not bypass the current threshold.
		r.Warnings = []string{"CPU steal time was detected: the guest waited for host CPU time."}
		s := output(t, r)
		found := strings.Contains(s, "check host CPU contention")
		if found != test.warning {
			t.Fatal(test, s)
		}
		if strings.Count(s, "check host CPU contention") > 1 {
			t.Fatal("duplicated steal warning", s)
		}
		if !strings.Contains(s, unit(test.value, "%", 2)) {
			t.Fatal("measured steal was hidden", s)
		}
	}
}
func TestDefaultWarningFilteringAndSingleStabilityNotice(t *testing.T) {
	r := nativeFixture("cpu test", 1)
	r.Warnings = []string{"Fewer than three repetitions; limited stability evidence", "Fewer than three repetitions; limited stability evidence", "Preliminary workload; unsuitable for rigorous cross-platform comparisons", "Virtualization, backend contention and cache effects are uncontrolled; precision does not guarantee absolute accuracy"}
	s := output(t, r)
	if strings.Count(s, "Insufficient data") != 1 || strings.Contains(s, "WARNINGS") || strings.Contains(s, "uncontrolled") || strings.Contains(s, "limited stability evidence") {
		t.Fatal(s)
	}
	r.Settings.Details = true
	s = output(t, r)
	if !strings.Contains(s, "MEASUREMENT NOTES") || !strings.Contains(s, "uncontrolled") || strings.Contains(s, "limited stability evidence") {
		t.Fatal(s)
	}
}
func TestSemanticWarningDedupAndCriticalMessagesNeverCapped(t *testing.T) {
	r := diskFixture("randread-4k", 3)
	r.Warnings = []string{"Disk target is on the system filesystem", "The disk test targets the system filesystem.", "Read tests also write dataset initialization. Direct I/O does not bypass all storage caches. Benchmark files limited to selected size.", "Read tests write preparation files; direct I/O does not bypass every storage cache.", "Integrity error: fixture 1", "Invalid measurements: fixture 2", "Data safety failure: fixture 3", "Engine failure: fixture 4"}
	r.Errors = []string{"critical engine error", "critical engine error"}
	s := output(t, r)
	if strings.Count(s, "system filesystem") != 1 || strings.Count(s, "preparation files") != 1 || strings.Count(s, "critical engine error") != 1 {
		t.Fatal(s)
	}
	for _, want := range []string{"fixture 1", "fixture 2", "fixture 3", "fixture 4"} {
		if !strings.Contains(s, want) {
			t.Fatal("critical message hidden", s)
		}
	}
	if strings.Contains(s, "Full report") || strings.Contains(s, "Additional warnings") {
		t.Fatal(s)
	}
}
func TestDiskMeanLatenciesRemainSeparateAndMissingIsNA(t *testing.T) {
	r := diskFixture("mixed-70-30", 3)
	average := metricValue(Modules(r)[0], text("average"))
	if average != "Read 0.010 ms / Write 0.030 ms" {
		t.Fatal(average)
	}
	r.Settings.Profile = "randwrite-4k"
	if got := metricValue(Modules(r)[0], text("average")); got != "0.030 ms" {
		t.Fatal("write profile used read mean", got)
	}
	runs := r.Results["disk test"].([]disk.Result)
	for i := range runs {
		runs[i].Jobs[0].Write.Completion.Mean = 0
	}
	r.Results["disk test"] = runs
	if got := metricValue(Modules(r)[0], text("average")); got != "N/A" {
		t.Fatal("missing mean invented", got)
	}
}
func TestDiskMeanRangeDoesNotInventPooledLatency(t *testing.T) {
	r := diskFixture("randread-4k", 2)
	runs := r.Results["disk test"].([]disk.Result)
	// Make the second job/run independent of the first fixture's shared slice.
	runs[1] = disk.Result{Jobs: []disk.Job{{Read: disk.Direction{IOPS: 75, BWBytes: 300000, Completion: disk.Latency{Mean: 90000}}}}}
	r.Results["disk test"] = runs
	if got := metricValue(Modules(r)[0], text("average")); got != "0.010 - 0.090 ms" {
		t.Fatal(got)
	}
}
func TestFullSummaryTotalsAndNoWarningsForUnrunModules(t *testing.T) {
	r := nativeFixture("cpu test", 3)
	ram := nativeFixture("ram test", 3)
	disk := diskFixture("randread-4k", 3)
	r.Command = "full test"
	r.Results["ram test"] = ram.Results["ram test"]
	r.Results["disk test"] = disk.Results["disk test"]
	r.Results["execution_status"] = map[string]string{"cpu test": "completed", "ram test": "completed", "disk test": "completed", "network test": "skipped"}
	s := output(t, r)
	if !strings.Contains(s, "Overall: 3 completed, 1 skipped, 0 failed, 0 not run") || strings.Contains(s, "PERFORMANCE") {
		t.Fatal(s)
	}
	r.Status = "failed"
	r.Results["execution_status"] = map[string]string{"cpu test": "failed", "ram test": "not_run", "disk test": "not_run", "network test": "not_run"}
	s = output(t, r)
	if !strings.Contains(s, "0 completed, 0 skipped, 1 failed, 3 not run") || strings.Contains(s, "LLC") || strings.Contains(s, "Integrity results are unavailable") {
		t.Fatal(s)
	}
}
func TestASCIICompatibleMetadataAndEngineMessages(t *testing.T) {
	r := nativeFixture("cpu test", 3)
	r.Errors = []string{"engine error: \u00e9\x1b[31m"}
	s := output(t, r)
	if !strings.Contains(s, "engine error: \\u00E9") {
		t.Fatal(s)
	}
	var b bytes.Buffer
	if err := RenderSystemInfo(&b, system.Info{Hostname: "fixture \u1ea1", CPUModel: "test CPU", Architecture: "amd64", Capacity: 2, OS: "PRETTY_NAME=\"Test Linux\""}); err != nil {
		t.Fatal(err)
	}
	s += b.String()
	for _, c := range s {
		if c > 127 || c == 27 {
			t.Fatalf("unsafe terminal character %q", c)
		}
	}
	if !strings.Contains(b.String(), "Test Linux") || json.Valid(b.Bytes()) {
		t.Fatal(b.String())
	}
}
func TestNetworkDoesNotDisplayUnmeasuredUDPFields(t *testing.T) {
	r := fixture("network test", 1)
	r.Settings.UDP = true
	r.Results["network test"] = []network.Result{{End: json.RawMessage(`{"sum_received":{"bits_per_second":100000000,"seconds":10}}`)}}
	s := output(t, r)
	if !strings.Contains(s, "UDP") || strings.Contains(s, "packet loss") || strings.Contains(s, "jitter") || strings.Contains(s, "RTT") {
		t.Fatal(s)
	}
}
