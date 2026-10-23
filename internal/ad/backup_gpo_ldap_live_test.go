package ad

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// defaultDomainPolicyGUID is the well-known GUID of the Default Domain Policy, which exists in
// every domain, so the parity test has a stable GPO to read without creating one.
const defaultDomainPolicyGUID = "31B2F340-016D-11D2-945F-00C04FB984F9"

// gpoModuleVersionRead is the GroupPolicy-module read of a GPO's version watermark, routed to
// powershell.exe (the compat layer under pwsh returns null versions). It is the oracle the LDAP
// version read is diffed against.
const gpoModuleVersionRead = `Import-Module GroupPolicy -ErrorAction Stop
$g = Get-GPO -Guid '%s'
[pscustomobject]@{
    guid                = $g.Id.ToString()
    name                = $g.DisplayName
    status              = $g.GpoStatus.ToString()
    domain              = $g.DomainName
    version_number      = ([int64]$g.User.DSVersion -shl 16) -bor [int64]$g.Computer.DSVersion
    computer_ad_version = [int64]$g.Computer.DSVersion
    user_ad_version     = [int64]$g.User.DSVersion
} | ConvertTo-Json -Compress`

type gpoModuleVersion struct {
	GUID              string `json:"guid"`
	Name              string `json:"name"`
	Status            string `json:"status"`
	Domain            string `json:"domain"`
	VersionNumber     int64  `json:"version_number"`
	ComputerADVersion int64  `json:"computer_ad_version"`
	UserADVersion     int64  `json:"user_ad_version"`
}

// TestBackupGPOReadLDAPParity asserts the LDAP GPO version read (versionNumber) matches
// the GroupPolicy module's Get-GPO for the Default Domain Policy. Host-gated: set ADDS_HOST.
func TestBackupGPOReadLDAPParity(t *testing.T) {
	c := liveADClient(t)
	ctx := context.Background()

	guid := strings.ToLower(defaultDomainPolicyGUID)

	ldap, err := ReadBackupGPO(ctx, c, guid)
	if err != nil {
		t.Fatalf("LDAP ReadBackupGPO: %v", err)
	}
	if !ldap.Exists {
		t.Fatalf("LDAP read reported the Default Domain Policy as missing")
	}

	var module gpoModuleVersion
	if err := c.RunPowerShellJSON(ctx, fmt.Sprintf(gpoModuleVersionRead, guid), &module); err != nil {
		t.Fatalf("module Get-GPO: %v", err)
	}

	strCases := []struct {
		name string
		got  string
		want string
	}{
		{"guid", ldap.GUID, module.GUID},
		{"name", ldap.Name, module.Name},
		{"status", ldap.Status, module.Status},
		{"domain", ldap.Domain, module.Domain},
	}
	for _, tc := range strCases {
		if tc.got != tc.want {
			t.Errorf("%s: LDAP=%q module=%q", tc.name, tc.got, tc.want)
		}
	}

	intCases := []struct {
		name string
		got  int64
		want int64
	}{
		{"version_number", ldap.VersionNumber, module.VersionNumber},
		{"computer_ad_version", ldap.ComputerADVersion, module.ComputerADVersion},
		{"user_ad_version", ldap.UserADVersion, module.UserADVersion},
	}
	for _, tc := range intCases {
		if tc.got != tc.want {
			t.Errorf("%s: LDAP=%d module=%d", tc.name, tc.got, tc.want)
		}
	}
}
