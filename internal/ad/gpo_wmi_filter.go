package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	gpoWMIFilterEnsure = "gpo_wmi_filter_ensure.ps1"
	gpoWMIFilterRead   = "gpo_wmi_filter_read.ps1"
	gpoWMIFilterDelete = "gpo_wmi_filter_delete.ps1"
)

// GPOWMIFilterInput assigns wmi_filter (a GUID or filter name) as the single WMI filter
// on gpo (a GUID or GPO display name). A GPO can only have one WMI filter at a time.
type GPOWMIFilterInput struct {
	GPO       string
	WMIFilter string
}

type GPOWMIFilter struct {
	Exists        bool   `json:"exists"`
	GPOGUID       string `json:"gpo_guid"`
	WMIFilterGUID string `json:"wmi_filter_guid"`
	GPODN         string `json:"gpo_dn"`
}

// EnsureGPOWMIFilter resolves both identities and sets gPCWQLFilter on the GPO. Re-running
// it (e.g. to change wmi_filter) is how updates are applied too.
func EnsureGPOWMIFilter(ctx context.Context, c *client.Client, input GPOWMIFilterInput) (*GPOWMIFilter, error) {
	script, err := buildScript(c, map[string]any{
		"gpo":        input.GPO,
		"wmi_filter": input.WMIFilter,
	}, commonScript, gpoLinkCommon, wmiFilterCommon, gpoWMIFilterEnsure)
	if err != nil {
		return nil, err
	}

	var result GPOWMIFilter
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadGPOWMIFilter(ctx context.Context, c *client.Client, gpoGUID string) (*GPOWMIFilter, error) {
	script, err := buildScript(c, map[string]any{
		"gpo_guid": gpoGUID,
	}, commonScript, gpoWMIFilterRead)
	if err != nil {
		return nil, err
	}

	var result GPOWMIFilter
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteGPOWMIFilter(ctx context.Context, c *client.Client, gpoGUID string) error {
	script, err := buildScript(c, map[string]any{
		"gpo_guid": gpoGUID,
	}, commonScript, gpoWMIFilterDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}
