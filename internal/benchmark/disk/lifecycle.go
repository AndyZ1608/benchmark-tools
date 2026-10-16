package disk

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/runner"
	"vmbench/internal/safety"
)

type Lifecycle struct {
	Target, Workspace, Engine  string
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
	size, err := config.Bytes(c.FileSize)
	if err != nil {
		return out, err
	}
	_, bs, _ := profile(c)
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
	c.Engine, err = SelectEngine(ctx, c, safety.ReserveBytes)
	if err != nil {
		return out, err
	}
	out.Lifecycle.Engine = c.Engine

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
	measurement, e := measurePrepared(ctx, c, target)
	out.Results, out.Raw = measurement.Results, measurement.Raw
	return out, e
}

func measurePrepared(ctx context.Context, c config.Config, target *safety.Target) (Outcome, error) {
	return measurePreparedLimited(ctx, c, target, false)
}
func measurePreparedLimited(ctx context.Context, c config.Config, target *safety.Target, probe bool) (out Outcome, err error) {
	if c.Jobs < 1 || c.Jobs > target.Plan.Jobs {
		return out, fmt.Errorf("requested jobs exceed prepared files")
	}
	rw, bs, depths := profile(c)
	block := uint64(4096)
	if bs == "1m" {
		block = 1 << 20
	}
	if target.Plan.FileBytes%block != 0 {
		return out, fmt.Errorf("prepared size is not aligned to workload block size")
	}
	// Thread jobs remain in the owned fio process; forked fio workers call setsid
	// and can escape process-group cancellation before the parent can reap them.
	base := []string{"--thread=1", "--output-format=json+", "--eta=never", "--ioengine=" + c.Engine, "--direct=" + strconv.Itoa(boolInt(c.Direct)), "--size=" + strconv.FormatUint(target.Plan.FileBytes, 10), "--refill_buffers=1", "--scramble_buffers=1", "--randrepeat=1", "--group_reporting=0", "--allow_file_create=0", "--unlink=0", "--unlink_each_loop=0", "--fallocate=none", "--invalidate=1", "--buffer_compress_percentage=0", "--dedupe_percentage=0"}
	for _, depth := range depths {
		for repeat := 0; repeat < c.Repetitions; repeat++ {
			if err = ctx.Err(); err != nil {
				return out, fmt.Errorf("disk measurement canceled before fio: %w", err)
			}
			if err = target.Validate(); err != nil {
				return out, err
			}
			timeBased := "1"
			if probe {
				timeBased = "0"
			}
			args := append(append([]string{}, base...), "--rw="+rw, "--bs="+bs, "--rwmixread=70", "--iodepth="+strconv.Itoa(depth), "--time_based="+timeBased, "--runtime="+strconv.Itoa(c.Duration), "--ramp_time="+strconv.Itoa(c.Warmup))
			timeout := time.Duration(c.Duration+c.Warmup+30) * time.Second
			if probe {
				args = append(args, "--number_ios=64", "--io_size=256k")
				timeout = 5 * time.Second
			}
			for i := 0; i < c.Jobs; i++ {
				args = append(args, fmt.Sprintf("--name=job-%03d", i+1), fmt.Sprintf("--filename=/proc/self/fd/%d", 3+i))
			}
			b, e := runner.RunFiles(ctx, "fio", args, timeout, target.EngineFiles()[:c.Jobs])
			result, parseError := Parse(b)
			if e == nil {
				e = parseError
			}
			if e == nil {
				result.Accounting, e = validateMeasured(result, c, rw, depth, probe)
				if e == nil {
					_, e = ExecutionEvidence(result, c.Engine, depth)
				}
			}
			if e != nil {
				return out, measurementFailure(c, depth, repeat+1, result, b, e)
			}
			out.Results = append(out.Results, result)
			out.Raw = append(out.Raw, json.RawMessage(b))
		}
	}
	if !probe {
		runner.Notice(ctx, "measured", "[OK] fio benchmark completed")
	}
	return out, nil
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
