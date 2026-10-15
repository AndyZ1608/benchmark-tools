package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
	automatic "vmbench/internal/benchmark/auto"
)

func TestAutomaticHelpAndSafetyFlags(t *testing.T) {
	for _, mode := range []string{"quick", "standard", "max"} {
		out, stderr, e := invokeCLI(t, mode, "--help")
		if e != nil || stderr != "" || !strings.Contains(out, "--ack-max") || !strings.Contains(out, "--disk-path") {
			t.Fatal(out, stderr, e)
		}
		_, stderr, e = invokeCLI(t, mode, "--max-workers", "0")
		if e == nil || !strings.Contains(stderr, "safety limit") {
			t.Fatal(stderr, e)
		}
	}
}

func menuFixture(calls *[]automatic.Options) automaticActions {
	return automaticActions{
		disk: func(o automatic.Options) (automatic.DiskInfo, error) {
			if o.DiskPath != "/data" {
				return automatic.DiskInfo{}, fmt.Errorf("fixture directory missing or insufficient space")
			}
			return automatic.DiskInfo{Path: o.DiskPath, Filesystem: "fixture", Mount: "/", System: true}, nil
		},
		peer: func(peer string) error {
			o := automatic.Defaults("quick")
			o.NetworkPeer = peer
			if peer == "" {
				return fmt.Errorf("remote IP required")
			}
			return o.Validate()
		},
		execute: func(ctx context.Context, o automatic.Options) error { *calls = append(*calls, o); return nil },
	}
}
func TestComponentMenuAndAllModeRoutes(t *testing.T) {
	for i, component := range []string{"cpu", "ram", "disk", "network"} {
		for j, mode := range []string{"quick", "standard", "max"} {
			t.Run(component+"/"+mode, func(t *testing.T) {
				input := fmt.Sprintf("%d\n%d\n", i+1, j+1)
				if component == "disk" {
					input += "/data\n"
				}
				if component == "network" {
					input += "10.0.0.20\n"
				}
				if mode == "max" {
					input += "y\n"
				}
				input += "0\n"
				var calls []automatic.Options
				var out bytes.Buffer
				if err := interactiveSession(context.Background(), strings.NewReader(input), &out, menuFixture(&calls)); err != nil {
					t.Fatal(err)
				}
				if len(calls) != 1 || calls[0].Component != component || calls[0].Mode != mode || !calls[0].NonInteractive {
					t.Fatalf("dispatch: %+v", calls)
				}
				text := out.String()
				main := strings.SplitN(text, "Select: ", 2)[0]
				for _, name := range []string{"[1] CPU", "[2] RAM", "[3] DISK", "[4] NETWORK", "[0] Exit"} {
					if !strings.Contains(main, name) {
						t.Fatal(main)
					}
				}
				for _, name := range []string{"QUICK", "STANDARD", "MAX", "FULL"} {
					if strings.Contains(main, name) {
						t.Fatal(main)
					}
				}
				for _, want := range []string{"VMBENCH - " + strings.ToUpper(component), "[3] MAX", "[0] Back", "Benchmark completed.", "Back to Main Menu"} {
					if !strings.Contains(text, want) {
						t.Fatal(want, text)
					}
				}
				if component != "disk" && strings.Contains(text, "benchmark directory") || component != "network" && strings.Contains(text, "Remote benchmark server") {
					t.Fatal(text)
				}
				if component == "disk" && !strings.Contains(text, "system filesystem") {
					t.Fatal(text)
				}
				if component == "network" && !strings.Contains(text, "network server --bind SERVER_IP") {
					t.Fatal(text)
				}
				if calls[0].IncludeWrites || calls[0].ConfirmWrites {
					t.Fatal("menu enabled write workloads")
				}
			})
		}
	}
}
func TestNavigationWithoutLoads(t *testing.T) {
	for name, input := range map[string]string{
		"exit": "0\n", "eof": "", "mode eof": "1\n", "disk eof": "3\n1\n", "network eof": "4\n1\n", "max eof": "1\n3\n",
		"all back":           "1\n0\n2\n0\n3\n0\n4\n0\n0\n",
		"invalid":            "wrong\n5\n\n1\nwrong\n4\n0\n0\n",
		"disk retry back":    "3\n1\n/missing\n0\n0\n0\n",
		"network retry back": "4\n1\n127.0.0.1\nnot-an-ip\n0\n0\n0\n",
		"max declined":       "1\n3\nn\n0\n0\n", "ctrl c byte": "1\n\x03\n",
	} {
		t.Run(name, func(t *testing.T) {
			var calls []automatic.Options
			var out bytes.Buffer
			if err := interactiveSession(context.Background(), strings.NewReader(input), &out, menuFixture(&calls)); err != nil {
				t.Fatal(err)
			}
			if len(calls) != 0 {
				t.Fatalf("unexpected load %+v", calls)
			}
			if strings.Contains(name, "retry") && !strings.Contains(out.String(), "Invalid") {
				t.Fatal(out.String())
			}
		})
	}
}
func TestRetryAndReturnToMainWithoutStateCarryover(t *testing.T) {
	var calls []automatic.Options
	var out bytes.Buffer
	input := "3\n1\n/missing\n/data\nwrong\n1\n4\n1\n127.0.0.1\n10.0.0.20\n1\n1\n1\n0\n"
	if err := interactiveSession(context.Background(), strings.NewReader(input), &out, menuFixture(&calls)); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[0].Component != "disk" || calls[1].Component != "network" || calls[2].Component != "cpu" || calls[1].DiskPath != "" || calls[2].NetworkPeer != "" {
		t.Fatalf("%+v", calls)
	}
}
func TestMenuFailureAndCancellation(t *testing.T) {
	var calls []automatic.Options
	a := menuFixture(&calls)
	a.execute = func(context.Context, automatic.Options) error { return fmt.Errorf("fixture engine unavailable") }
	var out bytes.Buffer
	if err := interactiveSession(context.Background(), strings.NewReader("1\n1\n0\n"), &out, a); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Benchmark failed: fixture engine unavailable") || strings.Contains(out.String(), "Benchmark completed.") {
		t.Fatal(out.String())
	}
	for _, stage := range []string{"main", "mode", "target", "max consent", "execution", "post result"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			reached := make(chan struct{}, 1)
			a := menuFixture(&calls)
			a.execute = func(ctx context.Context, o automatic.Options) error {
				if stage == "execution" {
					reached <- struct{}{}
					<-ctx.Done()
					return ctx.Err()
				}
				return nil
			}
			output := &promptObserver{stage: stage, reached: reached}
			done := make(chan error, 1)
			go func() { done <- interactiveSession(ctx, reader, output, a) }()
			inputs := map[string]string{"main": "", "mode": "1\n", "target": "3\n1\n", "max consent": "1\n3\n", "execution": "1\n1\n", "post result": "1\n1\n"}
			if input := inputs[stage]; input != "" {
				if _, err := io.WriteString(writer, input); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-reached:
			case <-time.After(3 * time.Second):
				t.Fatal("prompt not reached")
			}
			cancel() // signal.NotifyContext delivers Ctrl+C through this same context.
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancellation stuck")
			}
		})
	}
}

