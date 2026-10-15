package network

import (
	"strings"
	"testing"
	"vmbench/internal/config"
)

func TestParse(t *testing.T) {
	r, e := Parse([]byte(`{"end":{"sum":{"bits_per_second":123,"jitter_ms":0.2,"lost_percent":1}}}`))
	if e != nil || len(r.End) == 0 {
		t.Fatal(r, e)
	}
	if _, e = Parse([]byte(`{"error":"connection refused"}`)); e == nil {
		t.Fatal("ignored engine error")
	}
}

func TestRejectsMissingRequestedIperfFeature(t *testing.T) {
	base := "--json --parallel --omit --reverse --bitrate"
	if err := validateOptions(base, config.Config{Reverse: true, TCPBitrate: "10M"}); err != nil {
		t.Fatal(err)
	}
	if err := validateOptions(base, config.Config{Bidir: true}); err == nil || !strings.Contains(err.Error(), "--bidir") {
		t.Fatal(err)
	}
	if err := validateOptions("--json", config.Config{}); err == nil {
		t.Fatal("accepted missing parallel/omit capabilities")
	}
}

func TestLegacyIperfBandwidthAliasIsCompatible(t *testing.T) {
	help := "-J, --json -P, --parallel -O, --omit -b, --bandwidth"
	if err := validateOptions(help, config.Config{TCPBitrate: "10M"}); err != nil {
		t.Fatal(err)
	}
	if err := validateOptions("--json-format --parallelism --omitted", config.Config{}); err == nil {
		t.Fatal("accepted substrings instead of options")
	}
}
