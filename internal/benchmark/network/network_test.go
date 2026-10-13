package network

import "testing"

func TestParse(t *testing.T) {
	r, e := Parse([]byte(`{"end":{"sum":{"bits_per_second":123,"jitter_ms":0.2,"lost_percent":1}}}`))
	if e != nil || len(r.End) == 0 {
		t.Fatal(r, e)
	}
	if _, e = Parse([]byte(`{"error":"connection refused"}`)); e == nil {
		t.Fatal("ignored engine error")
	}
}
