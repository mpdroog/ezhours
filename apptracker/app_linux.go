//go:build linux

package apptracker

import (
	"errors"
	"os"
)

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
	f := activeAppX11
	if onWayland() {
		f = activeAppWayland
	}
	if f == nil {
		return "", errNotBuilt
	}
	return f()
}

// activeAppX11 and activeAppWayland are set by app_linux_x11.go and
// app_linux_wayland.go, and nil in a binary built without that half: build
// with -tags x11 or -tags wayland for only one, as osxflow does.
var activeAppX11, activeAppWayland func() (string, error)

var errNotBuilt = errors.New("this binary was built without support for the session's display server")

// onWayland is whether the Wayland half applies: there is a compositor,
// or there is no X half to fall back on.
func onWayland() bool {
	return activeAppWayland != nil && (os.Getenv("WAYLAND_DISPLAY") != "" || activeAppX11 == nil)
}
