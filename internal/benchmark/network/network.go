package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/platform"
	"vmbench/internal/runner"
)

type Result struct {
	Start     json.RawMessage `json:"start"`
	Intervals json.RawMessage `json:"intervals"`
	End       json.RawMessage `json:"end"`
	Error     string          `json:"error"`
}

func Parse(b []byte) (Result, error) {
	var r Result
	e := json.Unmarshal(b, &r)
	if e != nil {
		return r, e
	}
	if r.Error != "" {
		return r, fmt.Errorf("iperf3: %s", r.Error)
	}
	if len(r.End) == 0 {
		return r, fmt.Errorf("iperf3 missing end metrics")
	}
	return r, nil
}
func Run(ctx context.Context, c config.Config) (Result, error) {
	if net.ParseIP(c.Target) == nil {
		return Result{}, fmt.Errorf("--target must be a literal IP address")
	}
	help, err := runner.RunText(ctx, "iperf3", []string{"--help"}, 5*time.Second)
	if err != nil {
		return Result{}, fmt.Errorf("iperf3 capability check: %w", err)
	}
	if err = validateOptions(string(help), c); err != nil {
		return Result{}, err
	}
	args := []string{"-J", "-c", c.Target, "-p", strconv.Itoa(c.Port), "-t", strconv.Itoa(c.Duration), "-O", strconv.Itoa(c.Warmup), "-P", strconv.Itoa(c.Streams)}
	if c.UDP {
		args = append(args, "-u", "-b", c.Bitrate, "-l", strconv.Itoa(c.Packet))
	} else if c.TCPBitrate != "" {
		args = append(args, "-b", c.TCPBitrate)
	}
	if c.Reverse {
		args = append(args, "-R")
	}
	if c.Bidir {
		args = append(args, "--bidir")
	}
	if c.Window != "" {
		args = append(args, "-w", c.Window)
	}
	b, e := runner.Run(ctx, "iperf3", args, time.Duration(c.Duration+c.Warmup+30)*time.Second)
	if e != nil {
		return Result{}, e
	}
	return Parse(b)
}
func Server(ctx context.Context, c config.Config) error { return ServerOutput(ctx, c, os.Stdout) }

// ServerOutput changes only the destination of the listening notice.
func ServerOutput(ctx context.Context, c config.Config, output io.Writer) error {
	if net.ParseIP(c.Bind) == nil {
		return fmt.Errorf("bind must be a literal IP address")
	}
	fmt.Fprintf(output, "Listening on %s:%d. Use trusted benchmark networks only.\n", c.Bind, c.Port)
	_, e := runner.Run(ctx, "iperf3", []string{"-s", "-B", c.Bind, "-p", strconv.Itoa(c.Port)}, 24*time.Hour)
	return e
}

func validateOptions(help string, c config.Config) error {
	required := [][]string{{"--json", "-J"}, {"--parallel", "-P"}, {"--omit", "-O"}}
	if c.Bidir {
		required = append(required, []string{"--bidir"})
	}
	if c.Reverse {
		required = append(required, []string{"--reverse", "-R"})
	}
	if c.TCPBitrate != "" || c.UDP {
		required = append(required, []string{"--bitrate", "--bandwidth", "-b"})
	}
	for _, aliases := range required {
		if !platform.HasOption(help, aliases...) {
			return fmt.Errorf("installed iperf3 does not support requested option %s; supply a compatible version", aliases[0])
		}
	}
	return nil
}
