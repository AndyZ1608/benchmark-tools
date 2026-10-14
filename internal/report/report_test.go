package report

import (
	"testing"
	"vmbench/internal/config"
	"vmbench/internal/version"
)

func TestUsesApplicationVersion(t *testing.T) {
	old := version.Value
	defer func() { version.Value = old }()
	version.Value = "9.8.7-report-test"
	c, _ := config.Defaults("quick")
	if got := New("system info", c).Version; got != version.Current() {
		t.Fatalf("report version %q != application %q", got, version.Current())
	}
}
