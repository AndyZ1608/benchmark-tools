package statistics

import (
	"math"
	"testing"
)

func TestSummary(t *testing.T) {
	s, e := Describe([]float64{4, 1, 3, 2})
	if e != nil || s.Mean != 2.5 || s.Median != 2.5 || s.Min != 1 || s.Max != 4 || math.Abs(s.StdDev-math.Sqrt(5.0/3)) > 1e-12 {
		t.Fatalf("%+v %v", s, e)
	}
	p, e := Percentile([]float64{0, 10}, 95)
	if e != nil || p != 9.5 {
		t.Fatal(p, e)
	}
}
func TestInvalid(t *testing.T) {
	for _, v := range [][]float64{nil, {math.NaN()}, {math.Inf(1)}} {
		if _, e := Describe(v); e == nil {
			t.Fatal("accepted invalid observations")
		}
	}
}
