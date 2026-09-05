//go:build !linux

package icon

import "image/color"

// GlyphColor is only consulted on Linux; macOS and Windows draw the template
// icon themselves and adapt it to the menu bar. It exists here so callers can
// branch on runtime.GOOS instead of build tags.
func GlyphColor() color.RGBA {
	return color.RGBA{R: 0, G: 0, B: 0, A: 255}
}
