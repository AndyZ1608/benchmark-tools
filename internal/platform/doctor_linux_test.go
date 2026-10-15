package platform

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("VMBENCH_DOCTOR_HELPER") == "1" {
		name := filepath.Base(os.Args[0])
		arg := os.Args[1]
		if name == "fio" && os.Getenv("VMBENCH_DOCTOR_MODE") == "broken" {
			fmt.Fprintln(os.Stderr, "error while loading shared libraries: libfixture.so")
			os.Exit(127)
		}
		if arg == "--version" {
			fmt.Println(map[string]string{"compute-bench": "compute-bench-v1", "fio": "fio-3.35", "stress-ng": "stress-ng 0.19.03", "iperf3": "iperf 3.9"}[name])
			os.Exit(0)
		}
		if name == "fio" && arg == "--enghelp" {
			if os.Getenv("VMBENCH_DOCTOR_MODE") == "enghelp-fails" {
				fmt.Fprintln(os.Stderr, "engine query failed")
				os.Exit(1)
			}
			if os.Getenv("VMBENCH_DOCTOR_MODE") == "no-async" {
				fmt.Println("sync psync")
			} else {
				fmt.Println("Available IO engines:\n io_uring\n libaio")
			}
			os.Exit(0)
		}
		if name == "iperf3" && arg == "--help" {
			fmt.Println("--json --parallel --omit")
			os.Exit(0)
		}
		os.Exit(99)
	}
	os.Exit(m.Run())
}
func TestDoctorDefersDiskButRetainsExecutableFailures(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.Mkdir(filepath.Join(root, "engines"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"compute-bench", "fio", "stress-ng", "iperf3"} {
		if err = os.WriteFile(filepath.Join(root, "engines", name), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("VMBENCH_DOCTOR_HELPER", "1")
	for _, mode := range []string{"", "no-async", "broken", "enghelp-fails"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("VMBENCH_DOCTOR_MODE", mode)
			var out bytes.Buffer
			err := doctor(context.Background(), &out, root, false, "operation not permitted; io_uring_disabled=2", true, "setup/destruction succeeded; no I/O submitted")
			fatal := mode == "broken" || mode == "enghelp-fails"
			if (err != nil) != fatal {
				t.Fatal(mode, out.String(), err)
			}
			if !fatal {
				for _, want := range []string{"CPU engine: READY", "Memory engine: READY", "fio: READY", "io_uring: UNAVAILABLE", "Disk validation deferred"} {
					if !strings.Contains(out.String(), want) {
						t.Fatal(want, out.String())
					}
				}
			}
			if mode == "" && !strings.Contains(out.String(), "libaio: AVAILABLE_NOT_VALIDATED") {
				t.Fatal(out.String())
			}
			if mode == "no-async" && !strings.Contains(out.String(), "libaio: UNAVAILABLE") {
				t.Fatal(out.String())
			}
			if fatal && !strings.Contains(out.String(), "FAILED") {
				t.Fatal(out.String())
			}
		})
	}
}
