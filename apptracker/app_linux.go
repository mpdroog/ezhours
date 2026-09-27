package apptracker

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/mpdroog/osxflow/wl"

	"github.com/mpdroog/ezhours/session"
)

// wmClassRe matches the quoted values of `xprop WM_CLASS`, which reports
// instance and class: WM_CLASS(STRING) = "Navigator", "firefox"
var wmClassRe = regexp.MustCompile(`"([^"]*)"`)

// getActiveApp returns the name of the currently active application on Linux.
// The window class is preferred over the title, because titles change per
// browser tab, per shell command and even per spinner frame, which would
// fragment a single app into dozens of entries.
//
// An empty name with no error means there is nothing to report right now -- no
// window is focused, or the one that is names itself nothing. A tracker that
// cannot see the display at all is an error, and the caller logs it.
//
// Under Wayland this must not go through xdotool and xprop. They do not fail
// there, which is the problem: Xwayland answers them happily while seeing only
// its own clients, so every native Wayland window comes back as no window at
// all and the day's tracking quietly records the two or three X11 apps left.
// The compositor's own answer covers both kinds, because an Xwayland window is
// a toplevel to it like any other.
func getActiveApp() (string, error) {
	if onWayland() {
		return activeAppWayland()
	}
	return activeAppX11()
}

func onWayland() bool { return os.Getenv("WAYLAND_DISPLAY") != "" }

var (
	compMu sync.Mutex
	comp   *wl.Conn
)

// compositor is the connection to the compositor, opened once and kept. The
// toplevel list is built from events, so a fresh connection per poll would pay
// two roundtrips every two seconds to learn what this one already knows.
func compositor() (*wl.Conn, error) {
	compMu.Lock()
	defer compMu.Unlock()
	if comp != nil {
		if err := comp.Err(); err != nil {
			// The compositor went away -- a restart, or the session ending.
			// Drop it so the next poll dials again rather than reporting the
			// same corpse forever.
			comp = nil
			return nil, fmt.Errorf("compositor connection lost: %w", err)
		}
		return comp, nil
	}
	c, err := wl.Dial()
	if err != nil {
		return nil, err
	}
	comp = c
	return comp, nil
}

// activeAppWayland asks the compositor directly, over
// zwlr_foreign_toplevel_management_v1.
//
// Nothing here drains the connection's event channel, which is deliberate and
// safe: wl.Conn drops events nobody is reading rather than blocking its read
// loop, and the toplevel state this reads is kept up to date by that loop
// regardless.
func activeAppWayland() (string, error) {
	c, err := compositor()
	if err != nil {
		return "", err
	}
	t, ok := c.Active()
	if !ok {
		// Focus is on the desktop, a panel, or nothing.
		return "", nil
	}
	// AppID is the Wayland equivalent of WM_CLASS and is the better key for
	// the same reason: it names the application, not the document.
	if s := appIDName(t.AppID); s != "" {
		return prettify(s), nil
	}
	return titleToApp(t.Title), nil
}

// appIDName reduces a Wayland app_id to something that reads like the name of
// an application.
//
// app_ids are commonly reverse-DNS -- "org.mozilla.firefox", and every flatpak
// on this desk -- where the last component is the application and everything
// before it is the vendor. WM_CLASS never looked like that, so the X11 path
// had no equivalent of this and recorded "Firefox" where an untrimmed app_id
// would give "Org.mozilla.firefox" and split one app across two names.
func appIDName(id string) string {
	s := normalize(id)
	if i := strings.LastIndex(s, "."); i != -1 && i+1 < len(s) {
		s = s[i+1:]
	}
	return s
}

// activeAppX11 is the original path, kept for a real X11 session where it is
// still the right answer. It needs xdotool, which a Wayland-only machine has
// no reason to install.
func activeAppX11() (string, error) {
	out, err := session.Output("xdotool", "getactivewindow")
	if err != nil {
		return "", err
	}
	win := strings.TrimSpace(out)
	if win == "" {
		return "", nil
	}

	class, err := windowClass(win)
	if err != nil {
		return "", err
	}
	if class != "" {
		return prettify(class), nil
	}

	// Fall back to the title for windows that expose no class.
	out, err = session.Output("xdotool", "getwindowname", win)
	if err != nil {
		return "", err
	}
	return titleToApp(out), nil
}

// windowClass returns the WM_CLASS class of a window. A window that carries no
// class is "" with no error: plenty do not, which is what the title fallback
// above is for.
func windowClass(win string) (string, error) {
	out, err := session.Output("xprop", "-id", win, "WM_CLASS")
	if err != nil {
		return "", fmt.Errorf("window %s: %w", win, err)
	}
	m := wmClassRe.FindAllStringSubmatch(out, -1)
	if len(m) == 0 {
		return "", nil
	}
	// Last value is the class ("firefox"); the first is the instance.
	return normalize(m[len(m)-1][1]), nil
}
