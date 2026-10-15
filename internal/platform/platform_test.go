package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestRejectsTamperingBeforeExecution(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "release-manifest.json"), []byte(`{"schema":1,"files":{"vmbench":{"sha256":"invalid"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vmbench"), []byte("changed"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := verifyManifest(root); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatal(err)
	}
}

func TestReleaseIsData(t *testing.T) {
	got := ParseRelease("ID=rocky\nID_LIKE=\"rhel centos fedora\"\nVERSION_ID='9.5'\nPRETTY_NAME=\"$(touch /unsafe)\"\nUNKNOWN=ignored\n")
	if got["ID"] != "rocky" || got["VERSION_ID"] != "9.5" || got["PRETTY_NAME"] != "$(touch /unsafe)" || len(got) != 4 {
		t.Fatal(got)
	}
	if len(ParseRelease("missing\n#comment\n")) != 0 {
		t.Fatal("incomplete metadata")
	}
}
func TestInspectRejectsForeignELF(t *testing.T) {
	p := filepath.Join(t.TempDir(), "engine")
	b := make([]byte, 64)
	copy(b, "\x7fELF")
	b[4] = 2
	b[5] = 1
	b[6] = 1
	b[18] = 183
	if e := os.WriteFile(p, b, 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := Inspect(p, "amd64"); e == nil {
		t.Fatal("foreign ELF accepted")
	}
	if _, e := Inspect(p, "riscv64"); e == nil {
		t.Fatal("unsupported arch accepted")
	}
}

func TestAsyncCandidateStatuses(t *testing.T) {
	for _, test := range []struct {
		names        string
		uring, aio   bool
		wantU, wantA string
	}{
		{"io_uring libaio", true, true, AvailableNotValidated, AvailableNotValidated},
		{"io_uring libaio", false, true, Unavailable, AvailableNotValidated},
		{"sync", true, true, Unavailable, Unavailable},
		{"io_uring", false, true, Unavailable, Unavailable},
	} {
		caps := AsyncCapabilities(test.names, test.uring, "operation not permitted", test.aio, "setup only")
		if caps[0].Status != test.wantU || caps[1].Status != test.wantA {
			t.Fatal(test, caps)
		}
		for _, c := range caps {
			if c.Status == Ready {
				t.Fatal("setup probe claimed completed I/O", c)
			}
		}
	}
	for input, want := range map[string]string{"0": "other runtime restrictions", "1": "CAP_SYS_ADMIN", "2": "disabled for all", "invalid": "unrecognized"} {
		if got := DescribeUringPolicy(input, "1000"); !strings.Contains(got, want) {
			t.Fatal(input, got)
		}
	}
}
