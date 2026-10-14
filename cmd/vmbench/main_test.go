package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"vmbench/internal/config"
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
			if len(os.Args) > 1 && os.Args[1] == "fixture-summary" {
				c, _ := config.Defaults("quick")
				r := report.New("cpu test", c)
				r.Status = "completed"
				r.DurationSeconds = 12
				r.Results["execution_status"] = map[string]string{"cpu test": "completed"}
				r.Results["cpu test"] = []json.RawMessage{json.RawMessage(`{"kernel":"compute-bench-v1","workers":1,"measurements":[{"workload":"integer-xorshift64","unit":"defined_iterations/s","value":100,"elapsed_seconds":2,"integrity_errors":0},{"workload":"fp-multiply-add","unit":"defined_iterations/s","value":50,"elapsed_seconds":2,"integrity_errors":0}]}`)}
				var cause error
				if len(os.Args) > 2 {
					r.Status = "failed"
					r.Results["execution_status"] = map[string]string{"cpu test": "failed"}
					cause = fmt.Errorf("fixture engine failure")
					r.Errors = append(r.Errors, cause.Error())
				}
				if err := finishReport(r, cause); err != nil {
					fmt.Fprintln(os.Stderr, "ERROR:", err)
					os.Exit(1)
				}
				os.Exit(0)
			}
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func invokeCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return invokeCLIInDir(t, "", args...)
}
func invokeCLIInDir(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "VMBENCH_TEST_CLI_PROCESS=1", "NO_COLOR=1")
	cmd.Dir = dir
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
	if err != nil || stderr != "" || json.Valid([]byte(out)) || !strings.Contains(out, "VMBENCH SYSTEM INFO") || !strings.Contains(out, "Architecture") {
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

func TestFinalSummaryDoesNotWriteReports(t *testing.T) {
	for _, failure := range []bool{false, true} {
		dir := t.TempDir()
		historical := filepath.Join(dir, "results", "historical.json")
		if err := os.MkdirAll(filepath.Dir(historical), 0755); err != nil {
			t.Fatal(err)
		}
		original := []byte("historical user result - preserve exactly\n")
		if err := os.WriteFile(historical, original, 0644); err != nil {
			t.Fatal(err)
		}
		args := []string{"fixture-summary"}
		if failure {
			args = append(args, "failed")
		}
		out, stderr, err := invokeCLIInDir(t, dir, args...)
		if (err != nil) != failure || json.Valid([]byte(out)) || strings.Contains(out, "Full report") || strings.Contains(out, "Report saved") {
			t.Fatal(out, stderr, err)
		}
		if !failure && !strings.Contains(out, "Status: COMPLETED") {
			t.Fatal(out)
		}
		if failure && (!strings.Contains(out, "Status: FAILED") || !strings.Contains(stderr, "fixture engine failure")) {
			t.Fatal(out, stderr)
		}
		entries, e := os.ReadDir(filepath.Dir(historical))
		if e != nil || len(entries) != 1 {
			t.Fatal(entries, e)
		}
		data, e := os.ReadFile(historical)
		if e != nil || !bytes.Equal(data, original) {
			t.Fatal("historical file changed", e)
		}
		entries, e = os.ReadDir(dir)
		if e != nil || len(entries) != 1 {
			t.Fatal("unexpected automatic files", entries, e)
		}
	}
	dir := t.TempDir()
	out, stderr, err := invokeCLIInDir(t, dir, "fixture-summary")
	entries, e := os.ReadDir(dir)
	if err != nil || e != nil || len(entries) != 0 {
		t.Fatal(out, stderr, err, entries, e)
	}
}
func TestRemovedOptionsFailWithoutCreatingResults(t *testing.T) {
	for _, flags := range [][]string{{"--json"}, {"--output", "results"}, {"--lang", "vi"}, {"--lang", "en"}} {
		dir := t.TempDir()
		args := append([]string{"cpu", "test"}, flags...)
		out, stderr, err := invokeCLIInDir(t, dir, args...)
		entries, e := os.ReadDir(dir)
		if err == nil || out != "" || !strings.Contains(stderr, "flag provided but not defined") || e != nil || len(entries) != 0 {
			t.Fatal(out, stderr, err, entries, e)
		}
	}
}
func TestHelpIsASCIIAndAdvertisesSupportedOptions(t *testing.T) {
	out, _, err := invokeCLI(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range out {
		if c > 127 {
			t.Fatalf("non-ASCII help character %q", c)
		}
	}
	for _, flag := range []string{"--json", "--lang", "--output"} {
		if strings.Contains(out, flag) {
			t.Fatal("obsolete option", out)
		}
	}
	if !strings.Contains(out, "--details") || !strings.Contains(out, "--steal-warning-percent") {
		t.Fatal(out)
	}
}
