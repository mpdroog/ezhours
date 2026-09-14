// Command ezhours is a time tracker that lives in the system tray: start it,
// stop it, say what the time went into, and it appends the entry to a plain
// text file per project.
package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/mpdroog/ezhours/apptracker"
	"github.com/mpdroog/ezhours/gitsync"
	"github.com/mpdroog/ezhours/icon"
	"github.com/mpdroog/ezhours/session"
	"github.com/mpdroog/ezhours/storage"
	"github.com/mpdroog/ezhours/timer"
	"github.com/mpdroog/ezhours/ui"
)

// openDirTimeout bounds the file-manager launcher. It only has to live long
// enough for the file manager to take over from it.
const openDirTimeout = 30 * time.Second

var (
	timerState  *timer.Timer
	appTracker  *apptracker.Tracker
	stopUpdater chan struct{}
	dialogOpen  bool
	mToggle     *systray.MenuItem
	mSync       *systray.MenuItem

	syncer *gitsync.Syncer

	iconNormal []byte
	iconActive []byte
	// The same two icons carrying the sync warning badge.
	iconNormalFailed []byte
	iconActiveFailed []byte

	// syncMu guards syncFailed: a sync runs on its own goroutine while the
	// title updater repaints the icon every second.
	syncMu     sync.Mutex
	syncFailed bool
)

func main() {
	timerState = timer.New()
	appTracker = apptracker.New()
	stopUpdater = make(chan struct{})

	if dir, err := storage.GetHoursDir(); err != nil {
		log.Printf("hours dir: %v", err)
	} else if syncer, err = gitsync.New(dir); err != nil {
		log.Printf("gitsync: %v", err)
		syncer = nil
	}

	// On Linux the SNI host scales our pixmap to the panel, often 2x on HiDPI, so
	// draw the glyph large and anti-aliased rather than stretching the 22px PNG;
	// and in a panel-matched colour, because nothing there honours a template
	// icon and the black glyph is invisible on the dark panel most desktops use.
	if runtime.GOOS == "linux" {
		iconNormal = icon.Glyph(64, icon.GlyphColor())
		iconActive = icon.WithRecordingDot(iconNormal)
	} else {
		iconNormal = icon.Data
		iconActive = icon.ActiveData()
	}
	iconNormalFailed = icon.WithWarningBadge(iconNormal)
	iconActiveFailed = icon.WithWarningBadge(iconActive)
	if runtime.GOOS == "linux" {
		for _, p := range []*[]byte{&iconNormal, &iconActive, &iconNormalFailed, &iconActiveFailed} {
			*p = icon.ForStatusNotifier(*p)
		}
	}

	systray.Run(onReady, onExit)
}

