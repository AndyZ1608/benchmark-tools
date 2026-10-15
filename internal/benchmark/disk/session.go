package disk

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/runner"
	"vmbench/internal/safety"
)

// Session retains one verified private dataset throughout a bounded parameter search.
// Only the first Jobs owned descriptors are exposed to each fio invocation.
type Session struct {
	target    *safety.Target
	Lifecycle Lifecycle
	closed    bool
}

func Engines(ctx context.Context) ([]string, error) {
	version, err := runner.Run(ctx, "fio", []string{"--version"}, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("fio preflight: %w", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(version)), "fio-") {
		return nil, fmt.Errorf("bundled engine does not identify as fio")
	}
	data, err := runner.Run(ctx, "fio", []string{"--enghelp"}, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("fio engine detection: %w", err)
	}
	var names []string
	for _, name := range strings.Fields(string(data)) {
		if name == "io_uring" || name == "libaio" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("fio has no supported asynchronous engine")
	}
	return names, nil
}
func OpenSession(ctx context.Context, c config.Config, reserve uint64) (_ *Session, err error) {
	if err = c.Validate(); err != nil {
		return nil, err
	}
	if !c.Direct {
		return nil, fmt.Errorf("automatic disk discovery requires direct I/O")
	}
	metadata, err := safety.ValidateDirectory(c.Path)
	if err != nil {
		return nil, err
	}
	engines, err := Engines(ctx)
	if err != nil {
		return nil, err
	}
	supported := false
	for _, name := range engines {
		if name == c.Engine {
			supported = true
		}
	}
	if !supported {
		return nil, fmt.Errorf("fio does not support selected engine %q", c.Engine)
	}
	size, err := config.Bytes(c.FileSize)
	if err != nil {
		return nil, err
	}
	seed, err := seedValue(c.DataSeed)
	if err != nil {
		return nil, err
	}
	target, err := safety.PrepareReserved(c.Path, "", size, c.Jobs, false, reserve)
	if err != nil {
		return nil, err
	}
	s := &Session{target: target, Lifecycle: Lifecycle{Target: metadata["path"].(string), Workspace: target.Dir, Engine: c.Engine, FileBytes: target.Plan.FileBytes, TotalBytes: target.Plan.TotalBytes, Jobs: c.Jobs, Seed: seed}}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.Close())
		}
	}()
	runner.Notice(ctx, "workspace", "[DISK] Private dataset created; read tests also write preparation data")
	started := time.Now()
	err = target.Fill(ctx, seed, nil)
	s.Lifecycle.PreparationSeconds = time.Since(started).Seconds()
	if err != nil {
		return nil, err
	}
	s.Lifecycle.Prepared = true
	runner.Notice(ctx, "prepared", fmt.Sprintf("[DISK] %d job files prepared and verified (%d bytes total); preparation excluded from measurements", c.Jobs, target.Plan.TotalBytes))
	return s, nil
}
func (s *Session) Measure(ctx context.Context, c config.Config) (Outcome, error) {
	if s.closed {
		return Outcome{}, fmt.Errorf("disk session is closed")
	}
	if c.Engine != s.Lifecycle.Engine {
		return Outcome{}, fmt.Errorf("selected I/O engine changed within the prepared session; refusing mixed-engine results")
	}
	if err := c.Validate(); err != nil {
		return Outcome{}, err
	}
	if (strings.Contains(c.Profile, "write") || c.Profile == "mixed-70-30") && !c.ConfirmWrites {
		return Outcome{}, fmt.Errorf("write workload requires separate confirmation")
	}
	out, err := measurePrepared(ctx, c, s.target)
	out.Lifecycle = s.Lifecycle
	return out, err
}
func (s *Session) Close() error {
	s.closed = true
	err := s.target.Close()
	s.Lifecycle.CleanupCompleted = err == nil
	return err
}

// ExecutionEvidence checks the engine recorded by fio and summarizes its observed
// depth buckets. Bucket labels are ranges, not exact achieved mean queue depths.
func ExecutionEvidence(result Result, engine string, requested int) (string, error) {
	var descriptions []string
	if requested < 1 {
		return "", fmt.Errorf("invalid requested I/O depth")
	}
	for i, job := range result.Jobs {
		if job.Error != 0 {
			return "", fmt.Errorf("job %d returned fio I/O error %d", i+1, job.Error)
		}
		active := false
		for _, v := range []Direction{job.Read, job.Write} {
			if math.IsNaN(v.IOPS) || math.IsInf(v.IOPS, 0) || v.IOPS < 0 {
				return "", fmt.Errorf("invalid fio IOPS")
			}
			if v.IOPS > 0 {
				active = true
				if v.TotalIOs == 0 || v.IOBytes == 0 || v.ShortIOs != 0 || v.DropIOs != 0 {
					return "", fmt.Errorf("job %d missing valid completed I/O counters", i+1)
				}
			}
		}
		if !active {
			return "", fmt.Errorf("job %d completed no measured I/O", i+1)
		}
		actual := job.Options["ioengine"]
		if actual == "" {
			actual = result.Options["ioengine"]
		}
		if actual != engine || (actual != "io_uring" && actual != "libaio") {
			return "", fmt.Errorf("job %d effective fio engine %q differs from selected %q", i+1, actual, engine)
		}
		total, dominant, best, meaningful := 0.0, "", -1.0, 0.0
		for label, pct := range job.Depth {
			if pct < 0 || pct > 100 || math.IsNaN(pct) || math.IsInf(pct, 0) {
				return "", fmt.Errorf("invalid achieved I/O depth distribution")
			}
			bucket, e := strconv.Atoi(strings.TrimPrefix(label, ">="))
			if e != nil || !(label == "1" || label == "2" || label == "4" || label == "8" || label == "16" || label == "32" || label == ">=64") {
				return "", fmt.Errorf("unknown fio depth bucket %q", label)
			}
			threshold := 2
			for threshold*2 <= requested/2 {
				threshold *= 2
			}
			if bucket >= threshold {
				meaningful += pct
			}
			total += pct
			if pct > best {
				best = pct
				dominant = label
			}
		}
		if total < 99 || total > 101 {
			return "", fmt.Errorf("missing or incomplete achieved I/O depth distribution")
		}
		descriptions = append(descriptions, fmt.Sprintf("job %d: bucket %s %.1f%%", i+1, dominant, best))
		if requested > 1 && meaningful < 50 {
			return "", fmt.Errorf("measurement limitation: %s requested QD%d but job %d achieved insufficient asynchronous depth (%.1f%% near requested range, dominant bucket %s %.1f%%); refusing misleading results", engine, requested, i+1, meaningful, dominant, best)
		}
	}
	if len(descriptions) == 0 {
		return "", fmt.Errorf("no fio jobs for achieved-depth validation")
	}
	return fmt.Sprintf("%s; requested QD%d; %s", engine, requested, strings.Join(descriptions, "; ")), nil
}
