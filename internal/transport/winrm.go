package transport

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/masterzen/winrm"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

// emptyStdinExitCode and emptyStdinMarker mirror the sentinel the PowerShell bootstrap emits
// when it receives no script on stdin (see internal/powershell command.go, EmptyStdinExitCode
// /EmptyStdinMarker). They are duplicated here rather than imported to avoid a
// transport -> powershell dependency, which would form a test import cycle.
const (
	emptyStdinExitCode = 97
	emptyStdinMarker   = "ADDS_EMPTY_STDIN"
)

type winrmRunner struct {
	newClient func() (*winrm.Client, error)
	endpoint  string
	auth      string
	timeout   time.Duration

	// mu guards client and shell. The Client serializes operations, so the shell runs one
	// command at a time; the lock only protects the create/drop handoff on a reconnect.
	mu     sync.Mutex
	client *winrm.Client
	shell  *winrm.Shell
}

// winrmMaxAttempts bounds how many times Run re-issues a request on a transient fault. It
// covers 401s (NTLM authenticates a connection and the server closes connections between
// operations, so a stale pooled connection is rejected), network timeouts/resets, and an
// empty stdin payload (a truncated WinRM Send leaves the remote bootstrap with no script).
// All of these mean the script either never ran or ran empty, so retrying is safe.
const winrmMaxAttempts = 3

func NewWinRMRunner(cfg config.Config) (Runner, error) {
	endpoint := winrm.NewEndpoint(
		cfg.Host,
		cfg.Port,
		cfg.WinRMUseTLS,
		cfg.Insecure,
		nil,
		nil,
		nil,
		cfg.Timeout,
	)

	// DefaultParameters is a package-level pointer; copy it before mutating.
	params := *winrm.DefaultParameters
	switch strings.ToLower(cfg.WinRMAuth) {
	case "", "basic":
	case "ntlm":
		params.TransportDecorator = func() winrm.Transporter {
			return &winrm.ClientNTLM{}
		}
	case "kerberos":
		proto := "http"
		if cfg.WinRMUseTLS {
			proto = "https"
		}

		spn := cfg.WinRMKerberosSPN
		if spn == "" {
			spn = fmt.Sprintf("HTTP/%s", cfg.Host)
		}

		// gokrb5 needs a krb5.conf file; Windows has no default. When the user did
		// not supply one, synthesise a minimal config from the realm and target host
		// so Kerberos works out of the box on a domain-joined machine.
		krbConf := cfg.WinRMKerberosConfig
		if krbConf == "" {
			generated, err := writeKerberosConfig(cfg.WinRMKerberosRealm, cfg.Host)
			if err != nil {
				return nil, err
			}
			krbConf = generated
		}

		// The Kerberos principal is a bare account name; strip any NetBIOS/domain
		// prefix (e.g. "UTV\Administrator") since the realm is supplied separately.
		krbUser := cfg.Username
		if idx := strings.IndexAny(krbUser, `\/`); idx >= 0 {
			krbUser = krbUser[idx+1:]
		}
		if idx := strings.Index(krbUser, "@"); idx >= 0 {
			krbUser = krbUser[:idx]
		}

		params.TransportDecorator = func() winrm.Transporter {
			return &winrm.ClientKerberos{
				Username:  krbUser,
				Password:  cfg.Password,
				Realm:     strings.ToUpper(cfg.WinRMKerberosRealm),
				Hostname:  cfg.Host,
				Port:      cfg.Port,
				Proto:     proto,
				SPN:       spn,
				KrbConf:   krbConf,
				KrbCCache: cfg.WinRMKerberosCCache,
			}
		}
	default:
		return nil, fmt.Errorf("unsupported WinRM auth %q", cfg.WinRMAuth)
	}

	newClient := func() (*winrm.Client, error) {
		client, err := winrm.NewClientWithParameters(endpoint, cfg.Username, cfg.Password, &params)
		if err != nil {
			return nil, fmt.Errorf("creating WinRM client: %w", err)
		}

		return client, nil
	}

	if _, err := newClient(); err != nil {
		return nil, err
	}

	scheme := "http"
	if cfg.WinRMUseTLS {
		scheme = "https"
	}

	auth := strings.ToLower(cfg.WinRMAuth)
	if auth == "" {
		auth = "basic"
	}

	return &winrmRunner{
		newClient: newClient,
		endpoint:  fmt.Sprintf("%s://%s:%d/wsman", scheme, cfg.Host, cfg.Port),
		auth:      auth,
		timeout:   cfg.Timeout,
	}, nil
}

