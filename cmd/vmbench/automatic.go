package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	automatic "vmbench/internal/benchmark/auto"
	"vmbench/internal/presentation"
)

const menu = `================================================
                    VMBENCH
           VM PERFORMANCE BENCHMARK
================================================

Select Benchmark Component:

  [1] CPU
      CPU performance and stress testing

  [2] RAM
      Memory bandwidth and latency

  [3] DISK
      IOPS, throughput and latency

  [4] NETWORK
      Network throughput and performance

  [0] Exit

Select: `

const modeMenu = `================================================
             VMBENCH - %s
================================================

Select Benchmark Mode:

  [1] QUICK
      Fast preliminary performance check

  [2] STANDARD
      Detailed performance evaluation

  [3] MAX
      Discover maximum observed performance

  [0] Back

Select: `

// One input pump serves the whole session, preserving buffered input across
// menus. Cancellation also releases the caller while a terminal read is idle.
// At most one read remains blocked until stdin closes or the process exits.
type console struct {
	ctx    context.Context
	lines  <-chan string
	errors <-chan error
	out    io.Writer
	start  func()
}

func newConsole(ctx context.Context, input io.Reader, output io.Writer) *console {
	lines := make(chan string)
	errs := make(chan error, 1)
	start := func() {
		go func() {
			defer close(lines)
			scanner := bufio.NewScanner(input)
			for scanner.Scan() {
				select {
				case lines <- strings.TrimSpace(scanner.Text()):
				case <-ctx.Done():
					return
				}
			}
			if err := scanner.Err(); err != nil {
				errs <- err
			}
		}()
	}
	return &console{ctx: ctx, lines: lines, errors: errs, out: output, start: start}
}
func (c *console) read(prompt string) (string, error) {
	if err := c.ctx.Err(); err != nil {
		return "", err
	}
	if c.start != nil {
		c.start()
		c.start = nil
	}
	fmt.Fprint(c.out, prompt)
	select {
	case <-c.ctx.Done():
		return "", c.ctx.Err()
	case line, ok := <-c.lines:
		if !ok {
			select {
			case err := <-c.errors:
				return "", err
			default:
				return "", io.EOF
			}
		}
		if strings.ContainsRune(line, '\x03') {
			return "", context.Canceled
		}
		return line, c.ctx.Err()
	}
}
func (c *console) choose(prompt string, choices []string) (string, error) {
	for {
		line, err := c.read(prompt)
		if err != nil {
			return "", err
		}
		for _, v := range choices {
			if line == v {
				return line, nil
			}
		}
		fmt.Fprintln(c.out, "Invalid selection. Please choose one of the listed numbers.")
	}
}
func finishedInput(err error) bool { return errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) }

// Dependencies are shared with direct commands and injectable for navigation
// tests. No benchmark engines are invoked by menu tests.
type automaticActions struct {
	disk    func(automatic.Options) (automatic.DiskInfo, error)
	peer    func(string) error
	execute func(context.Context, automatic.Options) error
	details func() error // Renders the retained result without executing a workload.
}

func realActions(output, progress io.Writer) automaticActions {
	var last *automatic.Result
	return automaticActions{
		disk: func(o automatic.Options) (automatic.DiskInfo, error) {
			return automatic.CheckDiskTarget(o, automatic.RealBackend{})
		},
		peer: automatic.ValidatePeer,
		execute: func(ctx context.Context, o automatic.Options) error {
			last = nil
			if runtime.GOOS != "linux" {
				return fmt.Errorf("automatic benchmark execution requires Linux")
			}
			result, cause := automatic.Run(ctx, o, automatic.RealBackend{}, presentation.AutomaticProgress(progress, o.Details))
			last = &result
			if err := presentation.RenderAutomatic(output, result, o.Details); err != nil {
				return err
			}
			return cause
		},
		details: func() error {
			if last == nil {
				return fmt.Errorf("no benchmark result is available")
			}
			return presentation.RenderAutomatic(output, *last, true)
		},
	}
}

