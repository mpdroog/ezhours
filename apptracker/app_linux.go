package apptracker

import (
	"os/exec"
	"strings"
)

// getActiveApp returns the name of the currently active application on Linux
func getActiveApp() string {
	cmd := exec.Command("xdotool", "getactivewindow", "getwindowname")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
