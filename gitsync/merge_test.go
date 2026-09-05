package gitsync

import "testing"

func TestMergeFileBothAppendDifferentDays(t *testing.T) {
	base := "02sep\n 09:00 - 10:00\nkickoff\n"
	ours := base + "\n03sep\n 11:00 - 12:00\nlaptop work\n"
	theirs := base + "\n04sep\n 08:00 - 09:30\ndesktop work\n"

	got := mergeFile(base, ours, theirs)
	want := "02sep\n 09:00 - 10:00\nkickoff\n" +
		"\n03sep\n 11:00 - 12:00\nlaptop work\n" +
		"\n04sep\n 08:00 - 09:30\ndesktop work\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMergeFileBothAppendSameDaySortsByStart(t *testing.T) {
	base := "02sep\n 09:00 - 10:00\nkickoff\n"
	ours := "02sep\n 09:00 - 10:00\nkickoff\n 14:00 - 15:00\nafternoon\n"
	theirs := "02sep\n 09:00 - 10:00\nkickoff\n 11:00 - 12:00\nlate morning\n"

	got := mergeFile(base, ours, theirs)
	want := "02sep\n 09:00 - 10:00\nkickoff\n" +
		" 11:00 - 12:00\nlate morning\n" +
		" 14:00 - 15:00\nafternoon\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMergeFileIdenticalEntryIsNotDuplicated(t *testing.T) {
	base := "02sep\n 09:00 - 10:00\nkickoff\n"
	entry := base + " 11:00 - 12:00\nsame work\n"

	got := mergeFile(base, entry, entry)
	if got != entry {
		t.Errorf("got:\n%q\nwant:\n%q", got, entry)
	}
}

func TestMergeFileRespectsDeletion(t *testing.T) {
	base := "02sep\n 09:00 - 10:00\nkickoff\n 11:00 - 12:00\nmistake\n"
	ours := "02sep\n 09:00 - 10:00\nkickoff\n" // we removed the bad entry
	theirs := base + " 13:00 - 14:00\nnew work\n"

	got := mergeFile(base, ours, theirs)
	want := "02sep\n 09:00 - 10:00\nkickoff\n 13:00 - 14:00\nnew work\n"
	if got != want {
		t.Errorf("deleted entry came back:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestMergeFileRespectsWholeDayDeletion(t *testing.T) {
	base := "02sep\n 09:00 - 10:00\nkickoff\n\n03sep\n 09:00 - 10:00\noops\n"
	ours := "02sep\n 09:00 - 10:00\nkickoff\n"
	theirs := base

	got := mergeFile(base, ours, theirs)
	want := "02sep\n 09:00 - 10:00\nkickoff\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMergeFileEditedDescriptionOnOneSideWins(t *testing.T) {
	base := "02sep\n 09:00 - 10:00\ntyop\n"
	ours := "02sep\n 09:00 - 10:00\ntypo fixed\n"
	theirs := base

	if got := mergeFile(base, ours, theirs); got != ours {
		t.Errorf("got %q want %q", got, ours)
	}
	if got := mergeFile(base, theirs, ours); got != ours {
		t.Errorf("reversed sides: got %q want %q", got, ours)
	}
}

func TestMergeFileConcurrentEditKeepsBothLines(t *testing.T) {
	base := "02sep\n 09:00 - 10:00\n"
	ours := "02sep\n 09:00 - 10:00\nreviewed the PR\n"
	theirs := "02sep\n 09:00 - 10:00\npaired on the parser\n"

	got := mergeFile(base, ours, theirs)
	want := "02sep\n 09:00 - 10:00\nreviewed the PR\npaired on the parser\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMergeFileUnrelatedHistories(t *testing.T) {
	ours := "02sep\n 09:00 - 10:00\nlaptop\n"
	theirs := "02sep\n 14:00 - 15:00\ndesktop\n"

	got := mergeFile("", ours, theirs)
	want := "02sep\n 09:00 - 10:00\nlaptop\n 14:00 - 15:00\ndesktop\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMergeFileKeepsAppsLineWithItsEntry(t *testing.T) {
	base := "02sep\n 09:00 - 10:00\nkickoff\n  [Apps: vim 42m]\n"
	ours := base
	theirs := base + " 11:00 - 12:00\nmore\n  [Apps: firefox 55m]\n"

	got := mergeFile(base, ours, theirs)
	want := "02sep\n 09:00 - 10:00\nkickoff\n  [Apps: vim 42m]\n" +
		" 11:00 - 12:00\nmore\n  [Apps: firefox 55m]\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMergeFilePreservesLeadingNotes(t *testing.T) {
	base := "# invoice sent up to 01sep\n\n02sep\n 09:00 - 10:00\nkickoff\n"
	ours := base
	theirs := base + "\n03sep\n 09:00 - 10:00\nmore\n"

	got := mergeFile(base, ours, theirs)
	want := "# invoice sent up to 01sep\n\n02sep\n 09:00 - 10:00\nkickoff\n" +
		"\n03sep\n 09:00 - 10:00\nmore\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMergeTreesFileAddedOnEachSide(t *testing.T) {
	base := map[string]string{"a.txt": "02sep\n 09:00 - 10:00\n"}
	ours := map[string]string{"a.txt": base["a.txt"], "b.txt": "02sep\n 11:00 - 12:00\n"}
	theirs := map[string]string{"a.txt": base["a.txt"], "c.txt": "02sep\n 13:00 - 14:00\n"}

	got := mergeTrees(base, ours, theirs)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if _, ok := got[name]; !ok {
			t.Errorf("%s missing from merge", name)
		}
	}
}

func TestMergeTreesFileDeletedOnOneSideStaysDeleted(t *testing.T) {
	base := map[string]string{"old.txt": "02sep\n 09:00 - 10:00\n"}
	ours := map[string]string{}
	theirs := map[string]string{"old.txt": base["old.txt"]}

	if got := mergeTrees(base, ours, theirs); len(got) != 0 {
		t.Errorf("deleted file came back: %v", got)
	}
}

func TestMergeTreesNonHoursFileKeepsOurs(t *testing.T) {
	base := map[string]string{"notes.md": "a\n"}
	ours := map[string]string{"notes.md": "ours\n"}
	theirs := map[string]string{"notes.md": "theirs\n"}

	if got := mergeTrees(base, ours, theirs)["notes.md"]; got != "ours\n" {
		t.Errorf("got %q want %q", got, "ours\n")
	}
}

func TestParseRoundTrip(t *testing.T) {
	in := "02sep\n 09:00 - 10:00\nkickoff\n  [Apps: vim 42m]\n\n03sep\n 11:00 - 12:00\nmore\n"
	if got := parseDoc(in).render(); got != in {
		t.Errorf("round trip changed the file:\ngot:\n%q\nwant:\n%q", got, in)
	}
}
