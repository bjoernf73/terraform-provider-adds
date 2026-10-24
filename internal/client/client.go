package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
	"github.com/bjoernf73/terraform-provider-adds/internal/powershell"
	"github.com/bjoernf73/terraform-provider-adds/internal/transport"
)

// transientScriptErrorAttempts covers a known-transient SSPI hiccup: AD cmdlets such as
// Get-ADDomain occasionally fail with "A local error has occurred" under concurrent WinRM
// load. The client mutex should prevent this from this process; retrying is a narrow
// defense against other concurrent callers (for example two CI jobs against one DC).
const transientScriptErrorAttempts = 3

func isTransientScriptError(errText string) bool {
	return strings.Contains(errText, "A local error has occurred")
}

// noOutputAttempts bounds how many times RunPowerShellJSON retries when a remote operation exits
// 0 but returns no JSON on stdout. Every script emits JSON on success, so an empty stdout is a
// lost/truncated response; the scripts are idempotent, so retrying is safe.
const noOutputAttempts = 3

// usesGroupPolicyModule reports whether a composed script loads the GroupPolicy module. That
// module is not native to PowerShell 7; under pwsh it loads through the Windows PowerShell
// Compatibility layer, which warns and returns deserialized objects. Such scripts run under
// the GPO PowerShell path (Windows PowerShell) instead, where the module is native.
func usesGroupPolicyModule(script string) bool {
	return strings.Contains(script, "Import-Module GroupPolicy")
}

type Client struct {
	config config.Config
	runner transport.Runner

	// mu serializes every remote operation. Terraform runs independent resources
	// concurrently (parallelism 10 by default), but concurrent WinRM shells over the
	// same NTLM-authenticated connection are unreliable and fail with errors such as
	// "A local error has occurred". ref/terraform-provider-ad works around the same
	// issue with a mutex in its provider config; we do the same rather than relying
	// on the caller to serialize calls. It also guards host and hostDisabled, which are
	// only ever touched while an operation holds the lock.
	mu sync.Mutex

	// host is a lazily started warm pwsh process that keeps the ActiveDirectory module loaded
	// across operations. hostDisabled latches on when the host proves unusable (a malformed
	// response, e.g. antivirus terminated it), so the rest of the process uses the per-operation
	// path instead of paying failed host attempts on every call.
	host         transport.HostSession
	hostDisabled bool
}

func New(cfg config.Config) (*Client, error) {
	runner, err := transport.NewRunner(cfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		config: cfg,
		runner: runner,
	}, nil
}

func (c *Client) Config() config.Config {
	return c.config
}

func (c *Client) RunPowerShell(ctx context.Context, script string) (transport.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastResult transport.Result
	var lastErr error

	for attempt := 1; attempt <= transientScriptErrorAttempts; attempt++ {
		result, err := c.runPowerShellOnce(ctx, script)
		if err == nil || !isTransientScriptError(err.Error()) {
			return result, err
		}

		lastResult, lastErr = result, err

		if attempt == transientScriptErrorAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}

	return lastResult, lastErr
}

// hostMaxAttempts bounds how many times runViaHost restarts the warm host on a transport fault
// (a dropped connection, a stale shell) before giving up and letting the operation fall back to
// the per-operation path. The scripts are idempotent, so re-running after a restart is safe.
const hostMaxAttempts = 3

func (c *Client) runPowerShellOnce(ctx context.Context, script string) (transport.Result, error) {
	powerShellPath := c.config.PowerShellPath
	if usesGroupPolicyModule(script) && strings.TrimSpace(c.config.GPOPowerShellPath) != "" {
		powerShellPath = c.config.GPOPowerShellPath
	}

	// Default pwsh operations run on the warm host, which keeps the ActiveDirectory module loaded
	// across operations so the ~600 ms import is paid once per process rather than every call. GPO
	// operations run under Windows PowerShell through the per-operation path. A host that cannot
	// start (or has been disabled) falls back there too.
	if powerShellPath == c.config.PowerShellPath {
		if result, handled := c.runViaHost(ctx, script); handled {
			return c.finishResult(powerShellPath, result, nil)
		}
	}

	result, err := c.runViaProcess(ctx, powerShellPath, script)
	return c.finishResult(powerShellPath, result, err)
}

// runViaProcess stages, runs and removes the script in a single remote process. Splitting these
// into three separate shells tripled the per-operation connection and server-process count
// (sshd.exe, winrshost.exe, conhost.exe) and the latency; one shell per operation keeps it bounded.
// The body is a plain .ps1 run from disk - not an in-memory decompress-and-Invoke-Expression, which
// behavioural antivirus blocks as a stager (Defender Behavior:Win32/PShellCobStager).
func (c *Client) runViaProcess(ctx context.Context, powerShellPath, script string) (transport.Result, error) {
	token, err := powershell.NewScriptToken()
	if err != nil {
		return transport.Result{}, err
	}
	remotePath := powershell.RemoteScriptPath(token)

	runCommand, err := powershell.BuildRunCommand(powerShellPath, remotePath)
	if err != nil {
		return transport.Result{}, err
	}

	return c.runner.Run(ctx, runCommand, script)
}

