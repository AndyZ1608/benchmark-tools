package report

import (
	"encoding/json"
	"os"
	"testing"
	"vmbench/internal/config"
)

func TestSave(t *testing.T) {
	c, _ := config.Defaults("quick")
	c.Output = t.TempDir()
	r := New("cpu test", c)
	r.Results["actual"] = 12.5
	p, e := r.Save()
	if e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	var x Report
	if e = json.Unmarshal(b, &x); e != nil || x.ID != r.ID || x.Settings.Duration != 2 {
		t.Fatal(x, e)
	}
	if _, e = r.Save(); e == nil {
		t.Fatal("overwrote report")
	}
}
