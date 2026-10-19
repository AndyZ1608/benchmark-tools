package presentation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	automatic "vmbench/internal/benchmark/auto"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/benchmark/network"
	"vmbench/internal/statistics"
)

// All values are synthetic renderer fixtures, never host benchmark results.
func unifiedFixture(component, mode string) automatic.Result {
	r := automatic.Result{Mode: mode, Status: "COMPLETED", Limits: automatic.Defaults(mode), States: map[string]string{}}
	r.Limits.Component = component
	module := strings.ToUpper(component)
	if component == "ram" {
		module = "MEMORY"
	}
	r.States[module] = "COMPLETED"
	makePeak := func(name, unitName string, value float64) automatic.Peak {
		trial := automatic.Trial{Setting: automatic.Setting{Workers: 4}, Duration: 5, Stable: true, Stats: statistics.Summary{Count: 3, Median: value, CV: .01}}
		return automatic.Peak{Name: name, Unit: unitName, Best: &trial, Confirmed: mode != "quick", Trials: []automatic.Trial{trial}}
	}
	switch component {
	case "cpu":
		for i, name := range []string{"integer-xorshift64", "fp-multiply-add"} {
			value, base := 3600.0, 1000.0
			if i == 1 {
				value, base = 1900, 500
			}
			peak := makePeak(name, "defined iterations/s", value)
			peak.Trials[0].Setting.Workers = 1
			peak.Trials[0].Stats.Median = base
			for n := 0; n < 3; n++ {
				peak.Best.Samples = append(peak.Best.Samples, automatic.Sample{Value: value, Extras: map[string]float64{"CPU steal percent": .02}, Raw: cpu.NativeResult{Workers: 4, Dataset: 16 << 20, Measurements: []cpu.Measurement{{Workload: name, Value: value, Unit: "defined_iterations/s", Elapsed: 5}}}})
			}
			r.CPU = append(r.CPU, peak)
		}
	case "ram":
		peak := makePeak("triad", "GB/s", 28.6)
		peak.Best.Samples = []automatic.Sample{{Value: 28.6, LatencyNS: 85, Raw: cpu.NativeResult{Workers: 4, Dataset: 128 << 20, Measurements: []cpu.Measurement{{Workload: "triad", Value: 28.6, Unit: "GB/s", Elapsed: 5}, {Workload: "copy", Value: 24, Unit: "GB/s", Elapsed: 5}, {Workload: "dependent-pointer-chase", Value: 85, Unit: "ns/access mean per worker", Elapsed: 5}}}}}
		r.Memory = []automatic.Peak{peak}
	case "network":
		for index, name := range []string{"TCP TX", "TCP RX"} {
			if mode == "quick" && index == 1 {
				break
			}
			value := .842
			if index == 1 {
				value = 1.2
			}
			peak := makePeak(name, "Gbps", value)
			peak.Best.Setting = automatic.Setting{Streams: 2}
			for n := 0; n < 3; n++ {
				raw := network.Result{End: json.RawMessage(fmt.Sprintf(`{"sum_sent":{"retransmits":%d},"sum_received":{"bits_per_second":%.0f,"seconds":5,"lost_percent":80,"jitter_ms":100}}`, n+3, value*1e9))}
				peak.Best.Samples = append(peak.Best.Samples, automatic.Sample{Value: value, Raw: raw})
			}
			r.Network = append(r.Network, peak)
		}
	case "disk":
		r = compactDiskFixture()
		r.Mode, r.Limits.Mode = mode, mode
		for i := range r.Disk {
			r.Disk[i].Confirmed = mode != "quick"
		}
	}
	return r
}

func TestUnifiedDefaultAndDetailsAcrossComponentsAndModes(t *testing.T) {
	for _, component := range []string{"cpu", "ram", "disk", "network"} {
		for _, mode := range []string{"quick", "standard", "max"} {
			t.Run(component+"/"+mode, func(t *testing.T) {
				r, original := unifiedFixture(component, mode), unifiedFixture(component, mode)
				out := renderAutomaticTest(t, r, false)
				if !strings.Contains(out, "VMBENCH - "+strings.ToUpper(component+" "+mode)) || !strings.Contains(out, "Status: COMPLETED") {
					t.Fatal(out)
				}
				limit := 26
				if component == "disk" {
					limit = 32
				}
				if len(strings.Split(strings.TrimSpace(out), "\n")) > limit {
					t.Fatal("default too verbose", out)
				}
				for _, hidden := range []string{"Configuration:", "Dataset:", "Detected resources:", "affinity", "Searched configurations", "Integrity errors:", "N/A"} {
					if strings.Contains(out, hidden) {
						t.Fatal(hidden, out)
					}
				}
				if mode == "quick" && !strings.Contains(out, "Results: Preliminary") {
					t.Fatal(out)
				}
				if mode == "max" && !strings.Contains(out, "within tested limits") {
					t.Fatal(out)
				}
				details := renderAutomaticTest(t, r, true)
				for _, required := range []string{"Configuration:", "Searched configurations", "Repetitions", "CV", "Total time ceiling:", "Detected resources:"} {
					if !strings.Contains(details, required) {
						t.Fatal(required, details)
					}
				}
				if !reflect.DeepEqual(r, original) {
					t.Fatal("presentation mutated source measurements")
				}
			})
		}
	}
}

