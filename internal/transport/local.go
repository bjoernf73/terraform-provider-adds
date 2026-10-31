package transport

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

// localRunner executes the composed PowerShell command as a child process of the provider itself,
// with no remoting. It is for running Terraform directly on a domain controller or domain member
// under the caller's current Windows logon - no host, username or password is needed. It
// implements HostRunner too, so the warm-host optimization works exactly as it does over SSH/WinRM.
type localRunner struct {
	config config.Config
}

func NewLocalRunner(cfg config.Config) (Runner, error) {
	return &localRunner{config: cfg}, nil
}

func (r *localRunner) timeout() time.Duration {
	timeout := r.config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return timeout
}

// splitLocalCommand splits a command line of the shape internal/powershell produces
// (`"<exe>" <arg> <arg> ...`) into the executable and its arguments. The provider builds that line
// itself and its only variable-length argument is a base64 -EncodedCommand with no spaces, so
// splitting the remainder on whitespace is sufficient and avoids invoking a shell.
func splitLocalCommand(command string) (string, []string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, fmt.Errorf("empty command")
	}

	if strings.HasPrefix(command, `"`) {
		end := strings.IndexByte(command[1:], '"')
		if end < 0 {
			return "", nil, fmt.Errorf("unterminated quote in command %q", command)
		}
		return command[1 : 1+end], strings.Fields(command[2+end:]), nil
	}

	fields := strings.Fields(command)
	return fields[0], fields[1:], nil
}

func (r *localRunner) Run(ctx context.Context, command string, stdin string) (Result, error) {
	exe, args, err := splitLocalCommand(command)
	if err != nil {
		return Result{}, err
	}

	// Bound the whole operation. Terraform may pass a context without a deadline, and a hung
	// PowerShell command would otherwise stall the apply indefinitely.
	ctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}

	if runErr == nil {
		return result, nil
	}

	if ctx.Err() != nil {
		return result, fmt.Errorf("local command timed out after %s: %w", r.timeout(), ctx.Err())
	}

	// A clean non-zero exit comes back as (result, nil) with ExitCode set, exactly like the SSH
	// and WinRM runners, so the client can turn it into a PowerShell error with the stderr text.
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}

	return result, fmt.Errorf("running local command: %w", runErr)
}

// StartHost launches the warm host as a child process and returns a session that streams framed
// requests and responses over its stdin/stdout, matching the SSH and WinRM host sessions.
func (r *localRunner) StartHost(_ context.Context, command string) (HostSession, error) {
	exe, args, err := splitLocalCommand(command)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(exe, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening local host stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opening local host stdout: %w", err)
	}
	// The host writes framed responses only to stdout; discard stderr so a stray line cannot block.
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting local host: %w", err)
	}

	return &localHostSession{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		timeout: r.timeout(),
	}, nil
}

type localHostSession struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	timeout time.Duration
}

func (s *localHostSession) Exchange(ctx context.Context, request []byte) ([]byte, error) {
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
		// Killing the process unblocks the goroutine's read; the client drops and restarts the
		// host on the next operation.
		s.kill()
		return nil, fmt.Errorf("local host exchange timed out after %s: %w", s.timeout, ctx.Err())
	case o := <-done:
		return o.payload, o.err
	}
}

func (s *localHostSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.stdin.Close()
	s.kill()
	return nil
}

func (s *localHostSession) kill() {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.cmd.Wait()
}
