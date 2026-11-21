package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mpdroog/ezhours/session"
)

// TestCancelled separates the two failures that share an exit status: the user
// dismissing the dialog, which is routine, and a dialog that never appeared,
// which loses the entry and has to reach the log.
func TestCancelled(t *testing.T) {
	pressedCancel := func() error {
		_, err := session.Output("sh", "-c", "exit 1")
		return err
	}()
	if !cancelled(pressedCancel) {
		t.Errorf("cancelled(%v) = false, want true", pressedCancel)
	}

	// A cancel on a healthy desktop still prints GLib noise on stderr.
	cancelWithNoise := func() error {
		_, err := session.Output("sh", "-c", "echo '(zenity:1): Gdk-WARNING **: Cannot transform xsetting' >&2; exit 1")
		return err
	}()
	if !cancelled(cancelWithNoise) {
		t.Errorf("cancelled(%v) = false, want true", cancelWithNoise)
	}

	noDisplay := func() error {
		_, err := session.Output("sh", "-c", "echo '(zenity:1): Gtk-WARNING **: cannot open display: :99' >&2; exit 1")
		return err
	}()
	if cancelled(noDisplay) {
		t.Errorf("cancelled(%v) = true, want false", noDisplay)
	}

	notInstalled := func() error {
		_, err := session.Output("ezhours-no-such-zenity")
		return err
	}()
	if cancelled(notInstalled) {
		t.Errorf("cancelled(%v) = true, want false", notInstalled)
	}

	// Any other exit status is a failure, not a dismissal.
	crashed := func() error {
		_, err := session.Output("sh", "-c", "exit 139")
		return err
	}()
	if cancelled(crashed) {
		t.Errorf("cancelled(%v) = true, want false", crashed)
	}

	if cancelled(errors.New("something else")) {
		t.Error("cancelled(plain error) = true, want false")
	}
}

// fakeAsk stands a shell script in for ask for the length of a test.
func fakeAsk(t *testing.T, script string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ask")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := askBin
	askBin = path
	t.Cleanup(func() { askBin = old })
}

// The project and the description come out of one dialog: ask prints them
// one per line.
func TestSaveDialogIsOneWindow(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	fakeAsk(t, `echo "$@" >> `+calls+`
printf 'hitnews\nfixed the parser\n'`)

	got := showLinuxDialog([]string{"hitnews", "xsnews"}, "13:00 - 14:00 (1h 0m)")
	want := DialogResult{Project: "hitnews", Description: "fixed the parser"}
	if got != want {
		t.Errorf("showLinuxDialog() = %+v, want %+v", got, want)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "-mode="); n != 1 {
		t.Errorf("ask was run %d times, want once:\n%s", n, b)
	}
	if !strings.Contains(string(b), "-describe=") {
		t.Errorf("ask was not asked for the description: %s", b)
	}
}

// A blank description is still a saved hour, not a Cancel.
func TestSaveDialogBlankDescription(t *testing.T) {
	fakeAsk(t, `printf 'hitnews\n\n'`)
	got := showLinuxDialog(nil, "")
	if got != (DialogResult{Project: "hitnews"}) {
		t.Errorf("showLinuxDialog() = %+v, want hitnews with no description", got)
	}
}

// A dialog that never answers -- the one that came up where nobody could see
// it -- must not hold the hour hostage: reopening replaces it with a fresh
// dialog, and the answer to that one is what gets saved.
func TestReopenReplacesAStuckDialog(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	fakeAsk(t, `if [ ! -e `+first+` ]; then
	touch `+first+`
	exec sleep 60
fi
printf 'xsnews\nsecond try\n'`)

	if ReopenSaveDialog() {
		t.Error("ReopenSaveDialog() = true with no dialog open")
	}

	done := make(chan DialogResult, 1)
	go func() { done <- showLinuxDialog(nil, "") }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(first); err == nil && ReopenSaveDialog() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the first dialog never started")
		}
		time.Sleep(10 * time.Millisecond)
	}

	select {
	case got := <-done:
		want := DialogResult{Project: "xsnews", Description: "second try"}
		if got != want {
			t.Errorf("after reopening: %+v, want %+v", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reopening did not replace the stuck dialog")
	}
}
