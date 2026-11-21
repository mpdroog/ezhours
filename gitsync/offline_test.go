package gitsync

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestIsNetworkError(t *testing.T) {
	dns := &net.DNSError{Err: "connection refused", Name: "github.com", Server: "[::1]:53"}
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("fetch: %w", dns), true},
		{fmt.Errorf("fetch: %w", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("network is unreachable")}), true},
		{errors.New("fetch: dial tcp: lookup github.com on [::1]:53: read udp [::1]:57092->[::1]:53: read: connection refused"), true},
		{ErrSyncDisabled, false},
		{errors.New("ssh: unable to authenticate"), false},
	} {
		if got := IsNetworkError(tc.err); got != tc.want {
			t.Errorf("IsNetworkError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
