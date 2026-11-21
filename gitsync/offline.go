package gitsync

import (
	"errors"
	"net"
	"strings"
)

// IsNetworkError reports whether a sync failed because the remote could not be
// reached at all: no route, no DNS, nothing listening. That is what a sync
// started at login runs into when the network (or the local resolver) is not
// up yet, and unlike an auth error it fixes itself by waiting.
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	var opErr *net.OpError
	if errors.As(err, &dnsErr) || errors.As(err, &opErr) {
		return true
	}
	// go-git does not always keep the net error in the chain.
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"dial tcp",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"temporary failure in name resolution",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
