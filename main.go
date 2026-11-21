// Command ezhours is a time tracker that lives in the system tray: start it,
// stop it, say what the time went into, and it appends the entry to a plain
// text file per project.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/mpdroog/ezhours/apptracker"
	"github.com/mpdroog/ezhours/gitsync"
	"github.com/mpdroog/ezhours/icon"
	"github.com/mpdroog/ezhours/invoiced"
	"github.com/mpdroog/ezhours/session"
	"github.com/mpdroog/ezhours/storage"
	"github.com/mpdroog/ezhours/timer"
	"github.com/mpdroog/ezhours/tray"
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
	mToggle     *tray.MenuItem
	mSync       *tray.MenuItem

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

	// iconMu guards what setIcon last put in the tray, so that repeated
	// calls with the same state cost nothing.
	iconMu        sync.Mutex
	iconSet       bool
	iconActiveNow bool
	iconFailedNow bool
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

	tray.Run(onReady, onExit)
}

func onReady() {
	// Use template icon - macOS will automatically handle light/dark mode
	setIcon(false)
	tray.SetTooltip("EZHours - Click to start/stop timer")

	// Menu items
	mToggle = tray.AddMenuItem("Start Timer", "Start tracking time")
	tray.AddSeparator()
	mOpenDir := tray.AddMenuItem("Open Hours Folder", "Open the hours folder in file manager")
	mSync = tray.AddMenuItem("Sync Now", "Pull and push the hours folder")
	mExport := tray.AddMenuItem("Export to InvoiceD", "Export hours to InvoiceD")
	tray.AddSeparator()
	mQuit := tray.AddMenuItem("Quit", "Exit EZHours")

	// On macOS/Windows left-click toggles the timer directly.
	// On Linux, skip SetOnTapped so ItemIsMenu=true and left-click opens the menu instead.
	if runtime.GOOS != "linux" {
		tray.SetOnTapped(onTrayClicked)
	}

	// Pick up entries other devices pushed while this one was closed.
	go startupPull()

	go func() {
		for {
			select {
			case <-mToggle.Clicked():
				onTrayClicked()
			case <-mOpenDir.Clicked():
				openHoursDir()
			case <-mSync.Clicked():
				go syncNow()
			case <-mExport.Clicked():
				exportToInvoiced()
			case <-mQuit.Clicked():
				tray.Quit()
			}
		}
	}()
}

func onExit() {
	// Cleanup
}

// unfiledProject is where an hour goes when the dialog could not ask which
// project it belongs to. A real project name never has to be guessed, so a
// file by this name in the hours folder means exactly one thing: go and sort
// these by hand.
const unfiledProject = "UNFILED"

