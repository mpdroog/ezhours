package gitsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// newDevice sets up a hours directory wired to the given bare remote, the way
// a freshly installed ezhours on a second machine would look.
func newDevice(t *testing.T, remote string) (*Syncer, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(RemoteEnv, remote)
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, dir
}

func bareRemote(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "hours.git")
	if _, err := git.PlainInit(dir, true); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	return dir
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestSyncBetweenTwoDevices(t *testing.T) {
	remote := bareRemote(t)
	a, dirA := newDevice(t, remote)
	b, dirB := newDevice(t, remote)

	// Device A logs an entry and pushes it.
	write(t, dirA, "portal.txt", "02sep\n 09:00 - 10:00\nkickoff\n")
	if err := a.CommitAndPush("entry one"); err != nil {
		t.Fatalf("A push: %v", err)
	}

	// Device B, which has no commits at all, adopts the remote.
	if err := b.Pull(); err != nil {
		t.Fatalf("B pull: %v", err)
	}
	if got := read(t, dirB, "portal.txt"); !strings.Contains(got, "kickoff") {
		t.Fatalf("B did not receive A's entry: %q", got)
	}

	// B appends its own entry and pushes.
	write(t, dirB, "portal.txt", "02sep\n 09:00 - 10:00\nkickoff\n 11:00 - 12:00\nfrom B\n")
	if err := b.CommitAndPush("entry two"); err != nil {
		t.Fatalf("B push: %v", err)
	}

	// A pulls it back; this is a plain fast-forward.
	if err := a.Pull(); err != nil {
		t.Fatalf("A pull: %v", err)
	}
	if got := read(t, dirA, "portal.txt"); !strings.Contains(got, "from B") {
		t.Fatalf("A did not receive B's entry: %q", got)
	}
}

func TestConcurrentEntriesAreUnionMerged(t *testing.T) {
	remote := bareRemote(t)
	a, dirA := newDevice(t, remote)
	b, dirB := newDevice(t, remote)

	// Shared starting point.
	write(t, dirA, "portal.txt", "02sep\n 09:00 - 10:00\nkickoff\n")
	if err := a.CommitAndPush("base"); err != nil {
		t.Fatalf("A push: %v", err)
	}
	if err := b.Pull(); err != nil {
		t.Fatalf("B pull: %v", err)
	}

	// Both devices log a different day while offline.
	write(t, dirA, "portal.txt", "02sep\n 09:00 - 10:00\nkickoff\n\n03sep\n 09:00 - 10:00\nfrom A\n")
	write(t, dirB, "portal.txt", "02sep\n 09:00 - 10:00\nkickoff\n\n04sep\n 09:00 - 10:00\nfrom B\n")

	// A gets there first, B's push is rejected and must merge before retrying.
	if err := a.CommitAndPush("from A"); err != nil {
		t.Fatalf("A push: %v", err)
	}
	if err := b.CommitAndPush("from B"); err != nil {
		t.Fatalf("B push: %v", err)
	}

	got := read(t, dirB, "portal.txt")
	for _, want := range []string{"kickoff", "from A", "from B"} {
		if !strings.Contains(got, want) {
			t.Errorf("B is missing %q after merge:\n%s", want, got)
		}
	}

	// A pulls the merge and ends up with exactly the same file.
	if err := a.Pull(); err != nil {
		t.Fatalf("A pull: %v", err)
	}
	if a, b := read(t, dirA, "portal.txt"), got; a != b {
		t.Errorf("devices diverged:\nA:\n%s\nB:\n%s", a, b)
	}
}

func TestUnrelatedHistoriesAreMerged(t *testing.T) {
	remote := bareRemote(t)
	a, dirA := newDevice(t, remote)
	b, dirB := newDevice(t, remote)

	// Both machines already had hours before a remote existed.
	write(t, dirA, "portal.txt", "02sep\n 09:00 - 10:00\nfrom A\n")
	write(t, dirB, "portal.txt", "02sep\n 14:00 - 15:00\nfrom B\n")

	if err := a.CommitAndPush("A history"); err != nil {
		t.Fatalf("A push: %v", err)
	}
	if err := b.CommitAndPush("B history"); err != nil {
		t.Fatalf("B push: %v", err)
	}

	got := read(t, dirB, "portal.txt")
	if !strings.Contains(got, "from A") || !strings.Contains(got, "from B") {
		t.Errorf("unrelated histories lost entries:\n%s", got)
	}
	_ = dirA
}

func TestPullIsNoOpWithoutRemote(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(RemoteEnv, "")
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.Enabled() {
		t.Fatal("Enabled without a remote")
	}
	if err := s.Pull(); err != nil {
		t.Errorf("Pull without remote: %v", err)
	}
	// Entries are still committed locally so nothing is lost.
	write(t, dir, "portal.txt", "02sep\n 09:00 - 10:00\nsolo\n")
	if err := s.CommitAndPush("solo"); err != nil {
		t.Errorf("CommitAndPush without remote: %v", err)
	}
	if _, err := s.repo.Head(); err != nil {
		t.Errorf("no local commit was made: %v", err)
	}
}

func TestExistingRemoteIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: remoteName,
		URLs: []string{"git@github.com:me/hours.git"},
	}); err != nil {
		t.Fatal(err)
	}

	t.Setenv(RemoteEnv, "git@github.com:someone-else/hours.git")
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	url, err := s.remoteURL()
	if err != nil {
		t.Fatal(err)
	}
	if url != "git@github.com:me/hours.git" {
		t.Errorf("EZHOURS_REMOTE overwrote the configured remote: %s", url)
	}
}
