package ui

import (
	"errors"
	"testing"

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
