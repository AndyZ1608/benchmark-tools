package auto

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/config"
	"vmbench/internal/runner"
	"vmbench/internal/system"
)

type Result struct {
	Mode, Status               string
	Resources                  system.Info
	Plan                       Plan
	CPU, Memory, Disk, Network []Peak
	States                     map[string]string
	Warnings, Errors           []string
	DiskInfo                   DiskInfo
	DatasetBytes               uint64
	DiskCleaned                bool
	Duration                   time.Duration
	Limits                     Options
}

func Run(ctx context.Context, o Options, b Backend, progress func(string)) (out Result, err error) {
	out = Result{Mode: o.Mode, Limits: o, Status: "COMPLETED", States: map[string]string{}}
	for _, module := range []string{"CPU", "MEMORY", "DISK", "NETWORK"} {
		if o.Selects(module) {
			out.States[module] = "NOT RUN"
		}
	}
	if err = o.Validate(); err != nil {
		out.Status = "FAILED"
		return out, err
	}
	if o.Component == "disk" && o.DiskPath == "" || o.Component == "network" && o.NetworkPeer == "" {
		out.Status = "FAILED"
		return out, fmt.Errorf("%s requires an explicit target (--path or --target)", o.Component)
	}
	if o.Mode == "max" && !o.AckMax {
		out.Status = "FAILED"
		return out, fmt.Errorf("MAX requires explicit high-intensity acknowledgment (--ack-max)")
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, o.MaxDuration)
	defer cancel()
	defer func() {
		out.Duration = time.Since(started)
		if err != nil {
			out.Status = "FAILED"
		}
	}()
	out.Resources = b.Detect()
	out.Plan = MakePlan(o, out.Resources)
	if o.Selects("MEMORY") {
		out.Warnings = append(out.Warnings, out.Plan.Warnings...)
	} else {
		out.Warnings = append(out.Warnings, out.Resources.DetectionWarnings...)
	}
	if progress != nil {
		progress("[OK] System detected")
	}
	// A single synchronous loop prevents cross-module benchmark interference.
	for _, module := range []string{"CPU", "MEMORY", "DISK", "NETWORK"} {
		if !o.Selects(module) {
			continue
		}
		if e := ctx.Err(); e != nil {
			out.Errors = append(out.Errors, e.Error())
			err = errors.Join(err, e)
			break
		}
		if module == "MEMORY" && out.Plan.MemoryBytes == 0 {
			out.States[module] = "SKIPPED"
			continue
		}
		if module == "DISK" && o.DiskPath == "" {
			out.States[module] = "SKIPPED"
			out.Warnings = append(out.Warnings, "Disk skipped: no validated existing target configured")
			continue
		}
		if module == "NETWORK" && o.NetworkPeer == "" {
			out.States[module] = "SKIPPED"
			out.Warnings = append(out.Warnings, "Network skipped: no remote peer configured")
			continue
		}
		out.States[module] = "RUNNING"
		var e error
		switch module {
		case "CPU":
			out.CPU, e = runNative(ctx, o, out.Plan, "cpu", b, progress)
		case "MEMORY":
			out.Memory, e = runNative(ctx, o, out.Plan, "ram", b, progress)
		case "DISK":
			out.Disk, out.DiskInfo, out.DatasetBytes, out.DiskCleaned, e = runDisk(ctx, o, out.Plan, out.Resources, b, progress)
		case "NETWORK":
			out.Network, e = runNetwork(ctx, o, b, progress)
		}
		out.States[module] = "COMPLETED"
		var observations []Peak
		switch module {
		case "CPU":
			observations = out.CPU
		case "MEMORY":
			observations = out.Memory
		case "DISK":
			observations = out.Disk
		case "NETWORK":
			observations = out.Network
		}
		for _, peak := range observations {
			if peak.Best == nil {
				out.States[module] = "UNCONFIRMED"
				out.Warnings = append(out.Warnings, module+": Maximum performance could not be confirmed reliably.")
				if out.Status == "COMPLETED" {
					out.Status = "COMPLETED WITH LIMITATIONS"
				}
			}
		}
		if e != nil {
			out.States[module] = "FAILED"
			out.Errors = append(out.Errors, module+": "+e.Error())
			err = errors.Join(err, fmt.Errorf("%s: %w", module, e))
		}
		if progress != nil {
			progress("[" + out.States[module] + "] " + module + " benchmark")
		}
		// Incomplete filesystem cleanup is a safety stop; do not generate more load.
		if module == "DISK" && strings.Contains(fmt.Sprint(e), "cleanup") {
			break
		}
	}
	return out, err
}
func baseConfig(seconds, warm int) config.Config {
	c, _ := config.Defaults("quick")
	c.Duration = seconds
	c.Warmup = warm
	c.Repetitions = 1
	c.NonInteractive = true
	return c
}
func runNative(ctx context.Context, o Options, p Plan, kind string, b Backend, progress func(string)) ([]Peak, error) {
	names, units := []string{"integer-xorshift64", "fp-multiply-add"}, []string{"defined iterations/s", "defined iterations/s"}
	if kind == "ram" {
		names = []string{"triad"}
		units = []string{"GB/s"}
	}
	var results []Peak
	type nativeObservation struct {
		result cpu.NativeResult
		extras map[string]float64
	}
	// One native CPU invocation measures both Integer and FP. Reuse only the
	// exploratory observations across the two optimizers; confirmations are fresh.
	cache := map[string][]nativeObservation{}
	for index, name := range names {
		confirming := false
		cursor := map[string]int{}
		fn := func(ctx context.Context, s Setting, seconds, warm int) (Sample, error) {
			c := baseConfig(seconds, warm)
			c.Workers = s.Workers
			c.Size = "16777216"
			if kind == "ram" {
				c.Size = strconv.FormatInt(p.MemoryBytes, 10)
			}
			key := fmt.Sprintf("%d/%d/%d", s.Workers, seconds, warm)
			position := cursor[key]
			cursor[key]++
			var raw cpu.NativeResult
			var extras map[string]float64
			var err error
			if kind == "cpu" && !confirming && position < len(cache[key]) {
				raw = cache[key][position].result
				extras = cache[key][position].extras
			} else {
				raw, extras, err = b.Native(ctx, kind, c)
				if err == nil && kind == "cpu" && !confirming {
					cache[key] = append(cache[key], nativeObservation{raw, extras})
				}
			}
			if err != nil {
				return Sample{}, err
			}
			if extras == nil {
				extras = map[string]float64{}
			}
			sample := Sample{Extras: extras, Raw: raw}
			expected := map[string]bool{}
			for _, m := range raw.Measurements {
				if m.Errors != 0 || m.Elapsed <= 0 || !finite(m.Value) || m.Value <= 0 {
					return sample, fmt.Errorf("native integrity or metric validation failed")
				}
				expected[m.Workload] = true
				if m.Workload == name {
					sample.Value = m.Value
				}
				if m.Workload == "dependent-pointer-chase" {
					sample.LatencyNS = m.Value
				}
				if m.Workload == "copy" || m.Workload == "scale" || m.Workload == "add" {
					sample.Extras[m.Workload+" GB/s"] = m.Value
				}
			}
			required := []string{"integer-xorshift64", "fp-multiply-add"}
			if kind == "ram" {
				required = []string{"copy", "scale", "add", "triad", "dependent-pointer-chase"}
			}
			for _, key := range required {
				if !expected[key] {
					return sample, fmt.Errorf("native result missing %s", key)
				}
			}
			return sample, nil
		}
		e := newExplorer(name, units[index], o, fn, progress)
		workers := concurrency(p.Workers)
		if o.Mode != "max" {
			workers = []int{1}
			if p.Workers > 1 {
				workers = append(workers, p.Workers)
			}
			if kind == "ram" && o.Mode == "quick" {
				workers = []int{p.Workers}
			}
		}
		settings := []Setting{}
		for _, n := range workers {
			settings = append(settings, Setting{Workers: n})
		}
		if err := e.survey(ctx, settings, o.Mode == "max"); err != nil {
			return results, err
		}
		if o.Mode == "max" {
			if best := e.promising(); best != nil {
				w := best.Setting.Workers
				refine := []Setting{}
				for _, n := range []int{max(1, w-w/4), min(p.Workers, w+max(1, w/4))} {
					refine = append(refine, Setting{Workers: n})
				}
				if err := e.survey(ctx, refine, false); err != nil {
					return results, err
				}
			}
		}
		confirming = true
		peak, err := e.finish(ctx, func(s Setting) bool { return s.Workers == p.Workers })
		results = append(results, peak)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}
func diskSize(o Options, p Plan, s system.Info, info DiskInfo, jobs int) (int64, error) {
	cap, _ := config.Bytes(o.MaxDisk)
	reserve, _ := config.Bytes(o.MinFree)
	if info.Free <= uint64(reserve)+(1<<20) {
		return 0, fmt.Errorf("insufficient storage after minimum free reserve")
	}
	budget := min(uint64(cap), (info.Free-uint64(reserve)-(1<<20))/4)
	if o.Mode == "quick" {
		budget = min(budget, 128<<20)
	} else if s.AvailableMemory > 0 {
		budget = min(budget, uint64(s.AvailableMemory))
	}
	perJob := int64(budget/uint64(jobs)) / (1 << 20) * (1 << 20)
	if perJob < 16<<20 {
		return 0, fmt.Errorf("insufficient storage for safe 16MiB job datasets")
	}
	return perJob, nil
}

// CheckDiskTarget only inspects metadata and capacity. Execution revalidates
// the target and reserves space through the existing secure disk session.
func CheckDiskTarget(o Options, b Backend) (DiskInfo, error) {
	if err := o.Validate(); err != nil {
		return DiskInfo{}, err
	}
	info, err := b.Filesystem(o.DiskPath)
	if err != nil {
		return info, err
	}
	s := b.Detect()
	_, err = diskSize(o, MakePlan(o, s), s, info, 1)
	return info, err
}
func runDisk(ctx context.Context, o Options, p Plan, resources system.Info, b Backend, progress func(string)) (peaks []Peak, info DiskInfo, total uint64, cleaned bool, err error) {
	info, err = b.Filesystem(o.DiskPath)
	if err != nil {
		return
	}
	jobs := min(o.MaxJobs, p.Workers, o.MaxOutstanding)
	if o.Mode == "quick" {
		jobs = 1
	}
	size, e := diskSize(o, p, resources, info, jobs)
	for e != nil && jobs > 1 {
		jobs = max(1, jobs/2)
		size, e = diskSize(o, p, resources, info, jobs)
	}
	if e != nil {
		err = e
		return
	}
	total = uint64(size) * uint64(jobs)
	info.PreparedJobs = jobs
	info.FileBytes = uint64(size)
	c := baseConfig(1, 0)
	c.Path = o.DiskPath
	c.Jobs = jobs
	c.FileSize = strconv.FormatInt(size, 10)
	c.DiskSizeExplicit = true
	c.Engine = o.DiskEngine
	c.FIODiagnostics = o.FIODiagnostics
	c.ConfirmWrites = o.ConfirmWrites
	reserve, _ := config.Bytes(o.MinFree)
	if progress != nil {
		ctx = runner.WithObserver(ctx, func(event runner.Event) {
			if event.Phase == "capability" {
				progress(event.Message)
			}
		})
	}
	engine, e := b.SelectDiskEngine(ctx, c, uint64(reserve))
	if e != nil {
		err = e
		return
	}
	c.Engine = engine
	info.Engine = engine
	if progress != nil {
		progress(fmt.Sprintf("[DISK] Preparing %d bytes across %d private job files (read preparation writes data)", total, jobs))
	}
	session, e := b.OpenDisk(ctx, c, uint64(reserve))
	if e != nil {
		err = e
		return
	}
	if progress != nil {
		progress("[DISK] Prepared dataset; finding observed IOPS and sequential throughput")
	}
	info.Prepared = true
	defer func() {
		cleanup := session.Close()
		cleaned = cleanup == nil
		if cleanup != nil {
			err = errors.Join(err, fmt.Errorf("disk cleanup incomplete: %w", cleanup))
		} else if progress != nil {
			progress("[OK] Temporary files removed")
		}
	}()
	profiles := []string{"randread-4k", "seqread-1m"}
	if o.IncludeWrites {
		profiles = append(profiles, "randwrite-4k", "seqwrite-1m", "mixed-70-30")
	}
	for _, profile := range profiles {
		unit := "IOPS"
		if strings.Contains(profile, "1m") {
			unit = "MiB/s"
		}
		fn := func(ctx context.Context, s Setting, seconds, warm int) (Sample, error) {
			current := c
			current.Profile = profile
			current.Depth = s.Depth
			current.Jobs = s.Jobs
			current.Duration = seconds
			current.Warmup = warm
			outcome, e := session.Measure(ctx, current)
			if e != nil {
				return Sample{}, e
			}
			if len(outcome.Results) != 1 {
				return Sample{}, fmt.Errorf("expected one fio measurement")
			}
			r := outcome.Results[0]
			evidence, e := disk.ExecutionEvidence(r, engine, s.Depth)
			if e != nil {
				return Sample{}, e
			}
			if o.Details {
				for _, observation := range r.Accounting {
					if observation.ByteDifference != "0" {
						evidence += fmt.Sprintf("; %s %s io_bytes=%d total_ios=%d difference=%s B (%s)", observation.Job, observation.Direction, observation.IOBytes, observation.TotalIOs, observation.ByteDifference, observation.Explanation)
					}
				}
			}
			sample := Sample{Raw: r, Evidence: evidence, P99LowMS: math.Inf(1), MeanLowMS: math.Inf(1), Extras: map[string]float64{}}
			for _, job := range r.Jobs {
				directions := []disk.Direction{job.Read}
				if strings.Contains(profile, "write") {
					directions = []disk.Direction{job.Write}
				}
				if profile == "mixed-70-30" {
					directions = append(directions, job.Write)
				}
				for directionIndex, v := range directions {
					if v.IOPS <= 0 || v.BWBytes <= 0 || v.Runtime <= 0 || v.Completion.Mean <= 0 {
						return sample, fmt.Errorf("invalid active fio metrics")
					}
					sample.Value += v.IOPS
					label := "Read"
					if strings.Contains(profile, "write") || directionIndex == 1 {
						label = "Write"
					}
					sample.Extras[label+" IOPS"] += v.IOPS
					sample.Extras[label+" MiB/s"] += v.BWBytes / (1 << 20)
					sample.MiBps += v.BWBytes / (1 << 20)
					sample.MeanMS = max(sample.MeanMS, v.Completion.Mean/1e6)
					sample.MeanLowMS = min(sample.MeanLowMS, v.Completion.Mean/1e6)
					found := false
					for key, n := range v.Completion.Percentile {
						percent, _ := strconv.ParseFloat(key, 64)
						if percent == 99 && n > 0 && finite(n) {
							found = true
							sample.P99LowMS = min(sample.P99LowMS, n/1e6)
							sample.P99HighMS = max(sample.P99HighMS, n/1e6)
						}
					}
					if !found {
						return sample, fmt.Errorf("fio p99 completion latency missing")
					}
				}
			}
			if unit == "MiB/s" {
				sample.Value = sample.MiBps
			}
			return sample, nil
		}
		ex := newExplorer(profile, unit, o, fn, progress)
		depthLimit := min(o.MaxDepth, o.MaxOutstanding)
		depths := concurrency(depthLimit)
		if o.Mode == "quick" {
			depths = []int{min(8, depthLimit)}
		}
		if o.Mode == "standard" {
			depths = []int{1, min(8, depthLimit), min(32, depthLimit)}
		}
		initial := []Setting{}
		for _, depth := range depths {
			initial = append(initial, Setting{Depth: depth, Jobs: 1})
		}
		// Complete the bounded Disk depth sweep before confirming a MAX result.
		if err = ex.survey(ctx, initial, false); err != nil {
			return
		}
		if o.Mode == "max" {
			if best := ex.promising(); best != nil {
				additional := []Setting{}
				for _, n := range concurrency(jobs) {
					additional = append(additional, Setting{Depth: min(best.Setting.Depth, o.MaxOutstanding/n), Jobs: n})
				}
				if err = ex.survey(ctx, additional, true); err != nil {
					return
				}
				best = ex.promising()
				refined := []Setting{}
				for _, depth := range []int{max(1, best.Setting.Depth/2), min(depthLimit, best.Setting.Depth*2)} {
					if depth*best.Setting.Jobs <= o.MaxOutstanding {
						refined = append(refined, Setting{Depth: depth, Jobs: best.Setting.Jobs})
					}
				}
				if err = ex.survey(ctx, refined, false); err != nil {
					return
				}
			}
		}
		peak, e := ex.finish(ctx, func(s Setting) bool {
			return s.Depth == depthLimit || s.Jobs == jobs || s.Depth*s.Jobs >= o.MaxOutstanding
		})
		if e != nil {
			err = e
			return
		}
		if total < uint64(max(int64(0), resources.AvailableMemory)) {
			peak.Notes = append(peak.Notes, "Dataset is smaller than available RAM; backend/cache effects remain possible despite direct I/O")
		}
		peaks = append(peaks, peak)
	}
	return
}
func runNetwork(ctx context.Context, o Options, b Backend, progress func(string)) ([]Peak, error) {
	directions := []bool{false}
	if o.Mode != "quick" {
		directions = append(directions, true)
	}
	var peaks []Peak
	bitrate, _ := Bitrate(o.NetworkBitrate)
	streamLimit := int(min(int64(o.MaxStreams), bitrate)) // At least one offered bit/s per stream.
	for _, reverse := range directions {
		name := "TCP TX"
		if reverse {
			name = "TCP RX"
		}
		fn := func(ctx context.Context, s Setting, seconds, warm int) (Sample, error) {
			c := baseConfig(seconds, warm)
			c.Target = o.NetworkPeer
			c.Port = o.Port
			c.Reverse = reverse
			c.Streams = s.Streams
			c.TCPBitrate = strconv.FormatInt(max(int64(1), bitrate/int64(s.Streams)), 10)
			result, err := b.Network(ctx, c)
			if err != nil {
				return Sample{}, err
			}
			return networkSample(result)
		}
		ex := newExplorer(name, "Gbps", o, fn, progress)
		streams := []int{1}
		if o.Mode == "standard" {
			streams = concurrency(min(4, streamLimit))
		}
		if o.Mode == "max" {
			streams = concurrency(streamLimit)
		}
		settings := []Setting{}
		for _, n := range streams {
			settings = append(settings, Setting{Streams: n})
		}
		if err := ex.survey(ctx, settings, o.Mode == "max"); err != nil {
			return peaks, err
		}
		peak, err := ex.finish(ctx, func(s Setting) bool { return s.Streams == streamLimit })
		if err != nil {
			return peaks, err
		}
		peak.Notes = append(peak.Notes, "TCP traffic paced to at most "+o.NetworkBitrate+" aggregate offered bits/s; no UDP or RTT test")
		peaks = append(peaks, peak)
	}
	return peaks, nil
}