func TestUnifiedCPUUsesThroughputScalingAndMeasuredSteal(t *testing.T) {
	r := unifiedFixture("cpu", "max")
	out := renderAutomaticTest(t, r, false)
	for _, value := range []string{"Integer Compute: 3,600 defined iterations/s (scaling 90.0 %)", "Float Compute  : 1,900 defined iterations/s (scaling 95.0 %)", "CPU Steal      : 0.02 %"} {
		if !strings.Contains(out, value) {
			t.Fatal(out)
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), -1, 101} {
		for i := range r.CPU {
			for j := range r.CPU[i].Best.Samples {
				r.CPU[i].Best.Samples[j].Extras["CPU steal percent"] = value
			}
		}
		if strings.Contains(renderAutomaticTest(t, r, false), "CPU Steal ") {
			t.Fatal("invalid steal was displayed")
		}
	}
	for i := range r.CPU {
		r.CPU[i].Best.Samples[0].Extras = map[string]float64{"CPU steal percent": 0}
	}
	if !strings.Contains(renderAutomaticTest(t, r, false), "CPU Steal      : 0.00 %") {
		t.Fatal("measured zero hidden")
	}
	r.CPU[0].Trials = nil
	if strings.Contains(strings.Split(renderAutomaticTest(t, r, false), "Float Compute")[0], "scaling") {
		t.Fatal("missing baseline fabricated")
	}
}

func TestUnifiedRAMLatencyAndIntegrityEvidence(t *testing.T) {
	r := unifiedFixture("ram", "max")
	out := renderAutomaticTest(t, r, false)
	for _, value := range []string{"28.60 GB/s (TRIAD)", "85.00 ns/access"} {
		if !strings.Contains(out, value) {
			t.Fatal(out)
		}
	}
	if strings.Contains(out, "24.00 GB/s") || strings.Contains(out, "Integrity") {
		t.Fatal(out)
	}
	r.Memory[0].Best.Samples[0].LatencyNS = 0
	if strings.Contains(renderAutomaticTest(t, r, false), "Memory Latency") {
		t.Fatal("latency derived from throughput")
	}
	for _, component := range []string{"cpu", "ram"} {
		r := unifiedFixture(component, "max")
		peaks := r.CPU
		if component == "ram" {
			peaks = r.Memory
		}
		raw := peaks[0].Best.Samples[0].Raw.(cpu.NativeResult)
		raw.Measurements[0].Errors = 1
		peaks[0].Best.Samples[0].Raw = raw
		for _, details := range []bool{false, true} {
			out := renderAutomaticTest(t, r, details)
			if !strings.Contains(out, "Status: FAILED") || !strings.Contains(out, "Data integrity errors") {
				t.Fatal(out)
			}
		}
	}
}

func TestUnifiedNetworkUnitsDirectionsAndRetransmitCounts(t *testing.T) {
	r := unifiedFixture("network", "quick")
	out := renderAutomaticTest(t, r, false)
	for _, want := range []string{"TCP Throughput : 842.00 Mbps", "TCP Retransmits: 3 - 5 (per-run range)", "Direction      : Upload"} {
		if !strings.Contains(out, want) {
			t.Fatal(want, out)
		}
	}
	for _, hidden := range []string{"Download", "Loss", "Jitter", "12 (per-run"} {
		if strings.Contains(out, hidden) {
			t.Fatal(out)
		}
	}
	r.Network[0].Name = "TCP RX"
	if !strings.Contains(renderAutomaticTest(t, r, false), "Direction      : Download") {
		t.Fatal("reverse direction mislabeled")
	}
	r = unifiedFixture("network", "max")
	out = renderAutomaticTest(t, r, false)
	for _, want := range []string{"TCP Upload", "842.00 Mbps", "TCP Download", "1.20 Gbps", "Download Retransmits"} {
		if !strings.Contains(out, want) {
			t.Fatal(out)
		}
	}
	for _, sample := range []string{`{"sum_received":{"bits_per_second":842000000,"seconds":5}}`, `{"sum_sent":{"retransmits":null},"sum_received":{"bits_per_second":842000000,"seconds":5}}`} {
		r.Network[0].Best.Samples = []automatic.Sample{{Raw: network.Result{End: json.RawMessage(sample)}}}
		r.Network = r.Network[:1]
		if strings.Contains(renderAutomaticTest(t, r, false), "Retransmits") {
			t.Fatal("unmeasured retransmits invented")
		}
	}
	if networkRate(1e9) != "1.00 Gbps" || networkRate(100e6) != "100.00 Mbps" || countRange([]uint64{1234, 5678}) != "1,234 - 5,678 (per-run range)" {
		t.Fatal("unit/count formatting")
	}
}

