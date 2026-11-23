//go:build linux && !wayland

package apptracker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mpdroog/ezhours/session"
)

func init() { activeAppX11 = activeAppFromX }

// wmClassRe matches the quoted values of `xprop WM_CLASS`, which reports
// instance and class: WM_CLASS(STRING) = "Navigator", "firefox"
var wmClassRe = regexp.MustCompile(`"([^"]*)"`)

// activeAppFromX is the original path, kept for a real X11 session where it is
// still the right answer. It needs xdotool, which a Wayland-only machine has
// no reason to install.
func activeAppFromX() (string, error) {
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
