package report

import (
	"crypto/rand"
	"encoding/hex"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/system"
	"vmbench/internal/version"
)

// Report holds observations only in memory. There is no persistence or export API.
type Report struct {
	ID, Version, Command, Status string
	Started                      time.Time
	DurationSeconds              float64
	Settings                     config.Config
	System                       system.Info
	Results                      map[string]any
	Warnings, Errors             []string
}

func New(command string, c config.Config) Report {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return Report{ID: hex.EncodeToString(b), Version: version.Current(), Command: command, Status: "running", Started: time.Now().UTC(), Settings: c, System: system.Detect(), Results: map[string]any{}, Warnings: []string{}, Errors: []string{}}
}
