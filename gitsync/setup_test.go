package gitsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

func TestIsAuthError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("fetch: %w", transport.ErrAuthenticationRequired), true},
		{fmt.Errorf("fetch: %w", transport.ErrRepositoryNotFound), true},
		{fmt.Errorf("%w: /home/x/.ezhours.priv", errNoKey), true},
		{errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey]"), true},
		{errors.New("push: ERROR: The key you are authenticating with has been marked as read only."), true},
		{errors.New("dial tcp: lookup github.com: no such host"), false},
		{errors.New("object not found"), false},
	} {
		if got := IsAuthError(tc.err); got != tc.want {
			t.Errorf("IsAuthError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestHTTPSToSSH(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/mpdroog/hour.d":       "git@github.com:mpdroog/hour.d.git",
		"https://github.com/mpdroog/hours.git/":   "git@github.com:mpdroog/hours.git",
		"https://user@gitlab.example.com/a/b.git": "git@gitlab.example.com:a/b.git",
	} {
		if got, ok := httpsToSSH(in); !ok || got != want {
			t.Errorf("httpsToSSH(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"/srv/git/hours.git", "file:///srv/hours.git", "https://github.com/"} {
		if got, ok := httpsToSSH(in); ok {
			t.Errorf("httpsToSSH(%q) = %q, want no conversion", in, got)
		}
	}
}

func TestDeployKeysURL(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:mpdroog/hour.d.git":      "https://github.com/mpdroog/hour.d/settings/keys/new",
		"ssh://git@github.com/mpdroog/hours.git": "",
		"git@gitlab.com:a/b.git":                 "",
	} {
		if got := deployKeysURL(in); got != want {
			t.Errorf("deployKeysURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetupAccess(t *testing.T) {
	old := KeyFile
	KeyFile = filepath.Join(t.TempDir(), "ezhours.priv")
	t.Cleanup(func() { KeyFile = old })

	dir := t.TempDir()
	t.Setenv(RemoteEnv, "https://github.com/mpdroog/hour.d")
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	setup, err := s.SetupAccess()
	if err != nil {
		t.Fatalf("SetupAccess: %v", err)
	}
	if !setup.Changed {
		t.Error("first setup reported no change")
	}
	if setup.Remote != "git@github.com:mpdroog/hour.d.git" {
		t.Errorf("remote = %q", setup.Remote)
	}
	if got, _ := s.remoteURL(); got != setup.Remote {
		t.Errorf("repo remote = %q, want %q", got, setup.Remote)
	}
	if setup.KeysURL != "https://github.com/mpdroog/hour.d/settings/keys/new" {
		t.Errorf("keys url = %q", setup.KeysURL)
	}
	if !strings.HasPrefix(setup.PublicKey, "ssh-ed25519 ") {
		t.Errorf("public key = %q", setup.PublicKey)
	}
	if fi, err := os.Stat(KeyFile); err != nil || fi.Mode().Perm() != 0600 {
		t.Errorf("key file: %v, %v", fi, err)
	}
	// The key it wrote must be one ezhours can log in with.
	if _, err := authMethod(setup.Remote); err != nil {
		t.Errorf("authMethod with generated key: %v", err)
	}

	again, err := s.SetupAccess()
	if err != nil {
		t.Fatalf("second SetupAccess: %v", err)
	}
	if again.Changed || again.PublicKey != setup.PublicKey {
		t.Errorf("second setup changed things: %+v", again)
	}
}
