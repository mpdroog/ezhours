// Package storage reads and writes the hours folder: one plain-text file per
// project, entries grouped under a date header, meant to stay readable and
// editable by hand.
package storage

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mpdroog/ezhours/apptracker"
)

// HoursDir is where project files live, relative to the working directory
// unless set to an absolute path. Tests point it at a scratch folder.
var HoursDir = "hours"

// GetHoursDir returns the absolute path to the hours directory, creating it if needed.
func GetHoursDir() (string, error) {
	abs, err := filepath.Abs(HoursDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return "", err
	}
	return abs, nil
}

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

// ProjectPath is the file a project's entries are appended to.
func ProjectPath(project string) string {
	return filepath.Join(HoursDir, project+".txt")
}

// EntryText renders an entry exactly as it would appear in the project file,
// date header included. It is what the user is shown when the write failed, so
// the hour can be pasted in by hand rather than lost.
func EntryText(start, end time.Time, description string, appUsage []apptracker.AppUsage) string {
	return entryText(start, end, description, appUsage, true)
}

// entryText renders one entry. Whether it carries a date header is the caller's
// decision: appending adds one only when the day changes, while a block meant to
// be pasted in by hand always needs to say which day it belongs to.
func entryText(start, end time.Time, description string, appUsage []apptracker.AppUsage, withHeader bool) string {
	var buf bytes.Buffer

	if withHeader {
		// Format: "02jan" (day + lowercase month abbreviation)
		fmt.Fprintf(&buf, "%s\n", strings.ToLower(start.Format("02Jan")))
	}

	// Format: " HH:mm - HH:mm" (leading space for indentation)
	fmt.Fprintf(&buf, " %s - %s\n",
		start.Format("15:04"),
		end.Format("15:04"))

	// Description - write each non-empty line
	description = strings.TrimSpace(description)
	if description != "" {
		for _, line := range strings.Split(description, "\n") {
			line = strings.TrimRight(line, " \t")
			if line != "" {
				buf.WriteString(line + "\n")
			}
		}
	}

	// App usage - write as indented list
	buf.WriteString(FormatApps(appUsage))

	return buf.String()
}

// SaveEntry appends a time entry to the project file.
//
// The entry is built in full before the file is touched, so a failure leaves the
// file exactly as it was rather than half an entry appended to it, and it goes
// out in one write. Every error here reaches the caller: this is the one thing
// the app exists to do, and an hour that was silently not written is worse than
// one that fails loudly.
func SaveEntry(project string, start, end time.Time, description string, appUsage []apptracker.AppUsage) (err error) {
	if err := os.MkdirAll(HoursDir, 0755); err != nil {
		return err
	}

	filePath := ProjectPath(project)

	// Check if we need to add date header
	needsDateHeader, err := needsNewDateHeader(filePath, start)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		// A close that fails on an append is the write failing late -- the disk
		// filled, the network mount went away -- so it must not be dropped, but
		// it must not hide an earlier error either.
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", filePath, cerr)
		}
	}()

	entry := entryText(start, end, description, appUsage, needsDateHeader)

	if needsDateHeader {
		info, err := f.Stat()
		if err != nil {
			return fmt.Errorf("stat %s: %w", filePath, err)
		}
		// Add newline before date header if file is not empty
		if info.Size() > 0 {
			entry = "\n" + entry
		}
	}

	if _, err := f.Write([]byte(entry)); err != nil {
		return fmt.Errorf("write %s: %w", filePath, err)
	}
	return nil
}

// FormatApps renders app usage as the indented "  [Apps: ...]" line, including
// its trailing newline. Returns "" when there is nothing to report.
func FormatApps(appUsage []apptracker.AppUsage) string {
	if len(appUsage) == 0 {
		return ""
	}

	apps := make([]string, 0, len(appUsage))
	for _, app := range appUsage {
		if mins := int(app.Duration.Minutes()); mins > 0 {
			apps = append(apps, fmt.Sprintf("%s %dm", app.Name, mins))
		} else {
			apps = append(apps, fmt.Sprintf("%s %ds", app.Name, int(app.Duration.Seconds())))
		}
	}
	return "  [Apps: " + strings.Join(apps, ", ") + "]\n"
}

// needsNewDateHeader checks if the last entry in file is from a different day.
//
// A file that is not there yet needs one; a file that cannot be read is an
// error, because the alternative is guessing and filing today's hours under
// yesterday's header.
func needsNewDateHeader(filePath string, entryDate time.Time) (bool, error) {
	f, err := os.Open(filePath)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	// Nothing was written to this handle, so a failed close has nothing to
	// report and nothing to lose -- unlike the append in SaveEntry, which
	// checks it.
	defer func() { _ = f.Close() }()

	// Scan for last date header
	var lastDateHeader string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Date headers are like "31jan", "01feb" - 5 characters
		if len(line) == 5 && IsDateHeader(line) {
			lastDateHeader = line
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("read %s: %w", filePath, err)
	}

	currentDateHeader := strings.ToLower(entryDate.Format("02Jan"))
	return lastDateHeader != currentDateHeader, nil
}

// IsDateHeader checks if a string looks like a date header (e.g., "31jan")
func IsDateHeader(s string) bool {
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
