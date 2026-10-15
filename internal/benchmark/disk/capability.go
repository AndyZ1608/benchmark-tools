package disk

import (
	"context"
	"errors"
	"fmt"
	"vmbench/internal/config"
	"vmbench/internal/runner"
	"vmbench/internal/safety"
)

type preparationFailure struct{ error }

func (e *preparationFailure) Unwrap() error { return e.error }

// selectAsync never changes engines after measurements have started. Explicit
// engine requests have exactly one candidate and never fall back.
func selectAsync(ctx context.Context, names []string, requested string, probe func(string) error) (string, error) {
	order := []string{"io_uring", "libaio"}
	if requested != "auto" {
		order = []string{requested}
	}
	var failures []error
	for _, name := range order {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		found := false
		for _, compiled := range names {
			if compiled == name {
				found = true
			}
		}
		if !found {
			failures = append(failures, fmt.Errorf("%s: not advertised by fio --enghelp", name))
			continue
		}
		if err := probe(name); err != nil {
			var unsafeTarget *preparationFailure
			if errors.As(err, &unsafeTarget) {
				return "", err
			}
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
			runner.Notice(ctx, "capability", "[INFO] "+name+": FAILED on selected target: "+err.Error())
			continue
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return name, nil
	}
	extra := "; no synchronous fallback"
	if requested != "auto" {
		extra += "; no substitution used; try --ioengine auto or explicitly validate the other async engine"
	}
	return "", fmt.Errorf("Disk asynchronous I/O unsupported on selected target%s: %w", extra, errors.Join(failures...))
}

// SelectEngine validates only in the explicitly selected filesystem. Its unique
// owned 1MiB file is populated, synchronized and verified by the existing safety
// lifecycle. Each candidate reads at most 64 x 4KiB, with QD4 and a 5s deadline.
// Probe results are discarded and never enter benchmark measurements.
func SelectEngine(ctx context.Context, c config.Config, reserve uint64) (selected string, err error) {
	if c.Engine != "auto" && c.Engine != "io_uring" && c.Engine != "libaio" {
		return "", fmt.Errorf("only auto, io_uring and libaio selection is supported")
	}
	if !c.Direct {
		return "", fmt.Errorf("asynchronous Disk validation requires direct I/O")
	}
	if _, err = safety.ValidateDirectory(c.Path); err != nil {
		return "", err
	}
	names, err := Engines(ctx)
	if err != nil {
		return "", err
	}
	var target *safety.Target
	var preparationError error
	defer func() {
		if target != nil {
			if cleanup := target.Close(); cleanup != nil {
				selected = ""
				err = errors.Join(err, fmt.Errorf("capability probe cleanup incomplete: %w", cleanup))
			}
		}
		if err == nil && selected != "" {
			runner.Notice(ctx, "capability", "[OK] "+selected+": READY - verified with completed direct reads and asynchronous depth; probe files removed; selected I/O engine: "+selected)
		}
	}()
	probe := func(engine string) error {
		if preparationError != nil {
			return &preparationFailure{preparationError}
		}
		if target == nil {
			target, preparationError = safety.PrepareReserved(c.Path, "", 1<<20, 1, false, reserve)
			if preparationError != nil {
				return &preparationFailure{preparationError}
			}
			var seed uint64
			seed, preparationError = seedValue("random")
			if preparationError != nil {
				return &preparationFailure{preparationError}
			}
			preparationError = target.Fill(ctx, seed, nil)
			if preparationError != nil {
				return &preparationFailure{preparationError}
			}
		}
		p := c
		p.Engine = engine
		p.Profile = "randread-4k"
		p.Depth = 4
		p.Jobs = 1
		p.Duration = 1
		p.Warmup = 0
		p.Repetitions = 1
		p.Direct = true
		p.ConfirmWrites = false
		out, e := measurePreparedLimited(ctx, p, target, true)
		if e != nil {
			return e
		}
		if len(out.Results) != 1 {
			return fmt.Errorf("functional probe returned no complete result")
		}
		r := out.Results[0]
		if len(r.Jobs) != 1 || r.Jobs[0].Read.TotalIOs != 64 || r.Jobs[0].Read.IOBytes != 64*4096 {
			return fmt.Errorf("probe did not complete exactly 64 direct 4KiB reads")
		}
		return nil
	}
	selected, err = selectAsync(ctx, names, c.Engine, probe)
	return selected, err
}
