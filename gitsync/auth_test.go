package gitsync

import "testing"

func TestIsSSHURL(t *testing.T) {
	for url, want := range map[string]bool{
		"git@github.com:mpdroog/hours.git":       true,
		"ssh://git@github.com/mpdroog/hours.git": true,
		"https://github.com/mpdroog/hours.git":   false,
		"/srv/git/hours.git":                     false,
		"file:///srv/git/hours.git":              false,
	} {
		if got := isSSHURL(url); got != want {
			t.Errorf("isSSHURL(%q) = %v, want %v", url, got, want)
		}
	}
}
