package apptracker

import (
	"strings"

	"github.com/mpdroog/ezhours/session"
)

// getActiveApp returns the name of the currently active application on macOS.
// The error is the caller's to log: without Accessibility permission this fails
// every time, and silence there looks exactly like an idle machine.
func getActiveApp() (string, error) {
	script := `tell application "System Events" to get name of first process whose frontmost is true`
	out, err := session.Output("osascript", "-e", script)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
