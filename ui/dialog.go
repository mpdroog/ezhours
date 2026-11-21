// Package ui is every window ezhours puts on screen, drawn by the desktop's own
// dialog program rather than a toolkit linked into this process: zenity on
// Linux, osascript on macOS, PowerShell forms on Windows.
package ui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mpdroog/ezhours/session"
	"github.com/mpdroog/ezhours/storage"
)

// DialogResult holds the result from the save dialog
type DialogResult struct {
	Project     string
	Description string
	Cancelled   bool

	// Failed is why the dialog never reached the screen, when it did not.
	//
	// Cancelled stays false then, and the two must not be conflated: Cancel
	// is the user deciding to drop the hour, and this is nobody deciding
	// anything. Every error path here used to report Cancelled, so on a box
	// with no zenity installed every single stop looked like a deliberate
	// discard and the entry went in the bin without a word on screen.
	Failed error
}

// ShowSaveDialog displays a native OS dialog for saving time entries
func ShowSaveDialog(startTime, endTime time.Time) DialogResult {
	// An unreadable hours folder is not fatal -- the dialog still takes a name
	// typed by hand -- but it is why the list came up empty, and the entry is
	// about to be written to that same folder.
	projects, err := storage.GetProjects()
	if err != nil {
		log.Printf("list projects: %v", err)
	}

	duration := endTime.Sub(startTime)
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	timeInfo := fmt.Sprintf("%s - %s (%dh %dm)",
		startTime.Format("15:04"),
		endTime.Format("15:04"),
		hours, minutes)

	switch runtime.GOOS {
	case "darwin":
		return showMacDialog(projects, timeInfo)
	case "windows":
		return showWindowsDialog(projects, timeInfo)
	default:
		return showLinuxDialog(projects, timeInfo)
	}
}

func showLinuxDialog(projects []string, timeInfo string) DialogResult {
	for {
		ctx, cancel := context.WithCancel(context.Background())
		reopen.Lock()
		reopen.cancel = cancel
		reopen.Unlock()

		result := askProjectAndDescription(ctx, projects, timeInfo)

		reopen.Lock()
		reopen.cancel = nil
		reopen.Unlock()
		superseded := ctx.Err() != nil
		cancel()
		if !superseded {
			return result
		}
		// ReopenSaveDialog closed it to put up a fresh one: nobody
		// answered anything, so ask again.
		log.Printf("save dialog reopened")
	}
}

// askProjectAndDescription is the whole save dialog, in one window.
//
// It used to be two dialogs, project and then description, and the second
// was started the instant the first closed. On labwc that second window could
// come up behind everything, unfocused and missing from Alt+Tab, while ezhours
// waited on it with every tray click ignored. One window has no handoff to
// lose.
//
// The project list is editable: ask offers an "Add ..." row as soon as what is
// typed is not one of them, so naming a new project and picking an existing
// one are the same gesture. The description is optional -- ask accepts it
// blank -- and comes back on the second line.
func askProjectAndDescription(ctx context.Context, projects []string, timeInfo string) DialogResult {
	out, err := ask(ctx, strings.Join(projects, "\n"),
		"-mode=list", "-editable",
		"-title=EZHours \u2014 save this hour",
		"-text=Which project is this for?\n"+timeInfo,
		"-describe=What did you work on?")
	if err != nil {
		return dialogFailure(err)
	}
	project, description, _ := strings.Cut(out, "\n")
	project = strings.TrimSpace(project)
	if project == "" {
		return DialogResult{Cancelled: true}
	}
	return DialogResult{Project: project, Description: strings.TrimSpace(description)}
}

// reopen is the save dialog currently on screen, so a click on the tray can
// replace it.
var reopen struct {
	sync.Mutex
	cancel context.CancelFunc
}

// ReopenSaveDialog closes the save dialog and puts up a fresh one, front and
// centre, for the same hour. It reports whether there was one to reopen.
//
// This is for a dialog nobody can see. The tray ignores clicks while one is
// open, so a dialog that came up hidden used to leave ezhours unusable until
// it was restarted, with the hour lost. A fresh window is what reliably
// appears -- it is how the first one got on screen -- and nothing typed into
// an invisible one is lost by closing it.
func ReopenSaveDialog() bool {
	reopen.Lock()
	defer reopen.Unlock()
	if reopen.cancel == nil {
		return false
	}
	reopen.cancel()
	return true
}

