package bindings

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.5.0", "0.4.0", 1},
		{"0.4.0", "0.4.0", 0},
		{"0.4.0", "0.5.0", -1},
		{"v1.0.0", "0.9.9", 1},
		{"1.10.0", "1.9.0", 1},
		{"0.4", "0.4.0", 0},
		{"0.5.0-rc.1", "0.4.9", 1},
		{"0.4.0+build", "0.4.0", 0},
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		sign := 0
		if got > 0 {
			sign = 1
		} else if got < 0 {
			sign = -1
		}
		if sign != c.want {
			t.Errorf("compareVersions(%q,%q) = %d, attendu signe %d", c.a, c.b, got, c.want)
		}
	}
}
