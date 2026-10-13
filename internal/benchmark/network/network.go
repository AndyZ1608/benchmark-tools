package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"
	"vmbench/internal/config"
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
	args := []string{"-J", "-c", c.Target, "-p", strconv.Itoa(c.Port), "-t", strconv.Itoa(c.Duration), "-O", strconv.Itoa(c.Warmup), "-P", strconv.Itoa(c.Streams)}
	if c.UDP {
		args = append(args, "-u", "-b", c.Bitrate, "-l", strconv.Itoa(c.Packet))
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
func Server(ctx context.Context, c config.Config) error {
	if net.ParseIP(c.Bind) == nil {
		return fmt.Errorf("bind must be a literal IP address")
	}
	fmt.Printf("Listening on %s:%d. Use trusted benchmark networks only.\n", c.Bind, c.Port)
	_, e := runner.Run(ctx, "iperf3", []string{"-s", "-B", c.Bind, "-p", strconv.Itoa(c.Port)}, 24*time.Hour)
	return e
}
