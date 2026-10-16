package auto

import (
	"context"
	"strings"
	"testing"
)

func TestDiskMAXCompletesAllBoundedDepthsBeforeConfirmation(t *testing.T) {
	o := Defaults("max")
	o.Component, o.DiskPath, o.AckMax = "disk", "/fixture", true
	o.MaxJobs, o.MaxOutstanding = 1, 64
	o.FIODiagnostics = ".vmbench/failed-fio"
	f := &fakeBackend{}
	r, err := Run(context.Background(), o, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"randread-4k", "seqread-1m"} {
		seen := map[int]bool{}
		for _, c := range f.configs {
			if c.Profile == profile {
				seen[c.Depth] = true
			}
			if c.FIODiagnostics != o.FIODiagnostics {
				t.Fatal("diagnostic option lost", c)
			}
		}
		for _, depth := range []int{1, 2, 4, 8, 16, 32, 64} {
			if !seen[depth] {
				t.Fatal("Disk MAX stopped early", profile, depth, seen)
			}
		}
	}
	for _, peak := range r.Disk {
		if !peak.Confirmed {
			t.Fatal("stable complete search unconfirmed", peak)
		}
	}
}

func TestDiskMAXIncompleteDepthSweepIsNeverConfirmed(t *testing.T) {
	for _, depth := range []int{2, 8, 64} {
		o := Defaults("max")
		o.Component, o.DiskPath, o.AckMax = "disk", "/fixture", true
		f := &fakeBackend{failDiskDepth: depth}
		r, err := Run(context.Background(), o, f, nil)
		if err == nil || r.Status != "FAILED" || !strings.Contains(err.Error(), "fio completion failure") {
			t.Fatal(r, err)
		}
		for _, peak := range r.Disk {
			if peak.Confirmed {
				t.Fatal("incomplete search marked confirmed", peak)
			}
		}
		if f.opened != 1 || f.closed != 1 || !r.DiskCleaned {
			t.Fatal("failure lost safe cleanup", f.opened, f.closed, r.DiskCleaned)
		}
	}
}

func TestAccountingDetailsDoNotExpandNormalDiskOutput(t *testing.T) {
	for _, details := range []bool{false, true} {
		o := Defaults("quick")
		o.Component = "disk"
		o.DiskPath = "/fixture"
		o.Details = details
		r, err := Run(context.Background(), o, &fakeBackend{diskAccounting: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, peak := range r.Disk {
			evidence := peak.Best.Samples[0].Evidence
			if strings.Contains(evidence, "difference=+4096") != details {
				t.Fatal(details, evidence)
			}
		}
	}
}
