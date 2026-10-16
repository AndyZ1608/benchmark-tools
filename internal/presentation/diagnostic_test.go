package presentation

import (
	"bytes"
	"strings"
	"testing"
	automatic "vmbench/internal/benchmark/auto"
)

func TestStructuredErrorLinesRemainReadableAndSafe(t *testing.T) {
	message := "fio rejected\nRead Bytes      : 123\nJob             : bad\x1b[31m\rname\nByte Difference : +4096"
	clean := SafeLines(message)
	if strings.Count(clean, "\n") != 3 || strings.ContainsAny(clean, "\x1b\r") {
		t.Fatal(clean)
	}
	var output bytes.Buffer
	r := automatic.Result{Mode: "max", Status: "FAILED", Errors: []string{message}, States: map[string]string{}}
	if err := RenderAutomatic(&output, r, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\nRead Bytes      : 123\n") || strings.Contains(output.String(), "\x1b") {
		t.Fatal(output.String())
	}
}
