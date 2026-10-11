package ad

import (
	"context"
	"encoding/xml"

	"github.com/bjoernf73/terraform-provider-adlc/internal/client"
)

const (
	backupGPOCommon     = "backup_gpo_common.ps1"
	backupGPOEnsure     = "backup_gpo_ensure.ps1"
	backupGPORead       = "backup_gpo_read.ps1"
	backupGPODelete     = "backup_gpo_delete.ps1"
	backupGPOExportRead = "backup_gpo_export_read.ps1"
)

// BackupGPOFile is a single file copied into the payload, relative to the backup
// folder (path/backup_name), forward-slash separated, content base64-encoded.
type BackupGPOFile struct {
	Path    string
	Content string
}

// BackupGPOMigration is one entry of a GPO migration table, applied by Import-GPO to
// remap security principals and UNC paths baked into the backed-up GPO. Exactly one of
// Destination or SameAsSource should be set: SameAsSource tells Import-GPO to re-resolve
// the same name in the target domain/forest instead of substituting a fixed value -
// MTEdit emits this as <DestinationSameAsSource/> whenever no explicit mapping is given.
type BackupGPOMigration struct {
	Source       string
	Destination  string
	SameAsSource bool
	Type         string
}

type BackupGPOInput struct {
	BackupName string
	TargetName string
	Files      []BackupGPOFile
	Migrations []BackupGPOMigration
}

func (i BackupGPOInput) payload() map[string]any {
	files := make([]map[string]any, 0, len(i.Files))
	for _, f := range i.Files {
		files = append(files, map[string]any{
			"path":    f.Path,
			"content": f.Content,
		})
	}

	return map[string]any{
		"backup_name":         i.BackupName,
		"target_name":         i.TargetName,
		"files":               files,
		"migration_table_xml": BuildMigrationTableXML(i.Migrations),
	}
}

type BackupGPO struct {
	Exists            bool   `json:"exists"`
	GUID              string `json:"guid"`
	Name              string `json:"name"`
	DistinguishedName string `json:"distinguished_name"`
	Domain            string `json:"domain"`
	Status            string `json:"status"`

	// VersionNumber is the groupPolicyContainer's raw versionNumber, the single watermark the
	// resource tracks for drift: AD bumps it on every settings change (any tool) and never lowers
	// it. ComputerADVersion/UserADVersion are its low/high words, kept only as readable values.
	VersionNumber     int64 `json:"version_number"`
	ComputerADVersion int64 `json:"computer_ad_version"`
	UserADVersion     int64 `json:"user_ad_version"`
}

type BackupGPOExport struct {
	Exists   bool              `json:"exists"`
	GUID     string            `json:"guid"`
	BackupID string            `json:"backup_id"`
	Files    map[string]string `json:"files"`
}

func ExportBackupGPO(ctx context.Context, c *client.Client, name string) (*BackupGPOExport, error) {
	script, err := buildScript(c, map[string]any{"name": name}, commonScript, backupGPOCommon, backupGPOExportRead)
	if err != nil {
		return nil, err
	}

	var result BackupGPOExport
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// EnsureBackupGPO imports a GPO backup into the target GPO, creating it if needed.
// Re-running it against an existing target re-imports the settings in place, preserving
// the target's GUID and links, so it also serves as the update path.
func EnsureBackupGPO(ctx context.Context, c *client.Client, input BackupGPOInput) (*BackupGPO, error) {
	script, err := buildScript(c, input.payload(), commonScript, backupGPOCommon, backupGPOEnsure)
	if err != nil {
		return nil, err
	}

	var result BackupGPO
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadBackupGPO(ctx context.Context, c *client.Client, guid string) (*BackupGPO, error) {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, backupGPORead)
	if err != nil {
		return nil, err
	}

	var result BackupGPO
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteBackupGPO(ctx context.Context, c *client.Client, guid string) error {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, backupGPOCommon, backupGPODelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

type migrationTableXML struct {
	XMLName xml.Name              `xml:"MigrationTable"`
	Xmlns   string                `xml:"xmlns,attr"`
	Mapping []migrationMappingXML `xml:"Mapping"`
}

type migrationMappingXML struct {
	Type                 string    `xml:"Type"`
	Source               string    `xml:"Source"`
	Destination          string    `xml:"Destination,omitempty"`
	DestinationSameAsSrc *struct{} `xml:"DestinationSameAsSource,omitempty"`
}

// BuildMigrationTableXML renders migrations as a GPMC migration table (.migtable) file,
// the format Import-GPO's -MigrationTable parameter expects. Returns "" when there are
// no migrations, so callers can skip passing -MigrationTable entirely.
func BuildMigrationTableXML(migrations []BackupGPOMigration) string {
	if len(migrations) == 0 {
		return ""
	}

	table := migrationTableXML{
		Xmlns: "http://www.microsoft.com/GroupPolicy/GPOOperations/MigrationTable",
	}
	for _, m := range migrations {
		mapping := migrationMappingXML{
			Type:   m.Type,
			Source: m.Source,
		}
		if m.SameAsSource {
			mapping.DestinationSameAsSrc = &struct{}{}
		} else {
			mapping.Destination = m.Destination
		}
		table.Mapping = append(table.Mapping, mapping)
	}

	out, err := xml.MarshalIndent(table, "", "  ")
	if err != nil {
		return ""
	}

	return xml.Header + string(out) + "\n"
}
