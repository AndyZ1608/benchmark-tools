package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrecedence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(p, []byte("preset: precision\nduration: 7\nworkers: 2\n"), 0600)
	c, e := Parse([]string{"--config", p, "--preset", "quick", "--duration", "4"})
	if e != nil || c.Duration != 4 || c.Warmup != 1 || c.Workers != 2 {
		t.Fatal(c, e)
	}
}
func TestValidation(t *testing.T) {
	for _, a := range [][]string{{"--workers", "0"}, {"--duration", "-1"}, {"--preset", "unknown"}, {"--profile", "bogus"}, {"--ioengine", "sync"}, {"--size", "0MiB"}, {"--unknown"}, {"extra"}} {
		if _, e := Parse(a); e == nil {
			t.Fatal(a)
		}
	}
}
func TestSizes(t *testing.T) {
	n, e := Bytes("4GiB")
	if e != nil || n != 4<<30 {
		t.Fatal(n, e)
	}
}

func TestPresentationOptions(t *testing.T) {
	c, e := Parse([]string{"--details", "--stability-cv", "10", "--steal-warning-percent", "2"})
	if e != nil || !c.Details || c.StealWarningPercent != 2 || c.StabilityCV != 10 {
		t.Fatal(c, e)
	}
	for _, a := range [][]string{{"--lang", "vi"}, {"--lang", "en"}, {"--json"}, {"--output", "results"}, {"--steal-warning-percent", "NaN"}, {"--steal-warning-percent", "101"}, {"--stability-cv", "NaN"}, {"--stability-cv", "-1"}} {
		if _, e := Parse(a); e == nil {
			t.Fatal(a)
		}
	}
	c, e = Parse(nil)
	if e != nil || c.Details || c.StealWarningPercent != 1 {
		t.Fatal(c, e)
	}
}