func onReady() {
	// Use template icon - macOS will automatically handle light/dark mode
	setIcon(false)
	systray.SetTooltip("EZHours - Click to start/stop timer")

	// Menu items
	mToggle = systray.AddMenuItem("Start Timer", "Start tracking time")
	systray.AddSeparator()
	mOpenDir := systray.AddMenuItem("Open Hours Folder", "Open the hours folder in file manager")
	mSync = systray.AddMenuItem("Sync Now", "Pull and push the hours folder")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit EZHours")

	// On macOS/Windows left-click toggles the timer directly.
	// On Linux, skip SetOnTapped so ItemIsMenu=true and left-click opens the menu instead.
	if runtime.GOOS != "linux" {
		systray.SetOnTapped(onTrayClicked)
	}

	// Pick up entries other devices pushed while this one was closed.
	go pull()

	go func() {
		for {
			select {
			case <-mToggle.ClickedCh:
				onTrayClicked()
			case <-mOpenDir.ClickedCh:
				openHoursDir()
			case <-mSync.ClickedCh:
				go syncNow()
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	// Cleanup
}

func onTrayClicked() {
	if dialogOpen {
		return
	}

	if timerState.IsRunning() {
		// Stop timer and app tracker
		timerState.Stop()
		appTracker.Stop()

		// Stop the title updater
		select {
		case stopUpdater <- struct{}{}:
		default:
		}

		// Get app usage data
		appUsage := appTracker.GetUsage()

		// Show save dialog
		dialogOpen = true
		go func() {
			result := ui.ShowSaveDialog(timerState.StartTime(), timerState.EndTime())

			saved := ""
			if !result.Cancelled && result.Project != "" {
				if err := storage.SaveEntry(result.Project, timerState.StartTime(), timerState.EndTime(), result.Description, appUsage); err != nil {
					log.Printf("save entry: %v", err)
				} else {
					saved = fmt.Sprintf("%s: %s - %s",
						result.Project,
						timerState.StartTime().Format("2006-01-02 15:04"),
						timerState.EndTime().Format("15:04"))
				}
			}

			systray.SetTitle("")
			systray.SetTooltip("EZHours - Click to start/stop timer")
			setIcon(false)
			updateMenuText("Start Timer")
			dialogOpen = false

			// Publish straight away so the other devices see the entry.
			if saved != "" {
				push(saved)
			}
		}()
	} else {
		// Fetch what the other devices logged before this session gets added
		// to the files. Runs in the background so the menu stays responsive;
		// nothing is written until the timer is stopped anyway.
		go pull()

		// Start timer and app tracker
		timerState.Start()
		appTracker.Start()
		updateMenuText("Stop Timer")

		// Start goroutine to update title every second
		go updateTitle()
	}
}

func updateTitle() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	blink := false
	update := func() {
		elapsed := timer.FormatDuration(timerState.Elapsed())
		systray.SetTitle(elapsed)
		systray.SetTooltip("EZHours - Recording: " + elapsed)
		updateMenuText("Stop Timer (" + elapsed + ")")
		// Blink between normal and active icon each second
		setIcon(blink)
		blink = !blink
	}

	// Initial update
	update()

	for {
		select {
		case <-ticker.C:
			if timerState.IsRunning() {
				update()
			} else {
				return
			}
		case <-stopUpdater:
			return
		}
	}
}

func updateMenuText(text string) {
	if mToggle != nil {
		mToggle.SetTitle(text)
	}
}

func openHoursDir() {
	dir, err := storage.GetHoursDir()
	if err != nil {
		log.Printf("hours dir: %v", err)
		return
	}
	// The launcher hands the folder to the file manager and exits; it is not
	// waiting for the person looking at it, so it does not get to hang forever.
	ctx, cancel := context.WithTimeout(context.Background(), openDirTimeout)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", dir)
	case "windows":
		cmd = exec.CommandContext(ctx, "explorer", dir)
	default:
		cmd = session.Command(ctx, "xdg-open", dir)
	}
	// The menu entry gives no sign either way, so a file manager that never
	// opens is only explicable from the log.
	if err := cmd.Start(); err != nil {
		cancel()
		log.Printf("open hours dir: %v", err)
		return
	}
	// Reap it: xdg-open exits as soon as the file manager is up, and a zombie
	// per click is not much, but this process runs for weeks.
	go func() {
		defer cancel()
		if err := cmd.Wait(); err != nil {
			log.Printf("open hours dir: %v", err)
		}
	}()
}

// pull brings in remote entries. Failures are reported in the menu and never
// block the timer: worst case this device works offline and syncs later.
func pull() {
	if syncer == nil {
		return
	}
	setSyncStatus("Syncing...")
	if err := syncer.Pull(); err != nil {
		log.Printf("gitsync pull: %v", err)
		syncFail(err)
		return
	}
	syncOK()
}

// push commits the hours folder and publishes it under the given message.
func push(message string) {
	if syncer == nil {
		return
	}
	setSyncStatus("Syncing...")
	if err := syncer.CommitAndPush("ezhours: " + message); err != nil {
		log.Printf("gitsync push: %v", err)
		syncFail(err)
		return
	}
	syncOK()
}

// syncNow is the manual "Sync Now" action: pull, then publish anything local.
func syncNow() {
	pull()
	push("manual sync")
}

func setSyncStatus(text string) {
	if mSync != nil {
		mSync.SetTitle(text)
	}
}

// syncOK resets the menu entry and records when the last sync succeeded, so a
// stale folder is visible at a glance. It also clears the warning badge.
func syncOK() {
	setSyncStatus("Sync Now (" + time.Now().Format("15:04") + ")")
	setSyncFailed(false)
}

// syncFail reports a failed sync in the menu and badges the tray icon, so an
// unpublished folder is visible without opening the menu at all.
func syncFail(err error) {
	setSyncStatus("Sync failed: " + shortErr(err))
	setSyncFailed(true)
}

func setSyncFailed(failed bool) {
	syncMu.Lock()
	changed := syncFailed != failed
	syncFailed = failed
	syncMu.Unlock()

	// While the timer runs the title updater repaints every second anyway;
	// repaint here so an idle tray picks the change up straight away.
	if changed {
		setIcon(timerState.IsRunning())
	}
}

// setIcon paints the tray icon for the current state: active adds the recording
// dot, a failed sync adds the warning badge, and the two combine. Only the bare
// clock goes out as a template icon -- macOS renders those as a monochrome mask,
// which would throw away the colour that makes either badge readable.
func setIcon(active bool) {
	syncMu.Lock()
	failed := syncFailed
	syncMu.Unlock()

	switch {
	case active && failed:
		systray.SetIcon(iconActiveFailed)
	case active:
		systray.SetIcon(iconActive)
	case failed:
		systray.SetIcon(iconNormalFailed)
	default:
		systray.SetTemplateIcon(iconNormal, iconNormal)
	}
}

// shortErr keeps the menu readable; the full error goes to the log.
func shortErr(err error) string {
	msg := err.Error()
	if len(msg) > 60 {
		msg = msg[:57] + "..."
	}
	return msg
}
