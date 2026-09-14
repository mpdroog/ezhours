package apptracker

import (
	"fmt"
	"regexp"
	"strings"

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
func getActiveApp() (string, error) {
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
