package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mpdroog/ezhours/apptracker"
)

// useTempHours points the package at a scratch folder for one test.
func useTempHours(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := HoursDir
	HoursDir = dir
	t.Cleanup(func() { HoursDir = old })
	return dir
}

func at(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

// TestSaveEntryFormat pins the file format, which is read by hand and by
// cmd/tidyhours: a date header per day, a blank line between days, times
// indented by one space.
func TestSaveEntryFormat(t *testing.T) {
	dir := useTempHours(t)

	if err := SaveEntry("portal", at(t, "2026-03-13 14:01"), at(t, "2026-03-13 16:49"), "work on activeusers", nil); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}
	// Same day: no second header.
	if err := SaveEntry("portal", at(t, "2026-03-13 16:53"), at(t, "2026-03-13 17:12"), "fix enduser form", nil); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}
	// Next day: blank line, then a new header.
	if err := SaveEntry("portal", at(t, "2026-03-16 12:22"), at(t, "2026-03-16 12:28"), "30d btn", nil); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}

	want := "13mar\n 14:01 - 16:49\nwork on activeusers\n 16:53 - 17:12\nfix enduser form\n\n16mar\n 12:22 - 12:28\n30d btn\n"
	got, err := os.ReadFile(filepath.Join(dir, "portal.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestSaveEntryDescriptionAndApps(t *testing.T) {
	dir := useTempHours(t)

	apps := []apptracker.AppUsage{
		{Name: "Firefox", Duration: 90 * time.Minute},
		{Name: "Terminal", Duration: 30 * time.Minute},
	}
	// Blank and trailing-whitespace lines are dropped, so a description typed
	// with a stray newline does not put a gap in the file. Note the asymmetry:
	// the description is trimmed as a whole, so the first line loses its indent
	// and the rest keep theirs. That is long-standing behaviour and files in the
	// wild are written that way, so it is pinned here rather than fixed.
	desc := "  first line\n\n  second line   \n"
	if err := SaveEntry("nima", at(t, "2026-04-20 19:00"), at(t, "2026-04-20 20:25"), desc, apps); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}

	want := "20apr\n 19:00 - 20:25\nfirst line\n  second line\n" + FormatApps(apps)
	got, err := os.ReadFile(filepath.Join(dir, "nima.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
}

// TestSaveEntryReportsFailure is the point of the rewrite: a save that cannot
// happen has to say so, because the caller resets the tray and drops the entry
// on the strength of a nil error.
func TestSaveEntryReportsFailure(t *testing.T) {
	dir := useTempHours(t)

	// A directory where the project file belongs: every write to it fails.
	if err := os.Mkdir(filepath.Join(dir, "blocked.txt"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := SaveEntry("blocked", at(t, "2026-04-20 19:00"), at(t, "2026-04-20 20:25"), "lost hour", nil); err == nil {
		t.Error("SaveEntry() = nil, want an error when the file cannot be written")
	}
}

func TestNeedsNewDateHeader(t *testing.T) {
	dir := useTempHours(t)
	path := filepath.Join(dir, "x.txt")

	// A file that does not exist yet always needs one.
	need, err := needsNewDateHeader(path, at(t, "2026-03-13 09:00"))
	if err != nil {
		t.Fatalf("needsNewDateHeader: %v", err)
	}
	if !need {
		t.Error("needsNewDateHeader(missing file) = false, want true")
	}

	if err := os.WriteFile(path, []byte("13mar\n 14:01 - 16:49\nwork\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if need, err = needsNewDateHeader(path, at(t, "2026-03-13 17:00")); err != nil || need {
		t.Errorf("needsNewDateHeader(same day) = %v, %v; want false, nil", need, err)
	}
	if need, err = needsNewDateHeader(path, at(t, "2026-03-14 09:00")); err != nil || !need {
		t.Errorf("needsNewDateHeader(next day) = %v, %v; want true, nil", need, err)
	}

	// An unreadable file is an error, not a guess.
	if _, err := needsNewDateHeader(dir, at(t, "2026-03-14 09:00")); err == nil {
		t.Error("needsNewDateHeader(directory) = nil error, want a failure")
	}
}
