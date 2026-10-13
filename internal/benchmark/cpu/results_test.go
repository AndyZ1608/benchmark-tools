package cpu

import "testing"

func TestNativeValidation(t *testing.T) {
	for _, s := range []string{`{}`, `{"kernel":"compute-bench-v1","workers":1,"measurements":[{"value":1,"elapsed_seconds":1,"integrity_errors":1}]}`} {
		if _, e := Parse([]byte(s)); e == nil {
			t.Fatal(s)
		}
	}
	r, e := Parse([]byte(`{"kernel":"compute-bench-v1","workers":1,"measurements":[{"workload":"integer-xorshift64","unit":"defined_iterations/s","value":10,"elapsed_seconds":1}]}`))
	if e != nil || r.Measurements[0].Value != 10 {
		t.Fatal(r, e)
	}
}
