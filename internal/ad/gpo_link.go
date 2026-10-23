package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	gpoLinkCommon     = "gpo_link_common.ps1"
	gpoLinkLDAPCommon = "gpo_link_ldap_common.ps1"
	gpoLinkEnsure     = "gpo_link_ensure.ps1"
	gpoLinkRead       = "gpo_link_read.ps1"
	gpoLinkDelete     = "gpo_link_delete.ps1"
)

// GPOLinkEntry is one desired link, in the order it should take precedence (first has
// the highest precedence, matching Set-GPLink's -Order).
type GPOLinkEntry struct {
	GPO      string
	Enabled  bool
	Enforced bool
}

type GPOLinksInput struct {
	Target           string
	BlockInheritance bool
	Links            []GPOLinkEntry
}

func (i GPOLinksInput) payload() map[string]any {
	links := make([]map[string]any, 0, len(i.Links))
	for _, l := range i.Links {
		links = append(links, map[string]any{
			"gpo":      strings.TrimSpace(l.GPO),
			"enabled":  l.Enabled,
			"enforced": l.Enforced,
		})
	}

	return map[string]any{
		"target":            NormalizePath(i.Target),
		"block_inheritance": i.BlockInheritance,
		"links":             links,
	}
}

type GPOLinkResult struct {
	GPOGUID  string `json:"gpo_guid"`
	GPOName  string `json:"gpo_name"`
	Enabled  bool   `json:"enabled"`
	Enforced bool   `json:"enforced"`
	Order    int    `json:"order"`
}

type GPOLinks struct {
	Exists           bool            `json:"exists"`
	TargetDN         string          `json:"target_dn"`
	BlockInheritance bool            `json:"block_inheritance"`
	Links            []GPOLinkResult `json:"links"`
}

// EnsureGPOLinks makes the target's GPO links authoritative: it removes any link not in
// Links, then creates/updates the rest in order (first entry gets the highest
// precedence), and sets block_inheritance. Re-running it is how updates are applied too.
func EnsureGPOLinks(ctx context.Context, c *client.Client, input GPOLinksInput) (*GPOLinks, error) {
	script, err := buildScript(c, input.payload(), commonScript, gpoLinkLDAPCommon, gpoLinkEnsure)
	if err != nil {
		return nil, err
	}

	var result GPOLinks
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadGPOLinks(ctx context.Context, c *client.Client, targetDN string) (*GPOLinks, error) {
	script, err := buildScript(c, map[string]any{
		"target_dn": targetDN,
	}, commonScript, gpoLinkLDAPCommon, gpoLinkRead)
	if err != nil {
		return nil, err
	}

	var result GPOLinks
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteGPOLinks(ctx context.Context, c *client.Client, targetDN string) error {
	script, err := buildScript(c, map[string]any{
		"target_dn": targetDN,
	}, commonScript, gpoLinkLDAPCommon, gpoLinkDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}
