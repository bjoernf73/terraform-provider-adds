package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	siteCommon = "site_common.ps1"
	siteEnsure = "site_ensure.ps1"
	siteRead   = "site_read.ps1"
	siteDelete = "site_delete.ps1"
)

type SiteInput struct {
	Name        string
	Description string
}

type Site struct {
	Exists            bool   `json:"exists"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	DistinguishedName string `json:"distinguished_name"`
}

func (i SiteInput) payload() map[string]any {
	return map[string]any{"name": i.Name, "description": i.Description}
}

func EnsureSite(ctx context.Context, c *client.Client, input SiteInput) (*Site, error) {
	return runSiteScript(ctx, c, input, siteEnsure)
}

func ReadSite(ctx context.Context, c *client.Client, name string) (*Site, error) {
	return runSiteScript(ctx, c, SiteInput{Name: name}, siteRead)
}

func DeleteSite(ctx context.Context, c *client.Client, name string) error {
	script, err := buildScript(c, SiteInput{Name: name}.payload(), commonScript, siteCommon, siteDelete)
	if err != nil {
		return err
	}
	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

func runSiteScript(ctx context.Context, c *client.Client, input SiteInput, operation string) (*Site, error) {
	script, err := buildScript(c, input.payload(), commonScript, siteCommon, operation)
	if err != nil {
		return nil, err
	}
	var result Site
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
