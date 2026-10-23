package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const domainRead = "domain_read.ps1"

type Domain struct {
	DistinguishedName          string `json:"distinguished_name"`
	DNSRoot                    string `json:"dns_root"`
	NetBIOSName                string `json:"netbios_name"`
	SID                        string `json:"sid"`
	DomainMode                 string `json:"domain_mode"`
	Forest                     string `json:"forest"`
	UsersContainer             string `json:"users_container"`
	ComputersContainer         string `json:"computers_container"`
	DomainControllersContainer string `json:"domain_controllers_container"`
	PDCEmulator                string `json:"pdc_emulator"`
	InfrastructureMaster       string `json:"infrastructure_master"`
}

func ReadDomain(ctx context.Context, c *client.Client) (*Domain, error) {
	script, err := buildScript(c, map[string]any{}, commonScript, domainRead)
	if err != nil {
		return nil, err
	}

	var result Domain
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
