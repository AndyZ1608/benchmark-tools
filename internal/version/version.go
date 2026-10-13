// Package version is the application version source shared by the CLI and reports.
package version

import "strings"

const Development = "1.0.0-dev"

// Value is injected from VERSION with -ldflags "-X vmbench/internal/version.Value=...".
// Plain go builds retain a usable development version.
var Value = Development

func Current() string {
	if value := strings.TrimSpace(Value); value != "" {
		return value
	}
	return Development
}
