package disk

import "testing"

func TestFioEngineAndAchievedDepthEvidence(t *testing.T) {
	r := Result{Options: map[string]string{"ioengine": "libaio"}, Jobs: []Job{{Depth: map[string]float64{"1": 90, "32": 10}}}}
	evidence, e := ExecutionEvidence(r, "libaio", 32)
	if e != nil || evidence == "" {
		t.Fatal(evidence, e)
	}
	r.Options["ioengine"] = "sync"
	if _, e = ExecutionEvidence(r, "libaio", 32); e == nil {
		t.Fatal("engine mismatch accepted")
	}
	r.Options["ioengine"] = "libaio"
	r.Jobs[0].Depth = nil
	if _, e = ExecutionEvidence(r, "libaio", 32); e == nil {
		t.Fatal("missing achieved depth accepted")
	}
	r.Jobs[0].Depth = map[string]float64{"32": 101}
	if _, e = ExecutionEvidence(r, "libaio", 32); e == nil {
		t.Fatal("invalid depth accepted")
	}
}
