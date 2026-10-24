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
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

type sshRunner struct {
	config config.Config

	// mu guards conn. The Client serializes operations, so conn carries one command at a time;
	// the lock only protects the dial/drop handoff between an operation and a reconnect.
	mu   sync.Mutex
	conn *ssh.Client
}

func NewSSHRunner(cfg config.Config) (Runner, error) {
	if strings.TrimSpace(cfg.Username) == "" {
		return nil, fmt.Errorf("username is required for SSH transport")
	}

	if strings.TrimSpace(cfg.Password) == "" && strings.TrimSpace(cfg.SSHPrivateKeyPEM) == "" {
		return nil, fmt.Errorf("either password or ssh_private_key_pem is required for SSH transport")
	}

	return &sshRunner{config: cfg}, nil
}

// sshMaxAttempts bounds how many times Run will re-establish the connection on a
// transport-level failure. Win32-OpenSSH intermittently tears the stdin pipe down mid-command
// ("The pipe has been ended"), which surfaces here as a dial error or an abnormal termination
// rather than a clean exit status; a fresh connection almost always succeeds.
const sshMaxAttempts = 3

func (r *sshRunner) Run(ctx context.Context, command string, stdin string) (Result, error) {
	var result Result
	var err error

	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= sshMaxAttempts; attempt++ {
		result, err = r.runOnce(ctx, command, stdin)
		if err == nil {
			// A clean exit, including a non-zero one, comes back as (result, nil) with
			// ExitCode set: that is a real remote result, not a transport fault, so never retry it.
			// The exception is an empty stdin payload (the script body never arrived), which the
			// bootstrap reports with a sentinel exit code; treat that like a transport fault.
			if !isEmptyStdinResult(result) {
				return result, nil
			}
			err = fmt.Errorf("ssh command received an empty script payload (stdin was not delivered)")
		}

		if attempt == sshMaxAttempts {
			break
		}

		tflog.Warn(ctx, "ssh command failed with a transport-level error; retrying", map[string]any{
			"attempt":      attempt,
			"max_attempts": sshMaxAttempts,
			"backoff":      backoff.String(),
			"error":        err.Error(),
		})

		// Space out retries, but abandon them immediately if the caller gave up.
		select {
		case <-ctx.Done():
			return result, err
		case <-time.After(backoff):
		}
		backoff *= 2
	}

	return result, err
}

func (r *sshRunner) runOnce(ctx context.Context, command string, stdin string) (Result, error) {
	// Bound the whole operation. Terraform may pass a context without a deadline, and
	// session.Run blocks with no timeout of its own, so a hung remote command would otherwise
	// stall the apply indefinitely. The dial has its own timeout from the client config.
	timeout := r.config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := r.connection()
	if err != nil {
		return Result{}, err
	}

	session, err := conn.NewSession()
	if err != nil {
		// Opening a channel failed, so the shared connection is almost certainly dead; drop it
		// so the retry in Run re-dials a fresh one.
		r.dropConnection(conn)
		return Result{}, fmt.Errorf("creating SSH session: %w", err)
	}
	defer func() { _ = session.Close() }()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	session.Stdout = &stdout
	session.Stderr = &stderr
	session.Stdin = strings.NewReader(stdin)

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()

	var runErr error
	select {
	case runErr = <-done:
	case <-ctx.Done():
		// Closing the shared connection unblocks session.Run; wait for it to return so the
		// output buffers are no longer being written, then drop the connection so the next
		// operation re-dials rather than reusing a half-torn-down one.
		r.dropConnection(conn)
		<-done
		return Result{Stdout: stdout.String(), Stderr: stderr.String()},
			fmt.Errorf("ssh command timed out after %s: %w", timeout, ctx.Err())
	}

	result := Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if runErr == nil {
		return result, nil
	}

	var exitErr *ssh.ExitError
	if errors.As(runErr, &exitErr) {
		result.ExitCode = exitErr.ExitStatus()
		return result, nil
	}

	// A non-exit error is a transport fault (Win32-OpenSSH tearing the stdin pipe down, a
	// dropped connection); drop the connection so the retry re-dials.
	r.dropConnection(conn)
	return result, runErr
}

