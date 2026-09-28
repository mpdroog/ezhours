//go:build !linux

package tray

// macOS and Windows: fyne.io/systray, thinly wrapped so the call sites
// match the Linux ones. The re-registration bug that made Linux use
// something else is a StatusNotifierItem problem, which is a Linux
// protocol; neither of these platforms goes near it.

import "fyne.io/systray"

// MenuItem is one row of the tray menu.
type MenuItem struct{ sys *systray.MenuItem }

// SetTitle changes the row's label.
func (m *MenuItem) SetTitle(s string) { m.sys.SetTitle(s) }

// Clicked fires when the row is chosen.
func (m *MenuItem) Clicked() <-chan struct{} { return m.sys.ClickedCh }

// Run shows the icon and blocks until Quit.
func Run(onReady, onExit func()) { systray.Run(onReady, onExit) }

// Quit takes the icon down and lets Run return.
func Quit() { systray.Quit() }

// AddMenuItem appends a row.
func AddMenuItem(title, tooltip string) *MenuItem {
	return &MenuItem{sys: systray.AddMenuItem(title, tooltip)}
}

// AddSeparator appends a divider.
func AddSeparator() { systray.AddSeparator() }

// SetIcon replaces the icon with the given PNG.
func SetIcon(png []byte) { systray.SetIcon(png) }

// SetTemplateIcon replaces the icon, telling macOS it may be drawn as a
// monochrome mask.
func SetTemplateIcon(template, regular []byte) { systray.SetTemplateIcon(template, regular) }

// SetTitle sets the text beside the icon.
func SetTitle(s string) { systray.SetTitle(s) }

// SetTooltip sets the hover text.
func SetTooltip(s string) { systray.SetTooltip(s) }

// SetOnTapped makes a primary click run f instead of opening the menu.
func SetOnTapped(f func()) { systray.SetOnTapped(f) }
