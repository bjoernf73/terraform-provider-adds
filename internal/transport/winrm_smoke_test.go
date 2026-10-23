package transport

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

// Manual connectivity check. Set ADDS_HOST, ADDS_USERNAME and ADDS_PASSWORD to run it:
//
//	ADDS_HOST=10.0.13.6 ADDS_USERNAME='UTV\Administrator' ADDS_PASSWORD=... \
//	  go test ./internal/transport -run TestWinRMSmoke -v
func TestWinRMSmoke(t *testing.T) {
	host := os.Getenv("ADDS_HOST")
	if host == "" {
		t.Skip("ADDS_HOST not set")
	}

	port := 5985
	if raw := os.Getenv("ADDS_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("invalid ADDS_PORT: %v", err)
		}
		port = parsed
	}

	auth := os.Getenv("ADDS_WINRM_AUTH")
	if auth == "" {
		auth = "ntlm"
	}

	cfg := config.Config{
		Host:               host,
		Port:               port,
		Transport:          "winrm",
		Username:           os.Getenv("ADDS_USERNAME"),
		Password:           os.Getenv("ADDS_PASSWORD"),
		Insecure:           true,
		PowerShellPath:     "pwsh",
		Timeout:            30 * time.Second,
		WinRMUseTLS:        os.Getenv("ADDS_WINRM_USE_TLS") == "true",
		WinRMAuth:          auth,
		WinRMKerberosRealm: os.Getenv("ADDS_WINRM_KERBEROS_REALM"),
	}

	runner, err := NewWinRMRunner(cfg)
	if err != nil {
		t.Fatalf("creating runner: %v", err)
	}

	result, err := runner.Run(context.Background(), "whoami", "")
	t.Logf("exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	if err != nil {
		t.Fatalf("running command: %v", err)
	}
}
