package apptracker

import (
	"os/exec"
	"strings"
)

// getActiveApp returns the name of the currently active application on macOS
func getActiveApp() string {
	script := `tell application "System Events" to get name of first process whose frontmost is true`
	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