func TestUnifiedFailuresPartialResultsAndOptionalCounters(t *testing.T) {
	for _, component := range []string{"cpu", "ram", "disk", "network"} {
		r := unifiedFixture(component, "max")
		r.Status = "PARTIAL"
		r.Warnings = []string{"Resource pressure detected; results incomplete"}
		for _, details := range []bool{false, true} {
			out := renderAutomaticTest(t, r, details)
			if !strings.Contains(out, "Status: PARTIAL") || !strings.Contains(out, "Resource pressure") {
				t.Fatal(out)
			}
		}
	}
	r := unifiedFixture("network", "max")
	r.Network[0].Best.Samples[0].Raw = network.Result{End: json.RawMessage(`{"sum_received":{"bits_per_second":842000000}}`)}
	out := renderAutomaticTest(t, r, false)
	if !strings.Contains(out, "Status: PARTIAL") || !strings.Contains(out, "transfer measurements are incomplete") {
		t.Fatal(out)
	}
	r.Network[0].Best.Samples[0].Raw = network.Result{End: json.RawMessage(`{"sum_sent":{"retransmits":0},"sum_received":{"bits_per_second":842000000,"seconds":5}}`)}
	out = renderAutomaticTest(t, r, false)
	if !strings.Contains(out, "0 - 5 (per-run range)") {
		t.Fatal("recorded zero lost", out)
	}
}

func TestAdvancedNetworkMissingAndDirectionalCounters(t *testing.T) {
	r := fixture("network test", 1)
	r.Results["network test"] = []network.Result{{End: json.RawMessage(`{"sum_sent":{"retransmits":7},"sum_received":{"bits_per_second":100000000,"seconds":5,"lost_percent":50},"sum_sent_bidir_reverse":{"retransmits":11},"sum_received_bidir_reverse":{"bits_per_second":2000000000,"seconds":5}}`)}}
	for _, reverse := range []bool{false, true} {
		r.Settings.Reverse = reverse
		out := output(t, r)
		want := "Direction      : Upload"
		if reverse {
			want = "Direction      : Download"
		}
		if !strings.Contains(out, want) || !strings.Contains(out, "TCP Retransmits: 7") || strings.Contains(out, "Loss") {
			t.Fatal(out)
		}
	}
	r.Settings.Bidir = true
	out := output(t, r)
	for _, want := range []string{"TCP Upload", "TCP Download", "2.00 Gbps", "Upload Retransmits  : 7", "Download Retransmits: 11"} {
		if !strings.Contains(out, want) {
			t.Fatal(out)
		}
	}
	r.Results["network test"] = []network.Result{{End: json.RawMessage(`{"sum_received":{"bits_per_second":100000000,"seconds":5}}`)}}
	out = output(t, r)
	if !strings.Contains(out, "Status: FAILED") || strings.Contains(out, "TCP Download") {
		t.Fatal(out)
	}
}

func TestFullSummaryCountsPartialNetworkMeasurements(t *testing.T) {
	r := nativeFixture("cpu test", 3)
	ram, disk := nativeFixture("ram test", 3), diskFixture("randread-4k", 3)
	r.Command = "full test"
	r.Results["ram test"], r.Results["disk test"] = ram.Results["ram test"], disk.Results["disk test"]
	r.Results["network test"] = []network.Result{{End: json.RawMessage(`{"sum_received":{"bits_per_second":100000000}}`)}}
	r.Results["execution_status"] = map[string]string{"cpu test": "completed", "ram test": "completed", "disk test": "completed", "network test": "completed"}
	out := output(t, r)
	if !strings.Contains(out, "Status: PARTIAL") || !strings.Contains(out, "3 completed, 0 skipped, 0 failed, 1 partial, 0 not run") || !strings.Contains(out, "transfer measurements are incomplete") {
		t.Fatal(out)
	}
}
