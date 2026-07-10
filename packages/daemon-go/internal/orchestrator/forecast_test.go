package orchestrator

import "testing"

func TestWillLikelyBreach(t *testing.T) {
	cases := []struct {
		name      string
		remaining int64
		lastRun   float64
		safety    float64
		want      bool
	}{
		{"plenty left", 10000, 1000, 1.0, false},
		{"exactly enough", 1000, 1000, 1.0, false}, // not strictly less
		{"not enough for another run", 800, 1000, 1.0, true},
		{"safety margin trips it", 1200, 1000, 1.5, true},
		{"unknown remaining", -1, 1000, 1.0, false},
		{"no burn estimate yet", 50, 0, 1.0, false},
	}
	for _, c := range cases {
		if got := willLikelyBreach(c.remaining, c.lastRun, c.safety); got != c.want {
			t.Errorf("%s: willLikelyBreach(%d,%.0f,%.1f)=%v want %v",
				c.name, c.remaining, c.lastRun, c.safety, got, c.want)
		}
	}
}
