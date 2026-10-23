package ad

import (
	"context"
	"fmt"
	"testing"
)

// ouModuleReadScript is the ActiveDirectory-module OU read, kept here only so the parity test
// can diff it against the LDAP implementation now in organizational_unit_read.ps1.
const ouModuleReadScript = `Import-Module ActiveDirectory -ErrorAction Stop
$ou = Get-ADOrganizationalUnit -Identity '%s' -Properties Description, Name
[pscustomobject]@{
    name               = [string]$ou.Name
    description        = $ou.Description
    distinguished_name = [string]$ou.DistinguishedName
} | ConvertTo-Json -Compress`

type ouModuleRead struct {
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	DistinguishedName string  `json:"distinguished_name"`
}

// TestOrganizationalUnitReadLDAPParity reads the always-present Domain Controllers OU via the
// LDAP path and via Get-ADOrganizationalUnit and asserts they agree. Host-gated: set ADDS_HOST.
func TestOrganizationalUnitReadLDAPParity(t *testing.T) {
	c := liveADClient(t)
	ctx := context.Background()

	dom, err := ReadDomain(ctx, c)
	if err != nil {
		t.Fatalf("ReadDomain: %v", err)
	}

	ouDN := "OU=Domain Controllers," + dom.DistinguishedName

	ldap, err := ReadOrganizationalUnit(ctx, c, ouDN)
	if err != nil {
		t.Fatalf("LDAP ReadOrganizationalUnit: %v", err)
	}
	if !ldap.Exists {
		t.Fatalf("LDAP read reported the Domain Controllers OU as missing")
	}

	var module ouModuleRead
	if err := c.RunPowerShellJSON(ctx, fmt.Sprintf(ouModuleReadScript, ouDN), &module); err != nil {
		t.Fatalf("module Get-ADOrganizationalUnit: %v", err)
	}

	if ldap.Name != module.Name {
		t.Errorf("name: LDAP=%q module=%q", ldap.Name, module.Name)
	}
	if ldap.DistinguishedName != module.DistinguishedName {
		t.Errorf("distinguished_name: LDAP=%q module=%q", ldap.DistinguishedName, module.DistinguishedName)
	}

	ldapDesc, moduleDesc := "", ""
	if ldap.Description != nil {
		ldapDesc = *ldap.Description
	}
	if module.Description != nil {
		moduleDesc = *module.Description
	}
	if ldapDesc != moduleDesc {
		t.Errorf("description: LDAP=%q module=%q", ldapDesc, moduleDesc)
	}
}
