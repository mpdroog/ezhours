// Package apptracker records which applications were focused while the timer
// ran, so a session summary says what the time went into.
package apptracker

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

const (
	// minDuration is the least time an app must be focused to be listed on its
	// own. The poll interval is 2s, so anything shorter is noise.
	minDuration = time.Minute
	// maxApps caps how many apps are listed before the rest is summarised.
	maxApps = 8
)

// AppUsage represents time spent in an application
type AppUsage struct {
	Name     string
	Duration time.Duration
}

// Tracker monitors which applications are being used
type Tracker struct {
	mu           sync.RWMutex
	running      bool
	stopCh       chan struct{}
	appTime      map[string]time.Duration
	lastApp      string
	lastSwitchAt time.Time
	pollInterval time.Duration
}

// New creates a new application tracker
func New() *Tracker {
	return &Tracker{
		appTime:      make(map[string]time.Duration),
		pollInterval: 2 * time.Second, // Poll every 2 seconds
	}
}

// Start begins tracking active applications
func (t *Tracker) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return
	}

	t.running = true
	t.stopCh = make(chan struct{})
	t.appTime = make(map[string]time.Duration)
	t.lastApp = ""
	t.lastSwitchAt = time.Now()

	go t.poll()
}

// Stop stops tracking and finalizes the last app's time
func (t *Tracker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	t.running = false
	close(t.stopCh)

	// Record time for the last active app
	if t.lastApp != "" {
		t.appTime[t.lastApp] += time.Since(t.lastSwitchAt)
	}
}

// GetUsage returns a sorted, summarised list of app usage (most used first).
func (t *Tracker) GetUsage() []AppUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return Summarize(t.appTime)
}

// Summarize sorts app time by duration and folds everything below minDuration
// or past maxApps into a single "other" entry, so a session summary stays
// readable instead of listing every window that was briefly focused.
func Summarize(appTime map[string]time.Duration) []AppUsage {
	var usage []AppUsage
	for app, dur := range appTime {
		usage = append(usage, AppUsage{Name: app, Duration: dur})
	}

	// Sort by duration (descending), name as tie-breaker for stable output
	sort.Slice(usage, func(i, j int) bool {
		if usage[i].Duration != usage[j].Duration {
			return usage[i].Duration > usage[j].Duration
		}
		return usage[i].Name < usage[j].Name
	})

	var (
		top       []AppUsage
		other     time.Duration
		otherApps int
	)
	for _, u := range usage {
		if len(top) < maxApps && u.Duration >= minDuration {
			top = append(top, u)
			continue
		}
		other += u.Duration
		otherApps++
	}

	if other >= minDuration {
		top = append(top, AppUsage{
			Name:     fmt.Sprintf("other (%d apps)", otherApps),
			Duration: other,
		})
	}

	return top
}

// probeLog reports what the active-window probe says, without saying it twice.
//
// The probe runs every couple of seconds, so a display it cannot reach -- the
// usual cause, and one that lasts for the whole session -- would otherwise
// repeat one sentence tens of thousands of times a day and bury the rest of the
// log. Each distinct failure is reported once, and so is the recovery, which is
// what tells you whether the gap in a session summary was real idle time or a
// tracker that was blind.
type probeLog struct {
	last string
}

func (p *probeLog) report(err error) {
	if err == nil {
		if p.last != "" {
			log.Printf("apptracker: active window readable again")
			p.last = ""
		}
		return
	}
	if msg := err.Error(); msg != p.last {
		p.last = msg
		log.Printf("apptracker: cannot read the active window, app usage will be incomplete: %v", err)
	}
}

func (t *Tracker) poll() {
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	var probe probeLog

	// Get initial app
	app, err := getActiveApp()
	probe.report(err)
	if app != "" {
		t.mu.Lock()
		t.lastApp = app
		t.lastSwitchAt = time.Now()
		t.mu.Unlock()
	}

	for {
		select {
		case <-ticker.C:
			app, err = getActiveApp()
			probe.report(err)
			if app == "" {
				continue
			}

			t.mu.Lock()
			if app != t.lastApp {
				// App switched - record time for previous app
				if t.lastApp != "" {
					t.appTime[t.lastApp] += time.Since(t.lastSwitchAt)
				}
				t.lastApp = app
				t.lastSwitchAt = time.Now()
			}
			t.mu.Unlock()

		case <-t.stopCh:
			return
		}
	}
}
