package ui

import (
	"encoding/base64"
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/mpdroog/ezhours/session"
)

// ShowSaveError reports an entry that could not be written, and hands the entry
// back to the user in something they can select and copy out of.
//
// Losing the write is bad; losing the hour with it is worse, and by the time
// this is called the timer has already been reset -- the text on screen is the
// only remaining copy. So it is shown in a text view rather than a message
// label: GTK message text is not selectable, and an hour you can read but not
// copy still has to be retyped.
func ShowSaveError(path, entry string, saveErr error) {
	body := saveErrorText(path, entry, saveErr)

	var err error
	switch runtime.GOOS {
	case "darwin":
		err = showMacError(body)
	case "windows":
		err = showWindowsError(body)
	default:
		err = showLinuxError(body)
	}
	if err != nil && !cancelled(err) {
		// The dialog failing is usually the same fault as the save failing, so
		// this is where the entry ends up if the screen never gets it.
		log.Printf("save error dialog: %v (entry was: %s)", err, strings.TrimSpace(entry))
	}
}

// saveErrorText is what the dialog shows: what went wrong, where it was meant to
// go, and then the entry on its own so it can be selected in one sweep.
func saveErrorText(path, entry string, saveErr error) string {
	var b strings.Builder
	b.WriteString("EZHours could not save this entry.\n\n")
	fmt.Fprintf(&b, "%v\n\n", saveErr)
	fmt.Fprintf(&b, "Copy the lines below into %s to keep the hour.\n", path)
	b.WriteString(strings.Repeat("-", 60) + "\n")
	b.WriteString(entry)
	return b.String()
}

// showLinuxError uses --text-info, which is a real text view: its content can be
// selected and copied, which --error (a label) cannot.
func showLinuxError(body string) error {
	_, err := session.Interactive(body, "zenity", "--text-info",
		"--title=EZHours - Entry not saved",
		"--width=620", "--height=460",
		"--ok-label=Close")
	return err
}

// showMacError puts the text in an editable field, because that is the only
// part of an AppleScript dialog the user can select from.
func showMacError(body string) error {
	script := fmt.Sprintf(`display dialog "EZHours could not save this entry. Copy the text below." default answer %s with title "EZHours - Entry not saved" buttons {"Close"} default button "Close" with icon caution`,
		appleScriptString(body))
	_, err := session.Interactive("", "osascript", "-e", script)
	return err
}

// appleScriptString renders a Go string as an AppleScript expression.
// AppleScript has no escape for a newline inside a literal, so the lines are
// concatenated with `return` instead.
func appleScriptString(s string) string {
	lines := strings.Split(s, "\n")
	quoted := make([]string, len(lines))
	for i, line := range lines {
		line = strings.ReplaceAll(line, `\`, `\\`)
		line = strings.ReplaceAll(line, `"`, `\"`)
		quoted[i] = `"` + line + `"`
	}
	return strings.Join(quoted, " & return & ")
}

// base64Std hands the text to PowerShell without quoting it: the entry is
// arbitrary user text, and a description containing a quote would otherwise
// rewrite the script.
func base64Std(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// showWindowsError uses a read-only text box, which is still selectable.
func showWindowsError(body string) error {
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$form = New-Object System.Windows.Forms.Form
$form.Text = 'EZHours - Entry not saved'
$form.Size = New-Object System.Drawing.Size(620,480)
$form.StartPosition = 'CenterScreen'

$box = New-Object System.Windows.Forms.TextBox
$box.Multiline = $true
$box.ReadOnly = $true
$box.ScrollBars = 'Vertical'
$box.Location = New-Object System.Drawing.Point(10,10)
$box.Size = New-Object System.Drawing.Size(580,390)
$box.Font = New-Object System.Drawing.Font('Consolas',10)
$box.Lines = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')).Split([char]10)
$form.Controls.Add($box)

$close = New-Object System.Windows.Forms.Button
$close.Location = New-Object System.Drawing.Point(510,410)
$close.Size = New-Object System.Drawing.Size(80,30)
$close.Text = 'Close'
$close.DialogResult = [System.Windows.Forms.DialogResult]::OK
$form.AcceptButton = $close
$form.Controls.Add($close)

$form.ShowDialog() | Out-Null
`, base64Std(body))

	_, err := session.Interactive("", "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	return err
}
