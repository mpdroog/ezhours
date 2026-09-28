// Package tray puts EZHours in the system tray.
//
// It exists because the tray needs two different implementations and the
// rest of the app should not know which one it has.
//
// On macOS and Windows it is fyne.io/systray, which is what those
// platforms need and where it works well.
//
// On Linux it is osxflow's own sni and menu packages, and the reason is a
// bug: systray watches for the tray restarting so it can register again,
// but its watcher loop reads a channel carrying *every* signal the
// connection receives and does `return` on any it does not recognise --
// including the bus's own NameAcquired, which has one body field where it
// expects three. The goroutine dies for good the first time one arrives.
// After that, restarting the panel loses the icon permanently: the process
// is fine, its object still answers, it is simply no longer registered,
// and nothing says so. osxflow's sni does the same job and skips what it
// does not recognise instead of giving up.
//
// The API deliberately mirrors the part of systray this app used, so the
// call sites read the same on every platform. One thing does differ, and
// it is documented at SetTitle.
package tray
