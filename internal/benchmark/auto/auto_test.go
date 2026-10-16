package auto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/benchmark/network"
	"vmbench/internal/config"
	"vmbench/internal/system"
)

// These deliberately synthetic deterministic curves test the controller, never
// stand in for measurements in the production backend.
type fakeBackend struct {
	events                 []string
	configs                []config.Config
	opened, closed         int
	fail, invalid, cleanup bool
	cancel                 context.CancelFunc
	free                   uint64
	networkConfigs         []config.Config
	selectedEngine         string
	engineFailure          bool
	failDiskDepth          int
	diskAccounting         bool
}

func (f *fakeBackend) Detect() system.Info {
	return system.Info{LogicalCPUs: 16, Affinity: "0-15", Capacity: 8, AvailableMemory: 8 << 30, TotalMemory: 16 << 30, LastLevelCache: 8 << 20, NUMANodes: 2}
}
func (f *fakeBackend) Native(ctx context.Context, kind string, c config.Config) (cpu.NativeResult, map[string]float64, error) {
	f.events = append(f.events, kind)
	if f.fail && kind == "cpu" {
		return cpu.NativeResult{}, nil, fmt.Errorf("fixture CPU failure")
	}
	r := cpu.NativeResult{Kernel: "compute-bench-v1", Workers: c.Workers}
	names := []string{"integer-xorshift64", "fp-multiply-add"}
	if kind == "ram" {
		names = []string{"copy", "scale", "add", "triad", "dependent-pointer-chase"}
	}
	for _, name := range names {
		optimal := 4
		if name == "fp-multiply-add" {
			optimal = 2
		}
		if kind == "ram" {
			optimal = 2
		}
		value := 1000 - float64(abs(c.Workers-optimal))*100
		if name == "dependent-pointer-chase" {
			value = 70
		}
		if f.invalid {
			value = math.NaN()
		}
		r.Measurements = append(r.Measurements, cpu.Measurement{Workload: name, Value: value, Elapsed: float64(c.Duration)})
	}
	return r, nil, nil
}
func (f *fakeBackend) Filesystem(path string) (DiskInfo, error) {
	free := f.free
	if free == 0 {
		free = 16 << 30
	}
	return DiskInfo{Path: path, Filesystem: "fixture", Mount: "/temporary", Free: free}, nil
}
func (f *fakeBackend) SelectDiskEngine(context.Context, config.Config, uint64) (string, error) {
	if f.engineFailure {
		return "", fmt.Errorf("io_uring: operation not permitted; libaio: submitted I/O failed")
	}
	if f.selectedEngine != "" {
		return f.selectedEngine, nil
	}
	return "io_uring", nil
}
func (f *fakeBackend) OpenDisk(ctx context.Context, c config.Config, reserve uint64) (DiskSession, error) {
	f.events = append(f.events, "disk-open")
	f.opened++
	return f, nil
}
func (f *fakeBackend) Measure(ctx context.Context, c config.Config) (disk.Outcome, error) {
	f.events = append(f.events, "disk")
	f.configs = append(f.configs, c)
	if f.failDiskDepth == c.Depth {
		return disk.Outcome{}, fmt.Errorf("fio completion failure at QD%d", c.Depth)
	}
	if f.cancel != nil {
		f.cancel()
		return disk.Outcome{}, ctx.Err()
	}
	score := float64(1000 - abs(c.Depth-4)*30 - abs(c.Jobs-2)*100)
	if score < 10 {
		score = 10
	}
	r := disk.Result{Options: map[string]string{"ioengine": c.Engine}}
	for i := 0; i < c.Jobs; i++ {
		v := disk.Direction{TotalIOs: 100, IOBytes: 409600, IOPS: score / float64(c.Jobs), BWBytes: score * 4096 / float64(c.Jobs), Runtime: float64(c.Duration * 1000), Completion: disk.Latency{Mean: 100000, Percentile: map[string]float64{"99.000000": float64(1000000 + c.Depth*1000)}}}
		bucket := fmt.Sprint(c.Depth)
		if c.Depth >= 64 {
			bucket = ">=64"
		}
		r.Jobs = append(r.Jobs, disk.Job{Read: v, Write: v, Depth: map[string]float64{bucket: 100}})
	}
	if f.diskAccounting {
		r.Accounting = []disk.CounterObservation{{Job: "job-001", Direction: "read", IOBytes: 413696, TotalIOs: 100, BlockBytes: 4096, ByteDifference: "+4096", Explanation: "fixture ramp accounting"}}
	}
	return disk.Outcome{Results: []disk.Result{r}}, nil
}
func (f *fakeBackend) Close() error {
	f.closed++
	if f.cleanup {
		return fmt.Errorf("fixture ownership changed")
	}
	return nil
}
func (f *fakeBackend) Network(ctx context.Context, c config.Config) (network.Result, error) {
	f.events = append(f.events, "network")
	f.networkConfigs = append(f.networkConfigs, c)
	value := 1e9 - float64(abs(c.Streams-2))*1e8
	data, _ := json.Marshal(map[string]any{"sum_received": map[string]any{"bits_per_second": value, "seconds": c.Duration}})
	return network.Result{End: data}, nil
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func TestModePoliciesAndSafetySelection(t *testing.T) {
	for _, mode := range []string{"quick", "standard", "max"} {
		o, e := Parse(mode, nil)
		if e != nil {
			t.Fatal(e)
		}
		p := o.Policy()
		if mode == "quick" && p.ExploreReps != 1 || mode != "quick" && p.ConfirmReps < 3 || mode == "max" && p.ConfirmSeconds <= p.ExploreSeconds {
			t.Fatal(mode, p)
		}
	}
	o := Defaults("max")
	p := MakePlan(o, system.Info{LogicalCPUs: 16, Capacity: 8, AvailableMemory: 2 << 30, LastLevelCache: 16 << 20})
	if p.Workers != 8 || p.MemoryBytes > int64(2<<30)/4 {
		t.Fatal(p)
	}
	o.MaxWorkers = 3
	o.MaxRAM = "64MiB"
	p = MakePlan(o, system.Info{Capacity: 8, AvailableMemory: 2 << 30})
	if p.Workers != 3 || p.MemoryBytes > 48<<20 {
		t.Fatal(p)
	}
	for _, args := range [][]string{{"--min-free-disk", "1MiB"}, {"--max-disk-depth", "128"}, {"--max-workers", "129"}, {"--network-peer", "127.0.0.1"}, {"--include-writes"}, {"--network-bitrate", "1Garbage"}, {"--max-cv", "NaN"}} {
		if _, e := Parse("max", args); e == nil {
			t.Fatal("unsafe options accepted", args)
		}
	}
}
func TestAutomaticConfigurationPrecedenceAndAuthorization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auto.yaml")
	os.WriteFile(path, []byte("disk-path: /data\nnetwork-peer: 10.0.0.20\nmax-workers: 4\ninclude-writes: true\n"), 0600)
	o, e := Parse("max", []string{"--config", path, "--max-workers", "2", "--confirm-writes"})
	if e != nil || o.MaxWorkers != 2 || !o.IncludeWrites || o.NetworkPeer != "10.0.0.20" {
		t.Fatal(o, e)
	}
	os.WriteFile(path, []byte("ack-max: true\n"), 0600)
	if _, e = Parse("max", []string{"--config", path}); e == nil {
		t.Fatal("saved acknowledgment accepted")
	}
}
func TestQuickRunsSequentiallyAndSkipsMissingEndpoints(t *testing.T) {
	f := &fakeBackend{}
	o := Defaults("quick")
	r, e := Run(context.Background(), o, f, nil)
	if e != nil || r.States["NETWORK"] != "SKIPPED" || r.States["DISK"] != "SKIPPED" || r.CPU[0].Confirmed || r.CPU[0].Best == nil {
		t.Fatal(r, e)
	}
	if strings.Join(f.events, ",") != "cpu,cpu,ram" {
		t.Fatal("duplicate CPU kernels or module order", f.events)
	}
}
func TestMAXNativeDiscoverySeparatesWorkloadsAndConfirms(t *testing.T) {
	o := Defaults("max")
	o.AckMax = true
	f := &fakeBackend{}
	r, e := Run(context.Background(), o, f, nil)
	if e != nil || len(r.CPU) != 2 || r.CPU[0].Best.Setting.Workers != 4 || r.CPU[1].Best.Setting.Workers != 2 || r.Memory[0].Best.Setting.Workers != 2 {
		t.Fatal(r, e)
	}
	for _, p := range append(r.CPU, r.Memory...) {
		if !p.Confirmed || p.Best.Stats.Count < 3 || p.Best.Duration <= o.Policy().ExploreSeconds {
			t.Fatal(p)
		}
	}
}
func TestDiskAdaptiveDepthAndJobsReuseDataset(t *testing.T) {
	f := &fakeBackend{}
	o := Defaults("max")
	o.AckMax = true
	o.DiskPath = "/disposable fixture"
	o.MaxDepth = 8
	o.MaxJobs = 4
	o.MaxOutstanding = 16
	o.MaxDisk = "256MiB"
	p := MakePlan(o, f.Detect())
	peaks, _, total, cleaned, e := runDisk(context.Background(), o, p, f.Detect(), f, nil)
	if e != nil || !cleaned || f.opened != 1 || f.closed != 1 || total > 256<<20 || len(peaks) != 2 {
		t.Fatal(peaks, total, cleaned, e)
	}
	for _, p := range peaks {
		if !p.Confirmed || p.Best.Setting.Depth != 4 || p.Best.Setting.Jobs != 2 {
			t.Fatal(p)
		}
	}
	for _, c := range f.configs {
		if c.Depth*c.Jobs > o.MaxOutstanding || c.Jobs > o.MaxJobs || c.Depth > o.MaxDepth || !c.Direct {
			t.Fatal(c)
		}
	}
}
func TestStandardDepthsAndOptionalWriteConfirmation(t *testing.T) {
	o := Defaults("standard")
	o.DiskPath = "/fixture"
	o.IncludeWrites = true
	o.ConfirmWrites = true
	o.MaxDepth = 32
	f := &fakeBackend{}
	peaks, _, _, _, e := runDisk(context.Background(), o, MakePlan(o, f.Detect()), f.Detect(), f, nil)
	if e != nil || len(peaks) != 5 {
		t.Fatal(e, peaks)
	}
	seen := map[int]bool{}
	for _, c := range f.configs {
		seen[c.Depth] = true
		if strings.Contains(c.Profile, "write") && !c.ConfirmWrites {
			t.Fatal(c)
		}
	}
	for _, depth := range []int{1, 8, 32} {
		if !seen[depth] {
			t.Fatal(seen)
		}
	}
}
func TestInsufficientStorageAndCancellationCleanup(t *testing.T) {
	o := Defaults("max")
	o.AckMax = true
	o.DiskPath = "/fixture"
	f := &fakeBackend{free: 1 << 30}
	_, _, _, _, e := runDisk(context.Background(), o, MakePlan(o, f.Detect()), f.Detect(), f, nil)
	if e == nil || f.opened != 0 {
		t.Fatal(e, f)
	}
	ctx, cancel := context.WithCancel(context.Background())
	f = &fakeBackend{cancel: cancel}
	_, _, _, cleaned, e := runDisk(ctx, o, MakePlan(o, f.Detect()), f.Detect(), f, nil)
	if !errors.Is(e, context.Canceled) || !cleaned || f.closed != 1 {
		t.Fatal(e, cleaned, f)
	}
}
func TestNetworkStreamsDirectionsAndAggregateTrafficLimit(t *testing.T) {
	o := Defaults("max")
	o.NetworkPeer = "10.0.0.20"
	o.MaxStreams = 8
	f := &fakeBackend{}
	peaks, e := runNetwork(context.Background(), o, f, nil)
	if e != nil || len(peaks) != 2 || peaks[0].Best.Setting.Streams != 2 || peaks[1].Best.Setting.Streams != 2 {
		t.Fatal(peaks, e)
	}
	reverse := false
	for _, c := range f.networkConfigs {
		rate, e := Bitrate(c.TCPBitrate)
		if e != nil || rate*int64(c.Streams) > 1e9 || c.UDP {
			t.Fatal(c, e)
		}
		reverse = reverse || c.Reverse
	}
	if !reverse {
		t.Fatal("RX not tested")
	}
	o.NetworkBitrate = "1"
	f = &fakeBackend{}
	if _, e = runNetwork(context.Background(), o, f, nil); e != nil {
		t.Fatal(e)
	}
	for _, c := range f.networkConfigs {
		if c.Streams != 1 || c.TCPBitrate != "1" {
			t.Fatal("low-rate aggregate ceiling exceeded", c)
		}
	}
}
func TestPartialFailureInvalidResultsAndMAXAcknowledgment(t *testing.T) {
	o := Defaults("max")
	if _, e := Run(context.Background(), o, &fakeBackend{}, nil); e == nil {
		t.Fatal("MAX acknowledgment missing")
	}
	o.AckMax = true
	for _, invalid := range []bool{false, true} {
		f := &fakeBackend{fail: !invalid, invalid: invalid}
		r, e := Run(context.Background(), o, f, nil)
		if e == nil || r.States["CPU"] != "FAILED" || r.Status != "FAILED" {
			t.Fatal(r, e)
		}
		if !invalid && r.States["MEMORY"] != "COMPLETED" {
			t.Fatal("independent module did not continue", r)
		}
	}
}
func TestCleanupFailureStopsFurtherNetworkLoad(t *testing.T) {
	o := Defaults("quick")
	o.DiskPath = "/fixture"
	o.NetworkPeer = "10.0.0.20"
	f := &fakeBackend{cleanup: true}
	r, err := Run(context.Background(), o, f, nil)
	if err == nil || r.States["DISK"] != "FAILED" || r.States["NETWORK"] != "NOT RUN" || len(f.networkConfigs) != 0 || r.DiskCleaned {
		t.Fatal(r, err)
	}
}
func TestConfirmationRejectsSpikeAndUnstableMaximum(t *testing.T) {
	o := Defaults("max")
	fn := func(ctx context.Context, s Setting, seconds, warm int) (Sample, error) {
		v := float64(s.Workers) * 100
		if s.Workers == 4 {
			v = 10000
			if seconds > o.Policy().ExploreSeconds {
				v = 1
			}
		}
		return Sample{Value: v}, nil
	}
	e := newExplorer("fixture", "units", o, fn, nil)
	settings := []Setting{{Workers: 1}, {Workers: 2}, {Workers: 4}}
	if err := e.survey(context.Background(), settings, true); err != nil {
		t.Fatal(err)
	}
	p, err := e.finish(context.Background(), func(s Setting) bool { return s.Workers == 4 })
	if err != nil || p.Best.Setting.Workers != 2 || p.Boundary {
		t.Fatal(p, err)
	}
	count := 0
	e = newExplorer("unstable", "units", o, func(context.Context, Setting, int, int) (Sample, error) {
		count++
		return Sample{Value: float64(1 + count%2*100)}, nil
	}, nil)
	e.survey(context.Background(), settings, false)
	p, err = e.finish(context.Background(), func(Setting) bool { return false })
	if err != nil || p.Confirmed || p.Best != nil || !strings.Contains(strings.Join(p.Notes, " "), "could not be confirmed") {
		t.Fatal(p, err)
	}
}
func TestSaturationBoundaryInvalidMetricsAndDeadline(t *testing.T) {
	o := Defaults("max")
	e := newExplorer("plateau", "units", o, func(ctx context.Context, s Setting, seconds, warm int) (Sample, error) {
		return Sample{Value: 100, P99HighMS: float64(s.Depth)}, nil
	}, nil)
	e.survey(context.Background(), []Setting{{Depth: 1, Jobs: 1}, {Depth: 2, Jobs: 1}, {Depth: 4, Jobs: 1}, {Depth: 8, Jobs: 1}}, true)
	if len(e.peak.Trials) != 3 {
		t.Fatal(e.peak)
	}
	e = newExplorer("boundary", "units", o, func(ctx context.Context, s Setting, seconds, warm int) (Sample, error) {
		return Sample{Value: float64(s.Workers)}, nil
	}, nil)
	e.survey(context.Background(), []Setting{{Workers: 1}, {Workers: 2}}, false)
	p, err := e.finish(context.Background(), func(s Setting) bool { return s.Workers == 2 })
	if err != nil || !p.Boundary {
		t.Fatal(p, err)
	}
	_, err = evaluate(context.Background(), Setting{}, 1, 1, 0, .1, func(context.Context, Setting, int, int) (Sample, error) { return Sample{Value: math.Inf(1)}, nil })
	if err == nil {
		t.Fatal("invalid result accepted")
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = Run(ctx, Defaults("quick"), &fakeBackend{}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestScopedControllerRunsOnlySelectedComponent(t *testing.T) {
	for _, component := range []string{"cpu", "ram", "disk", "network"} {
		for _, mode := range []string{"quick", "standard", "max"} {
			t.Run(component+"/"+mode, func(t *testing.T) {
				o := Defaults(mode)
				o.Component = component
				o.AckMax = true
				// Even explicitly supplied unrelated endpoints must never run.
				o.DiskPath = "/fixture"
				o.NetworkPeer = "10.0.0.20"
				f := &fakeBackend{}
				r, err := Run(context.Background(), o, f, nil)
				if err != nil || r.Status != "COMPLETED" {
					t.Fatal(err, r.Status, r.Errors)
				}
				module := strings.ToUpper(component)
				if module == "RAM" {
					module = "MEMORY"
				}
				if len(r.States) != 1 || r.States[module] != "COMPLETED" {
					t.Fatal(r.States)
				}
				if len(f.events) == 0 {
					t.Fatal("selected workload did not execute")
				}
				for _, event := range f.events {
					if event != component && !(component == "disk" && event == "disk-open") {
						t.Fatalf("%s invoked %s", component, event)
					}
				}
				if component == "disk" {
					if f.opened != 1 || f.closed != 1 || !r.DiskCleaned {
						t.Fatal("disk cleanup not retained", r)
					}
				} else if f.opened != 0 || len(f.configs) != 0 {
					t.Fatal("unselected disk opened")
				}
				if component != "ram" {
					for _, w := range r.Warnings {
						if strings.Contains(w, "main memory bandwidth") || strings.Contains(w, "NUMA") {
							t.Fatal("unrelated RAM warning", w)
						}
					}
				}
			})
		}
	}
}
func TestScopedParsingTargetsAndSharedConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auto.yaml")
	if err := os.WriteFile(path, []byte("disk-path: /configured\nnetwork-peer: 10.0.0.30\ninclude-writes: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, component := range []string{"cpu", "ram", "network"} {
		args := []string{"--config", path}
		if component == "network" {
			args = append(args, "--target", "10.0.0.20")
		}
		o, err := ParseComponent(component, "quick", args)
		if err != nil || o.Component != component || o.DiskPath != "" || o.IncludeWrites || o.ConfirmWrites {
			t.Fatal(o, err)
		}
		if component == "network" && o.NetworkPeer != "10.0.0.20" || component != "network" && o.NetworkPeer != "" {
			t.Fatal(o)
		}
	}
	o, err := ParseComponent("disk", "max", []string{"--config", path, "--path", "/data with spaces", "--confirm-writes"})
	if err != nil || o.DiskPath != "/data with spaces" || o.NetworkPeer != "" || !o.IncludeWrites || !o.ConfirmWrites {
		t.Fatal(o, err)
	}
	if _, err := ParseComponent("disk", "quick", []string{"--config", path}); err == nil {
		t.Fatal("write acknowledgment bypassed")
	}
	if _, err := ParseComponent("full", "quick", nil); err == nil {
		t.Fatal("invalid scope accepted")
	}
	for _, component := range []string{"disk", "network"} {
		o := Defaults("quick")
		o.Component = component
		f := &fakeBackend{}
		if _, err := Run(context.Background(), o, f, nil); err == nil || len(f.events) > 0 {
			t.Fatal("missing target generated load", component, err, f.events)
		}
	}
}
func TestDiskPreflightDoesNotPrepareData(t *testing.T) {
	o := Defaults("max")
	o.Component = "disk"
	o.DiskPath = "/fixture"
	for _, free := range []uint64{1 << 30, 1<<30 + 32<<20, 16 << 30} {
		f := &fakeBackend{free: free}
		_, err := CheckDiskTarget(o, f)
		if (free < 2<<30) != (err != nil) {
			t.Fatal(free, err)
		}
		if len(f.events) != 0 || f.opened != 0 {
			t.Fatal("preflight wrote data", f.events)
		}
	}
}
func TestRemotePeerValidation(t *testing.T) {
	for _, peer := range []string{"", "example.com", "127.0.0.1", "::1", "0.0.0.0", "::", "224.0.0.1", "255.255.255.255"} {
		if err := ValidatePeer(peer); err == nil {
			t.Fatal("unsafe peer accepted", peer)
		}
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidatePeer(ip.String()); err == nil {
			t.Fatal("local interface accepted", ip)
		}
	}
}

func TestDiskEngineFailureDoesNotDisableIndependentModules(t *testing.T) {
	o := Defaults("quick")
	o.DiskPath = "/fixture"
	o.NetworkPeer = "10.0.0.20"
	f := &fakeBackend{engineFailure: true}
	r, err := Run(context.Background(), o, f, nil)
	if err == nil || r.States["DISK"] != "FAILED" || f.opened != 0 || len(r.Disk) != 0 {
		t.Fatal(r, err)
	}
	for _, module := range []string{"CPU", "MEMORY", "NETWORK"} {
		if r.States[module] != "COMPLETED" {
			t.Fatal(module, r.States)
		}
	}
	for _, engine := range []string{"io_uring", "libaio"} {
		f := &fakeBackend{selectedEngine: engine}
		o.Component = "disk"
		r, err := Run(context.Background(), o, f, nil)
		if err != nil || r.DiskInfo.Engine != engine {
			t.Fatal(r, err)
		}
		for _, c := range f.configs {
			if c.Engine != engine {
				t.Fatal("engines mixed within run", c)
			}
		}
	}
}

func TestAutomaticDiskEngineOptions(t *testing.T) {
	for _, engine := range []string{"auto", "io_uring", "libaio"} {
		o, err := ParseComponent("disk", "quick", []string{"--path", "/fixture", "--ioengine", engine})
		if err != nil || o.DiskEngine != engine {
			t.Fatal(o, err)
		}
	}
	if _, err := ParseComponent("disk", "quick", []string{"--ioengine", "sync"}); err == nil {
		t.Fatal("synchronous fallback accepted")
	}
}
