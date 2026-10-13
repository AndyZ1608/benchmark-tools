package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/benchmark/memory"
	"vmbench/internal/benchmark/network"
	"vmbench/internal/config"
	"vmbench/internal/report"
	"vmbench/internal/runner"
	"vmbench/internal/system"
	"vmbench/internal/version"
)

const helpBody = ` — offline Linux VM benchmarking
Commands: cpu test | cpu stress | ram test | ram stress | disk test
          network server | network test | full test | system info | version
Flags: --version|-v | --help
       --preset quick|standard|precision --config FILE --non-interactive
       --workers N --duration SECONDS --warmup SECONDS --repetitions N
       --size 256MiB --path DIRECTORY --file-size 1GiB --confirm-writes
       --profile randread-4k|randwrite-4k|mixed-70-30|seqread-1m|seqwrite-1m|latency|qd-scaling
       --ioengine io_uring|libaio --iodepth N --numjobs N --target IP
       --bind IP --port N --streams N --udp --bitrate 100M --reverse --bidir
       --output results
`

func helpText() string { return "VMBench " + version.Current() + helpBody }

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", e)
		os.Exit(1)
	}
}
func run() error {
	a := os.Args[1:]
	if len(a) == 0 {
		fmt.Print(helpText(), "\nSelect command (e.g. cpu test): ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		a = strings.Fields(line)
	}
	if len(a) == 1 && (a[0] == "--help" || a[0] == "help") {
		fmt.Print(helpText())
		return nil
	}
	if len(a) == 1 && (a[0] == "version" || a[0] == "--version" || a[0] == "-v") {
		fmt.Printf("VMBench %s; kernel v1; native flags -O2 -fno-fast-math -pthread\n", version.Current())
		return nil
	}
	if len(a) < 2 {
		return fmt.Errorf("expected component and action; use --help")
	}
	command := a[0] + " " + a[1]
	switch command {
	case "cpu test", "cpu stress", "ram test", "ram stress", "disk test", "network test", "network server", "full test", "system info":
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	c, e := config.Parse(a[2:])
	if e != nil {
		return e
	}
	if command == "system info" {
		b, _ := json.MarshalIndent(system.Detect(), "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("benchmark execution requires Linux; this build supports help/version only")
	}
	if len(a) == 2 && !c.NonInteractive {
		fmt.Printf("CPU: %s; available capacity %.2f\n", system.Detect().CPUModel, system.Detect().Capacity)
		fmt.Printf("Defaults: workers=%d duration=%ds warmup=%ds repetitions=%d size=%s\n", c.Workers, c.Duration, c.Warmup, c.Repetitions, c.Size)
		reader := bufio.NewReader(os.Stdin)
		for _, q := range []struct {
			label string
			p     *int
		}{{"Workers", &c.Workers}, {"Duration seconds", &c.Duration}, {"Warmup seconds", &c.Warmup}, {"Repetitions", &c.Repetitions}} {
			fmt.Printf("%s [%d]: ", q.label, *q.p)
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			if line != "" {
				value, err := strconv.Atoi(line)
				if err != nil {
					return fmt.Errorf("invalid %s: %w", q.label, err)
				}
				*q.p = value
			}
		}
		if command == "disk test" {
			fmt.Print("Target directory: ")
			line, _ := reader.ReadString('\n')
			c.Path = strings.TrimSpace(line)
		}
		fmt.Print("Start? [y/N]: ")
		line, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) != "y" {
			return nil
		}
		if e = c.Validate(); e != nil {
			return e
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if command == "network server" {
		return network.Server(ctx, c)
	}
	r := report.New(command, c)
	telemetryStart := system.Counters()
	if c.Preset == "quick" {
		r.Warnings = append(r.Warnings, "Preliminary workload; unsuitable for rigorous cross-platform comparisons")
	}
	r.Warnings = append(r.Warnings, "Virtualization, backend contention and cache effects are uncontrolled; precision does not guarantee absolute accuracy")
	for _, name := range []string{"compute-bench", "fio", "stress-ng", "iperf3"} {
		b, err := runner.Run(ctx, name, []string{"--version"}, 5*time.Second)
		if err == nil {
			r.Results[name+"_version"] = string(b)
		}
	}
	execute := func(cmd string) error {
		fmt.Printf("\n%-24s RUNNING\n", strings.ToUpper(cmd))
		var err error
		switch cmd {
		case "cpu test":
			r.Results[cmd], err = cpu.Run(ctx, c, "cpu")
		case "cpu stress":
			r.Results[cmd], err = cpu.Stress(ctx, c)
		case "ram test", "ram stress":
			r.Results[cmd], err = memory.Run(ctx, c, cmd == "ram stress")
		case "disk test":
			r.Warnings = append(r.Warnings, "Read tests also write dataset initialization. Direct I/O does not bypass all storage caches. Benchmark files limited to selected size.")
			metadata, x := system.Filesystem(c.Path)
			if x != nil {
				return x
			}
			r.Results["filesystem"] = metadata
			if same, _ := metadata["system_filesystem"].(bool); same {
				r.Warnings = append(r.Warnings, "Disk target is on the system filesystem")
			}
			res, raw, x := disk.Run(ctx, c, r.ID)
			r.Results[cmd] = res
			r.Results["fio_raw"] = raw
			err = x
		case "network test":
			if c.Target == "" {
				r.Warnings = append(r.Warnings, "Network skipped: no peer configured")
				return nil
			}
			var observations []network.Result
			for i := 0; i < c.Repetitions; i++ {
				value, x := network.Run(ctx, c)
				if x != nil {
					err = x
					break
				}
				observations = append(observations, value)
			}
			r.Results[cmd] = observations
		}
		return err
	}
	if command == "full test" {
		for _, cmd := range []string{"cpu test", "ram test", "disk test", "network test"} {
			if e = execute(cmd); e != nil {
				break
			}
		}
	} else {
		e = execute(command)
	}
	r.Results["cpu_telemetry"] = system.Delta(telemetryStart, system.Counters())
	for _, key := range []string{"cpu test", "ram test", "ram stress"} {
		if raw, ok := r.Results[key].([]json.RawMessage); ok && len(raw) > 0 {
			summaries, x := cpu.Summarize(raw)
			if x != nil && e == nil {
				e = x
			}
			sort.Slice(summaries, func(i, j int) bool {
				if summaries[i].Workload == summaries[j].Workload {
					return summaries[i].Workers < summaries[j].Workers
				}
				return summaries[i].Workload < summaries[j].Workload
			})
			r.Results[key+"_statistics"] = summaries
			fmt.Printf("\n%-28s %7s %14s %10s\n", "WORKLOAD", "WORKERS", "MEDIAN", "CV")
			for _, s := range summaries {
				fmt.Printf("%-28s %7d %14.4f %9.2f%% %s\n", s.Workload, s.Workers, s.Statistics.Median, s.Statistics.CV*100, s.Unit)
				r.Warnings = append(r.Warnings, s.Warnings...)
			}
		}
	}
	r.DurationSeconds = time.Since(r.Started).Seconds()
	r.Status = "completed"
	if e != nil {
		r.Status = "failed"
		r.Errors = append(r.Errors, e.Error())
	}
	p, se := r.Save()
	b, _ := json.MarshalIndent(r.Results, "", "  ")
	fmt.Println(string(b))
	for _, w := range r.Warnings {
		fmt.Println("WARNING:", w)
	}
	fmt.Printf("Status: %s | Elapsed: %.2fs | Report: %s\n", r.Status, r.DurationSeconds, p)
	if se != nil {
		return se
	}
	return e
}
