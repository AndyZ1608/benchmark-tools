package auto

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"vmbench/internal/system"
)

func TestDiskLatencyWarningOptionOnlyChangesPresentation(t *testing.T) {
	config := filepath.Join(t.TempDir(), "auto.yaml")
	if err := os.WriteFile(config, []byte("disk-p99-warning-ms: 2000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"quick", "standard", "max"} {
		base, err := ParseComponent("disk", mode, []string{"--config", config})
		if err != nil || base.DiskP99WarningMS != 2000 {
			t.Fatal(base, err)
		}
		changed, err := ParseComponent("disk", mode, []string{"--config", config, "--disk-p99-warning-ms", "500"})
		if err != nil || changed.DiskP99WarningMS != 500 {
			t.Fatal(changed, err)
		}
		resources := system.Info{Capacity: 4, AvailableMemory: 8 << 30}
		if base.Policy() != changed.Policy() || !reflect.DeepEqual(MakePlan(base, resources), MakePlan(changed, resources)) {
			t.Fatal("presentation threshold changed benchmark policy")
		}
		changed.DiskP99WarningMS = base.DiskP99WarningMS
		if !reflect.DeepEqual(base, changed) {
			t.Fatal("unexpected workload option change")
		}
	}
	for _, value := range []string{"-1", "NaN", "+Inf", "-Inf"} {
		_, err := Parse("quick", []string{"--config", config, "--disk-p99-warning-ms", value})
		if err == nil || !strings.Contains(err.Error(), "finite and nonnegative") {
			t.Fatalf("%s: %v", value, err)
		}
	}
	options, err := Parse("quick", []string{"--config", config, "--disk-p99-warning-ms", "0"})
	if err != nil || options.DiskP99WarningMS != 0 {
		t.Fatal(options, err)
	}
}