// connection returns the shared SSH client, dialing one on first use. The Client serializes
// operations, so a single connection carries every command: each operation opens a fresh session
// (an SSH channel) on it instead of re-dialing and re-authenticating, which collapses the
// per-operation sshd.exe and TCP/auth-handshake cost that otherwise dominated read latency.
func (r *sshRunner) connection() (*ssh.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conn != nil {
		return r.conn, nil
	}

	sshConfig, err := r.buildClientConfig()
	if err != nil {
		return nil, err
	}

	address := net.JoinHostPort(r.config.Host, fmt.Sprintf("%d", r.config.Port))
	conn, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("dialing SSH target: %w", err)
	}

	r.conn = conn
	return conn, nil
}

// dropConnection closes conn and clears it from the runner if it is still the current one, so the
// next operation dials fresh. It is called when a session fails at the transport level, where
// reusing the connection would keep failing.
func (r *sshRunner) dropConnection(conn *ssh.Client) {
	r.mu.Lock()
	if r.conn == conn {
		r.conn = nil
	}
	r.mu.Unlock()
	_ = conn.Close()
}

// StartHost opens a long-running session on the shared connection and starts the warm host
// command on it, returning a session that streams framed requests and responses. The session is
// one SSH channel on the already-dialed connection, so the host adds no extra sshd handshake.
func (r *sshRunner) StartHost(_ context.Context, command string) (HostSession, error) {
	conn, err := r.connection()
	if err != nil {
		return nil, err
	}

	session, err := conn.NewSession()
	if err != nil {
		r.dropConnection(conn)
		return nil, fmt.Errorf("creating SSH host session: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("opening SSH host stdin: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("opening SSH host stdout: %w", err)
	}

	// The host writes framed responses only to stdout; drain stderr so a diagnostic line cannot
	// fill the channel window and wedge the remote process.
	session.Stderr = io.Discard

	if err := session.Start(command); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("starting SSH host: %w", err)
	}

	timeout := r.config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &sshHostSession{
		session: session,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		timeout: timeout,
	}, nil
}

type sshHostSession struct {
	mu      sync.Mutex
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	timeout time.Duration
}

func (s *sshHostSession) Exchange(ctx context.Context, request []byte) ([]byte, error) {
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
		if err := writeFrame(s.stdin, request); err != nil {
			done <- outcome{nil, err}
			return
		}
		payload, err := readFrame(s.stdout)
		done <- outcome{payload, err}
	}()

	select {
	case <-ctx.Done():
		// Closing the session unblocks the goroutine's read and kills the host; the client drops
		// and restarts it on the next operation.
		_ = s.session.Close()
		return nil, fmt.Errorf("ssh host exchange timed out after %s: %w", s.timeout, ctx.Err())
	case o := <-done:
		return o.payload, o.err
	}
}

func (s *sshHostSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.stdin.Close()
	return s.session.Close()
}

func (r *sshRunner) buildClientConfig() (*ssh.ClientConfig, error) {
	authMethods := make([]ssh.AuthMethod, 0, 2)

	if r.config.Password != "" {
		authMethods = append(authMethods, ssh.Password(r.config.Password))
	}

	if r.config.SSHPrivateKeyPEM != "" {
		signer, err := ssh.ParsePrivateKey([]byte(r.config.SSHPrivateKeyPEM))
		if err != nil {
			return nil, fmt.Errorf("parsing ssh_private_key_pem: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	hostKeyCallback, err := r.hostKeyCallback()
	if err != nil {
		return nil, err
	}

	timeout := r.config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &ssh.ClientConfig{
		User:            r.config.Username,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}, nil
}

func (r *sshRunner) hostKeyCallback() (ssh.HostKeyCallback, error) {
	if r.config.Insecure {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	if r.config.SSHKnownHostsPath != "" {
		callback, err := knownhosts.New(r.config.SSHKnownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("loading known hosts: %w", err)
		}
		return callback, nil
	}

	if r.config.SSHHostKey != "" {
		publicKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(r.config.SSHHostKey))
		if err != nil {
			return nil, fmt.Errorf("parsing ssh_host_key: %w", err)
		}

		return func(_ string, _ net.Addr, presented ssh.PublicKey) error {
			if bytes.Equal(publicKey.Marshal(), presented.Marshal()) {
				return nil
			}
			return fmt.Errorf("ssh host key mismatch")
		}, nil
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		defaultKnownHosts := homeDir + "/.ssh/known_hosts"
		if _, statErr := os.Stat(defaultKnownHosts); statErr == nil {
			callback, callbackErr := knownhosts.New(defaultKnownHosts)
			if callbackErr == nil {
				return callback, nil
			}
		}
	}

	return nil, fmt.Errorf("ssh host key verification is required unless insecure is true")
}
