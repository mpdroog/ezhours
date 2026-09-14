// Package session runs the helper programs that need the user's graphical
// session: the save dialog, the file manager, the window queries behind app
// tracking.
//
// It exists because ezhours is normally started by the systemd user manager at
// login, which is early enough that the session has not published DISPLAY yet.
// See desktopEnv for what that breaks and how it is repaired.
package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// probeTimeout bounds a helper that nobody is waiting on: the window queries the
// tracker runs every two seconds, and the environment lookup. An X server that
// has wedged answers none of them and never returns either, which would hang the
// goroutine that called it for the rest of the session. Dialogs are deliberately
// not bounded this way -- see Interactive.
const probeTimeout = 10 * time.Second

// Command is exec.CommandContext for a program that has to reach the display.
func Command(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	if extra := desktopEnv(); len(extra) > 0 {
		cmd.Env = append(os.Environ(), extra...)
	}
	return cmd
}

// CmdError is what a helper failed with, keeping stderr as its own field rather
// than only in the message.
//
// Callers need it apart: for zenity an exit status with nothing on stderr is the
// user pressing Cancel, and the same status with "cannot open display" is the
// dialog never appearing. Those want opposite treatment and are indistinguishable
// from the exit code alone.
type CmdError struct {
	Name   string
	Err    error
	Stderr string
}

func (e *CmdError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("%s: %v: %s", e.Name, e.Err, compact(e.Stderr))
	}
	return fmt.Sprintf("%s: %v", e.Name, e.Err)
}

func (e *CmdError) Unwrap() error { return e.Err }

// Output runs a helper nobody is waiting on and returns its standard output,
// giving up after probeTimeout. A failure comes back as a *CmdError carrying
// what the program said on stderr, because for these tools that line is the
// entire diagnosis.
func Output(name string, arg ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	return run(ctx, "", name, arg...)
}

// Interactive runs a dialog and waits for as long as the person in front of it
// takes. It is deliberately unbounded: a timeout here would close the save
// dialog under someone still typing a description, which loses the entry -- the
// exact failure the dialog exists to prevent.
//
// stdin is passed to the program when non-empty, which is how zenity takes a
// body of text too long to fit in an argument.
func Interactive(stdin, name string, arg ...string) (string, error) {
	return run(context.Background(), stdin, name, arg...)
}

func run(ctx context.Context, stdin, name string, arg ...string) (string, error) {
	cmd := Command(ctx, name, arg...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return "", &CmdError{Name: name, Err: err, Stderr: strings.TrimSpace(stderr.String())}
	}
	return string(out), nil
}

// maxStderr caps a logged message. GTK can answer with a wall of warnings and
// the log has to stay readable.
const maxStderr = 300

// compact folds stderr onto one line for logging.
//
// It keeps every line rather than just the first, because the line that matters
// is not reliably first: zenity prints its "cannot open display" as the same
// kind of GLib warning it prints harmless ones as, so anything that tries to
// pick the interesting line by shape throws away the diagnosis sooner or later.
func compact(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			kept = append(kept, line)
		}
	}
	out := strings.Join(kept, " | ")
	if len(out) > maxStderr {
		out = out[:maxStderr-3] + "..."
	}
	return out
}
