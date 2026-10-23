package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	subnetCommon = "subnet_common.ps1"
	subnetEnsure = "subnet_ensure.ps1"
	subnetRead   = "subnet_read.ps1"
	subnetDelete = "subnet_delete.ps1"
)

type SubnetInput struct {
	Name        string
	Site        string
	Description string
	Location    string
}

type Subnet struct {
	Exists            bool   `json:"exists"`
	Name              string `json:"name"`
	SiteName          string `json:"site_name"`
	Description       string `json:"description"`
	Location          string `json:"location"`
	DistinguishedName string `json:"distinguished_name"`
}

func (i SubnetInput) payload() map[string]any {
	return map[string]any{"name": i.Name, "site": i.Site, "description": i.Description, "location": i.Location}
}

func EnsureSubnet(ctx context.Context, c *client.Client, input SubnetInput) (*Subnet, error) {
	return runSubnetScript(ctx, c, input, subnetEnsure)
}

func ReadSubnet(ctx context.Context, c *client.Client, name string) (*Subnet, error) {
	return runSubnetScript(ctx, c, SubnetInput{Name: name}, subnetRead)
}

func DeleteSubnet(ctx context.Context, c *client.Client, name string) error {
	script, err := buildScript(c, SubnetInput{Name: name}.payload(), commonScript, siteCommon, subnetCommon, subnetDelete)
	if err != nil {
		return err
	}
	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

func runSubnetScript(ctx context.Context, c *client.Client, input SubnetInput, operation string) (*Subnet, error) {
	script, err := buildScript(c, input.payload(), commonScript, siteCommon, subnetCommon, operation)
	if err != nil {
		return nil, err
	}
	var result Subnet
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