type promptObserver struct {
	stage   string
	reached chan struct{}
}

func (p *promptObserver) Write(b []byte) (int, error) {
	targets := map[string]string{"main": "Select Benchmark Component:", "mode": "Select Benchmark Mode:", "target": "Existing benchmark directory", "max consent": "Continue?", "post result": "Back to Main Menu"}
	if target := targets[p.stage]; target != "" && strings.Contains(string(b), target) {
		select {
		case p.reached <- struct{}{}:
		default:
		}
	}
	return len(b), nil
}
func TestComponentDirectRouting(t *testing.T) {
	for _, component := range []string{"cpu", "ram", "disk", "network"} {
		for _, mode := range []string{"quick", "standard", "max"} {
			out, stderr, err := invokeCLI(t, component, mode, "--help")
			if err != nil || stderr != "" || !strings.Contains(out, "--path") || !strings.Contains(out, "--target") {
				t.Fatal(out, stderr, err)
			}
			_, stderr, err = invokeCLI(t, component, mode, "--non-interactive", "--max-workers", "0")
			if err == nil || !strings.Contains(stderr, "safety limit") {
				t.Fatal(stderr, err)
			}
		}
	}
	for _, component := range []string{"disk", "network"} {
		for _, flags := range [][]string{nil, {"--non-interactive"}} {
			_, stderr, err := invokeCLI(t, append([]string{component, "quick"}, flags...)...)
			if err == nil || !strings.Contains(stderr, "requires --") {
				t.Fatal(stderr, err)
			}
		}
	}
	_, stderr, err := invokeCLI(t, "cpu", "max")
	if err == nil || !strings.Contains(stderr, "requires --ack-max") {
		t.Fatal(stderr, err)
	}
	for _, peer := range []string{"bad", "127.0.0.1", "::1", "0.0.0.0", "224.0.0.1"} {
		_, stderr, err := invokeCLI(t, "network", "quick", "--target", peer, "--non-interactive")
		if err == nil || !strings.Contains(stderr, "remote IP") {
			t.Fatal(stderr, err)
		}
	}
}
