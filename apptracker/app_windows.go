package apptracker

import (
	"strings"

	"github.com/mpdroog/ezhours/session"
)

// getActiveApp returns the name of the currently active application on Windows.
func getActiveApp() (string, error) {
	script := `Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Text;
public class Win32 {
    [DllImport("user32.dll")]
    public static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")]
    public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint processId);
}
"@
$hwnd = [Win32]::GetForegroundWindow()
$pid = 0
[Win32]::GetWindowThreadProcessId($hwnd, [ref]$pid) | Out-Null
(Get-Process -Id $pid -ErrorAction SilentlyContinue).ProcessName`

	out, err := session.Output("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
