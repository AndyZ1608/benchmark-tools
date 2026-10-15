package disk

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"vmbench/internal/safety"
)

func TestSessionReusesPrivateDataAcrossSweeps(t *testing.T) {
	fixtureEngine(t)
	c := diskConfig(t)
	c.FileSize = "1MiB"
	sentinel := filepath.Join(c.Path, "user-file")
	os.WriteFile(sentinel, []byte("unchanged"), 0600)
	s, e := OpenSession(context.Background(), c, safety.ReserveBytes)
	if e != nil {
		t.Fatal(e)
	}
	workspace := s.Lifecycle.Workspace
	info, e := os.Stat(filepath.Join(workspace, "job-001.dat"))
	if e != nil {
		t.Fatal(e)
	}
	for _, jobs := range []int{1, 2, 4} {
		for _, profile := range []string{"randread-4k", "seqread-1m"} {
			current := c
			current.Jobs = jobs
			current.Profile = profile
			out, e := s.Measure(context.Background(), current)
			if e != nil || len(out.Results) != 1 || len(out.Results[0].Jobs) != jobs {
				t.Fatal(out, e)
			}
			if _, e = ExecutionEvidence(out.Results[0], c.Engine, c.Depth); e != nil {
				t.Fatal(e)
			}
			after, e := os.Stat(filepath.Join(workspace, "job-001.dat"))
			if e != nil || !os.SameFile(info, after) {
				t.Fatal("dataset recreated", e)
			}
		}
	}
	if e = s.Close(); e != nil || !s.Lifecycle.CleanupCompleted {
		t.Fatal(e)
	}
	if _, e = os.Stat(workspace); !os.IsNotExist(e) {
		t.Fatal("workspace remains", e)
	}
	if data, e := os.ReadFile(sentinel); e != nil || string(data) != "unchanged" {
		t.Fatal("user file touched", e)
	}
}
