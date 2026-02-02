package timer

import (
	"fmt"
	"sync"
	"time"
)

type Timer struct {
	mu        sync.RWMutex
	running   bool
	startTime time.Time
	endTime   time.Time
}

func New() *Timer {
	return &Timer{}
}

func (t *Timer) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = true
	t.startTime = time.Now()
}

func (t *Timer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = false
	t.endTime = time.Now()
}

func (t *Timer) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
}

func (t *Timer) Elapsed() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.running {
		return time.Since(t.startTime)
	}
	return t.endTime.Sub(t.startTime)
}

func (t *Timer) StartTime() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.startTime
}

func (t *Timer) EndTime() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.endTime
}

func FormatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
