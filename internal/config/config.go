package config

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"vmbench/internal/system"
)

type Config struct {
	Preset, Path, Profile, Engine, Target, Bind, Method, Size, FileSize, Window, Bitrate     string
	Workers, Duration, Warmup, Repetitions, Depth, Jobs, Port, Streams, Load, Sample, Packet int
	NonInteractive, ConfirmWrites, Direct, UDP, Reverse, Bidir, Precondition                 bool
	Details                                                                                  bool

	StabilityCV, StealWarningPercent float64
}

func Defaults(p string) (Config, error) {
	c := Config{StabilityCV: 5, StealWarningPercent: 1, Preset: p, Workers: int(math.Ceil(system.Detect().Capacity)), Duration: 10, Warmup: 2, Repetitions: 3, Profile: "randread-4k", Engine: "io_uring", Size: "256MiB", FileSize: "1GiB", Depth: 32, Jobs: 1, Port: 5201, Streams: 1, Load: 100, Sample: 1, Packet: 1400, Bind: "127.0.0.1", Method: "all", Bitrate: "100M", Direct: true}
	switch p {
	case "quick":
		c.Duration = 2
		c.Warmup = 1
		c.Repetitions = 1
	case "standard":
	case "precision":
		c.Duration = 30
		c.Warmup = 5
		c.Repetitions = 5
	default:
		return c, fmt.Errorf("unknown preset %q", p)
	}
	return c, nil
}
func Bytes(s string) (int64, error) {
	for _, u := range []struct {
		s string
		n int64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}} {
		if strings.HasSuffix(s, u.s) {
			n, e := strconv.ParseInt(strings.TrimSuffix(s, u.s), 10, 64)
			if e != nil || n <= 0 || n > (1<<40)/u.n {
				return 0, fmt.Errorf("invalid size %q", s)
			}
			return n * u.n, nil
		}
	}
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n <= 0 || n > 1<<40 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return n, nil
}
func Parse(args []string) (Config, error) {
	preset := "standard"
	file := ""
	for i, a := range args {
		if a == "--preset" && i+1 < len(args) {
			preset = args[i+1]
		}
		if strings.HasPrefix(a, "--preset=") {
			preset = strings.TrimPrefix(a, "--preset=")
		}
		if a == "--config" && i+1 < len(args) {
			file = args[i+1]
		}
		if strings.HasPrefix(a, "--config=") {
			file = strings.TrimPrefix(a, "--config=")
		}
	}
	var fromFile []string
	if file != "" {
		b, e := os.ReadFile(file)
		if e != nil {
			return Config{}, e
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
			if line == "" {
				continue
			}
			kv := strings.SplitN(line, ":", 2)
			if len(kv) != 2 {
				return Config{}, fmt.Errorf("configuration supports flat YAML key: value only")
			}
			k := strings.TrimSpace(kv[0])
			v := strings.Trim(strings.TrimSpace(kv[1]), "\"'")
			if k == "preset" {
				preset = v
			}
			fromFile = append(fromFile, "--"+k+"="+v)
		}
	}
	for i, a := range args {
		if a == "--preset" && i+1 < len(args) {
			preset = args[i+1]
		}
		if strings.HasPrefix(a, "--preset=") {
			preset = strings.TrimPrefix(a, "--preset=")
		}
	}
	c, e := Defaults(preset)
	if e != nil {
		return c, e
	}
	f := flag.NewFlagSet("vmbench", flag.ContinueOnError)
	f.SetOutput(io.Discard) // Return one error; main owns terminal diagnostics.
	f.StringVar(&file, "config", file, "flat YAML configuration")
	f.StringVar(&c.Preset, "preset", c.Preset, "quick, standard, precision")
	for _, v := range []struct {
		k string
		p *string
	}{{"path", &c.Path}, {"profile", &c.Profile}, {"ioengine", &c.Engine}, {"target", &c.Target}, {"bind", &c.Bind}, {"method", &c.Method}, {"size", &c.Size}, {"file-size", &c.FileSize}, {"window", &c.Window}, {"bitrate", &c.Bitrate}} {
		f.StringVar(v.p, v.k, *v.p, v.k)
	}
	for _, v := range []struct {
		k string
		p *int
	}{{"workers", &c.Workers}, {"threads", &c.Workers}, {"duration", &c.Duration}, {"warmup", &c.Warmup}, {"repetitions", &c.Repetitions}, {"iodepth", &c.Depth}, {"numjobs", &c.Jobs}, {"port", &c.Port}, {"streams", &c.Streams}, {"load", &c.Load}, {"sample", &c.Sample}, {"packet", &c.Packet}} {
		f.IntVar(v.p, v.k, *v.p, v.k)
	}
	for _, v := range []struct {
		k string
		p *bool
	}{{"non-interactive", &c.NonInteractive}, {"confirm-writes", &c.ConfirmWrites}, {"direct", &c.Direct}, {"udp", &c.UDP}, {"reverse", &c.Reverse}, {"bidir", &c.Bidir}, {"precondition", &c.Precondition}, {"details", &c.Details}} {
		f.BoolVar(v.p, v.k, *v.p, v.k)
	}
	f.Func("stability-cv", "presentation CV threshold in percent (not a speed ranking)", func(value string) error {
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
			return fmt.Errorf("must be between 0 and 100 percent")
		}
		c.StabilityCV = v
		return nil
	})
	f.Func("steal-warning-percent", "CPU steal warning screening threshold in percent", func(value string) error {
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
			return fmt.Errorf("must be between 0 and 100 percent")
		}
		c.StealWarningPercent = v
		return nil
	})
	if e = f.Parse(append(fromFile, args...)); e != nil {
		return c, e
	}
	if len(f.Args()) > 0 {
		return c, fmt.Errorf("unexpected arguments: %v", f.Args())
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if math.IsNaN(c.StabilityCV) || math.IsInf(c.StabilityCV, 0) || c.StabilityCV < 0 || c.StabilityCV > 100 {
		return fmt.Errorf("--stability-cv must be between 0 and 100 percent")
	}
	if math.IsNaN(c.StealWarningPercent) || math.IsInf(c.StealWarningPercent, 0) || c.StealWarningPercent < 0 || c.StealWarningPercent > 100 {
		return fmt.Errorf("--steal-warning-percent must be between 0 and 100 percent")
	}
	if c.Sample != 1 {
		return fmt.Errorf("live sampling interval is not implemented; use default 1 and complete-execution telemetry")
	}
	if c.Precondition {
		return fmt.Errorf("steady-state preconditioning is not implemented; ordinary dataset initialization is always performed")
	}
	if c.Workers < 1 || c.Workers > 4096 || c.Duration < 1 || c.Duration > 86400 || c.Warmup < 0 || c.Warmup > 3600 || c.Repetitions < 1 || c.Repetitions > 30 || c.Depth < 1 || c.Depth > 1024 || c.Jobs < 1 || c.Jobs > 128 || c.Streams < 1 || c.Streams > 128 || c.Port < 1 || c.Port > 65535 || c.Load < 1 || c.Load > 100 || c.Sample < 1 || c.Packet < 64 || c.Packet > 65507 {
		return fmt.Errorf("parameter outside supported limits")
	}
	if _, e := Bytes(c.Size); e != nil {
		return e
	}
	if _, e := Bytes(c.FileSize); e != nil {
		return e
	}
	if c.Engine != "io_uring" && c.Engine != "libaio" {
		return fmt.Errorf("only asynchronous io_uring and libaio engines supported")
	}
	switch c.Profile {
	case "randread-4k", "randwrite-4k", "mixed-70-30", "seqread-1m", "seqwrite-1m", "latency", "qd-scaling":
	default:
		return fmt.Errorf("unknown disk profile")
	}
	return nil
}
