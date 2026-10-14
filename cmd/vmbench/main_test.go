package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"vmbench/internal/report"
	"vmbench/internal/version"
)

// Run the actual CLI entry point in an isolated process, preserving exit codes.
func TestCLIProcess(t *testing.T) {
	if os.Getenv("VMBENCH_TEST_CLI_PROCESS") != "1" {
		return
	}
	for i, a := range os.Args {
		if a == "--" {
			os.Args = append([]string{"vmbench"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func invokeCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "VMBENCH_TEST_CLI_PROCESS=1")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	return out.String(), stderr.String(), err
}

func TestVersionAliases(t *testing.T) {
	var expected string
	for _, argument := range []string{"--version", "-v", "version"} {
		t.Run(argument, func(t *testing.T) {
			out, stderr, err := invokeCLI(t, argument)
			if err != nil || stderr != "" {
				t.Fatalf("exit=%v stderr=%q", err, stderr)
			}
			if !strings.HasPrefix(out, "VMBench "+version.Current()+";") {
				t.Fatalf("unexpected version: %q", out)
			}
			if expected != "" && out != expected {
				t.Fatalf("version aliases differ: %q != %q", out, expected)
			}
			expected = out
		})
	}
}

func TestHelp(t *testing.T) {
	out, stderr, err := invokeCLI(t, "--help")
	if err != nil || stderr != "" || !strings.Contains(out, "VMBench "+version.Current()) || !strings.Contains(out, "--version|-v") {
		t.Fatalf("%q %q %v", out, stderr, err)
	}
}

func TestSystemInfoCommand(t *testing.T) {
	out, stderr, err := invokeCLI(t, "system", "info")
	var result map[string]any
	if err != nil || stderr != "" || json.Unmarshal([]byte(out), &result) != nil || result["Architecture"] == nil {
		t.Fatalf("%q %q %v", out, stderr, err)
	}
}

func TestComponentsStillValidateParameters(t *testing.T) {
	for _, command := range []string{"cpu test", "cpu stress", "ram test", "ram stress", "disk test", "network test", "network server", "full test"} {
		t.Run(command, func(t *testing.T) {
			args := append(strings.Fields(command), "--duration", "0")
			_, stderr, err := invokeCLI(t, args...)
			if err == nil || !strings.Contains(stderr, "parameter outside supported limits") {
				t.Fatalf("exit=%v stderr=%q", err, stderr)
			}
		})
	}
}

func TestJSONFailureStdoutAndSavedReport(t *testing.T) {
	dir := t.TempDir()
	out, stderr, err := invokeCLI(t, "cpu", "test", "--json", "--duration", "0", "--output", dir)
	if err == nil {
		t.Fatal("invalid duration succeeded")
	}
	var r report.Report
	if e := json.Unmarshal([]byte(out), &r); e != nil {
		t.Fatalf("stdout is not JSON: %s (%v)", out, e)
	}
	if r.Status != "failed" || len(r.Errors) == 0 || !strings.Contains(stderr, "parameter outside supported limits") {
		t.Fatal(r, stderr)
	}
	files, e := os.ReadDir(dir)
	if e != nil || len(files) != 1 {
		t.Fatal(files, e)
	}
	if strings.Contains(out, "Preparing workload") || strings.Contains(out, "WARNING:") {
		t.Fatal("stdout polluted", out)
	}
}

func TestJSONRejectsNonFinitePresentationThreshold(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf"} {
		t.Run(value, func(t *testing.T) {
			out, stderr, err := invokeCLI(t, "cpu", "test", "--json", "--output", t.TempDir(), "--stability-cv", value)
			var r report.Report
			if err == nil || json.Unmarshal([]byte(out), &r) != nil || r.Status != "failed" || !strings.Contains(stderr, value) {
				t.Fatalf("non-finite threshold did not yield valid failure JSON: %q %q %v", out, stderr, err)
			}
		})
	}
}
