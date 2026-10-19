package presentation

import (
	"reflect"
	"strings"
	"testing"

	automatic "vmbench/internal/benchmark/auto"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/statistics"
)

func TestDiskUnconfirmedMeasurementsStayVisibleBesideConfirmedWorkload(t *testing.T) {
	r := compactDiskFixture()
	r.Status = "COMPLETED WITH LIMITATIONS"
	r.Disk[0].Confirmed = false
	r.Disk[0].Best = nil
	r.Disk[0].Trials[0].Stable = false
	r.Disk[0].Trials[0].Stats.CV = .4
	for _, details := range []bool{false, true} {
		out := renderAutomaticTest(t, r, details)
		random := strings.SplitN(out, "SEQUENTIAL READ", 2)[0]
		for _, want := range []string{"IOPS           : 15,083", "Throughput     : 58.92 MiB/s", "18.5 - 25.6 ms (job/run range)", "Confidence     : UNCONFIRMED"} {
			if !strings.Contains(random, want) {
				t.Fatal(out)
			}
		}
		if !strings.Contains(out, "Confidence     : CONFIRMED") || !strings.Contains(out, "high measurement variability") {
			t.Fatal(out)
		}
		if !details && strings.Contains(out, "Results: Maximum observed") {
			t.Fatal("unconfirmed observation promoted", out)
		}
	}
}

func TestDiskPartialSingleSampleUsesOnlyItsOwnMeasurements(t *testing.T) {
	r := compactDiskFixture()
	r.Status = "FAILED"
	r.Errors = []string{"DISK: context deadline exceeded"}
	trial := r.Disk[0].Trials[0]
	trial.Setting = automatic.Setting{Depth: 8, Jobs: 1}
	trial.Stable = false
	trial.Failure = "context deadline exceeded"
	trial.Samples = trial.Samples[:2]
	trial.Samples[0].Value, trial.Samples[0].MiBps = 100, 100.0/256
	trial.Samples[0].P99LowMS, trial.Samples[0].P99HighMS = 1, 2
	trial.Samples[1].Value, trial.Samples[1].MiBps = 900, 900.0/256
	trial.Samples[1].P99LowMS, trial.Samples[1].P99HighMS = 80, 90
	trial.Stats, _ = statistics.Describe([]float64{100, 900})
	r.Disk[0] = automatic.Peak{Name: "randread-4k", Unit: "IOPS", Trials: []automatic.Trial{trial}, Failure: trial.Failure, SearchIncomplete: true}
	out := renderAutomaticTest(t, r, false)
	for _, want := range []string{"IOPS           : 900", "Throughput     : 3.52 MiB/s", "80.0 - 90.0 ms (job/run range)", "Observation    : Single sample", "Confidence     : UNCONFIRMED", "Status: FAILED", "deadline exceeded"} {
		if !strings.Contains(out, want) {
			t.Fatal(out)
		}
	}
	if strings.Contains(out, "IOPS           : 500") || strings.Contains(out, "1.00 - 2.00 ms") {
		t.Fatal("configurations or samples mixed", out)
	}
	details := renderAutomaticTest(t, r, true)
	for _, want := range []string{"Selection:", "original repetition 2", "QD8 / 1 jobs", "Search: INCOMPLETE", "incomplete: context deadline exceeded"} {
		if !strings.Contains(details, want) {
			t.Fatal(details)
		}
	}
}

