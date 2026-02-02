package main

import (
	"time"

	"fyne.io/systray"

	"github.com/mpdroog/ezhours/icon"
	"github.com/mpdroog/ezhours/storage"
	"github.com/mpdroog/ezhours/timer"
	"github.com/mpdroog/ezhours/ui"
)

var (
	timerState  *timer.Timer
	stopUpdater chan struct{}
	dialogOpen  bool
	mToggle     *systray.MenuItem
)

func main() {
	timerState = timer.New()
	stopUpdater = make(chan struct{})

	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetIcon(icon.Data)
	systray.SetTooltip("EZHours - Click to start/stop timer")

	// Menu items
	mToggle = systray.AddMenuItem("Start Timer", "Start tracking time")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit EZHours")

	// Handle tray icon click
	systray.SetOnTapped(onTrayClicked)

	go func() {
		for {
			select {
			case <-mToggle.ClickedCh:
				onTrayClicked()
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
		// Stop timer
		timerState.Stop()

		// Stop the title updater
		select {
		case stopUpdater <- struct{}{}:
		default:
		}

		// Show save dialog
		dialogOpen = true
		go func() {
			result := ui.ShowSaveDialog(timerState.StartTime(), timerState.EndTime())
			if !result.Cancelled && result.Project != "" {
				storage.SaveEntry(result.Project, timerState.StartTime(), timerState.EndTime(), result.Description)
			}
			systray.SetTitle("")
			updateMenuText("Start Timer")
			dialogOpen = false
		}()
	} else {
		// Start timer
		timerState.Start()
		updateMenuText("Stop Timer")

		// Start goroutine to update title every second
		go updateTitle()
	}
}

func updateTitle() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Initial update
	systray.SetTitle(timer.FormatDuration(timerState.Elapsed()))

	for {
		select {
		case <-ticker.C:
			if timerState.IsRunning() {
				systray.SetTitle(timer.FormatDuration(timerState.Elapsed()))
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