// dialogFailure turns one dialog's error into a result: a Cancel when the
// user dismissed it, and otherwise a failure the caller must not treat as
// one.
func dialogFailure(err error) DialogResult {
	if cancelled(err) {
		return DialogResult{Cancelled: true}
	}
	return DialogResult{Failed: err}
}

// askBin is osxflow's own dialog, which replaces zenity.
//
// zenity is not installed on this machine and never was, so every Stop
// Timer looked exactly like a Cancel and the hour went in the bin. ask is
// built from the pieces osxflow already had -- a window, a font, a list
// and a text field -- and ships as one static binary beside the rest of
// the desktop's tools.
//
// Its exit status is the contract: 0 accepted, 1 dismissed, 2 could not
// show a dialog at all. cancelled below turns that into the distinction
// the caller needs, and 2 is what stops a missing dialog from ever again
// being read as a decision to discard.
//
// A variable only so the tests can stand in a fake.
var askBin = "ask"

// ask runs one dialog and returns what came back.
func ask(ctx context.Context, stdin string, args ...string) (string, error) {
	out, err := session.InteractiveContext(ctx, stdin, askBin, args...)
	if err != nil {
		if ctx.Err() == nil && !cancelled(err) {
			log.Printf("save dialog: %v", err)
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// cancelled reports whether the dialog exited because the user dismissed it.
//
// Cancel is exit status 1 -- and so is a zenity that never got a window, which
// makes the status alone useless. Nor can the two be told apart by the shape of
// what lands on stderr: a healthy zenity prints GLib warnings about the theme
// there, and reports "cannot open display" as exactly the same kind of warning.
// So the failure is matched for by name, and everything else that is not a
// plain status-1 exit is treated as a failure too, because an entry is about to
// be dropped and the alternative is losing it quietly.
func cancelled(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return false
	}
	var cmdErr *session.CmdError
	if errors.As(err, &cmdErr) && neverAppeared(cmdErr.Stderr) {
		return false
	}
	return true
}

// neverAppeared reports whether stderr says the dialog was never drawn. These
// are what GTK says when it cannot reach a display.
func neverAppeared(stderr string) bool {
	stderr = strings.ToLower(stderr)
	for _, marker := range []string{
		"cannot open display",
		"failed to open display",
		"unable to init server",
		"unable to initialize gtk",
	} {
		if strings.Contains(stderr, marker) {
			return true
		}
	}
	return false
}

func showMacDialog(projects []string, timeInfo string) DialogResult {
	// Build project list for AppleScript
	projectList := ""
	if len(projects) > 0 {
		quoted := make([]string, len(projects))
		for i, p := range projects {
			quoted[i] = fmt.Sprintf("%q", p)
		}
		projectList = strings.Join(quoted, ", ")
	}

	// AppleScript for project selection with option to add new
	script := fmt.Sprintf(`
set projectList to {%s, "+ New Project..."}
set timeInfo to %q

set projectChoice to choose from list projectList with prompt "Select project for time entry:" & return & timeInfo default items {item 1 of projectList} with title "EZHours - Save Entry"

if projectChoice is false then
	return "CANCELLED"
end if

set selectedProject to item 1 of projectChoice

if selectedProject is "+ New Project..." then
	set newProject to text returned of (display dialog "Enter new project name:" default answer "" with title "EZHours - New Project")
	if newProject is "" then
		return "CANCELLED"
	end if
	set selectedProject to newProject
end if

set descriptionText to text returned of (display dialog "What did you work on?" & return & return & timeInfo default answer "" with title "EZHours - Description" buttons {"Cancel", "Save"} default button "Save")

return selectedProject & "|SEPARATOR|" & descriptionText
`, projectList, timeInfo)

	output, err := session.Interactive("", "osascript", "-e", script)
	if err != nil {
		// osascript reports a dismissed dialog as an error too, as error -128
		// on stderr; everything else there is a real failure.
		var cmdErr *session.CmdError
		if !errors.As(err, &cmdErr) || !strings.Contains(cmdErr.Stderr, "-128") {
			log.Printf("save dialog: %v", err)
		}
		return DialogResult{Cancelled: true}
	}

	result := strings.TrimSpace(output)
	if result == "CANCELLED" || result == "" {
		return DialogResult{Cancelled: true}
	}

	parts := strings.SplitN(result, "|SEPARATOR|", 2)
	if len(parts) != 2 {
		return DialogResult{Cancelled: true}
	}

	return DialogResult{
		Project:     strings.TrimSpace(parts[0]),
		Description: strings.TrimSpace(parts[1]),
		Cancelled:   false,
	}
}

func showWindowsDialog(projects []string, timeInfo string) DialogResult {
	// PowerShell script for Windows
	projectListPS := ""
	if len(projects) > 0 {
		quoted := make([]string, len(projects))
		for i, p := range projects {
			quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(p, "'", "''"))
		}
		projectListPS = strings.Join(quoted, ",")
	}

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$form = New-Object System.Windows.Forms.Form
$form.Text = 'EZHours - Save Entry'
$form.Size = New-Object System.Drawing.Size(420,350)
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false

$timeLabel = New-Object System.Windows.Forms.Label
$timeLabel.Location = New-Object System.Drawing.Point(10,10)
$timeLabel.Size = New-Object System.Drawing.Size(380,20)
$timeLabel.Text = 'Time: %s'
$form.Controls.Add($timeLabel)

$projectLabel = New-Object System.Windows.Forms.Label
$projectLabel.Location = New-Object System.Drawing.Point(10,40)
$projectLabel.Size = New-Object System.Drawing.Size(100,20)
$projectLabel.Text = 'Project:'
$form.Controls.Add($projectLabel)

$projectCombo = New-Object System.Windows.Forms.ComboBox
$projectCombo.Location = New-Object System.Drawing.Point(10,60)
$projectCombo.Size = New-Object System.Drawing.Size(380,20)
$projectCombo.DropDownStyle = 'DropDown'
@(%s,'+ New Project...') | ForEach-Object { $projectCombo.Items.Add($_) }
if ($projectCombo.Items.Count -gt 0) { $projectCombo.SelectedIndex = 0 }
$form.Controls.Add($projectCombo)

$descLabel = New-Object System.Windows.Forms.Label
$descLabel.Location = New-Object System.Drawing.Point(10,100)
$descLabel.Size = New-Object System.Drawing.Size(100,20)
$descLabel.Text = 'Description:'
$form.Controls.Add($descLabel)

$descBox = New-Object System.Windows.Forms.TextBox
$descBox.Multiline = $true
$descBox.Location = New-Object System.Drawing.Point(10,120)
$descBox.Size = New-Object System.Drawing.Size(380,130)
$descBox.ScrollBars = 'Vertical'
$form.Controls.Add($descBox)

$saveButton = New-Object System.Windows.Forms.Button
$saveButton.Location = New-Object System.Drawing.Point(220,260)
$saveButton.Size = New-Object System.Drawing.Size(80,30)
$saveButton.Text = 'Save'
$saveButton.DialogResult = [System.Windows.Forms.DialogResult]::OK
$form.AcceptButton = $saveButton
$form.Controls.Add($saveButton)

$cancelButton = New-Object System.Windows.Forms.Button
$cancelButton.Location = New-Object System.Drawing.Point(310,260)
$cancelButton.Size = New-Object System.Drawing.Size(80,30)
$cancelButton.Text = 'Cancel'
$cancelButton.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$form.CancelButton = $cancelButton
$form.Controls.Add($cancelButton)

$result = $form.ShowDialog()

if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
    $project = $projectCombo.Text
    if ($project -eq '+ New Project...') {
        $project = $projectCombo.Text
    }
    Write-Output ($project + '|SEPARATOR|' + $descBox.Text)
} else {
    Write-Output 'CANCELLED'
}
`, timeInfo, projectListPS)

	output, err := session.Interactive("", "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		// The script reports a cancelled dialog on stdout, so reaching here at
		// all means the dialog never ran.
		log.Printf("save dialog: %v", err)
		return DialogResult{Cancelled: true}
	}

	result := strings.TrimSpace(output)
	if result == "CANCELLED" || result == "" {
		return DialogResult{Cancelled: true}
	}

	parts := strings.SplitN(result, "|SEPARATOR|", 2)
	if len(parts) != 2 {
		return DialogResult{Cancelled: true}
	}

	return DialogResult{
		Project:     strings.TrimSpace(parts[0]),
		Description: strings.TrimSpace(parts[1]),
		Cancelled:   false,
	}
}
