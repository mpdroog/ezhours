// Package gitsync keeps the hours directory in sync with a git remote so the
// same time sheets can be edited from several machines.
//
// It is deliberately narrow: pull before the timer starts, commit and push the
// moment an entry is saved. Everything runs through go-git, so no git binary
// has to be installed.
package gitsync

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// RemoteEnv can point at the remote to use when the hours repo has no origin
// yet, e.g. EZHOURS_REMOTE=git@github.com:mpdroog/hours.git
const RemoteEnv = "EZHOURS_REMOTE"

const remoteName = "origin"

// Syncer owns the hours repository. All operations are serialised: a pull
// triggered by starting the timer must not overlap the push of the previous
// entry.
type Syncer struct {
	mu   sync.Mutex
	dir  string
	repo *git.Repository
}

// New opens the repository in dir, creating one if the directory is not a repo
// yet. It never fails because of a missing remote; use Enabled to check whether
// there is anything to sync with.
func New(dir string) (*Syncer, error) {
	repo, err := git.PlainOpen(dir)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		repo, err = git.PlainInit(dir, false)
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dir, err)
	}

	s := &Syncer{dir: dir, repo: repo}
	if err := s.ensureRemote(); err != nil {
		return nil, err
	}
	return s, nil
}

// ensureRemote wires up origin from RemoteEnv when the repo has none.
func (s *Syncer) ensureRemote() error {
	url := strings.TrimSpace(os.Getenv(RemoteEnv))
	if url == "" {
		return nil
	}
	if _, err := s.repo.Remote(remoteName); err == nil {
		return nil
	} else if !errors.Is(err, git.ErrRemoteNotFound) {
		return err
	}
	_, err := s.repo.CreateRemote(&config.RemoteConfig{Name: remoteName, URLs: []string{url}})
	return err
}

// Enabled reports whether a remote is configured. Without one ezhours still
// commits locally, it just has nowhere to push.
func (s *Syncer) Enabled() bool {
	if s == nil {
		return false
	}
	_, err := s.remoteURL()
	return err == nil
}

func (s *Syncer) remoteURL() (string, error) {
	r, err := s.repo.Remote(remoteName)
	if err != nil {
		return "", err
	}
	if len(r.Config().URLs) == 0 {
		return "", git.ErrRemoteNotFound
	}
	return r.Config().URLs[0], nil
}

// Pull brings in remote entries. Local changes are committed first so the merge
// has something to merge, then the remote branch is fast-forwarded in or union
// merged when both sides moved on.
func (s *Syncer) Pull() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pull()
}

func (s *Syncer) pull() error {
	if !s.Enabled() {
		return nil
	}
	auth, err := s.auth()
	if err != nil {
		return err
	}

	if err := s.commitLocal("ezhours: local changes"); err != nil {
		return err
	}

	err = s.repo.Fetch(&git.FetchOptions{
		RemoteName: remoteName,
		Auth:       auth,
		Tags:       git.NoTags,
	})
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		// Freshly created remote; the first push will populate it.
		return nil
	}
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetch: %w", err)
	}

	branch, err := s.branch()
	if err != nil {
		return err
	}

	remoteRef, err := s.repo.Reference(plumbing.NewRemoteReferenceName(remoteName, branch), true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil // remote does not have this branch yet
	}
	if err != nil {
		return err
	}

	head, err := s.repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		// No local commits at all (and nothing to commit): adopt the remote.
		return s.setBranch(branch, remoteRef.Hash())
	}
	if err != nil {
		return err
	}

	if head.Hash() == remoteRef.Hash() {
		return nil
	}

	ours, err := s.repo.CommitObject(head.Hash())
	if err != nil {
		return err
	}
	theirs, err := s.repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return err
	}

	if ahead, err := theirs.IsAncestor(ours); err != nil {
		return err
	} else if ahead {
		return nil // we already contain the remote; Push will publish the rest
	}

	if ff, err := ours.IsAncestor(theirs); err != nil {
		return err
	} else if ff {
		return s.setBranch(branch, theirs.Hash)
	}

	return s.merge(branch, ours, theirs)
}

// setBranch points the branch (and the worktree) at hash.
func (s *Syncer) setBranch(branch string, hash plumbing.Hash) error {
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), hash)
	if err := s.repo.Storer.SetReference(ref); err != nil {
		return err
	}
	wt, err := s.repo.Worktree()
	if err != nil {
		return err
	}
	return wt.Reset(&git.ResetOptions{Mode: git.HardReset, Commit: hash})
}

