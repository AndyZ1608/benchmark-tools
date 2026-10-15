package presentation

import (
	"bytes"
	"strings"
	"testing"
	automatic "vmbench/internal/benchmark/auto"
	"vmbench/internal/statistics"
)

func TestAutomaticCompactOutputUsesOnlyConfirmedPeaks(t *testing.T) {
	trial := automatic.Trial{Setting: automatic.Setting{Depth: 4, Jobs: 2}, Stats: statistics.Summary{Count: 3, Median: 1000, CV: .01}, Stable: true, Samples: []automatic.Sample{{Value: 1000, MiBps: 4, MeanMS: .1, MeanLowMS: .05, P99LowMS: .2, P99HighMS: .8, Evidence: "libaio; requested QD4; job 1: bucket 4 99.0%"}}}
	r := automatic.Result{Mode: "max", Status: "COMPLETED", States: map[string]string{"CPU": "SKIPPED", "MEMORY": "SKIPPED", "DISK": "COMPLETED", "NETWORK": "SKIPPED"}, Disk: []automatic.Peak{{Name: "randread-4k", Unit: "IOPS", Best: &trial, Confirmed: true, Trials: []automatic.Trial{trial}}}}
	var b bytes.Buffer
	if e := RenderAutomatic(&b, r, false); e != nil {
		t.Fatal(e)
	}
	out := b.String()
	if !strings.Contains(out, "\nCPU - SKIPPED\n") || !strings.Contains(out, "\nConfiguration: QD4 / 2 jobs\n") {
		t.Fatal("summary layout lost line breaks", out)
	}
	for _, expected := range []string{"Maximum Observed IOPS (Random Read 4K)", "QD4 / 2 jobs", "0.200 - 0.800 ms", "NETWORK - SKIPPED", "Search: 1 configurations"} {
		if !strings.Contains(out, expected) {
			t.Fatal(expected, out)
		}
	}
	if strings.Contains(out, "intermediate measurements") || strings.Contains(out, "0.500 ms") {
		t.Fatal(out)
	}
	for _, r := range out {
		if r > 127 {
			t.Fatal("non ASCII")
		}
	}
	r.Disk[0].Best = nil
	r.Disk[0].Confirmed = false
	b.Reset()
	RenderAutomatic(&b, r, false)
	if strings.Contains(b.String(), "Maximum Observed IOPS") {
		t.Fatal("unconfirmed maximum shown", b.String())
	}
}
