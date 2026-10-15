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

func TestAsyncEngineRequiresCompilationAndPermission(t *testing.T) {
	for _, c := range []struct {
		compiled         string
		uring, aio, want bool
	}{{"io_uring", false, true, false}, {"io_uring libaio", false, true, true}, {"libaio", true, false, false}, {"io_uring libaio", true, false, true}, {"sync", true, true, false}} {
		if got := usableAsync(c.compiled, c.uring, c.aio); got != c.want {
			t.Fatalf("%+v: got %v", c, got)
		}
	}
}
