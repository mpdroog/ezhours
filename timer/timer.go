// Package timer is the stopwatch behind the tray: one running session at a
// time, safe to read from the goroutine that repaints the icon every second.
package timer

import (
	"fmt"
	"sync"
	"time"
)

// Timer measures one work session. The zero value is not usable; call New.
type Timer struct {
	mu        sync.RWMutex
	running   bool
	startTime time.Time
	endTime   time.Time
}

// New returns a stopped timer.
func New() *Timer {
	return &Timer{}
}

// Start begins a session, discarding any previous one.
func (t *Timer) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = true
	t.startTime = time.Now()
}

// Stop ends the session and fixes its end time.
func (t *Timer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = false
	t.endTime = time.Now()
}

// IsRunning reports whether a session is in progress.
func (t *Timer) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
}

// Elapsed is how long the session has run, or ran if it has stopped.
func (t *Timer) Elapsed() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.running {
		return time.Since(t.startTime)
	}
	return t.endTime.Sub(t.startTime)
}

// StartTime is when the session began.
func (t *Timer) StartTime() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.startTime
}

// EndTime is when the session was stopped, zero while it runs.
func (t *Timer) EndTime() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.endTime
}

// FormatDuration renders a duration as HH:MM:SS for the tray title.
func FormatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
