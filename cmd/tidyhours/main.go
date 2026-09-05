// Command tidyhours rewrites the "[Apps: ...]" lines in existing hours/*.txt
// files. Older versions of ezhours logged raw window titles, so a single app
// showed up dozens of times (once per browser tab, per shell command, per
// spinner frame). This maps those legacy titles back onto app names and
// re-applies the current summarisation rules.
//
// Usage:
//
//	go run ./cmd/tidyhours hours/*.txt   # dry run, prints a diff
//	go run ./cmd/tidyhours -w hours/*.txt
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mpdroog/ezhours/apptracker"
	"github.com/mpdroog/ezhours/storage"
)

var (
	appsLineRe = regexp.MustCompile(`^(\s*)\[Apps: (.*)\]\s*$`)
	// entryRe matches one "name 12m" / "name 34s" field.
	entryRe = regexp.MustCompile(`^(.*) (\d+)([hms])$`)
)

// rule maps a legacy window title onto the app that owned the window.
type rule struct {
	re  *regexp.Regexp
	app string
}

// rules are applied in order; the first match wins.
var rules = []rule{
	// Claude Code decorates its terminal title with a spinner frame.
	{regexp.MustCompile(`^[✳✻✽✢·◐◑◒◓]\s`), "Claude Code"},
	{regexp.MustCompile(`^Claude Code$`), "Claude Code"},

	{regexp.MustCompile(`Mozilla Firefox$`), "Firefox"},
	{regexp.MustCompile(`Thunderbird$|^Sending Message - |^Write: `), "Thunderbird"},
	{regexp.MustCompile(`Discord$`), "Discord"},

	// Terminals: a bare path, a prompt, or a shell command with its cwd.
	{regexp.MustCompile(`^(~|/home/|/h/)`), "Terminal"},
	{regexp.MustCompile(`^[a-z_][-\w.]*@[-\w.]+:`), "Terminal"},
	{regexp.MustCompile(`^(git|vi|vim|nvim|claude|ssh|sudo|top|ls|cd|mkdir|cp|mv|rm|yt-dlp|go|npm|make)\s|^\.{1,2}/|^!\s`), "Terminal"},
	{regexp.MustCompile(`^Terminal\b|NVIM$|^Ghostty$|^Alacritty$`), "Terminal"},

	{regexp.MustCompile(`^Ulauncher\b`), "Ulauncher"},
	{regexp.MustCompile(`^Whisker Menu$`), "Whisker Menu"},
	{regexp.MustCompile(`Thunar$`), "Thunar"},
	{regexp.MustCompile(`^Xed$|^\*?Unsaved Document`), "Xed"},
	{regexp.MustCompile(`^Celluloid$|\.(webm|mp4|mkv)$`), "Celluloid"},
}

// classify maps a legacy window title onto an app name, or returns the title
// unchanged when no rule matches (it is nearly always brief enough to land in
// the "other" bucket anyway).
func classify(title string) string {
	// A leading bell is a notification marker, not part of the title.
	title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(title), "🔔"))

	for _, r := range rules {
		if r.re.MatchString(title) {
			return r.app
		}
	}
	return title
}

// parseApps splits an "[Apps: ...]" body into durations per app. Fields are
// comma-separated but titles may contain commas too, so a field only ends
// where a duration does.
func parseApps(body string) map[string]time.Duration {
	out := make(map[string]time.Duration)

	var field string
	for _, part := range strings.Split(body, ", ") {
		if field != "" {
			field += ", " + part
		} else {
			field = part
		}

		m := entryRe.FindStringSubmatch(field)
		if m == nil {
			continue // comma inside a title; keep collecting
		}
		field = ""

		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		unit := map[string]time.Duration{
			"h": time.Hour, "m": time.Minute, "s": time.Second,
		}[m[3]]
		out[classify(m[1])] += time.Duration(n) * unit
	}
	return out
}

func tidy(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		m := appsLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		usage := apptracker.Summarize(parseApps(m[2]))
		lines[i] = strings.TrimSuffix(storage.FormatApps(usage), "\n")
	}
	return strings.Join(lines, "\n")
}

func main() {
	write := flag.Bool("w", false, "write changes back to the files")
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: tidyhours [-w] file...")
		os.Exit(2)
	}

	for _, path := range flag.Args() {
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}

		out := tidy(string(raw))
		if out == string(raw) {
			continue
		}

		if !*write {
			fmt.Printf("--- %s (dry run)\n%s\n", path, out)
			continue
		}
		if err := os.WriteFile(path, []byte(out), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("rewrote %s\n", path)
	}
}
