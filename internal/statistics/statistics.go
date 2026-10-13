package statistics

import (
	"fmt"
	"math"
	"sort"
)

type Summary struct {
	Count                              int `json:"count"`
	Mean, Median, Min, Max, StdDev, CV float64
}

func Percentile(v []float64, p float64) (float64, error) {
	if len(v) == 0 || p < 0 || p > 100 {
		return 0, fmt.Errorf("invalid percentile observations")
	}
	a := append([]float64(nil), v...)
	for _, x := range a {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, fmt.Errorf("non-finite observation")
		}
	}
	sort.Float64s(a)
	i := p / 100 * float64(len(a)-1)
	lo := int(i)
	hi := int(math.Ceil(i))
	return a[lo] + (a[hi]-a[lo])*(i-float64(lo)), nil
}
func Describe(v []float64) (Summary, error) {
	m, e := Percentile(v, 50)
	if e != nil {
		return Summary{}, e
	}
	s := Summary{Count: len(v), Median: m, Min: v[0], Max: v[0]}
	for _, x := range v {
		s.Mean += x
		s.Min = math.Min(s.Min, x)
		s.Max = math.Max(s.Max, x)
	}
	s.Mean /= float64(len(v))
	for _, x := range v {
		s.StdDev += (x - s.Mean) * (x - s.Mean)
	}
	if len(v) > 1 {
		s.StdDev = math.Sqrt(s.StdDev / float64(len(v)-1))
	}
	if s.Mean != 0 {
		s.CV = s.StdDev / math.Abs(s.Mean)
	}
	return s, nil
}