// merge union merges the two sides and records a real merge commit, so the next
// push is a fast-forward for the remote.
func (s *Syncer) merge(branch string, ours, theirs *object.Commit) error {
	baseFiles := map[string]string{}
	if bases, err := ours.MergeBase(theirs); err == nil && len(bases) > 0 {
		if baseFiles, err = filesOf(bases[0]); err != nil {
			return err
		}
	}
	ourFiles, err := filesOf(ours)
	if err != nil {
		return err
	}
	theirFiles, err := filesOf(theirs)
	if err != nil {
		return err
	}

	merged := mergeTrees(baseFiles, ourFiles, theirFiles)

	// Drop files the merge resolved as deleted, then write the rest.
	for path := range ourFiles {
		if _, keep := merged[path]; !keep {
			if err := os.Remove(filepath.Join(s.dir, path)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	for path, content := range merged {
		full := filepath.Join(s.dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			return err
		}
	}

	wt, err := s.repo.Worktree()
	if err != nil {
		return err
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}
	_, err = wt.Commit("ezhours: merge remote hours", &git.CommitOptions{
		Author:            s.signature(),
		Parents:           []plumbing.Hash{ours.Hash, theirs.Hash},
		AllowEmptyCommits: true,
	})
	return err
}

// mergeTrees resolves every path across the three sides. base may be empty for
// unrelated histories.
func mergeTrees(base, ours, theirs map[string]string) map[string]string {
	out := make(map[string]string, len(ours)+len(theirs))

	paths := map[string]bool{}
	for p := range ours {
		paths[p] = true
	}
	for p := range theirs {
		paths[p] = true
	}

	for path := range paths {
		b, inBase := base[path]
		o, inOurs := ours[path]
		t, inTheirs := theirs[path]

		switch {
		case inOurs && inTheirs:
			switch {
			case o == t:
				out[path] = o
			case inBase && b == o:
				out[path] = t
			case inBase && b == t:
				out[path] = o
			case strings.HasSuffix(path, ".txt"):
				out[path] = mergeFile(b, o, t)
			default:
				// Not an hours file; keep this machine's version rather than
				// inventing a merge for a format we do not understand.
				out[path] = o
			}
		case inOurs && !inBase:
			out[path] = o // added here
		case inTheirs && !inBase:
			out[path] = t // added there
			// A file in base but gone on one side was deleted; leave it out.
		}
	}
	return out
}

func filesOf(c *object.Commit) (map[string]string, error) {
	tree, err := c.Tree()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	err = tree.Files().ForEach(func(f *object.File) error {
		content, err := f.Contents()
		if err != nil {
			return err
		}
		out[f.Name] = content
		return nil
	})
	return out, err
}

// CommitAndPush records the working tree and publishes it. On a rejected push
// it pulls (which merges) and tries once more.
func (s *Syncer) CommitAndPush(message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.commitLocal(message); err != nil {
		return err
	}
	if !s.Enabled() {
		return nil
	}

	err := s.push()
	if err == nil {
		return nil
	}
	if !isRejected(err) {
		return err
	}

	log.Printf("gitsync: push rejected (%v), merging remote and retrying", err)
	if err := s.pull(); err != nil {
		return err
	}
	return s.push()
}

func (s *Syncer) push() error {
	auth, err := s.auth()
	if err != nil {
		return err
	}
	branch, err := s.branch()
	if err != nil {
		return err
	}

	err = s.repo.Push(&git.PushOptions{
		RemoteName: remoteName,
		Auth:       auth,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)),
		},
	})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	return nil
}

// isRejected reports whether the remote refused the push because it has commits
// we do not have yet.
func isRejected(err error) bool {
	if errors.Is(err, git.ErrForceNeeded) {
		return true
	}
	return strings.Contains(err.Error(), "non-fast-forward")
}

// commitLocal commits everything in the worktree; a clean tree is a no-op.
func (s *Syncer) commitLocal(message string) error {
	wt, err := s.repo.Worktree()
	if err != nil {
		return err
	}
	status, err := wt.Status()
	if err != nil {
		return err
	}
	if status.IsClean() {
		return nil
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}
	_, err = wt.Commit(message, &git.CommitOptions{Author: s.signature()})
	return err
}

func (s *Syncer) auth() (transport.AuthMethod, error) {
	url, err := s.remoteURL()
	if err != nil {
		return nil, err
	}
	return authMethod(url)
}

// branch returns the checked out branch name, also when HEAD is still unborn.
func (s *Syncer) branch() (string, error) {
	ref, err := s.repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return "", err
	}
	if ref.Type() != plumbing.SymbolicReference {
		return "", fmt.Errorf("hours repo is in detached HEAD state")
	}
	return ref.Target().Short(), nil
}

// signature falls back to the machine name when git has no user configured, so
// a sync never fails just because ~/.gitconfig is missing.
func (s *Syncer) signature() *object.Signature {
	var name, email string
	if cfg, err := s.repo.ConfigScoped(config.SystemScope); err == nil {
		name, email = cfg.User.Name, cfg.User.Email
	}
	if name == "" {
		name = "ezhours"
		if u, err := user.Current(); err == nil && u.Username != "" {
			name = u.Username
		}
	}
	if email == "" {
		host, err := os.Hostname()
		if err != nil || host == "" {
			host = "localhost"
		}
		email = "ezhours@" + host
	}
	return &object.Signature{Name: name, Email: email, When: time.Now()}
}
