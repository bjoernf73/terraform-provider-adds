package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	sysvolFilesCommon = "sysvol_files_common.ps1"
	sysvolFilesEnsure = "sysvol_files_ensure.ps1"
	sysvolFilesRead   = "sysvol_files_read.ps1"
	sysvolFilesDelete = "sysvol_files_delete.ps1"
)

const (
	SysvolTreeNetlogon                = "netlogon"
	SysvolTreeAdministrativeTemplates = "administrative_templates"
)

type SysvolFilesInput struct {
	Kind          string
	RelativePath  string
	Files         []SysvolFile
	PreviousPaths []string
}

type SysvolFilesResult struct {
	Exists  bool `json:"exists"`
	Matches bool `json:"matches"`
}

func (i SysvolFilesInput) payload() map[string]any {
	files := make([]map[string]string, 0, len(i.Files))
	for _, file := range i.Files {
		files = append(files, map[string]string{
			"path":    file.Path,
			"content": file.Content,
			"sha256":  file.SHA256,
		})
	}
	return map[string]any{
		"kind":           i.Kind,
		"relative_path":  i.RelativePath,
		"files":          files,
		"previous_paths": i.PreviousPaths,
	}
}

func EnsureSysvolFiles(ctx context.Context, c *client.Client, input SysvolFilesInput) (*SysvolFilesResult, error) {
	return runSysvolFilesScript(ctx, c, input, sysvolFilesEnsure)
}

func ReadSysvolFiles(ctx context.Context, c *client.Client, input SysvolFilesInput) (*SysvolFilesResult, error) {
	return runSysvolFilesScript(ctx, c, input, sysvolFilesRead)
}

func DeleteSysvolFiles(ctx context.Context, c *client.Client, input SysvolFilesInput) error {
	script, err := buildScript(c, input.payload(), commonScript, sysvolFilesCommon, sysvolFilesDelete)
	if err != nil {
		return err
	}
	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

func runSysvolFilesScript(ctx context.Context, c *client.Client, input SysvolFilesInput, operation string) (*SysvolFilesResult, error) {
	script, err := buildScript(c, input.payload(), commonScript, sysvolFilesCommon, operation)
	if err != nil {
		return nil, err
	}
	var result SysvolFilesResult
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
