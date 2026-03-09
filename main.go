package main

import (
	"os/exec"
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/mpdroog/ezhours/apptracker"
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
)

func main() {
	timerState = timer.New()
	appTracker = apptracker.New()
	stopUpdater = make(chan struct{})

	systray.Run(onReady, onExit)
}

func onReady() {
	// Use template icon - macOS will automatically handle light/dark mode
	systray.SetTemplateIcon(icon.Data, icon.Data)
	systray.SetTooltip("EZHours - Click to start/stop timer")

	// Menu items
	mToggle = systray.AddMenuItem("Start Timer", "Start tracking time")
	systray.AddSeparator()
	mOpenDir := systray.AddMenuItem("Open Hours Folder", "Open the hours folder in file manager")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit EZHours")

	// On macOS/Windows left-click toggles the timer directly.
	// On Linux, skip SetOnTapped so ItemIsMenu=true and left-click opens the menu instead.
	if runtime.GOOS != "linux" {
		systray.SetOnTapped(onTrayClicked)
	}

	go func() {
		for {
			select {
			case <-mToggle.ClickedCh:
				onTrayClicked()
			case <-mOpenDir.ClickedCh:
				openHoursDir()
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
			if !result.Cancelled && result.Project != "" {
				storage.SaveEntry(result.Project, timerState.StartTime(), timerState.EndTime(), result.Description, appUsage)
			}
			systray.SetTitle("")
			systray.SetTooltip("EZHours - Click to start/stop timer")
			systray.SetTemplateIcon(icon.Data, icon.Data)
			updateMenuText("Start Timer")
			dialogOpen = false
		}()
	} else {
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
			systray.SetIcon(icon.ActiveData())
		} else {
			systray.SetTemplateIcon(icon.Data, icon.Data)
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
