//go:build linux && !x11

package apptracker

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mpdroog/osxflow/wl"
)

func init() { activeAppWayland = activeAppFromCompositor }

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

// activeAppFromCompositor asks the compositor directly, over
// zwlr_foreign_toplevel_management_v1.
//
// Nothing here drains the connection's event channel, which is deliberate and
// safe: wl.Conn drops events nobody is reading rather than blocking its read
// loop, and the toplevel state this reads is kept up to date by that loop
// regardless.
func activeAppFromCompositor() (string, error) {
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
