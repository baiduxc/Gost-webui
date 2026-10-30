package model

import "testing"

func TestNormalizeIPMode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", IPModeAuto},
		{"auto", IPModeAuto},
		{"AUTO", IPModeAuto},
		{" ipv4 ", IPModeIPv4},
		{"4", IPModeIPv4},
		{"IPv6", IPModeIPv6},
		{"6", IPModeIPv6},
		{"bogus", IPModeAuto},
	}
	for _, c := range cases {
		if got := NormalizeIPMode(c.in); got != c.want {
			t.Fatalf("NormalizeIPMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIPModeListen(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "::"},
		{"auto", "::"},
		{"ipv4", "0.0.0.0"},
		{"ipv6", "::"},
	}
	for _, c := range cases {
		if got := IPModeListen(c.in); got != c.want {
			t.Fatalf("IPModeListen(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
