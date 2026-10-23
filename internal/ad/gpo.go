package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	gpoCommon   = "gpo_common.ps1"
	gpoDataRead = "gpo_data_read.ps1"
)

// GPO is the read-only view of a Group Policy Object surfaced by the data source.
type GPO struct {
	GUID                  string  `json:"guid"`
	Name                  string  `json:"name"`
	DistinguishedName     string  `json:"distinguished_name"`
	Domain                string  `json:"domain"`
	Status                string  `json:"status"`
	Description           *string `json:"description"`
	CreationTime          *string `json:"creation_time"`
	ModificationTime      *string `json:"modification_time"`
	ComputerADVersion     int64   `json:"computer_ad_version"`
	ComputerSysvolVersion int64   `json:"computer_sysvol_version"`
	UserADVersion         int64   `json:"user_ad_version"`
	UserSysvolVersion     int64   `json:"user_sysvol_version"`
}

// ReadGPOByIdentity looks up an existing Group Policy Object by display name or GUID. It
// errors when no matching GPO exists.
func ReadGPOByIdentity(ctx context.Context, c *client.Client, identity string) (*GPO, error) {
	script, err := buildScript(c, map[string]any{
		"identity": strings.TrimSpace(identity),
	}, commonScript, gpoCommon, gpoDataRead)
	if err != nil {
		return nil, err
	}

	var result GPO
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
