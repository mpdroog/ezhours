package apptracker

import (
	"strings"
	"unicode"
)

// titleSeparators are the separators apps use to prefix a window title with the
// current document/tab/channel, e.g. "PAY — Mozilla Firefox" or
// "#algemeen | XS News - Discord". The app name is always the last segment.
var titleSeparators = []string{" — ", " – ", " | ", " - "}

// normalize strips transient decoration from a raw window identifier.
// Terminal titles carry spinner and notification glyphs ("✳", "◐", "🔔") that
// change every frame, which would otherwise count one window as many apps.
func normalize(s string) string {
	s = strings.TrimFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.TrimSpace(s)
}

// titleToApp reduces a window title to just its app name. Only used when the
// window exposes no class, since titles change per tab and per shell command.
func titleToApp(title string) string {
	s := normalize(title)
	for _, sep := range titleSeparators {
		if i := strings.LastIndex(s, sep); i != -1 {
			s = strings.TrimSpace(s[i+len(sep):])
		}
	}
	return s
}

// prettify upper-cases the first letter so window classes ("firefox") sit next
// to process names ("Discord") without looking out of place.
func prettify(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