func TestDiskInvalidSampleCannotPoisonEarlierValidSampleOrBePromoted(t *testing.T) {
	r := compactDiskFixture()
	r.Disk[0].Confirmed = false
	r.Disk[0].Best = nil
	valid := r.Disk[0].Trials[0]
	invalid := valid
	invalid.Samples = append([]automatic.Sample(nil), valid.Samples...)
	for i := range invalid.Samples {
		raw := invalid.Samples[i].Raw.(disk.Result)
		raw.Jobs = append([]disk.Job(nil), raw.Jobs...)
		raw.Jobs[0].Error = 5
		invalid.Samples[i].Raw = raw
	}
	r.Disk[0].Trials = []automatic.Trial{valid, invalid}
	r.Disk[0].Failure = "fio I/O error 5"
	out := renderAutomaticTest(t, r, false)
	if !strings.Contains(out, "IOPS           : 15,083") || !strings.Contains(out, "Confidence     : UNCONFIRMED") || !strings.Contains(out, "Invalid fio I/O evidence") {
		t.Fatal(out)
	}
	r.Disk[0].Trials = []automatic.Trial{invalid}
	out = renderAutomaticTest(t, r, false)
	random := strings.SplitN(out, "SEQUENTIAL READ", 2)[0]
	for _, want := range []string{"IOPS           : N/A", "Throughput     : N/A", "P99 Latency    : N/A", "Confidence     : NO VALID MEASUREMENT", "fio I/O error 5"} {
		if !strings.Contains(random, want) {
			t.Fatal(out)
		}
	}
	if strings.Contains(random, "UNCONFIRMED") || strings.Contains(random, "IOPS           : 0") {
		t.Fatal("invalid I/O treated as statistical uncertainty", out)
	}
}

func TestDiskAlwaysShowsBothWorkloadsAndActualFailureReasons(t *testing.T) {
	r := compactDiskFixture()
	r.Status = "FAILED"
	r.Disk = nil
	r.Errors = []string{"DISK: target directory is not writable"}
	for _, details := range []bool{false, true} {
		out := renderAutomaticTest(t, r, details)
		if !strings.Contains(out, "RANDOM READ 4K") || !strings.Contains(out, "SEQUENTIAL READ") || strings.Count(out, "Confidence     : NO VALID MEASUREMENT") != 2 || !strings.Contains(out, "target directory is not writable") {
			t.Fatal(out)
		}
	}
}

func TestDiskRetentionViewsDoNotMutateMeasuredHistory(t *testing.T) {
	r, original := compactDiskFixture(), compactDiskFixture()
	for _, value := range []*automatic.Result{&r, &original} {
		value.Disk[0].Best = nil
		value.Disk[0].Confirmed = false
	}
	for _, details := range []bool{false, true, false} {
		renderAutomaticTest(t, r, details)
	}
	if !reflect.DeepEqual(r, original) {
		t.Fatal("selection/rendering mutated historical measurements")
	}
}

func TestDiskPartialCleanupDoesNotEraseValidPerformance(t *testing.T) {
	r := compactDiskFixture()
	r.Status = "FAILED"
	r.DiskCleaned = false
	r.Errors = []string{"disk cleanup incomplete: ownership changed"}
	out := renderAutomaticTest(t, r, false)
	for _, want := range []string{"IOPS           : 15,083", "Throughput     : 95.61 MiB/s", "Temporary Files: Cleanup incomplete", "ownership changed"} {
		if !strings.Contains(out, want) {
			t.Fatal(out)
		}
	}
}

func TestDiskRepeatedSelectionDoesNotMixOtherConfigurationLatency(t *testing.T) {
	r := compactDiskFixture()
	strong, other := r.Disk[0].Trials[0], r.Disk[0].Trials[0]
	other.Samples = append([]automatic.Sample(nil), other.Samples...)
	other.Setting.Depth = 32
	for i := range other.Samples {
		other.Samples[i].Value = 100
		other.Samples[i].MiBps = 100.0 / 256
		other.Samples[i].P99LowMS = 1
		other.Samples[i].P99HighMS = 2
	}
	other.Stats, _ = statistics.Describe([]float64{100, 100, 100})
	r.Disk[0].Best = nil
	r.Disk[0].Confirmed = false
	r.Disk[0].Trials = []automatic.Trial{other, strong}
	out := strings.SplitN(renderAutomaticTest(t, r, false), "SEQUENTIAL READ", 2)[0]
	if !strings.Contains(out, "IOPS           : 15,083") || !strings.Contains(out, "58.92 MiB/s") || !strings.Contains(out, "18.5 - 25.6 ms") || strings.Contains(out, "1.00 - 2.00 ms") {
		t.Fatal(out)
	}
}
