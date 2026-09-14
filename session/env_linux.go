//go:build linux

package session

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// sessionVars are what a helper needs from the session. The display is the part
// without which nothing runs at all; the four after it are how xdg-open decides
// which file manager to open the hours folder in -- lacking them it falls back
// to a generic guess and can end up opening nothing. XDG_RUNTIME_DIR and the
// bus address are not here: systemd sets those before any unit starts, so we
// always have them already.
var sessionVars = []string{
	"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY",
	"XDG_CURRENT_DESKTOP", "XDG_SESSION_TYPE", "XDG_DATA_DIRS", "XDG_CONFIG_DIRS",
}

// retryEvery rate-limits the lookup while it keeps failing. The app tracker
// asks every two seconds, and a machine outside a graphical session will never
// answer; forking a systemctl per poll to hear that again is waste.
const retryEvery = 10 * time.Second

var (
	mu      sync.Mutex
	lastTry time.Time
	// lastErr is the failure already in the log. desktopEnv retries forever on
	// a machine that will never answer, and the same sentence every ten seconds
	// buries everything else in the journal -- so report each distinct failure
	// once, and again only if it changes.
	lastErr string
)

// desktopEnv returns the display variables we are missing, as KEY=VALUE, or nil
// when our own environment already has them.
//
// Started from ~/.config/systemd/user, ezhours is one of the first things login
// runs -- usually before the session pushes DISPLAY and XAUTHORITY into the user
// manager, and a service keeps the environment it was forked with. The tray
// still appears, because that is D-Bus and the bus address is set from the
// start, so the app looks healthy while every helper it runs exits immediately
// with "cannot open display": no save dialog, no file manager, no app tracking.
//
// The manager's own environment does have them by the time anyone clicks
// anything, so ask it then. What comes back is copied into our environment, so
// this runs once and later calls return at the check above -- and anything else
// in the process that reads DISPLAY sees it too.
func desktopEnv() []string {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return nil
	}

	mu.Lock()
	defer mu.Unlock()
	// Another goroutine may have filled these in while we waited for the lock.
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return nil
	}
	if !lastTry.IsZero() && time.Since(lastTry) < retryEvery {
		return nil
	}
	lastTry = time.Now()

	env, err := managerEnv()
	if err != nil {
		if msg := err.Error(); msg != lastErr {
			lastErr = msg
			log.Printf("session: no display of our own and cannot read the user manager: %v", err)
		}
		return nil
	}
	lastErr = ""

	var found []string
	for _, k := range sessionVars {
		v, ok := env[k]
		if !ok || v == "" {
			continue
		}
		found = append(found, k+"="+v)
		if err := os.Setenv(k, v); err != nil {
			log.Printf("session: set %s: %v", k, err)
		}
	}
	if found != nil {
		log.Printf("session: no display of our own, adopted %d session variables from the user manager", len(found))
	}
	return found
}

// managerEnv reads the environment block of the systemd user manager, which is
// where the session publishes DISPLAY (XFCE and friends do it by running
// dbus-update-activation-environment --systemd at login).
//
// This is the one place that cannot use Output: Command calls desktopEnv, which
// calls this.
func managerEnv() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "--user", "show-environment")
	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if msg := compact(stderr.String()); msg != "" {
			return nil, fmt.Errorf("systemctl --user show-environment: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("systemctl --user show-environment: %w", err)
	}

	env := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		env[k] = unquote(v)
	}
	return env, nil
}

// unquote undoes the shell quoting systemd puts on values that need it. Nothing
// we read here normally does, but a path with a space in it would.
func unquote(v string) string {
	if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
		return v
	}
	v = v[1 : len(v)-1]
	return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(v)
}