// Essential target input is validated before any workload. A target is never
// created or substituted; execution repeats filesystem safety checks.
func prepareTargets(c *console, o *automatic.Options, a automaticActions, interactive bool) (bool, error) {
	if o.Selects("DISK") {
		for {
			if o.DiskPath == "" && interactive {
				prompt := "Existing benchmark directory (0 to Back): "
				if o.Component == "" {
					prompt = "Existing disk target directory (blank to skip Disk, 0 to Back): "
				}
				line, err := c.read(prompt)
				if err != nil {
					return false, err
				}
				if line == "0" {
					return false, nil
				}
				o.DiskPath = line
				if line == "" && o.Component == "" {
					break
				}
			}
			if o.DiskPath == "" {
				if o.Component == "" {
					break
				}
				if !interactive {
					return false, fmt.Errorf("disk requires --path DIRECTORY")
				}
				fmt.Fprintln(c.out, "Enter an existing directory, or 0 to go Back.")
				continue
			}
			info, err := a.disk(*o)
			if err != nil {
				if !interactive {
					return false, err
				}
				fmt.Fprintln(c.out, "Invalid disk target: "+presentation.SafeText(err.Error()))
				o.DiskPath = ""
				continue
			}
			if o.Details {
				fmt.Fprintf(c.out, "Disk target: %s; filesystem %s; mount %s\n", presentation.SafeText(info.Path), presentation.SafeText(info.Filesystem), presentation.SafeText(info.Mount))
			} else {
				fmt.Fprintln(c.out, "Disk target: "+presentation.SafeText(info.Path))
			}
			if info.System {
				fmt.Fprintln(c.out, "WARNING: Disk target is on the system filesystem.")
			}
			fmt.Fprintln(c.out, "Read tests also write private preparation data.")
			break
		}
	}
	if o.Selects("NETWORK") {
		if o.Component == "network" {
			fmt.Fprintf(c.out, "On the trusted remote server, run: ./vmbench network server --bind SERVER_IP --port %d --non-interactive\nAllow TCP port %d between these hosts. The server must be running before this test.\n", o.Port, o.Port)
		}
		for {
			if o.NetworkPeer == "" {
				if o.Component == "" {
					break
				}
				if !interactive {
					return false, fmt.Errorf("network requires --target REMOTE_IP")
				}
				line, err := c.read("Remote benchmark server IP (0 to Back): ")
				if err != nil {
					return false, err
				}
				if line == "0" {
					return false, nil
				}
				o.NetworkPeer = line
			}
			err := a.peer(o.NetworkPeer)
			if err == nil {
				break
			}
			if !interactive {
				return false, err
			}
			fmt.Fprintln(c.out, "Invalid network target: "+presentation.SafeText(err.Error()))
			o.NetworkPeer = ""
		}
	}
	return true, nil
}
func acknowledgeMax(c *console, o *automatic.Options, interactive bool) (bool, error) {
	if o.Mode != "max" {
		return true, nil
	}
	component := strings.ToUpper(o.Component)
	if component == "" {
		component = "CPU, RAM, disk, and network"
	}
	fmt.Fprintf(c.out, "MAX PERFORMANCE BENCHMARK\nThis mode will generate high %s load. Other applications may be affected.\n", component)
	if o.AckMax {
		return true, nil
	}
	if !interactive {
		return false, fmt.Errorf("noninteractive MAX requires --ack-max; write authorization remains separate")
	}
	line, err := c.read("Continue? [y/N]: ")
	if err != nil {
		return false, err
	}
	if strings.ToLower(line) != "y" {
		return false, nil
	}
	o.AckMax = true
	return true, nil
}
func interactiveSession(ctx context.Context, input io.Reader, output io.Writer, a automaticActions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c := newConsole(ctx, input, output)
	for {
		component, err := c.choose(menu, []string{"0", "1", "2", "3", "4"})
		if finishedInput(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if component == "0" {
			return nil
		}
		component = map[string]string{"1": "cpu", "2": "ram", "3": "disk", "4": "network"}[component]
		for {
			mode, err := c.choose(fmt.Sprintf(modeMenu, strings.ToUpper(component)), []string{"0", "1", "2", "3"})
			if finishedInput(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if mode == "0" {
				break
			}
			mode = map[string]string{"1": "quick", "2": "standard", "3": "max"}[mode]
			o, err := automatic.ParseComponent(component, mode, nil)
			if err != nil {
				fmt.Fprintln(output, "Configuration error: "+presentation.SafeText(err.Error()))
				continue
			}
			ready, err := prepareTargets(c, &o, a, true)
			if finishedInput(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !ready {
				continue
			}
			ready, err = acknowledgeMax(c, &o, true)
			if finishedInput(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !ready {
				continue
			}
			if ctx.Err() != nil {
				return nil
			}
			o.NonInteractive = true
			err = a.execute(ctx, o)
			if ctx.Err() != nil {
				return nil
			}
			if err != nil {
				fmt.Fprintln(output, "Benchmark failed: "+presentation.SafeText(err.Error()))
			} else {
				fmt.Fprintln(output, "Benchmark completed.")
			}
			for {
				prompt := "\n[1] Back to Main Menu\n[0] Exit\n\nSelect: "
				choices := []string{"0", "1"}
				if a.details != nil {
					prompt = "\n[1] Back to Main Menu\n[2] View Details\n[0] Exit\n\nSelect: "
					choices = append(choices, "2")
				}
				next, err := c.choose(prompt, choices)
				if finishedInput(err) {
					return nil
				}
				if err != nil {
					return err
				}
				if next == "0" {
					return nil
				}
				if next == "2" {
					if err := a.details(); err != nil {
						fmt.Fprintln(output, "Details unavailable: "+presentation.SafeText(err.Error()))
					}
					continue
				}
				break
			}
			break
		}
	}
}
func runInteractive() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return interactiveSession(ctx, os.Stdin, os.Stdout, realActions(os.Stdout, os.Stderr))
}

const autoHelp = `Automatic component commands: cpu | ram | disk | network, then quick | standard | max
  disk MODE --path DIRECTORY / network MODE --target IP
  --ioengine auto|io_uring|libaio  Disk selection; explicit engines never substitute
  --disk-path DIRECTORY      Alias for Disk --path; must already exist
  --network-peer IP          Alias for Network --target; trusted remote server
  Missing targets prompt on a terminal; --non-interactive requires the target.
Legacy combined commands: quick | standard | max (missing endpoints are skipped)
  --non-interactive          Never prompt
  --ack-max                  Acknowledge high-intensity MAX load for this invocation
  --include-writes --confirm-writes  Separate optional write-test authorization
  --config FILE             Optional flat automatic configuration
  --details                 Show search observations and detected resources
  --disk-p99-warning-ms N    Optional per-workload P99 warning threshold; 0 disables
  --fio-diagnostics DIRECTORY  Retain raw fio output for failed workloads only
Safety ceilings:
  --max-workers 32 --max-ram 2GiB --max-disk-data 2GiB --min-free-disk 1GiB
  --max-disk-depth 64 --max-disk-jobs 4 --max-outstanding-io 128
  --max-network-streams 8 --network-bitrate 1G --max-duration 30m
  --sample-seconds N        Override measurement duration (MAX confirmations longer)
  --max-cv 0.10 --saturation-improvement 0.05
Advanced component commands remain available; see --help.
`

func runAutomatic(mode string, args []string) error { return runComponentAutomatic("", mode, args) }
func runComponentAutomatic(component, mode string, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Print(autoHelp)
		return nil
	}
	o, err := automatic.ParseComponent(component, mode, args)
	if err != nil {
		return err
	}
	interactive := interactiveStdin() && !o.NonInteractive
	o.NonInteractive = !interactive
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	c := newConsole(ctx, os.Stdin, os.Stdout)
	a := realActions(os.Stdout, os.Stderr)
	ready, err := prepareTargets(c, &o, a, interactive)
	if finishedInput(err) {
		return nil
	}
	if err != nil || !ready {
		return err
	}
	ready, err = acknowledgeMax(c, &o, interactive)
	if finishedInput(err) {
		return nil
	}
	if err != nil || !ready {
		return err
	}
	return a.execute(ctx, o)
}
