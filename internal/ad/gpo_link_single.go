package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	gpoSingleLinkEnsure = "gpo_single_link_ensure.ps1"
	gpoSingleLinkRead   = "gpo_single_link_read.ps1"
	gpoSingleLinkDelete = "gpo_single_link_delete.ps1"
)

// GPOLinkInput describes one non-authoritative GPO link on a target. Order is nil when the
// caller does not pin a position.
type GPOLinkInput struct {
	Target       string
	GPO          string
	Enabled      bool
	Enforced     bool
	Order        *int64
	MonitorOrder bool
}

func (i GPOLinkInput) payload() map[string]any {
	var order any
	if i.Order != nil {
		order = *i.Order
	}

	return map[string]any{
		"target":        NormalizePath(i.Target),
		"gpo":           strings.TrimSpace(i.GPO),
		"enabled":       i.Enabled,
		"enforced":      i.Enforced,
		"order":         order,
		"monitor_order": i.MonitorOrder,
	}
}

// GPOLink is the state of a single GPO link: its resolved GPO, option flags and current
// precedence position on the target.
type GPOLink struct {
	Exists       bool   `json:"exists"`
	TargetDN     string `json:"target_dn"`
	GPOGUID      string `json:"gpo_guid"`
	GPOName      string `json:"gpo_name"`
	Enabled      bool   `json:"enabled"`
	Enforced     bool   `json:"enforced"`
	CurrentOrder int    `json:"current_order"`
}

// EnsureGPOLink inserts or updates a single link on the target without touching the other
// links, and reports the position it ended up in. Shared by create and update.
func EnsureGPOLink(ctx context.Context, c *client.Client, input GPOLinkInput) (*GPOLink, error) {
	script, err := buildScript(c, input.payload(), commonScript, gpoLinkLDAPCommon, gpoSingleLinkEnsure)
	if err != nil {
		return nil, err
	}

	var result GPOLink
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadGPOLink(ctx context.Context, c *client.Client, targetDN, gpoGUID string) (*GPOLink, error) {
	script, err := buildScript(c, map[string]any{
		"target_dn": targetDN,
		"gpo_guid":  gpoGUID,
	}, commonScript, gpoLinkLDAPCommon, gpoSingleLinkRead)
	if err != nil {
		return nil, err
	}

	var result GPOLink
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteGPOLink(ctx context.Context, c *client.Client, targetDN, gpoGUID string) error {
	script, err := buildScript(c, map[string]any{
		"target_dn": targetDN,
		"gpo_guid":  gpoGUID,
	}, commonScript, gpoLinkLDAPCommon, gpoSingleLinkDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}
