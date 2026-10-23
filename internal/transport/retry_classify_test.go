package transport

import (
	"errors"
	"fmt"
	"testing"
)

// timeoutError is a net.Error whose Timeout() reports true, like a dial i/o timeout.
type timeoutError struct{}

func (timeoutError) Error() string   { return "simulated i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestIsTransientError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"net timeout", timeoutError{}, true},
		{"io timeout string", errors.New(`dial tcp 10.2.5.21:5986: i/o timeout`), true},
		{"connection reset", errors.New("read: connection reset by peer"), true},
		{"connection refused", errors.New("dial tcp: connect: connection refused"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
		{"tls handshake timeout", errors.New("net/http: TLS handshake timeout"), true},
		{"eof", fmt.Errorf("reading response: %w", errors.New("unexpected EOF")), true},
		{"auth 401", errors.New("http error 401"), false},
		{"plain failure", errors.New("the system cannot find the file specified"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientError(tc.err); got != tc.want {
				t.Fatalf("isTransientError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsEmptyStdinResult(t *testing.T) {
	cases := []struct {
		name   string
		result Result
		want   bool
	}{
		{"sentinel exit code", Result{ExitCode: emptyStdinExitCode}, true},
		{"marker on stderr", Result{ExitCode: 1, Stderr: "ADDS_EMPTY_STDIN"}, true},
		{"clean success", Result{ExitCode: 0, Stdout: `{"ok":true}`}, false},
		{"real script failure", Result{ExitCode: 1, Stderr: "Get-ADObject: not found"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEmptyStdinResult(tc.result); got != tc.want {
				t.Fatalf("isEmptyStdinResult(%+v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}

// TestEmptyStdinSentinelMatchesBootstrap guards against the transport mirror drifting from
// the powershell bootstrap's sentinel (they are duplicated to avoid an import cycle).
func TestEmptyStdinSentinelMatchesBootstrap(t *testing.T) {
	if emptyStdinExitCode != 97 {
		t.Fatalf("emptyStdinExitCode = %d, want 97 (must match powershell bootstrap)", emptyStdinExitCode)
	}
	if emptyStdinMarker != "ADDS_EMPTY_STDIN" {
		t.Fatalf("emptyStdinMarker = %q, want ADDS_EMPTY_STDIN (must match powershell bootstrap)", emptyStdinMarker)
	}
}
