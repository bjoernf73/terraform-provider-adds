package transport

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

// TestSSHRunTimeout verifies that a remote command which never returns is bounded by the
// configured timeout instead of blocking forever. Set ADDS_HOST, ADDS_USERNAME and
// ADDS_PASSWORD to run it.
func TestSSHRunTimeout(t *testing.T) {
	host := os.Getenv("ADDS_HOST")
	if host == "" {
		t.Skip("ADDS_HOST not set")
	}
	if os.Getenv("ADDS_PASSWORD") == "" {
		t.Skip("ADDS_PASSWORD not set; SSH timeout test needs password auth")
	}

	port := 22
	if raw := os.Getenv("ADDS_SSH_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("invalid ADDS_SSH_PORT: %v", err)
		}
		port = parsed
	}

	cfg := config.Config{
		Host:           host,
		Port:           port,
		Transport:      "ssh",
		Username:       os.Getenv("ADDS_USERNAME"),
		Password:       os.Getenv("ADDS_PASSWORD"),
		Insecure:       true, // test only: skip host key verification
		PowerShellPath: "pwsh",
		Timeout:        3 * time.Second,
	}

	runner, err := NewSSHRunner(cfg)
	if err != nil {
		t.Fatalf("creating ssh runner: %v", err)
	}

	start := time.Now()
	// A command that sleeps far longer than the 3s timeout. powershell.exe is always present
	// on Windows and resolvable regardless of the OpenSSH default shell.
	_, err = runner.Run(context.Background(), `powershell -NoProfile -Command "Start-Sleep -Seconds 60"`, "")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout error, got: %v", err)
	}
	if elapsed > 20*time.Second {
		t.Fatalf("expected the command to time out quickly, took %s", elapsed)
	}
	t.Logf("timed out as expected after %s: %v", elapsed, err)
}
