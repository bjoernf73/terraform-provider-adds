package client

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

// gpoVersionResult mirrors the nested Get-GPO shape that the Windows PowerShell Compatibility
// layer deserializes lossily (computer/user versions come back null under pwsh).
type gpoVersionResult struct {
	Edition         string `json:"edition"`
	Name            string `json:"name"`
	ComputerVersion *int   `json:"computer_version"`
	UserVersion     *int   `json:"user_version"`
}

const groupPolicyProbe = `$ErrorActionPreference='Stop'
$WarningPreference='SilentlyContinue'
if ($null -ne $PSStyle) { $PSStyle.OutputRendering = 'PlainText' }
Import-Module GroupPolicy -ErrorAction Stop 3>$null
$g = Get-GPO -All | Select-Object -First 1
[pscustomobject]@{
    edition          = $PSVersionTable.PSEdition
    name             = $g.DisplayName
    computer_version = $g.Computer.DSVersion
    user_version     = $g.User.DSVersion
} | ConvertTo-Json -Compress`

func liveClient(t *testing.T, gpoPath string) *Client {
	t.Helper()

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
		Insecure:           os.Getenv("ADDS_INSECURE") == "true",
		PowerShellPath:     "pwsh",
		GPOPowerShellPath:  gpoPath,
		Timeout:            60 * time.Second,
		WinRMUseTLS:        os.Getenv("ADDS_WINRM_USE_TLS") == "true",
		WinRMAuth:          auth,
		WinRMKerberosRealm: os.Getenv("ADDS_WINRM_KERBEROS_REALM"),
	}

	c, err := New(cfg)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	return c
}

// TestGroupPolicyRoutingLive proves, through the full client stack, that GroupPolicy scripts
// routed to powershell.exe return native (non-deserialized) objects with intact nested
// versions, whereas the pwsh compatibility layer loses them. This is the layer that failed in
// v0.0.24 with "remote PowerShell returned no JSON output".
func TestGroupPolicyRoutingLive(t *testing.T) {
	t.Run("powershell.exe (native)", func(t *testing.T) {
		c := liveClient(t, "powershell.exe")

		var got gpoVersionResult
		if err := c.RunPowerShellJSON(context.Background(), groupPolicyProbe, &got); err != nil {
			t.Fatalf("running GPO probe: %v", err)
		}
		t.Logf("result: %+v", got)

		if got.Edition != "Desktop" {
			t.Errorf("expected routing to Windows PowerShell (Desktop), got edition %q", got.Edition)
		}
		if got.ComputerVersion == nil || got.UserVersion == nil {
			t.Errorf("native powershell.exe should return non-null nested versions, got computer=%v user=%v", got.ComputerVersion, got.UserVersion)
		}
	})

	t.Run("pwsh (compat layer, demonstrates lossy deserialization)", func(t *testing.T) {
		c := liveClient(t, "pwsh")

		var got gpoVersionResult
		if err := c.RunPowerShellJSON(context.Background(), groupPolicyProbe, &got); err != nil {
			t.Fatalf("running GPO probe: %v", err)
		}
		t.Logf("result: %+v", got)

		if got.Edition != "Core" {
			t.Errorf("expected pwsh (Core), got edition %q", got.Edition)
		}
	})
}
