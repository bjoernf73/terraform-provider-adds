package ad

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

func liveADClient(t *testing.T) *client.Client {
	t.Helper()

	host := os.Getenv("ADDS_HOST")
	if host == "" {
		t.Skip("ADDS_HOST not set")
	}

	port := 5985
	if raw := os.Getenv("ADDS_PORT"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("invalid ADDS_PORT: %v", err)
		}
		port = p
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
		GPOPowerShellPath:  "powershell.exe",
		Timeout:            60 * time.Second,
		WinRMUseTLS:        os.Getenv("ADDS_WINRM_USE_TLS") == "true",
		WinRMAuth:          auth,
		WinRMKerberosRealm: os.Getenv("ADDS_WINRM_KERBEROS_REALM"),
	}

	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	return c
}

// TestEnsureUserEnabledNoPasswordBootstraps verifies that a user with enabled = true and no
// password is brought into the enabled state: EnsureUser sets a throwaway bootstrap password
// (never surfaced) so Active Directory allows enabling it. Adopting it again (the second
// EnsureUser, which runs the Set-ADUser branch) must stay enabled and must not fail with
// "password does not meet the length, complexity, or history requirement".
func TestEnsureUserEnabledNoPasswordBootstraps(t *testing.T) {
	c := liveADClient(t)
	ctx := context.Background()

	suffix := strconv.FormatInt(time.Now().UnixNano()%1_000_000, 10)
	ouPath := "adds-acc-userenable-" + suffix
	userName := "acc-ue-" + suffix

	ou, err := EnsureOrganizationalUnit(ctx, c, ouPath, nil)
	if err != nil {
		t.Fatalf("creating OU: %v", err)
	}
	t.Cleanup(func() {
		_ = DeleteOrganizationalUnit(ctx, c, ou.DistinguishedName, true, ou.CreatedOrganizationalUnits)
	})

	input := UserInput{
		Name:              userName,
		SamAccountName:    userName,
		UserPrincipalName: userName + "@example.test",
		Path:              ouPath,
		Enabled:           true,
	}

	created, err := EnsureUser(ctx, c, input)
	if err != nil {
		t.Fatalf("first EnsureUser: %v", err)
	}
	if !created.Enabled {
		t.Errorf("a passwordless account with enabled = true must be enabled via a bootstrap password, got enabled = false")
	}

	// Adopt the now-existing account. The account already has the bootstrap password, so the
	// enable attempt succeeds without resetting it again.
	adopted, err := EnsureUser(ctx, c, input)
	if err != nil {
		t.Fatalf("second EnsureUser (adopt) must not fail: %v", err)
	}
	if !adopted.Enabled {
		t.Errorf("adopting an enabled account must leave it enabled, got enabled = false")
	}
}
