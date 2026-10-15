package auto

import (
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/safety"
	"vmbench/internal/system"
)

type Options struct {
	Mode, DiskPath, NetworkPeer, ConfigFile                                        string
	NonInteractive, AckMax, IncludeWrites, ConfirmWrites, Details                  bool
	MaxWorkers, MaxDepth, MaxJobs, MaxOutstanding, MaxStreams, SampleSeconds, Port int
	MaxRAM, MaxDisk, MinFree, NetworkBitrate                                       string
	MaxDuration                                                                    time.Duration
	MaxCV, Improvement                                                             float64
}

func Defaults(mode string) Options {
	return Options{Mode: mode, MaxWorkers: 32, MaxDepth: 64, MaxJobs: 4, MaxOutstanding: 128, MaxStreams: 8, MaxRAM: "2GiB", MaxDisk: "2GiB", MinFree: "1GiB", MaxDuration: 30 * time.Minute, MaxCV: .10, Improvement: .05, Port: 5201, NetworkBitrate: "1G"}
}
func Parse(mode string, args []string) (Options, error) {
	o := Defaults(mode)
	if mode != "quick" && mode != "standard" && mode != "max" {
		return o, fmt.Errorf("unknown automatic mode %q", mode)
	}
	// An optional local flat configuration is data, never executable shell input.
	local := ".vmbench/auto.yaml"
	if _, e := os.Stat(local); e == nil {
		o.ConfigFile = local
	}
	for i, a := range args {
		if a == "--config" && i+1 < len(args) {
			o.ConfigFile = args[i+1]
		}
		if strings.HasPrefix(a, "--config=") {
			o.ConfigFile = strings.TrimPrefix(a, "--config=")
		}
	}
	var fromFile []string
	if o.ConfigFile != "" {
		data, e := os.ReadFile(o.ConfigFile)
		if e != nil {
			return o, e
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
			if line == "" {
				continue
			}
			kv := strings.SplitN(line, ":", 2)
			if len(kv) != 2 {
				return o, fmt.Errorf("automatic configuration requires flat key: value pairs")
			}
			key := strings.TrimSpace(kv[0])
			if key == "ack-max" || key == "confirm-writes" {
				return o, fmt.Errorf("%s must be acknowledged for each invocation, not saved in configuration", key)
			}
			fromFile = append(fromFile, "--"+key+"="+strings.Trim(strings.TrimSpace(kv[1]), "\"'"))
		}
	}
	f := flag.NewFlagSet(mode, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	for _, v := range []struct {
		name string
		p    *string
	}{{"disk-path", &o.DiskPath}, {"network-peer", &o.NetworkPeer}, {"config", &o.ConfigFile}, {"max-ram", &o.MaxRAM}, {"max-disk-data", &o.MaxDisk}, {"min-free-disk", &o.MinFree}, {"network-bitrate", &o.NetworkBitrate}} {
		f.StringVar(v.p, v.name, *v.p, v.name)
	}
	for _, v := range []struct {
		name string
		p    *int
	}{{"max-workers", &o.MaxWorkers}, {"max-disk-depth", &o.MaxDepth}, {"max-disk-jobs", &o.MaxJobs}, {"max-outstanding-io", &o.MaxOutstanding}, {"max-network-streams", &o.MaxStreams}, {"sample-seconds", &o.SampleSeconds}, {"network-port", &o.Port}} {
		f.IntVar(v.p, v.name, *v.p, v.name)
	}
	for _, v := range []struct {
		name string
		p    *bool
	}{{"non-interactive", &o.NonInteractive}, {"ack-max", &o.AckMax}, {"include-writes", &o.IncludeWrites}, {"confirm-writes", &o.ConfirmWrites}, {"details", &o.Details}} {
		f.BoolVar(v.p, v.name, *v.p, v.name)
	}
	f.DurationVar(&o.MaxDuration, "max-duration", o.MaxDuration, "complete run time ceiling")
	f.Float64Var(&o.MaxCV, "max-cv", o.MaxCV, "maximum confirmation CV, fraction")
	f.Float64Var(&o.Improvement, "saturation-improvement", o.Improvement, "heuristic relative improvement, fraction")
	if e := f.Parse(append(fromFile, args...)); e != nil {
		return o, e
	}
	if len(f.Args()) > 0 {
		return o, fmt.Errorf("unexpected automatic mode arguments: %v", f.Args())
	}
	return o, o.Validate()
}
func (o Options) Validate() error {
	if o.Mode != "quick" && o.Mode != "standard" && o.Mode != "max" {
		return fmt.Errorf("unknown automatic mode %q", o.Mode)
	}
	if o.MaxWorkers < 1 || o.MaxWorkers > 128 || o.MaxDepth < 1 || o.MaxDepth > 64 || o.MaxJobs < 1 || o.MaxJobs > 8 || o.MaxOutstanding < 1 || o.MaxOutstanding > 512 || o.MaxStreams < 1 || o.MaxStreams > 16 || o.SampleSeconds < 0 || o.SampleSeconds > 30 || o.Port < 1 || o.Port > 65535 || o.MaxDuration < time.Second || o.MaxDuration > 24*time.Hour {
		return fmt.Errorf("automatic safety limit outside supported bounds")
	}
	for _, s := range []string{o.MaxRAM, o.MaxDisk, o.MinFree} {
		if _, e := config.Bytes(s); e != nil {
			return e
		}
	}
	free, _ := config.Bytes(o.MinFree)
	if uint64(free) < safety.ReserveBytes {
		return fmt.Errorf("minimum free disk space cannot be below 1GiB")
	}
	if !finite(o.MaxCV) || o.MaxCV <= 0 || o.MaxCV > .5 || !finite(o.Improvement) || o.Improvement <= 0 || o.Improvement > .5 {
		return fmt.Errorf("CV and improvement thresholds must be finite fractions in (0, 0.5]")
	}
	if _, e := Bitrate(o.NetworkBitrate); e != nil {
		return e
	}
	if o.NetworkPeer != "" {
		ip := net.ParseIP(o.NetworkPeer)
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() || ip.Equal(net.IPv4bcast) {
			return fmt.Errorf("network peer must be an explicit trusted unicast remote IP; loopback is not network performance")
		}
	}
	if o.IncludeWrites && !o.ConfirmWrites {
		return fmt.Errorf("--include-writes requires separate --confirm-writes acknowledgment")
	}
	return nil
}
func Bitrate(s string) (int64, error) {
	multiplier := int64(1)
	if len(s) > 0 {
		switch s[len(s)-1] {
		case 'K', 'k':
			multiplier = 1000
			s = s[:len(s)-1]
		case 'M', 'm':
			multiplier = 1000000
			s = s[:len(s)-1]
		case 'G', 'g':
			multiplier = 1000000000
			s = s[:len(s)-1]
		}
	}
	v, e := strconv.ParseInt(s, 10, 64)
	if e != nil || v < 1 || v > 100000000000/multiplier {
		return 0, fmt.Errorf("network bitrate must be positive and at most 100G")
	}
	return v * multiplier, nil
}

type Policy struct{ ExploreSeconds, ConfirmSeconds, Warmup, ExploreReps, ConfirmReps int }

func (o Options) Policy() Policy {
	p := Policy{1, 1, 0, 1, 1}
	if o.Mode == "standard" {
		p = Policy{3, 3, 1, 3, 3}
	}
	if o.Mode == "max" {
		p = Policy{2, 5, 1, 2, 3}
	}
	if o.SampleSeconds > 0 {
		p.ExploreSeconds = o.SampleSeconds
		p.ConfirmSeconds = o.SampleSeconds
		if o.Mode == "max" {
			p.ConfirmSeconds = max(o.SampleSeconds+1, 2)
		}
	}
	return p
}

type Plan struct {
	Workers     int
	MemoryBytes int64
	Warnings    []string
}

func MakePlan(o Options, s system.Info) Plan {
	p := Plan{Workers: min(o.MaxWorkers, max(1, int(math.Ceil(s.Capacity)))), Warnings: append([]string{}, s.DetectionWarnings...)}
	maxRAM, _ := config.Bytes(o.MaxRAM)
	available := max(int64(0), s.AvailableMemory-(256<<20))
	p.MemoryBytes = min(available/4, maxRAM*3/4)
	if o.Mode == "quick" {
		p.MemoryBytes = min(p.MemoryBytes, 256<<20)
	}
	if s.SwapIn > 0 || s.SwapOut > 0 || pressureActive(s.MemoryPressure) {
		p.MemoryBytes /= 2
		p.Warnings = append(p.Warnings, "Swap history or memory pressure detected; RAM allocation reduced. History alone does not establish current swapping.")
	}
	p.MemoryBytes = p.MemoryBytes / (1 << 20) * (1 << 20)
	if p.MemoryBytes < 16<<20 {
		p.MemoryBytes = 0
		p.Warnings = append(p.Warnings, "Insufficient conservative RAM headroom; memory benchmark skipped")
	}
	if s.LastLevelCache == 0 || p.MemoryBytes/4 <= s.LastLevelCache {
		p.Warnings = append(p.Warnings, "Working set cannot be established as substantially larger than aggregate cache; main memory bandwidth is unconfirmed")
	}
	if s.NUMANodes > 1 {
		p.Warnings = append(p.Warnings, "Multiple NUMA nodes detected; native parent-thread initialization is used without forced binding. Cross-node bandwidth is not established")
	}
	return p
}
func pressureActive(s string) bool {
	for _, word := range strings.Fields(s) {
		if strings.HasPrefix(word, "avg10=") {
			var n float64
			fmt.Sscanf(word, "avg10=%f", &n)
			if n > 1 {
				return true
			}
		}
	}
	return false
}
func concurrency(limit int) []int {
	out := []int{1}
	for n := 2; n < limit; n *= 2 {
		out = append(out, n)
	}
	if limit > 1 {
		out = append(out, limit)
	}
	return out
}
