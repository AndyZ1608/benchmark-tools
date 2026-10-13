package version

import "testing"

func TestCurrent(t *testing.T) {
	original := Value
	defer func() { Value = original }()
	for _, c := range []struct{ input, want string }{{"9.8.7-test", "9.8.7-test"}, {"", Development}, {" \t\n", Development}} {
		Value = c.input
		if got := Current(); got != c.want {
			t.Fatalf("Current(%q)=%q; want %q", c.input, got, c.want)
		}
	}
}
