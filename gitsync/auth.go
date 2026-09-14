package gitsync

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/skeema/knownhosts"
	cryptossh "golang.org/x/crypto/ssh"
	xknownhosts "golang.org/x/crypto/ssh/knownhosts"
)

// KeyFile is the private key used to talk to the remote. A dedicated key keeps
// the hours sync independent of whatever else is loaded in the ssh-agent.
var KeyFile = filepath.Join(os.Getenv("HOME"), ".ezhours.priv")

// PassphraseEnv names the env var holding the key's passphrase, if it has one.
const PassphraseEnv = "EZHOURS_KEY_PASSPHRASE"

var errNoKey = errors.New("ssh key not found")

// authMethod builds the SSH auth from KeyFile. https:// remotes need no auth
// method here; go-git falls back to the credential-less transport.
func authMethod(remoteURL string) (transport.AuthMethod, error) {
	if !isSSHURL(remoteURL) {
		return nil, nil
	}
	if _, err := os.Stat(KeyFile); err != nil {
		return nil, fmt.Errorf("%w: %s", errNoKey, KeyFile)
	}

	user := "git"
	if i := strings.Index(remoteURL, "@"); i > 0 && !strings.Contains(remoteURL, "://") {
		user = remoteURL[:i]
	}

	auth, err := ssh.NewPublicKeysFromFile(user, KeyFile, os.Getenv(PassphraseEnv))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", KeyFile, err)
	}
	auth.HostKeyCallback = hostKeyCallback()
	return auth, nil
}

func isSSHURL(u string) bool {
	if strings.HasPrefix(u, "ssh://") {
		return true
	}
	if strings.Contains(u, "://") {
		return false
	}
	// scp-like syntax: git@github.com:user/repo.git
	return strings.Contains(u, ":") && strings.Contains(u, "@")
}

var knownHostsMu sync.Mutex

// hostKeyCallback verifies the server against ~/.ssh/known_hosts, adding the
// host on first contact the way `ssh -o StrictHostKeyChecking=accept-new` does.
// A host that is already known but presents a different key is still refused.
func hostKeyCallback() cryptossh.HostKeyCallback {
	path := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")

	// The result is named so the deferred close below can report a write that
	// failed on the way out.
	return func(hostname string, remote net.Addr, key cryptossh.PublicKey) (err error) {
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()

		// Ensure the file exists so knownhosts.New does not fail on a fresh box.
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		// Create it if it is not there; knownhosts.New fails on a missing file.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}

		db, err := knownhosts.New(path)
		if err != nil {
			return err
		}

		err = db(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *xknownhosts.KeyError
		if !errors.As(err, &keyErr) || len(keyErr.Want) > 0 {
			// len(Want) > 0 means we know this host under a different key:
			// that is a real mismatch, never auto-accept it.
			return err
		}

		// Unknown host: record it and continue.
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		defer func() {
			// This handle was written to, so a failed close is the write
			// failing late: the host would look accepted and not be recorded.
			if cerr := out.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()
		line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
		_, err = fmt.Fprintln(out, line)
		return err
	}
}