func onTrayClicked() {
	if dialogOpen {
		// The dialog is still waiting for an answer -- and if the click is
		// here, it is most likely nowhere to be seen. Put up a fresh one
		// rather than ignore the click: ignoring it is what left a hidden
		// dialog holding the app until a restart.
		ui.ReopenSaveDialog()
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

			// The dialog never opened: zenity missing, no display, and so
			// on. Nobody chose to throw this hour away, so do not throw it
			// away -- write it under a name that is plainly waiting to be
			// sorted, and say in the entry itself why it has no project.
			project, description := result.Project, result.Description
			if result.Failed != nil {
				project = unfiledProject
				description = "unfiled -- the save dialog could not run: " + shortErr(result.Failed)
				log.Printf("save dialog unavailable; filing the entry under %q instead", project)
			}

			saved := ""
			if !result.Cancelled && project != "" {
				if err := storage.SaveEntry(project, timerState.StartTime(), timerState.EndTime(), description, appUsage); err != nil {
					log.Printf("save entry: %v", err)
					// The timer is about to be reset, so this dialog is the only
					// remaining copy of the hour: it shows the entry as text the
					// user can copy into the file by hand.
					ui.ShowSaveError(
						storage.ProjectPath(project),
						storage.EntryText(timerState.StartTime(), timerState.EndTime(), description, appUsage),
						err)
				} else {
					saved = fmt.Sprintf("%s: %s - %s",
						project,
						timerState.StartTime().Format("2006-01-02 15:04"),
						timerState.EndTime().Format("15:04"))
				}
			}

			tray.SetTitle("")
			tray.SetTooltip("EZHours - Click to start/stop timer")
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

	update := func() {
		elapsed := timer.FormatDuration(timerState.Elapsed())
		tray.SetTitle(elapsed)
		tray.SetTooltip("EZHours - Recording: " + elapsed)
		updateMenuText("Stop Timer (" + elapsed + ")")
		// Steady, not blinking. The blink alternated the icon once a
		// second, and every frame of it was a NewIcon on the session bus
		// that made the tray host re-read this item and repaint its whole
		// bar -- measured at ~3% of a core in the bar and as much again in
		// the compositor, on an idle desktop, for an animation nobody
		// asked for. setIcon is a no-op unless the picture really changes,
		// so calling it per tick still picks up a sync failure at once.
		setIcon(true)
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
	openPath(dir)
}

// openPath hands a folder or URL to the desktop's default handler.
func openPath(dir string) {
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
		log.Printf("open %s: %v", dir, err)
		return
	}
	// Reap it: xdg-open exits as soon as the file manager is up, and a zombie
	// per click is not much, but this process runs for weeks.
	go func() {
		defer cancel()
		if err := cmd.Wait(); err != nil {
			log.Printf("open %s: %v", dir, err)
		}
	}()
}

// pull brings in remote entries. Failures are reported in the menu and never
// block the timer: worst case this device works offline and syncs later.
var errSyncerNil = errors.New("sync disabled: hours directory not initialized")

func pull() error {
	if syncer == nil {
		syncFail(errSyncerNil)
		return errSyncerNil
	}
	setSyncStatus("Syncing...")
	if err := syncer.Pull(); err != nil {
		log.Printf("gitsync pull: %v", err)
		syncFail(err)
		return err
	}
	syncOK()
	return nil
}

// startupRetries spaces out the login-time pull while the network comes up:
// about five minutes in all, after which the failure is shown like any other.
var startupRetries = []time.Duration{
	2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second,
	30 * time.Second, 60 * time.Second, 60 * time.Second, 60 * time.Second, 60 * time.Second,
}

// startupPull is pull for the moment the app starts, which at login is usually
// before the network or the local resolver is up. A network error then is
// expected, not a failure: it waits and tries again without badging the icon.
// Any other error (auth, broken repo, no remote) is reported straight away.
func startupPull() {
	if syncer == nil {
		syncFail(errSyncerNil)
		return
	}
	for attempt := 0; ; attempt++ {
		setSyncStatus("Syncing...")
		err := syncer.Pull()
		if err == nil {
			syncOK()
			return
		}
		if !gitsync.IsNetworkError(err) || attempt >= len(startupRetries) {
			log.Printf("gitsync pull: %v", err)
			syncFail(err)
			return
		}
		setSyncStatus("Waiting for network...")
		time.Sleep(startupRetries[attempt])
	}
}

// push commits the hours folder and publishes it under the given message.
func push(message string) error {
	if syncer == nil {
		syncFail(errSyncerNil)
		return errSyncerNil
	}
	setSyncStatus("Syncing...")
	if err := syncer.CommitAndPush("ezhours: " + message); err != nil {
		log.Printf("gitsync push: %v", err)
		syncFail(err)
		return err
	}
	syncOK()
	return nil
}

// syncNow is the manual "Sync Now" action: pull, then publish anything local.
// When the remote turns us away it also sets up access, since a click is the
// one moment someone is there to finish the job.
func syncNow() {
	errPull := pull()
	errPush := push("manual sync")
	if gitsync.IsAuthError(errPull) || gitsync.IsAuthError(errPush) {
		setupSyncAccess()
	}
}

// setupSyncAccess creates the ssh key and switches the remote to SSH, retries,
// and if the remote still refuses, shows which key to allow where.
func setupSyncAccess() {
	setup, err := syncer.SetupAccess()
	if err != nil {
		log.Printf("gitsync setup: %v", err)
		return
	}
	if setup.Changed {
		log.Printf("gitsync setup: remote %s, key %s", setup.Remote, gitsync.KeyFile)
		// The key may already be allowed (a re-clone of a set up repo).
		if pull() == nil && push("manual sync") == nil {
			return
		}
	}

	if setup.KeysURL != "" {
		openPath(setup.KeysURL)
	}
	ui.ShowSyncSetup(setup.PublicKey)
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

	// Pushing an icon is not free: it serialises the pixels onto the
	// session bus and every tray host watching has to re-read this item.
	// Callers repaint on a timer, so drop the writes that change nothing.
	iconMu.Lock()
	if iconSet && iconActiveNow == active && iconFailedNow == failed {
		iconMu.Unlock()
		return
	}
	iconSet, iconActiveNow, iconFailedNow = true, active, failed
	iconMu.Unlock()

	switch {
	case active && failed:
		tray.SetIcon(iconActiveFailed)
	case active:
		tray.SetIcon(iconActive)
	case failed:
		tray.SetIcon(iconNormalFailed)
	default:
		tray.SetTemplateIcon(iconNormal, iconNormal)
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

func exportToInvoiced() {
	// Get hours directory
	dir, err := storage.GetHoursDir()
	if err != nil {
		fmt.Printf("Error getting hours dir: %v\n", err)
		return
	}

	// Create client with default config
	config := invoiced.DefaultConfig()
	client := invoiced.NewClient(config)

	// Parse all hours files
	hours, err := invoiced.ParseAllHoursFiles(dir, config.Year)
	if err != nil {
		fmt.Printf("Error parsing hours files: %v\n", err)
		return
	}

	if len(hours) == 0 {
		fmt.Println("No hours to export")
		return
	}

	// Export to InvoiceD and truncate files on success
	for _, hour := range hours {
		if err := client.ExportHour(hour); err != nil {
			fmt.Printf("Error exporting %s: %v\n", hour.Name, err)
			return
		}
		// Truncate file after successful export
		if err := os.Truncate(hour.FilePath, 0); err != nil {
			fmt.Printf("Error truncating %s: %v\n", hour.FilePath, err)
		}
	}

	fmt.Printf("Exported %d hour files to InvoiceD\n", len(hours))

	// Open browser to hours page
	url := fmt.Sprintf("%s/static/#%s/%d/hours", client.GetBaseURL(), client.GetEntity(), client.GetYear())
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
