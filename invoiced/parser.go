package invoiced

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	// timeEntryRegex matches lines like " 10:09 - 12:34"
	timeEntryRegex = regexp.MustCompile(`^\s+(\d{2}:\d{2})\s*-\s*(\d{2}:\d{2})$`)
	// appUsageRegex matches lines like "  [Apps: ...]"
	appUsageRegex = regexp.MustCompile(`^\s+\[Apps:`)
)

// ParseHoursFile parses an EZHours file and returns an Hour struct for InvoiceD.
func ParseHoursFile(filePath string, year int) (*Hour, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Get project name from filename (without .txt)
	name := strings.TrimSuffix(filepath.Base(filePath), ".txt")

	hour := &Hour{
		Project:  name,
		Name:     name,
		Status:   "NEW",
		Lines:    []HourLine{},
		FilePath: filePath,
	}

	var currentDate string
	var currentLine *HourLine
	var descLines []string
	var totalHours float64

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Check if it's a date header (e.g., "03apr")
		if isDateHeader(line) {
			// Save previous entry if exists
			if currentLine != nil {
				currentLine.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
				hour.Lines = append(hour.Lines, *currentLine)
				currentLine = nil
				descLines = nil
			}
			currentDate = parseDateHeader(line, year)
			continue
		}

		// Check if it's a time entry (e.g., " 10:09 - 12:34")
		if matches := timeEntryRegex.FindStringSubmatch(line); matches != nil {
			// Save previous entry if exists
			if currentLine != nil {
				currentLine.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
				hour.Lines = append(hour.Lines, *currentLine)
				descLines = nil
			}

			hours := calculateHours(matches[1], matches[2])
			totalHours += hours

			currentLine = &HourLine{
				Day:   currentDate,
				Start: matches[1],
				Stop:  matches[2],
				Hours: hours,
			}
			continue
		}

		// Skip app usage lines
		if appUsageRegex.MatchString(line) {
			continue
		}

		// It's a description line
		if currentLine != nil {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				descLines = append(descLines, trimmed)
			}
		}
	}

	// Save last entry
	if currentLine != nil {
		currentLine.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
		hour.Lines = append(hour.Lines, *currentLine)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Format total as string with 2 decimal places
	hour.Total = fmt.Sprintf("%.2f", totalHours)

	return hour, nil
}

// isDateHeader checks if a string looks like a date header (e.g., "03apr")
func isDateHeader(s string) bool {
	s = strings.TrimSpace(s)
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

// parseDateHeader converts "03apr" to "2026-04-03" format
func parseDateHeader(header string, year int) string {
	header = strings.TrimSpace(header)
	if len(header) != 5 {
		return ""
	}

	day := header[0:2]
	monthAbbr := header[2:5]

	// Map month abbreviation to month number
	months := map[string]string{
		"jan": "01", "feb": "02", "mar": "03", "apr": "04",
		"may": "05", "jun": "06", "jul": "07", "aug": "08",
		"sep": "09", "oct": "10", "nov": "11", "dec": "12",
	}

	month, ok := months[monthAbbr]
	if !ok {
		return ""
	}

	return fmt.Sprintf("%d-%s-%s", year, month, day)
}

// calculateHours calculates the decimal hours between two times
func calculateHours(start, stop string) float64 {
	startTime, err1 := time.Parse("15:04", start)
	stopTime, err2 := time.Parse("15:04", stop)
	if err1 != nil || err2 != nil {
		return 0
	}

	duration := stopTime.Sub(startTime)
	return duration.Hours()
}

// ParseAllHoursFiles reads all .txt files from the hours directory and returns parsed Hours.
func ParseAllHoursFiles(hoursDir string, year int) ([]*Hour, error) {
	entries, err := os.ReadDir(hoursDir)
	if err != nil {
		return nil, err
	}

	var hours []*Hour
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		filePath := filepath.Join(hoursDir, entry.Name())

		// Skip empty files
		info, err := entry.Info()
		if err != nil || info.Size() == 0 {
			continue
		}

		hour, err := ParseHoursFile(filePath, year)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}

		// Skip files with no entries
		if len(hour.Lines) == 0 {
			continue
		}

		hours = append(hours, hour)
	}

	return hours, nil
}
