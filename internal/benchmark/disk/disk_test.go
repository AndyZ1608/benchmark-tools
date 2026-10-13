package disk

import (
	"os"
	"testing"
)

func TestMixedFixture(t *testing.T) {
	b, e := os.ReadFile("../../../tests/fixtures/fio.json")
	if e != nil {
		t.Fatal(e)
	}
	r, e := Parse(b)
	if e != nil {
		t.Fatal(e)
	}
	j := r.Jobs[0]
	if j.Read.IOPS != 70 || j.Write.IOPS != 30 || j.Read.BWBytes != 286720 || j.Write.BWBytes != 122880 || j.Read.Completion.Percentile["99.000000"] != 20000 || j.Write.Completion.Percentile["99.000000"] != 40000 {
		t.Fatal(j)
	}
}
func TestBadFio(t *testing.T) {
	for _, s := range []string{`{}`, `{"jobs":[{"error":5}]}`, `garbage`} {
		if _, e := Parse([]byte(s)); e == nil {
			t.Fatal(s)
		}
	}
}
