package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	gpoPermissionCommon = "gpo_permission_common.ps1"
	gpoPermissionEnsure = "gpo_permission_ensure.ps1"
	gpoPermissionRead   = "gpo_permission_read.ps1"
	gpoPermissionDelete = "gpo_permission_delete.ps1"
)

// GPOPermissionInput manages one trustee's named Group Policy permission.
type GPOPermissionInput struct {
	GPO        string
	Trustee    string
	Permission string
}

type GPOPermission struct {
	Exists     bool   `json:"exists"`
	GPOGUID    string `json:"gpo_guid"`
	GPODN      string `json:"gpo_dn"`
	TrusteeSID string `json:"trustee_sid"`
	Permission string `json:"permission"`
}

func (i GPOPermissionInput) payload() map[string]any {
	return map[string]any{
		"gpo":        i.GPO,
		"trustee":    i.Trustee,
		"permission": i.Permission,
	}
}

func EnsureGPOPermission(ctx context.Context, c *client.Client, input GPOPermissionInput) (*GPOPermission, error) {
	return runGPOPermissionScript(ctx, c, input, gpoPermissionEnsure)
}

func ReadGPOPermission(ctx context.Context, c *client.Client, input GPOPermissionInput) (*GPOPermission, error) {
	return runGPOPermissionScript(ctx, c, input, gpoPermissionRead)
}

func DeleteGPOPermission(ctx context.Context, c *client.Client, input GPOPermissionInput) error {
	script, err := buildScript(c, input.payload(), commonScript, gpoLinkCommon, gpoPermissionCommon, gpoPermissionDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

func runGPOPermissionScript(ctx context.Context, c *client.Client, input GPOPermissionInput, body string) (*GPOPermission, error) {
	script, err := buildScript(c, input.payload(), commonScript, gpoLinkCommon, gpoPermissionCommon, body)
	if err != nil {
		return nil, err
	}

	var result GPOPermission
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
