package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mpdroog/ezhours/storage"
)

// DialogResult holds the result from the save dialog
type DialogResult struct {
	Project     string
	Description string
	Cancelled   bool
}

// ShowSaveDialog displays a native OS dialog for saving time entries
func ShowSaveDialog(startTime, endTime time.Time) DialogResult {
	projects, _ := storage.GetProjects()

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
		return showMacDialog(projects, timeInfo) // fallback
	}
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

	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		return DialogResult{Cancelled: true}
	}

	result := strings.TrimSpace(string(output))
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

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return DialogResult{Cancelled: true}
	}

	result := strings.TrimSpace(string(output))
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
