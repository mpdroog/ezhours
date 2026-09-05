package gitsync

// Union merging for the append-only hours/*.txt files.
//
// go-git has no merge engine, so when two devices have both appended entries
// we merge the files ourselves. The format is regular enough to merge
// structurally rather than by diffing lines:
//
//	02sep                 <- date header, starts a block
//	 09:00 - 10:00        <- time range, starts an entry
//	fixed the parser      <- description lines belong to the entry
//	  [Apps: vim 42m]     <- app summary belongs to the entry
//
// Blocks are keyed by date, entries by their time range. Both levels are a
// three-way set merge against the merge base, so an entry deleted on one
// device stays deleted instead of being resurrected, while entries added on
// either device are kept.

import (
	"regexp"
	"sort"
	"strings"

	"github.com/mpdroog/ezhours/storage"
)

// timeRangeRe matches the " 09:00 - 10:00" line that opens an entry.
var timeRangeRe = regexp.MustCompile(`^(\d{2}:\d{2}) - (\d{2}:\d{2})$`)

type entry struct {
	key   string   // the trimmed time range, e.g. "09:00 - 10:00"
	lines []string // the whole entry verbatim, time range line included
}

type block struct {
	date    string
	entries []entry
}

// doc is a parsed hours file. Lines that appear before the first date header
// are kept verbatim in header so hand-written notes at the top survive.
type doc struct {
	header []string
	blocks []block
}

// parseDoc splits content into blocks and entries. Blank lines are dropped;
// render puts them back, which gives both sides a canonical shape to compare.
func parseDoc(content string) *doc {
	d := &doc{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, " \t\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		switch {
		case storage.IsDateHeader(trimmed):
			d.blocks = append(d.blocks, block{date: trimmed})

		case timeRangeRe.MatchString(trimmed):
			if len(d.blocks) == 0 {
				// Entry without a date header; keep it in an unnamed block so
				// it is not silently dropped.
				d.blocks = append(d.blocks, block{})
			}
			b := &d.blocks[len(d.blocks)-1]
			b.entries = append(b.entries, entry{key: trimmed, lines: []string{line}})

		default:
			if len(d.blocks) == 0 {
				d.header = append(d.header, line)
				continue
			}
			b := &d.blocks[len(d.blocks)-1]
			if len(b.entries) == 0 {
				// Stray line under a date header: attach it to a keyless entry
				// so it keeps its position.
				b.entries = append(b.entries, entry{})
			}
			e := &b.entries[len(b.entries)-1]
			e.lines = append(e.lines, line)
		}
	}
	return d
}

// render writes the document back out in the layout storage.SaveEntry uses:
// a blank line between date blocks, none within one.
func (d *doc) render() string {
	var sb strings.Builder
	for _, line := range d.header {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	first := len(d.header) == 0
	for _, b := range d.blocks {
		if b.date != "" {
			if !first {
				sb.WriteString("\n")
			}
			sb.WriteString(b.date)
			sb.WriteString("\n")
			first = false
		}
		for _, e := range b.entries {
			for _, line := range e.lines {
				sb.WriteString(line)
				sb.WriteString("\n")
			}
			first = false
		}
	}
	return sb.String()
}

// mergeDocs performs the three-way merge. base may be nil for unrelated
// histories, in which case everything on both sides counts as an addition.
func mergeDocs(base, ours, theirs *doc) *doc {
	if base == nil {
		base = &doc{}
	}

	out := &doc{header: mergeLines(base.header, ours.header, theirs.header)}

	baseBlocks := indexBlocks(base)
	ourBlocks := indexBlocks(ours)
	theirBlocks := indexBlocks(theirs)

	// Keep our block order, then append dates only the other side has. Date
	// headers carry no year, so ordering them by value is not safe; both
	// devices append chronologically, so preserving order is.
	done := map[string]bool{}
	for _, date := range append(blockOrder(ours), blockOrder(theirs)...) {
		if done[date] {
			continue
		}
		done[date] = true
		if b, keep := mergeBlock(date, baseBlocks[date], ourBlocks[date], theirBlocks[date]); keep {
			out.blocks = append(out.blocks, b)
		}
	}
	return out
}

// mergeBlock merges the entries of one date. Any of the three sides may be nil,
// meaning the date is absent there.
func mergeBlock(date string, base, ours, theirs *block) (block, bool) {
	// A date present in the base but dropped on one side was deleted there.
	if base != nil && (ours == nil || theirs == nil) {
		return block{}, false
	}

	out := block{date: date}
	baseEntries := indexEntries(base)
	ourEntries := indexEntries(ours)
	theirEntries := indexEntries(theirs)

	done := map[string]bool{}
	for _, key := range append(entryOrder(ours), entryOrder(theirs)...) {
		if done[key] {
			continue
		}
		done[key] = true

		b, inBase := baseEntries[key]
		o, inOurs := ourEntries[key]
		t, inTheirs := theirEntries[key]

		switch {
		case inOurs && inTheirs:
			out.entries = append(out.entries, entry{
				key:   key,
				lines: mergeLines(linesOf(b, inBase), o.lines, t.lines),
			})
		case inOurs:
			// Only ours: an addition unless the base had it, i.e. they deleted it.
			if !inBase {
				out.entries = append(out.entries, entry{key: key, lines: o.lines})
			}
		case inTheirs:
			if !inBase {
				out.entries = append(out.entries, entry{key: key, lines: t.lines})
			}
		}
	}

	// Within a day the start times are unambiguous, so sort by them. Keyless
	// entries (stray lines) sort first to stay under their date header.
	sort.SliceStable(out.entries, func(i, j int) bool {
		return out.entries[i].key < out.entries[j].key
	})

	if len(out.entries) == 0 {
		return block{}, false
	}
	return out, true
}

// mergeLines resolves one entry whose content differs between the two sides:
// take whichever side changed, or the union of both when both did.
func mergeLines(base, ours, theirs []string) []string {
	if equalLines(ours, theirs) {
		return ours
	}
	if equalLines(base, ours) {
		return theirs
	}
	if equalLines(base, theirs) {
		return ours
	}
	merged := append([]string{}, ours...)
	for _, line := range theirs {
		if !containsLine(merged, line) {
			merged = append(merged, line)
		}
	}
	return merged
}

func linesOf(e *entry, ok bool) []string {
	if !ok || e == nil {
		return nil
	}
	return e.lines
}

func indexBlocks(d *doc) map[string]*block {
	m := make(map[string]*block, len(d.blocks))
	for i := range d.blocks {
		m[d.blocks[i].date] = &d.blocks[i]
	}
	return m
}

func indexEntries(b *block) map[string]*entry {
	m := map[string]*entry{}
	if b == nil {
		return m
	}
	for i := range b.entries {
		m[b.entries[i].key] = &b.entries[i]
	}
	return m
}

func blockOrder(d *doc) []string {
	out := make([]string, 0, len(d.blocks))
	for _, b := range d.blocks {
		out = append(out, b.date)
	}
	return out
}

func entryOrder(b *block) []string {
	if b == nil {
		return nil
	}
	out := make([]string, 0, len(b.entries))
	for _, e := range b.entries {
		out = append(out, e.key)
	}
	return out
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

// mergeFile unions two versions of one hours file. base may be empty.
func mergeFile(base, ours, theirs string) string {
	var baseDoc *doc
	if base != "" {
		baseDoc = parseDoc(base)
	}
	return mergeDocs(baseDoc, parseDoc(ours), parseDoc(theirs)).render()
}
