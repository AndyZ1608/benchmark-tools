package main

import (
	"bufio"
	"context"
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

const menu = `============================================
                  VMBENCH
          VM PERFORMANCE BENCHMARK
============================================

Select Benchmark Mode:

  [1] QUICK
      Fast preliminary performance check

  [2] STANDARD
      Detailed performance evaluation

  [3] MAX
      Discover maximum observed performance

  [0] Exit

Select: `

func selectMode(input io.Reader, output io.Writer) (string, error) {
	reader := bufio.NewReader(input)
	for {
		fmt.Fprint(output, menu)
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		switch line {
		case "0", "":
			return "", nil
		case "1":
			return "quick", nil
		case "2":
			return "standard", nil
		case "3":
			return "max", nil
		}
		if err != nil {
			if err == io.EOF {
				return "", nil
			}
			return "", err
		}
		fmt.Fprintln(output, "Select 0, 1, 2, or 3.")
	}
}

const autoHelp = `Automatic modes: quick | standard | max
  --disk-path DIRECTORY      Existing writable target; otherwise Disk is skipped
  --network-peer IP          Trusted remote iperf3 peer; otherwise Network is skipped
  --non-interactive          Never prompt
  --ack-max                  Acknowledge high-intensity MAX load for this invocation
  --include-writes --confirm-writes  Separate optional write-test authorization
  --config FILE             Optional flat automatic configuration
  --details                 Show search observations and detected resources
Safety ceilings:
  --max-workers 32 --max-ram 2GiB --max-disk-data 2GiB --min-free-disk 1GiB
  --max-disk-depth 64 --max-disk-jobs 4 --max-outstanding-io 128
  --max-network-streams 8 --network-bitrate 1G --max-duration 30m
  --sample-seconds N        Override measurement duration (MAX confirmations longer)
  --max-cv 0.10 --saturation-improvement 0.05
Advanced component commands remain available; see --help.
`

func runAutomatic(mode string, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Print(autoHelp)
		return nil
	}
	o, err := automatic.Parse(mode, args)
	if err != nil {
		return err
	}
	inputStat, _ := os.Stdin.Stat()
	interactive := inputStat != nil && inputStat.Mode()&os.ModeCharDevice != 0 && !o.NonInteractive
	o.NonInteractive = !interactive
	if runtime.GOOS != "linux" {
		return fmt.Errorf("automatic benchmark execution requires Linux")
	}
	reader := bufio.NewReader(os.Stdin)
	b := automatic.RealBackend{}
	if o.DiskPath == "" && interactive {
		fmt.Print("Existing disk target directory (blank to skip Disk): ")
		line, _ := reader.ReadString('\n')
		o.DiskPath = strings.TrimSpace(line)
	}
	if o.DiskPath != "" {
		info, e := b.Filesystem(o.DiskPath)
		if e != nil {
			return e
		}
		fmt.Fprintf(os.Stderr, "Disk target: %s; filesystem %s; mount %s\n", presentation.SafeText(info.Path), presentation.SafeText(info.Filesystem), presentation.SafeText(info.Mount))
		if info.System {
			fmt.Fprintln(os.Stderr, "WARNING: Disk target is on the system filesystem.")
		}
		fmt.Fprintln(os.Stderr, "Read tests also write private preparation data.")
	}
	if mode == "max" {
		fmt.Fprintln(os.Stderr, "MAX PERFORMANCE BENCHMARK\nThis mode will generate high CPU, RAM, disk, and network load.\nOther applications may be affected.")
		if !o.AckMax {
			if !interactive {
				return fmt.Errorf("noninteractive MAX requires --ack-max; write authorization remains separate")
			}
			fmt.Print("Continue? [y/N]: ")
			line, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(line)) != "y" {
				return nil
			}
			o.AckMax = true
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, cause := automatic.Run(ctx, o, b, func(line string) { fmt.Fprintln(os.Stderr, presentation.SafeText(line)) })
	if err = presentation.RenderAutomatic(os.Stdout, result, o.Details); err != nil {
		return err
	}
	return cause
}
