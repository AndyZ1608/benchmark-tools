package disk

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
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

type Lifecycle struct {
	Target, Workspace          string
	FileBytes, TotalBytes      uint64
	Jobs                       int
	Seed                       uint64
	PreparationSeconds         float64
	Prepared, CleanupCompleted bool
}
type Outcome struct {
	Results   []Result
	Raw       []json.RawMessage
	Lifecycle Lifecycle
}

func seedValue(value string) (uint64, error) {
	if value == "random" {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return 0, fmt.Errorf("generate disk data seed: %w", err)
		}
		return binary.LittleEndian.Uint64(b[:]), nil
	}
	seed, err := strconv.ParseUint(value, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid --data-seed %q: %w", value, err)
	}
	return seed, nil
}
func profile(c config.Config) (string, string, []int) {
	rw, bs := "randread", "4k"
	switch c.Profile {
	case "randwrite-4k":
		rw = "randwrite"
	case "mixed-70-30":
		rw = "randrw"
	case "seqread-1m":
		rw, bs = "read", "1m"
	case "seqwrite-1m":
		rw, bs = "write", "1m"
	}
	depths := []int{c.Depth}
	if c.Profile == "latency" {
		depths = []int{1}
	}
	if c.Profile == "qd-scaling" {
		depths = []int{1, 4, 16, 32, 64}
	}
	return rw, bs, depths
}
func RunManaged(ctx context.Context, c config.Config, _ string) (out Outcome, err error) {
	if err = c.Validate(); err != nil {
		return out, err
	}
	if (strings.Contains(c.Profile, "write") || c.Profile == "mixed-70-30" || c.Precondition) && !c.ConfirmWrites {
		return out, fmt.Errorf("write-intensive profile %q requires --confirm-writes", c.Profile)
	}
	metadata, err := safety.ValidateDirectory(c.Path)
	if err != nil {
		return out, err
	}
	out.Lifecycle.Target = metadata["path"].(string)
	out.Lifecycle.Jobs = c.Jobs
	runner.Notice(ctx, "validated", fmt.Sprintf("[OK] Target directory validated: %s", out.Lifecycle.Target))
	// Probe execution/dependencies before creating a workspace or writing data.
	version, err := runner.Run(ctx, "fio", []string{"--version"}, 5*time.Second)
	if err != nil {
		return out, fmt.Errorf("fio preflight: %w", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(version)), "fio-") {
		return out, fmt.Errorf("fio preflight: executable does not identify as fio: %q", strings.TrimSpace(string(version)))
	}
	engines, err := runner.Run(ctx, "fio", []string{"--enghelp"}, 5*time.Second)
	if err != nil {
		return out, fmt.Errorf("fio engine capability check: %w", err)
	}
	found := false
	for _, name := range strings.Fields(string(engines)) {
		if name == c.Engine {
			found = true
		}
	}
	if !found {
		return out, fmt.Errorf("fio does not provide requested I/O engine %q; no fallback was used", c.Engine)
	}
	size, err := config.Bytes(c.FileSize)
	if err != nil {
		return out, err
	}
	rw, bs, depths := profile(c)
	block := int64(4096)
	if bs == "1m" {
		block = 1 << 20
	}
	if size < block || size%block != 0 {
		return out, fmt.Errorf("disk file size %d bytes per job must be a multiple of the %d-byte workload block size", size, block)
	}
	seed, err := seedValue(c.DataSeed)
	if err != nil {
		return out, err
	}
	out.Lifecycle.Seed = seed
	target, err := safety.PrepareSized(c.Path, "", size, c.Jobs, !c.DiskSizeExplicit)
	if err != nil {
		return out, err
	}
	out.Lifecycle.Workspace = target.Dir
	out.Lifecycle.FileBytes = target.Plan.FileBytes
	out.Lifecycle.TotalBytes = target.Plan.TotalBytes
	defer func() {
		cleanup := target.Close()
		out.Lifecycle.CleanupCompleted = cleanup == nil
		if cleanup != nil {
			runner.Notice(ctx, "cleanup-failed", "[WARNING] "+cleanup.Error())
			err = errors.Join(err, cleanup)
		} else {
			runner.Notice(ctx, "cleaned", "[OK] Temporary files removed")
		}
	}()
	runner.Notice(ctx, "workspace", "[OK] Private test workspace created: "+target.Dir)
	runner.Notice(ctx, "sizing", fmt.Sprintf("Jobs: %d | File size per job: %d bytes | Total test data: %d bytes", c.Jobs, target.Plan.FileBytes, target.Plan.TotalBytes))
	runner.Notice(ctx, "preparing", "Preparing and verifying real data (writes I/O, including for read profiles)...")
	started := time.Now()
	err = target.Fill(ctx, seed, func(done, total int) {
		runner.Notice(ctx, "file-prepared", fmt.Sprintf("Preparation: %d/%d job files written and verified", done, total))
	})
	out.Lifecycle.PreparationSeconds = time.Since(started).Seconds()
	if err != nil {
		return out, err
	}
	out.Lifecycle.Prepared = true
	runner.Notice(ctx, "prepared", fmt.Sprintf("[OK] Test data prepared in %.2f seconds; excluded from fio measurements", out.Lifecycle.PreparationSeconds))
	base := []string{"--output-format=json+", "--eta=never", "--ioengine=" + c.Engine, "--direct=" + strconv.Itoa(boolInt(c.Direct)), "--size=" + strconv.FormatUint(target.Plan.FileBytes, 10), "--refill_buffers=1", "--scramble_buffers=1", "--randrepeat=1", "--group_reporting=0", "--allow_file_create=0", "--unlink=0", "--unlink_each_loop=0", "--fallocate=none", "--invalidate=1", "--buffer_compress_percentage=0", "--dedupe_percentage=0"}
	for _, depth := range depths {
		for repeat := 0; repeat < c.Repetitions; repeat++ {
			if err = ctx.Err(); err != nil {
				return out, fmt.Errorf("disk measurement canceled before fio: %w", err)
			}
			if err = target.Validate(); err != nil {
				return out, err
			}
			args := append(append([]string{}, base...), "--rw="+rw, "--bs="+bs, "--rwmixread=70", "--iodepth="+strconv.Itoa(depth), "--time_based=1", "--runtime="+strconv.Itoa(c.Duration), "--ramp_time="+strconv.Itoa(c.Warmup))
			for i := 0; i < c.Jobs; i++ {
				args = append(args, fmt.Sprintf("--name=job-%03d", i+1), fmt.Sprintf("--filename=/proc/self/fd/%d", 3+i))
			}
			b, e := runner.RunFiles(ctx, "fio", args, time.Duration(c.Duration+c.Warmup+30)*time.Second, target.EngineFiles())
			if e != nil {
				return out, fmt.Errorf("fio %s at QD%d repetition %d in workspace %q: %w", c.Profile, depth, repeat+1, target.Dir, e)
			}
			result, e := Parse(b)
			if e == nil {
				e = validateMeasurement(result, c, rw)
			}
			if e != nil {
				return out, fmt.Errorf("invalid fio result for %s QD%d repetition %d: %w", c.Profile, depth, repeat+1, e)
			}
			out.Results = append(out.Results, result)
			out.Raw = append(out.Raw, json.RawMessage(b))
		}
	}
	runner.Notice(ctx, "measured", "[OK] fio benchmark completed")
	return out, nil
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
func validateMeasurement(result Result, c config.Config, rw string) error {
	if len(result.Jobs) != c.Jobs {
		return fmt.Errorf("expected %d independent jobs; received %d", c.Jobs, len(result.Jobs))
	}
	for i, job := range result.Jobs {
		directions := []struct {
			name  string
			value Direction
		}{}
		if rw == "randread" || rw == "read" || rw == "randrw" {
			directions = append(directions, struct {
				name  string
				value Direction
			}{"read", job.Read})
		}
		if rw == "randwrite" || rw == "write" || rw == "randrw" {
			directions = append(directions, struct {
				name  string
				value Direction
			}{"write", job.Write})
		}
		for _, direction := range directions {
			v := direction.value
			for _, metric := range []struct {
				name  string
				value float64
			}{{"IOPS", v.IOPS}, {"bandwidth", v.BWBytes}, {"runtime", v.Runtime}, {"average completion latency", v.Completion.Mean}} {
				if metric.value <= 0 || math.IsNaN(metric.value) || math.IsInf(metric.value, 0) {
					return fmt.Errorf("job-%03d %s %s is missing or invalid", i+1, direction.name, metric.name)
				}
			}
			percentile := false
			for key, value := range v.Completion.Percentile {
				p, e := strconv.ParseFloat(key, 64)
				if e == nil && p == 99 && value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
					percentile = true
				}
			}
			if !percentile {
				return fmt.Errorf("job-%03d %s p99 completion latency is missing or invalid", i+1, direction.name)
			}
		}
	}
	return nil
}
