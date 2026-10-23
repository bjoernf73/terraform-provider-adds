package ad

import (
	"context"
	"testing"
)

// domainModuleRead is the previous ActiveDirectory-module domain read, kept here only so the
// parity test can diff it against the LDAP implementation now in domain_read.ps1.
const domainModuleRead = `Import-Module ActiveDirectory -ErrorAction Stop
$d = Get-ADDomain
[pscustomobject]@{
    distinguished_name           = [string]$d.DistinguishedName
    dns_root                     = [string]$d.DNSRoot
    netbios_name                 = [string]$d.NetBIOSName
    sid                          = [string]$d.DomainSID
    domain_mode                  = [string]$d.DomainMode
    forest                       = [string]$d.Forest
    users_container              = [string]$d.UsersContainer
    computers_container          = [string]$d.ComputersContainer
    domain_controllers_container = [string]$d.DomainControllersContainer
    pdc_emulator                 = [string]$d.PDCEmulator
    infrastructure_master        = [string]$d.InfrastructureMaster
} | ConvertTo-Json -Compress`

// TestDomainReadLDAPParity asserts the LDAP domain read returns the same values as the
// ActiveDirectory module's Get-ADDomain. Host-gated: set ADDS_HOST (and the other ADDS_* vars)
// to run it against a live domain controller.
func TestDomainReadLDAPParity(t *testing.T) {
	c := liveADClient(t)
	ctx := context.Background()

	ldap, err := ReadDomain(ctx, c)
	if err != nil {
		t.Fatalf("LDAP ReadDomain: %v", err)
	}

	var module Domain
	if err := c.RunPowerShellJSON(ctx, domainModuleRead, &module); err != nil {
		t.Fatalf("module Get-ADDomain: %v", err)
	}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"distinguished_name", ldap.DistinguishedName, module.DistinguishedName},
		{"dns_root", ldap.DNSRoot, module.DNSRoot},
		{"netbios_name", ldap.NetBIOSName, module.NetBIOSName},
		{"sid", ldap.SID, module.SID},
		{"domain_mode", ldap.DomainMode, module.DomainMode},
		{"forest", ldap.Forest, module.Forest},
		{"users_container", ldap.UsersContainer, module.UsersContainer},
		{"computers_container", ldap.ComputersContainer, module.ComputersContainer},
		{"domain_controllers_container", ldap.DomainControllersContainer, module.DomainControllersContainer},
		{"pdc_emulator", ldap.PDCEmulator, module.PDCEmulator},
		{"infrastructure_master", ldap.InfrastructureMaster, module.InfrastructureMaster},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: LDAP=%q module=%q", tc.name, tc.got, tc.want)
		}
	}
}
