package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	gpoSecurityFilterEnsure = "gpo_security_filter_ensure.ps1"
	gpoSecurityFilterRead   = "gpo_security_filter_read.ps1"
	gpoSecurityFilterDelete = "gpo_security_filter_delete.ps1"
)

// GPOSecurityFilterInput makes the listed principals the complete set with the
// GpoApply permission on a GPO. Authenticated Users retains GpoRead.
type GPOSecurityFilterInput struct {
	GPO        string
	Principals []string
}

type GPOSecurityFilter struct {
	Exists        bool     `json:"exists"`
	GPOGUID       string   `json:"gpo_guid"`
	GPODN         string   `json:"gpo_dn"`
	PrincipalSIDs []string `json:"principal_sids"`
	Matches       bool     `json:"matches"`
}

func (i GPOSecurityFilterInput) payload() map[string]any {
	return map[string]any{
		"gpo":        i.GPO,
		"principals": i.Principals,
	}
}

func EnsureGPOSecurityFilter(ctx context.Context, c *client.Client, input GPOSecurityFilterInput) (*GPOSecurityFilter, error) {
	return runGPOSecurityFilterScript(ctx, c, input, gpoSecurityFilterEnsure)
}

func ReadGPOSecurityFilter(ctx context.Context, c *client.Client, input GPOSecurityFilterInput) (*GPOSecurityFilter, error) {
	return runGPOSecurityFilterScript(ctx, c, input, gpoSecurityFilterRead)
}

func DeleteGPOSecurityFilter(ctx context.Context, c *client.Client, input GPOSecurityFilterInput) error {
	script, err := buildScript(c, input.payload(), commonScript, gpoLinkCommon, gpoPermissionCommon, gpoSecurityFilterDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

func runGPOSecurityFilterScript(ctx context.Context, c *client.Client, input GPOSecurityFilterInput, body string) (*GPOSecurityFilter, error) {
	script, err := buildScript(c, input.payload(), commonScript, gpoLinkCommon, gpoPermissionCommon, body)
	if err != nil {
		return nil, err
	}

	var result GPOSecurityFilter
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
