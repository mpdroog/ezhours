package apptracker

import (
	"os/exec"
	"regexp"
	"strings"
)

// wmClassRe matches the quoted values of `xprop WM_CLASS`, which reports
// instance and class: WM_CLASS(STRING) = "Navigator", "firefox"
var wmClassRe = regexp.MustCompile(`"([^"]*)"`)

// getActiveApp returns the name of the currently active application on Linux.
// The window class is preferred over the title, because titles change per
// browser tab, per shell command and even per spinner frame, which would
// fragment a single app into dozens of entries.
func getActiveApp() string {
	out, err := exec.Command("xdotool", "getactivewindow").Output()
	if err != nil {
		return ""
	}
	win := strings.TrimSpace(string(out))
	if win == "" {
		return ""
	}

	if class := windowClass(win); class != "" {
		return prettify(class)
	}

	// Fall back to the title for windows that expose no class.
	out, err = exec.Command("xdotool", "getwindowname", win).Output()
	if err != nil {
		return ""
	}
	return titleToApp(string(out))
}

// windowClass returns the WM_CLASS class of a window, or "" if unavailable.
func windowClass(win string) string {
	out, err := exec.Command("xprop", "-id", win, "WM_CLASS").Output()
	if err != nil {
		return ""
	}
	m := wmClassRe.FindAllStringSubmatch(string(out), -1)
	if len(m) == 0 {
		return ""
	}
	// Last value is the class ("firefox"); the first is the instance.
	return normalize(m[len(m)-1][1])
}
