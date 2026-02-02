package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/mpdroog/ezhours/storage"
)

const newProjectOption = "+ Add New Project..."

// ShowSaveDialog displays a modal for saving time entries
func ShowSaveDialog(app fyne.App, startTime, endTime time.Time, onSave func(project, description string), onCancel func()) {
	win := app.NewWindow("Save Time Entry")
	win.Resize(fyne.NewSize(400, 350))
	win.CenterOnScreen()

	// Get existing projects
	projects, _ := storage.GetProjects()
	options := append(projects, newProjectOption)

	var selectedProject string

	// Project dropdown
	projectSelect := widget.NewSelect(options, nil)

	// New project entry (hidden by default)
	newProjectEntry := widget.NewEntry()
	newProjectEntry.SetPlaceHolder("Enter project name...")
	newProjectRow := container.NewBorder(nil, nil, widget.NewLabel("Name:"), nil, newProjectEntry)
	newProjectRow.Hide()

	projectSelect.OnChanged = func(s string) {
		if s == newProjectOption {
			newProjectRow.Show()
			selectedProject = ""
		} else {
			newProjectRow.Hide()
			selectedProject = s
		}
	}

	if len(projects) > 0 {
		projectSelect.SetSelected(projects[0])
		selectedProject = projects[0]
	}

	// Description textarea
	descriptionEntry := widget.NewMultiLineEntry()
	descriptionEntry.SetPlaceHolder("What did you work on?")
	descriptionEntry.SetMinRowsVisible(6)

	// Time display (read-only)
	duration := endTime.Sub(startTime)
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	timeLabel := widget.NewLabel(fmt.Sprintf("Time: %s - %s (%dh %dm)",
		startTime.Format("15:04"),
		endTime.Format("15:04"),
		hours, minutes))

	// Buttons
	saveBtn := widget.NewButton("Save", func() {
		project := selectedProject
		if project == "" && newProjectEntry.Text != "" {
			project = newProjectEntry.Text
		}
		if project != "" {
			onSave(project, descriptionEntry.Text)
		}
		win.Close()
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("Cancel", func() {
		if onCancel != nil {
			onCancel()
		}
		win.Close()
	})

	buttons := container.NewHBox(cancelBtn, saveBtn)

	// Layout
	content := container.NewVBox(
		timeLabel,
		widget.NewSeparator(),
		widget.NewLabel("Project:"),
		projectSelect,
		newProjectRow,
		widget.NewLabel("Description:"),
		descriptionEntry,
		widget.NewSeparator(),
		container.NewHBox(container.NewHBox(), buttons),
	)

	win.SetContent(container.NewPadded(content))
	win.SetOnClosed(func() {
		if onCancel != nil {
			onCancel()
		}
	})
	win.Show()
}
