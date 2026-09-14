//go:build !linux

package session

// desktopEnv has nothing to repair off Linux: on macOS and Windows the app is
// launched by the session itself, so it already runs inside one.
func desktopEnv() []string { return nil }
