//go:build linux

package icon

import (
	"errors"
	"image/color"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/mpdroog/ezhours/session"
)

// ColorEnv overrides the detected panel colour scheme. Set it to "light" for a
// light glyph (a dark panel) or "dark" for a dark one (a light panel).
const ColorEnv = "EZHOURS_ICON_COLOR"

var (
	light = color.RGBA{R: 235, G: 235, B: 235, A: 255}
	dark  = color.RGBA{R: 20, G: 20, B: 20, A: 255}
)

// GlyphColor picks the tray glyph colour for the desktop we are running on.
//
// There is no protocol for asking a StatusNotifier host what its panel looks
// like, and the pixmap we hand it is drawn verbatim, so the colour scheme of the
// GTK theme is the closest thing to an answer: panels follow it on every desktop
// that ships one. Each probe below is a desktop that reports it differently, and
// a machine that answers none of them gets the light glyph -- a dark panel is the
// common default, and it is the case that made this necessary.
//
// This runs once at startup. Change your theme and the icon follows on the next
// restart, not before.
func GlyphColor() color.RGBA {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(ColorEnv))) {
	case "light":
		return light
	case "dark":
		return dark
	}

	// GNOME, and the desktops that mirror the setting (Mint sets it under XFCE).
	if v, ok := run("gsettings", "get", "org.gnome.desktop.interface", "color-scheme"); ok {
		if strings.Contains(v, "prefer-dark") {
			return light
		}
		if strings.Contains(v, "prefer-light") {
			return dark
		}
	}
	// XFCE keeps its own copy in xsettings; the theme name is all it exposes.
	if v, ok := run("xfconf-query", "-c", "xsettings", "-p", "/Net/ThemeName"); ok {
		return byThemeName(v)
	}
	if v, ok := run("gsettings", "get", "org.gnome.desktop.interface", "gtk-theme"); ok {
		return byThemeName(v)
	}
	// GTK_THEME=Adwaita:dark wins over the settings above for the app that sees
	// it, so a session that sets it means it.
	if v := os.Getenv("GTK_THEME"); v != "" {
		return byThemeName(v)
	}
	return light
}

// byThemeName reads the light/dark suffix themes carry by convention
// ("Mint-Y-Dark", "Adwaita:dark", "WhiteSur-Dark"). A theme that is dark without
// saying so is indistinguishable from a light one here; that is what ColorEnv is
// for.
func byThemeName(name string) color.RGBA {
	if strings.Contains(strings.ToLower(name), "dark") {
		return light
	}
	return dark
}

// run returns the trimmed output of a command, or ok=false when it has nothing
// to say -- every probe here is a desktop-specific tool that is absent as often
// as it is present, and the next probe is the answer to that.
//
// A tool that is missing is therefore not worth reporting; a tool that is
// installed and still failed is, because it means the probe that should have
// answered did not, and the icon colour below it is a guess.
func run(name string, args ...string) (string, bool) {
	out, err := session.Output(name, args...)
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			log.Printf("icon: theme probe failed, falling through to the next: %v", err)
		}
		return "", false
	}
	s := strings.TrimSpace(out)
	if s == "" {
		return "", false
	}
	return s, true
}
