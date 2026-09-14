package session

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestOutputErrorCarriesStderr is why CmdError exists: the exit status alone
// does not say what went wrong, and the caller decides based on stderr.
func TestOutputErrorCarriesStderr(t *testing.T) {
	_, err := Output("sh", "-c", "echo 'cannot open display' >&2; exit 1")
	if err == nil {
		t.Fatal("Output() = nil error, want a failure")
	}

	var cmdErr *CmdError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Output() error is %T, want *CmdError", err)
	}
	if cmdErr.Stderr != "cannot open display" {
		t.Errorf("Stderr = %q, want %q", cmdErr.Stderr, "cannot open display")
	}
	if cmdErr.Name != "sh" {
		t.Errorf("Name = %q, want %q", cmdErr.Name, "sh")
	}
	// The exit status stays reachable through the wrapper.
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Error("error does not unwrap to *exec.ExitError")
	}
	if !strings.Contains(err.Error(), "cannot open display") {
		t.Errorf("Error() = %q, want it to quote stderr", err.Error())
	}
}

// TestOutputSilentFailure is the other shape: a program that fails saying
// nothing, which is how zenity reports that the user pressed Cancel.
func TestOutputSilentFailure(t *testing.T) {
	_, err := Output("sh", "-c", "exit 1")

	var cmdErr *CmdError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Output() error is %T, want *CmdError", err)
	}
	if cmdErr.Stderr != "" {
		t.Errorf("Stderr = %q, want empty", cmdErr.Stderr)
	}
}

func TestOutputMissingProgram(t *testing.T) {
	_, err := Output("ezhours-no-such-program")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("Output() error = %v, want it to unwrap to exec.ErrNotFound", err)
	}
}

func TestCompact(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"one\ntwo\nthree", "one | two | three"},
		{"  spaced  \n\nnext", "spaced | next"},
		{"single", "single"},
		{"", ""},
		{"\n\n", ""},
	} {
		if got := compact(tc.in); got != tc.want {
			t.Errorf("compact(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	long := strings.Repeat("x", maxStderr*2)
	if got := compact(long); len(got) != maxStderr {
		t.Errorf("compact(long) is %d chars, want %d", len(got), maxStderr)
	}
}

// TestStderrKeepsEveryLine is the trap this replaced: zenity reports a display
// it cannot open as the same kind of GLib warning it prints harmlessly, and it
// is not always the first line.
func TestStderrKeepsEveryLine(t *testing.T) {
	_, err := Output("sh", "-c", "echo 'Gdk-WARNING: harmless noise' >&2; echo 'Gtk-WARNING: cannot open display: :99' >&2; exit 1")

	var cmdErr *CmdError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Output() error is %T, want *CmdError", err)
	}
	if !strings.Contains(cmdErr.Stderr, "cannot open display") {
		t.Errorf("Stderr = %q, want it to keep the display failure", cmdErr.Stderr)
	}
	if !strings.Contains(err.Error(), "harmless noise") {
		t.Errorf("Error() = %q, want it to keep every line", err.Error())
	}
}
