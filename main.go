package main

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/mpdroog/ezhours/apptracker"
	"github.com/mpdroog/ezhours/gitsync"
	"github.com/mpdroog/ezhours/icon"
	"github.com/mpdroog/ezhours/storage"
	"github.com/mpdroog/ezhours/timer"
	"github.com/mpdroog/ezhours/ui"
)

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

	// On Linux the SNI protocol renders icons larger; scale up so they aren't tiny.
	if runtime.GOOS == "linux" {
		iconNormal = icon.Scale(icon.Data, 64)
		iconActive = icon.Scale(icon.ActiveData(), 64)
	} else {
		iconNormal = icon.Data
		iconActive = icon.ActiveData()
	}

	systray.Run(onReady, onExit)
}

func onReady() {
	// Use template icon - macOS will automatically handle light/dark mode
	systray.SetTemplateIcon(iconNormal, iconNormal)
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
			systray.SetTemplateIcon(iconNormal, iconNormal)
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
		if blink {
			systray.SetIcon(iconActive)
		} else {
			systray.SetTemplateIcon(iconNormal, iconNormal)
		}
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
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	cmd.Start()
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
		setSyncStatus("Sync failed: " + shortErr(err))
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
		setSyncStatus("Sync failed: " + shortErr(err))
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
// stale folder is visible at a glance.
func syncOK() {
	setSyncStatus("Sync Now (" + time.Now().Format("15:04") + ")")
}

// shortErr keeps the menu readable; the full error goes to the log.
func shortErr(err error) string {
	msg := err.Error()
	if len(msg) > 60 {
		msg = msg[:57] + "..."
	}
	return msg
}