// writeKerberosConfig synthesises a minimal krb5.conf for the given realm, using the
// target host as the KDC, and writes it to a stable per-realm temp file. gokrb5 requires
// a config file path and Windows has no default location for one.
func writeKerberosConfig(realm, host string) (string, error) {
	realm = strings.TrimSpace(realm)
	if realm == "" {
		return "", fmt.Errorf("winrm kerberos: realm is required to generate krb5.conf")
	}
	realm = strings.ToUpper(realm)

	content := fmt.Sprintf(`[libdefaults]
    default_realm = %[1]s
    dns_lookup_kdc = true
    dns_lookup_realm = false
    udp_preference_limit = 1

[realms]
    %[1]s = {
        kdc = %[2]s
        admin_server = %[2]s
    }
`, realm, host)

	path := filepath.Join(os.TempDir(), fmt.Sprintf("adds-krb5-%s.conf", strings.ToLower(realm)))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("winrm kerberos: writing krb5.conf: %w", err)
	}

	return path, nil
}

func (r *winrmRunner) Run(ctx context.Context, command string, stdin string) (Result, error) {
	var lastResult Result
	var lastErr error

	for attempt := 1; attempt <= winrmMaxAttempts; attempt++ {
		result, err := r.run(ctx, command, stdin)

		// Success is a clean run whose stdin actually arrived; a non-zero exit from the real
		// script also comes back here with err == nil and is returned as-is.
		if err == nil && !isEmptyStdinResult(result) {
			return result, nil
		}

		// Only transient faults are worth retrying: a stale-connection 401, a network
		// timeout/reset, or an empty stdin payload. Anything else is a real failure.
		if !isAuthFailure(err) && !isTransientError(err) && !isEmptyStdinResult(result) {
			return result, err
		}

		lastResult, lastErr = result, err

		if attempt == winrmMaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}

	if lastErr == nil && isEmptyStdinResult(lastResult) {
		return lastResult, fmt.Errorf("winrm request to %s returned an empty script payload (stdin was not delivered) after %d attempts", r.endpoint, winrmMaxAttempts)
	}
	if isAuthFailure(lastErr) {
		return lastResult, fmt.Errorf("winrm authentication failed against %s using %s auth after %d attempts: %w", r.endpoint, r.auth, winrmMaxAttempts, lastErr)
	}
	return lastResult, fmt.Errorf("winrm request to %s failed after %d attempts: %w", r.endpoint, winrmMaxAttempts, lastErr)
}

func (r *winrmRunner) run(ctx context.Context, command string, stdin string) (Result, error) {
	shell, err := r.acquireShell()
	if err != nil {
		return Result{}, err
	}

	exitCode, stdout, stderr, runErr := runOnShell(ctx, shell, command, stdin)
	result := Result{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
	}

	if runErr != nil {
		// The shell or its connection may be unusable (a stale NTLM connection rejected with a
		// 401, or a shell the server reaped on idle timeout); drop both so the retry in Run
		// recreates them. Run classifies runErr and decides whether to retry.
		r.dropShell(shell)
		return result, runErr
	}

	return result, nil
}

// acquireShell returns the shared WinRM shell, creating the client and opening the shell on first
// use. The Client serializes operations, so one server-side shell (one winrshost.exe) carries
// every command instead of the library opening and deleting a shell per command, which collapses
// the per-operation shell-create/delete round trips and process churn.
func (r *winrmRunner) acquireShell() (*winrm.Shell, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.shell != nil {
		return r.shell, nil
	}

	if r.client == nil {
		client, err := r.newClient()
		if err != nil {
			return nil, err
		}
		r.client = client
	}

	shell, err := r.client.CreateShell()
	if err != nil {
		// A failure opening the shell often means a stale connection; force a fresh client next
		// time so the retry does not reuse the same dead pool.
		r.client = nil
		return nil, err
	}

	r.shell = shell
	return shell, nil
}

// dropShell clears the shared shell and its client when a command fails, so the next attempt
// recreates both. The shell is not Closed here: the connection may already be dead, and the
// server reaps an orphaned shell on its idle timeout. Reusing a failed shell would keep failing.
func (r *winrmRunner) dropShell(shell *winrm.Shell) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shell == shell {
		r.shell = nil
		r.client = nil
	}
}

