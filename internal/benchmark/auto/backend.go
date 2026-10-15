package auto

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/benchmark/network"
	"vmbench/internal/config"
	"vmbench/internal/runner"
	"vmbench/internal/safety"
	"vmbench/internal/system"
)

type DiskInfo struct {
	Path, Filesystem, Mount, Engine string
	Free                            uint64
	System                          bool
	PreparedJobs                    int
	FileBytes                       uint64
	Prepared                        bool
}
type DiskSession interface {
	Measure(context.Context, config.Config) (disk.Outcome, error)
	Close() error
}
type Backend interface {
	Detect() system.Info
	Native(context.Context, string, config.Config) (cpu.NativeResult, map[string]float64, error)
	Filesystem(string) (DiskInfo, error)
	SelectDiskEngine(context.Context, config.Config, uint64) (string, error)
	OpenDisk(context.Context, config.Config, uint64) (DiskSession, error)
	Network(context.Context, config.Config) (network.Result, error)
}
type RealBackend struct{}

func (RealBackend) Detect() system.Info { return system.Detect() }
func (RealBackend) Native(ctx context.Context, kind string, c config.Config) (cpu.NativeResult, map[string]float64, error) {
	var memoryBefore system.Info
	if kind == "ram" {
		size, e := config.Bytes(c.Size)
		if e != nil {
			return cpu.NativeResult{}, nil, e
		}
		memoryBefore = system.Detect()
		available := memoryBefore.AvailableMemory
		if available <= 0 || size > available/3 {
			return cpu.NativeResult{}, nil, fmt.Errorf("RAM headroom changed; refusing allocation above one third of current available memory")
		}
	}
	before := system.Counters()
	b, err := runner.Run(ctx, "compute-bench", []string{kind, strconv.Itoa(c.Workers), strconv.Itoa(c.Duration), strconv.Itoa(c.Warmup), c.Size}, time.Duration(6*(c.Duration+c.Warmup)+30)*time.Second)
	after := system.Counters()
	if err != nil {
		return cpu.NativeResult{}, nil, err
	}
	r, err := cpu.Parse(b)
	if err != nil {
		return r, nil, err
	}
	if r.Workers != c.Workers {
		return r, nil, fmt.Errorf("native engine returned unexpected worker count")
	}
	extra := map[string]float64{}
	if kind == "ram" {
		after := system.Detect()
		if after.SwapIn >= memoryBefore.SwapIn {
			extra["Swap pages in"] = float64(after.SwapIn - memoryBefore.SwapIn)
		}
		if after.SwapOut >= memoryBefore.SwapOut {
			extra["Swap pages out"] = float64(after.SwapOut - memoryBefore.SwapOut)
		}
	}
	if steal, ok := system.Delta(before, after)["steal_percent"].(float64); ok {
		extra["CPU steal percent"] = steal
	}
	count := func(s string) float64 {
		n := 0.0
		for _, line := range strings.Split(s, "\n") {
			v := strings.Fields(line)
			if len(v) == 2 && v[0] == "nr_throttled" {
				value, _ := strconv.ParseFloat(v[1], 64)
				n += value
			}
		}
		return n
	}
	if before.Throttling != "" && after.Throttling != "" {
		extra["Throttled periods"] = max(0, count(after.Throttling)-count(before.Throttling))
	}
	return r, extra, nil
}
func (RealBackend) Filesystem(path string) (DiskInfo, error) {
	m, err := safety.ValidateDirectory(path)
	if err != nil {
		return DiskInfo{}, err
	}
	r := DiskInfo{Path: m["path"].(string), Free: m["available_bytes"].(uint64), System: m["system_filesystem"].(bool), Filesystem: "unknown", Mount: "unknown"}
	longest := 0
	for _, line := range strings.Split(system.Read("/proc/self/mountinfo"), "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		a, b := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(a) < 5 || len(b) < 1 {
			continue
		}
		mount := strings.NewReplacer(`\040`, " ", `\134`, `\`).Replace(a[4])
		if (r.Path == mount || strings.HasPrefix(r.Path, strings.TrimSuffix(mount, "/")+"/")) && len(mount) > longest {
			r.Mount = filepath.Clean(mount)
			r.Filesystem = b[0]
			longest = len(mount)
		}
	}
	return r, nil
}
func (RealBackend) SelectDiskEngine(ctx context.Context, c config.Config, reserve uint64) (string, error) {
	return disk.SelectEngine(ctx, c, reserve)
}

func (RealBackend) OpenDisk(ctx context.Context, c config.Config, reserve uint64) (DiskSession, error) {
	return disk.OpenSession(ctx, c, reserve)
}
func (RealBackend) Network(ctx context.Context, c config.Config) (network.Result, error) {
	if err := ValidatePeer(c.Target); err != nil {
		return network.Result{}, err
	}
	return network.Run(ctx, c)
}

// ValidatePeer performs no traffic and is shared by input validation and execution.
func ValidatePeer(target string) error {
	o := Defaults("quick")
	o.NetworkPeer = target
	if target == "" {
		return fmt.Errorf("remote server IP is required")
	}
	if err := o.Validate(); err != nil {
		return err
	}
	// An explicit remote address is the trust boundary; reject this guest's own addresses.
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return fmt.Errorf("cannot verify remote peer against local interfaces: %w", err)
	}
	for _, a := range addresses {
		ip, _, _ := net.ParseCIDR(a.String())
		if ip != nil && ip.Equal(net.ParseIP(target)) {
			return fmt.Errorf("configured peer resolves to a local interface; remote peer required")
		}
	}
	return nil
}
func networkSample(result network.Result) (Sample, error) {
	if result.Error != "" {
		return Sample{}, fmt.Errorf("iperf3: %s", result.Error)
	}
	var metrics map[string]json.RawMessage
	if err := json.Unmarshal(result.End, &metrics); err != nil {
		return Sample{}, err
	}
	var v struct {
		BPS     float64 `json:"bits_per_second"`
		Seconds float64 `json:"seconds"`
	}
	if err := json.Unmarshal(metrics["sum_received"], &v); err != nil || v.BPS <= 0 || v.Seconds <= 0 || !finite(v.Seconds) {
		return Sample{}, fmt.Errorf("iperf3 receiver throughput or runtime missing/invalid")
	}
	return Sample{Value: v.BPS / 1e9, Raw: result}, nil
}
