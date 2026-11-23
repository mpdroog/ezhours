//go:build linux && !x11

package apptracker

import "testing"

// TestAppIDName covers the shapes the compositor actually reports on this
// desk: a plain app_id, the reverse-DNS ones every flatpak uses, and the
// degenerate inputs that must not panic or produce a stray name.
func TestAppIDName(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"foot", "foot"},
		{"org.mozilla.firefox", "firefox"},
		{"de.schmidhuberj.Flare", "Flare"},
		{"com.rustdesk.RustDesk", "RustDesk"},
		{"Navigator", "Navigator"},
		{"", ""},
		{".", ""},
		{"trailing.", "trailing"},
		{".leading", "leading"},
	} {
		if got := appIDName(tc.in); got != tc.want {
			t.Errorf("appIDName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
