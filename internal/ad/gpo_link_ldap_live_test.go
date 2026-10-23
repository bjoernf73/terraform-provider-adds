package ad

import (
	"context"
	"strings"
	"testing"
)

// gpoLinksModuleRead reads the domain root's GPO links through the GroupPolicy module (routed to
// powershell.exe, where the module is native and returns live objects). It is the oracle the LDAP
// read is diffed against. The domain root always has at least the Default Domain Policy linked.
const gpoLinksModuleRead = `Import-Module ActiveDirectory -ErrorAction Stop
Import-Module GroupPolicy -ErrorAction Stop
$domainDN = (Get-ADDomain).DistinguishedName
$inheritance = Get-GPInheritance -Target $domainDN
$links = @()
foreach ($link in @($inheritance.GpoLinks | Sort-Object -Property Order)) {
    $links += [pscustomobject]@{
        gpo_guid = $link.GpoId.ToString().ToLower()
        enabled  = [bool]$link.Enabled
        enforced = [bool]$link.Enforced
        order    = [int]$link.Order
    }
}
[pscustomobject]@{
    target_dn         = $domainDN
    block_inheritance = ($inheritance.GpoInheritanceBlocked -eq 'Yes')
    links             = @($links)
} | ConvertTo-Json -Compress -Depth 5`

type gpoLinksModule struct {
	TargetDN         string `json:"target_dn"`
	BlockInheritance bool   `json:"block_inheritance"`
	Links            []struct {
		GPOGUID  string `json:"gpo_guid"`
		Enabled  bool   `json:"enabled"`
		Enforced bool   `json:"enforced"`
		Order    int    `json:"order"`
	} `json:"links"`
}

// TestGPOLinksReadLDAPParity asserts the LDAP gPLink read matches the GroupPolicy module's
// Get-GPInheritance for the domain root: same GUIDs, flags, order and block-inheritance. Host-gated:
// set ADDS_HOST. Ordering direction across multiple links is additionally exercised by the
// showcase's multi-link OUs (a reversed parse would make those non-idempotent).
func TestGPOLinksReadLDAPParity(t *testing.T) {
	c := liveADClient(t)
	ctx := context.Background()

	var module gpoLinksModule
	if err := c.RunPowerShellJSON(ctx, gpoLinksModuleRead, &module); err != nil {
		t.Fatalf("module Get-GPInheritance: %v", err)
	}

	ldap, err := ReadGPOLinks(ctx, c, module.TargetDN)
	if err != nil {
		t.Fatalf("LDAP ReadGPOLinks: %v", err)
	}
	if !ldap.Exists {
		t.Fatalf("LDAP read reported the domain root %q as missing", module.TargetDN)
	}

	if ldap.BlockInheritance != module.BlockInheritance {
		t.Errorf("block_inheritance: LDAP=%v module=%v", ldap.BlockInheritance, module.BlockInheritance)
	}

	if len(ldap.Links) != len(module.Links) {
		t.Fatalf("link count: LDAP=%d module=%d", len(ldap.Links), len(module.Links))
	}

	for _, m := range module.Links {
		var found *GPOLinkResult
		for i := range ldap.Links {
			if ldap.Links[i].Order == m.Order {
				found = &ldap.Links[i]
				break
			}
		}
		if found == nil {
			t.Errorf("order %d present in module but not LDAP", m.Order)
			continue
		}
		if !strings.EqualFold(found.GPOGUID, m.GPOGUID) {
			t.Errorf("order %d guid: LDAP=%s module=%s", m.Order, found.GPOGUID, m.GPOGUID)
		}
		if found.Enabled != m.Enabled {
			t.Errorf("order %d enabled: LDAP=%v module=%v", m.Order, found.Enabled, m.Enabled)
		}
		if found.Enforced != m.Enforced {
			t.Errorf("order %d enforced: LDAP=%v module=%v", m.Order, found.Enforced, m.Enforced)
		}
	}
}
