package gitsync

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	cryptossh "golang.org/x/crypto/ssh"
)

// KeySetup is what the user still has to do by hand after SetupAccess: allow
// PublicKey on the remote. KeysURL is the page to do that on, when the host is
// one we know (GitHub); it is empty otherwise.
type KeySetup struct {
	Remote    string
	PublicKey string
	KeysURL   string
	// Changed reports whether SetupAccess created the key or rewrote the
	// remote, i.e. whether a retry has any chance of going differently.
	Changed bool
}

// IsAuthError reports whether a sync failed because the remote did not accept
// us, as opposed to being offline or a broken repo. GitHub answers a private
// repo it will not show us with "repository not found", so that counts too.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errNoKey) ||
		errors.Is(err, transport.ErrAuthenticationRequired) ||
		errors.Is(err, transport.ErrAuthorizationFailed) ||
		errors.Is(err, transport.ErrRepositoryNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"unable to authenticate", // ssh: the key is not known to the host
		"repository not found",   // the key is known but not for this repo
		"marked as read only",    // a deploy key without write access
		"permission denied",      // other hosts' wording for the same
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// SetupAccess does the part of the README's sync setup that needs no human:
// it creates KeyFile when there is none, and points an https origin at the
// same repository over SSH, since ezhours only ever authenticates with that
// key. What is left -- allowing the key on the remote -- comes back in KeySetup.
func (s *Syncer) SetupAccess() (*KeySetup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.open(); err != nil {
		return nil, err
	}
	remote, err := s.remoteURL()
	if err != nil {
		return nil, ErrSyncDisabled
	}

	setup := &KeySetup{Remote: remote}
	if !isSSHURL(remote) {
		sshURL, ok := httpsToSSH(remote)
		if !ok {
			return nil, fmt.Errorf("cannot use %s over SSH", remote)
		}
		if err := s.setRemoteURL(sshURL); err != nil {
			return nil, err
		}
		setup.Remote = sshURL
		setup.Changed = true
	}

	pub, created, err := ensureKey(KeyFile)
	if err != nil {
		return nil, err
	}
	setup.PublicKey = pub
	setup.Changed = setup.Changed || created
	setup.KeysURL = deployKeysURL(setup.Remote)
	return setup, nil
}

func (s *Syncer) setRemoteURL(u string) error {
	cfg, err := s.repo.Config()
	if err != nil {
		return err
	}
	rc, ok := cfg.Remotes[remoteName]
	if !ok {
		rc = &config.RemoteConfig{Name: remoteName}
		cfg.Remotes[remoteName] = rc
	}
	rc.URLs = []string{u}
	return s.repo.SetConfig(cfg)
}

// httpsToSSH turns https://host/owner/repo into git@host:owner/repo.git.
func httpsToSSH(remote string) (string, bool) {
	u, err := url.Parse(remote)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", false
	}
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return "", false
	}
	if !strings.HasSuffix(path, ".git") {
		path += ".git"
	}
	return "git@" + u.Hostname() + ":" + path, true
}

// deployKeysURL is the page that adds a deploy key to a GitHub repository.
func deployKeysURL(remote string) string {
	host, path, ok := strings.Cut(strings.TrimPrefix(remote, "ssh://"), ":")
	if !ok {
		return ""
	}
	if i := strings.Index(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	if host != "github.com" {
		return ""
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	return "https://github.com/" + path + "/settings/keys/new"
}

// ensureKey returns the public half of keyFile in authorized_keys form,
// generating an ed25519 pair first if keyFile does not exist.
func ensureKey(keyFile string) (pub string, created bool, err error) {
	raw, err := os.ReadFile(keyFile)
	if err == nil {
		signer, err := parseKey(raw)
		if err != nil {
			return "", false, fmt.Errorf("read %s: %w", keyFile, err)
		}
		return authorizedKey(signer.PublicKey()), false, nil
	}
	if !os.IsNotExist(err) {
		return "", false, err
	}

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", false, err
	}
	block, err := cryptossh.MarshalPrivateKey(privKey, "ezhours")
	if err != nil {
		return "", false, err
	}
	sshPub, err := cryptossh.NewPublicKey(pubKey)
	if err != nil {
		return "", false, err
	}
	pub = authorizedKey(sshPub)

	// O_EXCL: never overwrite a key that appeared since the read above.
	f, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", false, err
	}
	if _, err := f.Write(pem.EncodeToMemory(block)); err != nil {
		f.Close()
		return "", false, err
	}
	if err := f.Close(); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(keyFile+".pub", []byte(pub+"\n"), 0644); err != nil {
		return "", false, err
	}
	return pub, true, nil
}

func parseKey(raw []byte) (cryptossh.Signer, error) {
	if pass := os.Getenv(PassphraseEnv); pass != "" {
		return cryptossh.ParsePrivateKeyWithPassphrase(raw, []byte(pass))
	}
	return cryptossh.ParsePrivateKey(raw)
}

func authorizedKey(k cryptossh.PublicKey) string {
	return strings.TrimSpace(string(cryptossh.MarshalAuthorizedKey(k))) + " ezhours"
}
