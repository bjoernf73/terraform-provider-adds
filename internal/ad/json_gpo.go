package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	gpRegistryPolicyParser = "gpregistrypolicyparser.ps1"
	jsonGPOCommon          = "json_gpo_common.ps1"
	jsonGPOEnsure          = "json_gpo_ensure.ps1"
	jsonGPOExportRead      = "json_gpo_export_read.ps1"
)

type JsonGPOInput struct {
	TargetName   string
	JSON         string
	Replacements map[string]string
}

func (i JsonGPOInput) payload() map[string]any {
	return map[string]any{
		"target_name":  i.TargetName,
		"json_raw":     i.JSON,
		"replacements": i.Replacements,
	}
}

// JsonGPO mirrors BackupGPO: both import into an ordinary GPO, so Read/Delete and the
// version-drift watermark are identical - the shared backup_gpo_read.ps1/backup_gpo_delete.ps1
// scripts are reused here. json_gpo writes the GPT.ini itself at import time and keeps it in
// lockstep with the AD versionNumber, so like backup_gpo it tracks only the AD versionNumber.
type JsonGPO struct {
	Exists            bool   `json:"exists"`
	GUID              string `json:"guid"`
	Name              string `json:"name"`
	DistinguishedName string `json:"distinguished_name"`
	Domain            string `json:"domain"`
	Status            string `json:"status"`

	// VersionNumber is the groupPolicyContainer's raw versionNumber, the single watermark the
	// resource tracks for drift. ComputerADVersion/UserADVersion are its low/high words, readable only.
	VersionNumber     int64 `json:"version_number"`
	ComputerADVersion int64 `json:"computer_ad_version"`
	UserADVersion     int64 `json:"user_ad_version"`
}

// EnsureJsonGPO imports a JSON GPO into the target GPO, creating it if needed.
// Re-running it against an existing target overwrites its SYSVOL content in place,
// preserving the target's GUID and links, so it also serves as the update path.
func EnsureJsonGPO(ctx context.Context, c *client.Client, input JsonGPOInput) (*JsonGPO, error) {
	script, err := buildScript(c, input.payload(), commonScript, backupGPOCommon, gpRegistryPolicyParser, jsonGPOCommon, jsonGPOEnsure)
	if err != nil {
		return nil, err
	}

	var result JsonGPO
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadJsonGPO(ctx context.Context, c *client.Client, guid string) (*JsonGPO, error) {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, backupGPORead)
	if err != nil {
		return nil, err
	}

	var result JsonGPO
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteJsonGPO(ctx context.Context, c *client.Client, guid string) error {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, backupGPOCommon, backupGPODelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

// JsonGPOExport is the read-only mirror of JsonGPOInput: JSON describing a live GPO's
// SYSVOL content, in the exact shape EnsureJsonGPO consumes.
type JsonGPOExport struct {
	Exists bool   `json:"exists"`
	GUID   string `json:"guid"`
	JSON   string `json:"json"`
}

// ExportJsonGPO reads a live GPO's SYSVOL content and relevant AD attributes and
// renders them as JSON, the mirror image of EnsureJsonGPO: SIDs become
// ####Replace[DOMAIN\Name] tokens here instead of being resolved from them.
func ExportJsonGPO(ctx context.Context, c *client.Client, name string) (*JsonGPOExport, error) {
	script, err := buildScript(c, map[string]any{
		"name": name,
	}, commonScript, backupGPOCommon, gpRegistryPolicyParser, jsonGPOCommon, jsonGPOExportRead)
	if err != nil {
		return nil, err
	}

	var result JsonGPOExport
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
