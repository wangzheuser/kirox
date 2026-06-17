package http

import "testing"

func TestChromeProfileKeyFromVersion(t *testing.T) {
	cases := map[string]string{
		"144.0.0.0": "chrome_144",
		"133.0.0.0": "chrome_133",
		"131.0.0.0": "chrome_131",
		"124.0.0.0": "chrome_124",
		"120.0.0.0": "chrome_120",
		"140.0.0.0": "chrome_144",
		"":          "chrome_144",
	}
	for input, want := range cases {
		if got := chromeProfileKeyFromVersion(input); got != want {
			t.Fatalf("chromeProfileKeyFromVersion(%q)=%q, want %q", input, got, want)
		}
	}
}