// runOnShell executes one command on an existing shell, pumping stdin in and stdout/stderr out,
// and returns the exit code, captured streams and any command error. It mirrors the library's
// RunWithContextWithInput but reuses the caller's shell instead of creating and deleting one per
// call.
func runOnShell(ctx context.Context, shell *winrm.Shell, command, stdin string) (int, string, string, error) {
	cmd, err := shell.ExecuteWithContext(ctx, command)
	if err != nil {
		return 0, "", "", err
	}

	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		defer func() { _ = cmd.Stdin.Close() }()
		_, _ = io.Copy(cmd.Stdin, strings.NewReader(stdin))
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stdout, cmd.Stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stderr, cmd.Stderr)
	}()

	cmd.Wait()
	wg.Wait()
	_ = cmd.Close()

	return cmd.ExitCode(), stdout.String(), stderr.String(), cmd.Error()
}

// StartHost opens a dedicated shell and starts the warm host command on it, returning a session
// that streams framed requests and responses. The host gets its own shell (separate from the
// per-operation run path) so the two never contend for one shell's command slot. The host command
// is bound to a background context: it outlives any single operation, and per-request timeouts are
// enforced in Exchange by closing the command.
func (r *winrmRunner) StartHost(_ context.Context, command string) (HostSession, error) {
	client, err := r.newClient()
	if err != nil {
		return nil, err
	}

	shell, err := client.CreateShell()
	if err != nil {
		return nil, fmt.Errorf("creating WinRM host shell: %w", err)
	}

	cmd, err := shell.ExecuteWithContext(context.Background(), command)
	if err != nil {
		_ = shell.Close()
		return nil, fmt.Errorf("starting WinRM host: %w", err)
	}

	timeout := r.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	// The host writes framed responses only to stdout; drain stderr so the library's output pump
	// never blocks on an unread stderr pipe.
	go func() { _, _ = io.Copy(io.Discard, cmd.Stderr) }()

	return &winrmHostSession{
		shell:   shell,
		cmd:     cmd,
		stdout:  bufio.NewReader(cmd.Stdout),
		timeout: timeout,
	}, nil
}

type winrmHostSession struct {
	mu      sync.Mutex
	shell   *winrm.Shell
	cmd     *winrm.Command
	stdout  *bufio.Reader
	timeout time.Duration
}

func (s *winrmHostSession) Exchange(ctx context.Context, request []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	type outcome struct {
		payload []byte
		err     error
	}
	done := make(chan outcome, 1)

	go func() {
		if err := writeFrame(s.cmd.Stdin, request); err != nil {
			done <- outcome{nil, err}
			return
		}
		payload, err := readFrame(s.stdout)
		done <- outcome{payload, err}
	}()

	select {
	case <-ctx.Done():
		// Closing the command unblocks the goroutine's read and terminates the host; the client
		// drops and restarts it on the next operation.
		_ = s.cmd.Close()
		return nil, fmt.Errorf("winrm host exchange timed out after %s: %w", s.timeout, ctx.Err())
	case o := <-done:
		return o.payload, o.err
	}
}

func (s *winrmHostSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.cmd.Stdin.Close()
	_ = s.cmd.Close()
	return s.shell.Close()
}

// isAuthFailure reports a 401, which means the (possibly stale pooled) connection was
// rejected before the script ran, so the request can be retried safely.
func isAuthFailure(err error) bool {
	return err != nil && strings.Contains(err.Error(), "401")
}

// isTransientError reports a network-level fault (timeout, reset, dropped connection) where
// the request likely never completed, so retrying is safe for idempotent remote scripts.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	msg := err.Error()
	for _, substr := range []string{
		"i/o timeout",
		"connection reset",
		"connection refused",
		"broken pipe",
		"TLS handshake timeout",
		"EOF",
	} {
		if strings.Contains(msg, substr) {
			return true
		}
	}

	return false
}

// isEmptyStdinResult reports that the remote bootstrap received no script on stdin (a
// truncated or undelivered WinRM Send), identified by the sentinel exit code and marker the
// bootstrap emits. Such a run never executed the real script, so it is safe to retry.
func isEmptyStdinResult(result Result) bool {
	return result.ExitCode == emptyStdinExitCode ||
		strings.Contains(result.Stderr, emptyStdinMarker)
}
