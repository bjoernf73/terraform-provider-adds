package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

type sshRunner struct {
	config config.Config
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
	sshConfig, err := r.buildClientConfig()
	if err != nil {
		return Result{}, err
	}

	// Bound the whole operation. Terraform may pass a context without a deadline, and
	// session.Run blocks with no timeout of its own, so a hung remote command would otherwise
	// stall the apply indefinitely. sshConfig.Timeout only covers the dial, not execution.
	timeout := r.config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	address := net.JoinHostPort(r.config.Host, fmt.Sprintf("%d", r.config.Port))
	conn, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return Result{}, fmt.Errorf("dialing SSH target: %w", err)
	}
	defer func() { _ = conn.Close() }()

	session, err := conn.NewSession()
	if err != nil {
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
		// Tear down the connection to unblock session.Run, wait for it to return so the
		// output buffers are no longer being written, then surface the timeout.
		_ = conn.Close()
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

	return result, runErr
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