// runViaHost runs the script on the warm pwsh host. handled is false when the host could not be
// used (the transport does not support it, the host would not start, or it has been disabled), so
// the caller falls back to the per-operation path; when true, result is authoritative (it may
// carry a non-zero exit code from a genuine operation failure). Transport faults restart the host
// and retry; a malformed response disables the host for the rest of the process.
func (c *Client) runViaHost(ctx context.Context, script string) (transport.Result, bool) {
	if c.hostDisabled {
		return transport.Result{}, false
	}

	hostRunner, ok := c.runner.(transport.HostRunner)
	if !ok {
		return transport.Result{}, false
	}

	for attempt := 1; attempt <= hostMaxAttempts; attempt++ {
		host, err := c.ensureHost(ctx, hostRunner)
		if err != nil {
			// The host would not start; use the per-operation path for this call and try again
			// on the next one.
			return transport.Result{}, false
		}

		payload, err := host.Exchange(ctx, []byte(script))
		if err != nil {
			c.dropHost()
			if ctx.Err() != nil || attempt == hostMaxAttempts {
				return transport.Result{}, false
			}
			select {
			case <-ctx.Done():
				return transport.Result{}, false
			case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
			}
			continue
		}

		stdout, stderr, exitCode, decodeErr := powershell.DecodeHostResponse(payload)
		if decodeErr != nil {
			// The host returned output we cannot parse: it is incompatible or was terminated
			// mid-stream (for example by antivirus). Disable it for this process and fall back to
			// the proven per-operation path.
			c.dropHost()
			c.hostDisabled = true
			return transport.Result{}, false
		}

		return transport.Result{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}, true
	}

	return transport.Result{}, false
}

// ensureHost returns the warm host, starting it on first use. It is only called while mu is held.
func (c *Client) ensureHost(ctx context.Context, hostRunner transport.HostRunner) (transport.HostSession, error) {
	if c.host != nil {
		return c.host, nil
	}

	command, err := powershell.BuildHostCommand(c.config.PowerShellPath)
	if err != nil {
		return nil, err
	}

	host, err := hostRunner.StartHost(ctx, command)
	if err != nil {
		return nil, err
	}

	c.host = host
	return host, nil
}

// dropHost closes and clears the warm host so the next operation starts a fresh one. It is only
// called while mu is held.
func (c *Client) dropHost() {
	if c.host != nil {
		_ = c.host.Close()
		c.host = nil
	}
}

// finishResult applies the shared post-processing for both execution paths: it decodes any CLIXML
// error text, wraps a transport error with the selected executable, and turns a non-zero exit code
// into an error.
func (c *Client) finishResult(powerShellPath string, result transport.Result, err error) (transport.Result, error) {
	result.Stderr = powershell.DecodeCLIXML(result.Stderr)

	if err != nil {
		if detail := strings.TrimSpace(result.Stderr); detail != "" {
			return result, fmt.Errorf("running remote PowerShell (%s): %w; stderr: %s", powerShellPath, err, detail)
		}
		return result, fmt.Errorf("running remote PowerShell (%s): %w", powerShellPath, err)
	}

	if result.ExitCode != 0 {
		return result, fmt.Errorf("remote PowerShell exited with code %d: %s", result.ExitCode, resultDetail(result))
	}

	return result, nil
}

// resultDetail returns the most useful human-readable text from a remote result, preferring
// stderr (already CLIXML-decoded) and falling back to stdout.
func resultDetail(result transport.Result) string {
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(result.Stdout)
	}
	return detail
}

func (c *Client) RunPowerShellJSON(ctx context.Context, script string, target any) error {
	var lastErr error

	for attempt := 1; attempt <= noOutputAttempts; attempt++ {
		result, err := c.RunPowerShell(ctx, script)
		if err != nil {
			return err
		}

		if strings.TrimSpace(result.Stdout) != "" {
			if err := json.Unmarshal([]byte(result.Stdout), target); err != nil {
				return fmt.Errorf("decoding remote JSON output: %w; output: %s", err, result.Stdout)
			}
			return nil
		}

		// Every operation script emits a JSON document on success, so an empty stdout at exit 0
		// means the output never came back - a truncated or lost WinRM response. Because the
		// scripts are idempotent, retrying a few times recovers it; only after exhausting the
		// attempts do we surface the exit code and stderr (already CLIXML-decoded) so the real
		// cause - a module warning, a permissions error, or a recycled WinRM shell - is visible.
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = "nothing on stdout or stderr"
		}
		lastErr = fmt.Errorf("remote PowerShell returned no JSON output (exit code %d): %s", result.ExitCode, detail)

		if attempt == noOutputAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}

	return lastErr
}
