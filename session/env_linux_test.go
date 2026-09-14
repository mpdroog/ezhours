//go:build linux

package session

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestUnquote(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`:0`, `:0`},
		{`/home/mp/.Xauthority`, `/home/mp/.Xauthority`},
		{`"/home/my user/.Xauthority"`, `/home/my user/.Xauthority`},
		{`"a \"quoted\" path"`, `a "quoted" path`},
		{`"`, `"`},
		{``, ``},
	} {
		if got := unquote(tc.in); got != tc.want {
			t.Errorf("unquote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestDesktopEnvHasDisplay covers the normal case: launched from the session,
// we already have a display and nothing is added to the child's environment.
func TestDesktopEnvHasDisplay(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	if got := desktopEnv(); got != nil {
		t.Errorf("desktopEnv() = %v, want nil", got)
	}
}

// TestDesktopEnvAdoptsDisplay is the bug this package exists for: started by the
// user manager before the session published DISPLAY, we have none, and have to
// take it from the manager instead.
func TestDesktopEnvAdoptsDisplay(t *testing.T) {
	env, err := managerEnv()
	if err != nil {
		t.Skipf("no systemd user manager to read: %v", err)
	}
	want := env["DISPLAY"]
	if want == "" {
		t.Skip("no DISPLAY in the systemd user manager to adopt")
	}

	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	resetLookup()

	got := desktopEnv()
	if !contains(got, "DISPLAY="+want) {
		t.Fatalf("desktopEnv() = %v, want it to carry DISPLAY=%s", got, want)
	}
	// It is copied into our own environment, so the next call short-circuits.
	if os.Getenv("DISPLAY") != want {
		t.Fatalf("DISPLAY = %q, want %q", os.Getenv("DISPLAY"), want)
	}
	if again := desktopEnv(); again != nil {
		t.Fatalf("second desktopEnv() = %v, want nil", again)
	}

	// And a command built now can reach the display.
	if _, err := exec.LookPath("xdotool"); err != nil {
		t.Skip("xdotool not installed")
	}
	t.Setenv("DISPLAY", "")
	resetLookup()
	out, err := Output("xdotool", "getactivewindow")
	if err != nil {
		t.Fatalf("xdotool through Output: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("xdotool returned no window")
	}
}

// resetLookup forgets what a previous call worked out, so a test starts from
// the state the app is in at login.
func resetLookup() {
	mu.Lock()
	lastTry = time.Time{}
	lastErr = ""
	mu.Unlock()
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
