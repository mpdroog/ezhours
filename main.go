package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/systray"

	"github.com/mpdroog/ezhours/icon"
	"github.com/mpdroog/ezhours/storage"
	"github.com/mpdroog/ezhours/timer"
	"github.com/mpdroog/ezhours/ui"
)

var (
	fyneApp     fyne.App
	timerState  *timer.Timer
	stopUpdater chan struct{}
	dialogOpen  bool
)

func main() {
	timerState = timer.New()
	stopUpdater = make(chan struct{})

	// Register systray before starting Fyne
	systray.Register(onReady, onExit)

	fyneApp = app.New()

	// Set app to run as system tray only (no dock icon on macOS)
	if drv, ok := fyneApp.Driver().(desktop.Driver); ok {
		_ = drv // Driver supports desktop features
	}

	fyneApp.Run()
}

func onReady() {
	systray.SetIcon(icon.Data)
	systray.SetTooltip("EZHours - Click to start/stop timer")

	// Menu items
	mQuit := systray.AddMenuItem("Quit", "Exit EZHours")

	// Handle tray icon click
	systray.SetOnTapped(onTrayClicked)

	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()
}

func onExit() {
	fyneApp.Quit()
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
		ui.ShowSaveDialog(
			fyneApp,
			timerState.StartTime(),
			timerState.EndTime(),
			func(project, description string) {
				// Save entry
				storage.SaveEntry(project, timerState.StartTime(), timerState.EndTime(), description)
				systray.SetTitle("")
				dialogOpen = false
			},
			func() {
				// Cancelled
				systray.SetTitle("")
				dialogOpen = false
			},
		)
	} else {
		// Start timer
		timerState.Start()

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
