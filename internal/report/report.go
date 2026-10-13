package report

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
	"vmbench/internal/config"
	"vmbench/internal/system"
)

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
	return Report{ID: hex.EncodeToString(b), Version: "1.0.0-dev", Command: command, Status: "running", Started: time.Now().UTC(), Settings: c, System: system.Detect(), Results: map[string]any{}, Warnings: []string{}, Errors: []string{}}
}
func (r *Report) Save() (string, error) {
	if e := os.MkdirAll(r.Settings.Output, 0755); e != nil {
		return "", e
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return "", e
	}
	p := filepath.Join(r.Settings.Output, r.ID+".json")
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if e != nil {
		return "", e
	}
	_, e = f.Write(b)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return p, e
}
