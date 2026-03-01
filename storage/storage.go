package storage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mpdroog/ezhours/apptracker"
)

var HoursDir = "hours"

// GetProjects returns list of existing project names from hours/*.txt files
func GetProjects() ([]string, error) {
	entries, err := os.ReadDir(HoursDir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(HoursDir, 0755); err != nil {
			return nil, err
		}
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var projects []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
			name := strings.TrimSuffix(e.Name(), ".txt")
			projects = append(projects, name)
		}
	}
	return projects, nil
}

// SaveEntry appends a time entry to the project file
func SaveEntry(project string, start, end time.Time, description string, appUsage []apptracker.AppUsage) error {
	if err := os.MkdirAll(HoursDir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(HoursDir, project+".txt")

	// Check if we need to add date header
	needsDateHeader := needsNewDateHeader(filePath, start)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Format: "02jan" (day + lowercase month abbreviation)
	dateHeader := strings.ToLower(start.Format("02Jan"))

	if needsDateHeader {
		// Add newline before date header if file is not empty
		if info, err := f.Stat(); err == nil && info.Size() > 0 {
			fmt.Fprintf(f, "\n%s\n", dateHeader)
		} else {
			fmt.Fprintf(f, "%s\n", dateHeader)
		}
	}

	// Format: " HH:mm - HH:mm" (leading space for indentation)
	timeRange := fmt.Sprintf(" %s - %s\n",
		start.Format("15:04"),
		end.Format("15:04"))
	f.WriteString(timeRange)

	// Description - write each non-empty line
	description = strings.TrimSpace(description)
	if description != "" {
		for _, line := range strings.Split(description, "\n") {
			line = strings.TrimRight(line, " \t")
			if line != "" {
				f.WriteString(line + "\n")
			}
		}
	}

	// App usage - write as indented list
	if len(appUsage) > 0 {
		f.WriteString("  [Apps: ")
		apps := make([]string, 0, len(appUsage))
		for _, app := range appUsage {
			mins := int(app.Duration.Minutes())
			if mins > 0 {
				apps = append(apps, fmt.Sprintf("%s %dm", app.Name, mins))
			} else {
				secs := int(app.Duration.Seconds())
				apps = append(apps, fmt.Sprintf("%s %ds", app.Name, secs))
			}
		}
		f.WriteString(strings.Join(apps, ", "))
		f.WriteString("]\n")
	}

	return nil
}

// needsNewDateHeader checks if the last entry in file is from a different day
func needsNewDateHeader(filePath string, entryDate time.Time) bool {
	f, err := os.Open(filePath)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return true
	}
	defer f.Close()

	// Scan for last date header
	var lastDateHeader string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Date headers are like "31jan", "01feb" - 5 characters
		if len(line) == 5 && isDateHeader(line) {
			lastDateHeader = line
		}
	}

	currentDateHeader := strings.ToLower(entryDate.Format("02Jan"))
	return lastDateHeader != currentDateHeader
}

// isDateHeader checks if a string looks like a date header (e.g., "31jan")
func isDateHeader(s string) bool {
	if len(s) != 5 {
		return false
	}
	// First two chars should be digits
	if s[0] < '0' || s[0] > '9' || s[1] < '0' || s[1] > '9' {
		return false
	}
	// Last three chars should be lowercase letters (month abbreviation)
	for i := 2; i < 5; i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}
