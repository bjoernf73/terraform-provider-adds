package client

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
	"github.com/bjoernf73/terraform-provider-adds/internal/transport"
)

type executionFailureRunner struct {
	calls  int
	err    error
	stderr string
}

func (runner *executionFailureRunner) Run(ctx context.Context, command, stdin string) (transport.Result, error) {
	runner.calls++
	return transport.Result{Stderr: runner.stderr}, runner.err
}

func TestRunPowerShellExecutionFailure(t *testing.T) {
	for _, stderr := range []string{"", "host process failed"} {
		t.Run("stderr="+stderr, func(t *testing.T) {
			fault := errors.New("WSManFault Code=1359")
			runner := &executionFailureRunner{err: fault, stderr: stderr}
			remote := &Client{
				config: config.Config{PowerShellPath: "pwsh", GPOPowerShellPath: "powershell.exe"},
				runner: runner,
			}
			_, err := remote.RunPowerShell(context.Background(), "Import-Module GroupPolicy")
			if !errors.Is(err, fault) {
				t.Fatalf("expected wrapped transport fault, got %v", err)
			}
			if !strings.Contains(err.Error(), "running remote PowerShell (powershell.exe)") {
				t.Fatalf("expected selected executable in error, got %v", err)
			}
			if stderr != "" && !strings.Contains(err.Error(), "stderr: "+stderr) {
				t.Fatalf("expected captured stderr in error, got %v", err)
			}
			if runner.calls != 1 {
				t.Fatalf("expected a single stage-run-delete call, got %d calls", runner.calls)
			}
		})
	}
}
