package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAutomaticMenu(t *testing.T) {
	for input, expected := range map[string]string{"1\n": "quick", "2\n": "standard", "3\n": "max", "0\n": "", "": "", "wrong\n2\n": "standard"} {
		var b bytes.Buffer
		mode, e := selectMode(strings.NewReader(input), &b)
		if e != nil || mode != expected || !strings.Contains(b.String(), "[3] MAX") || !strings.Contains(b.String(), "[0] Exit") {
			t.Fatal(mode, e, b.String())
		}
	}
}
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
