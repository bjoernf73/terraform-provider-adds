package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	organizationalUnitCommon   = "organizational_unit_common.ps1"
	organizationalUnitEnsure   = "organizational_unit_ensure.ps1"
	organizationalUnitRead     = "organizational_unit_read.ps1"
	organizationalUnitUpdate   = "organizational_unit_update.ps1"
	organizationalUnitDelete   = "organizational_unit_delete.ps1"
	organizationalUnitDataRead = "organizational_unit_data_read.ps1"
)

type OrganizationalUnit struct {
	Path                            string  `json:"path"`
	Description                     *string `json:"description"`
	DistinguishedName               string  `json:"distinguished_name"`
	Name                            string  `json:"name"`
	ProtectedFromAccidentalDeletion bool    `json:"protected_from_accidental_deletion"`
	Exists                          bool    `json:"exists"`
	// Distinguished names of ancestor OUs this resource created because they did not exist.
	// Populated by the ensure operation only; removed on delete while empty.
	CreatedOrganizationalUnits []string `json:"created_organizational_units"`
}

func EnsureOrganizationalUnit(ctx context.Context, c *client.Client, path string, description *string) (*OrganizationalUnit, error) {
	script, err := buildScript(c, map[string]any{
		"path":        NormalizePath(path),
		"description": description,
	}, commonScript, organizationalUnitCommon, organizationalUnitEnsure)
	if err != nil {
		return nil, err
	}

	var result OrganizationalUnit
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadOrganizationalUnit(ctx context.Context, c *client.Client, distinguishedName string) (*OrganizationalUnit, error) {
	script, err := buildScript(c, map[string]any{
		"distinguished_name": distinguishedName,
	}, commonScript, organizationalUnitCommon, organizationalUnitRead)
	if err != nil {
		return nil, err
	}

	var result OrganizationalUnit
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ReadOrganizationalUnitByPath looks up an existing organizational unit by a slash-delimited
// path relative to the domain root, a relative distinguished name, or a full distinguished
// name. It errors when no matching OU exists.
func ReadOrganizationalUnitByPath(ctx context.Context, c *client.Client, path string) (*OrganizationalUnit, error) {
	script, err := buildScript(c, map[string]any{
		"path": NormalizePath(path),
	}, commonScript, organizationalUnitCommon, organizationalUnitDataRead)
	if err != nil {
		return nil, err
	}

	var result OrganizationalUnit
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func UpdateOrganizationalUnit(ctx context.Context, c *client.Client, distinguishedName string, path string, description *string, createdOrganizationalUnits []string) (*OrganizationalUnit, error) {
	script, err := buildScript(c, map[string]any{
		"distinguished_name":           distinguishedName,
		"path":                         NormalizePath(path),
		"description":                  description,
		"created_organizational_units": createdOrganizationalUnits,
	}, commonScript, organizationalUnitCommon, organizationalUnitUpdate)
	if err != nil {
		return nil, err
	}

	var result OrganizationalUnit
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteOrganizationalUnit(ctx context.Context, c *client.Client, distinguishedName string, deleteSubtree bool, createdOrganizationalUnits []string) error {
	script, err := buildScript(c, map[string]any{
		"distinguished_name":           distinguishedName,
		"delete_subtree":               deleteSubtree,
		"created_organizational_units": createdOrganizationalUnits,
	}, commonScript, organizationalUnitCommon, organizationalUnitDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	return path
}
