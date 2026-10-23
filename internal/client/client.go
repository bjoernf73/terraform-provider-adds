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
	// on the caller to serialize calls.
	mu sync.Mutex
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

func (c *Client) runPowerShellOnce(ctx context.Context, script string) (transport.Result, error) {
	powerShellPath := c.config.PowerShellPath
	if usesGroupPolicyModule(script) && strings.TrimSpace(c.config.GPOPowerShellPath) != "" {
		powerShellPath = c.config.GPOPowerShellPath
	}

	token, err := powershell.NewScriptToken()
	if err != nil {
		return transport.Result{}, err
	}
	remotePath := powershell.RemoteScriptPath(token)

	// Stage the script as a plain .ps1 file and run it with -File, rather than piping a
	// compressed, base64-encoded body into an in-memory Invoke-Expression. That older bootstrap
	// is behaviourally identical to a PowerShell stager and is blocked by behavioural antivirus
	// (Defender flags it as Behavior:Win32/PShellCobStager), which surfaced as empty output.
	writeCommand, err := powershell.BuildWriteCommand(powerShellPath, remotePath)
	if err != nil {
		return transport.Result{}, err
	}

	writeResult, err := c.runner.Run(ctx, writeCommand, script)
	writeResult.Stderr = powershell.DecodeCLIXML(writeResult.Stderr)
	if err != nil {
		return writeResult, fmt.Errorf("uploading remote script: %w", err)
	}
	if writeResult.ExitCode != 0 {
		return writeResult, fmt.Errorf("uploading remote script exited with code %d: %s", writeResult.ExitCode, resultDetail(writeResult))
	}

	// The script is on disk now, so always attempt to remove it afterwards. Cleanup is best
	// effort and must not mask the real result; it reuses the operation context so a cancelled
	// operation skips it rather than blocking.
	defer func() {
		if deleteCommand, derr := powershell.BuildDeleteCommand(powerShellPath, remotePath); derr == nil {
			_, _ = c.runner.Run(ctx, deleteCommand, "")
		}
	}()

	runCommand, err := powershell.BuildFileCommand(powerShellPath, remotePath)
	if err != nil {
		return transport.Result{}, err
	}

	result, err := c.runner.Run(ctx, runCommand, "")
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
