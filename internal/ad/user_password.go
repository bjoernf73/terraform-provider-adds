package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adlc/internal/client"
)

const (
	userPasswordSet  = "user_password_set.ps1"
	userPasswordRead = "user_password_read.ps1"
)

// UserPasswordTarget identifies the user a adlc_user_password resource applies to.
// The password itself is never read back: AD has no attribute that exposes it, so the
// value in Terraform state is authoritative and is never compared against the live
// account. PwdLastSet is a timestamp, not the password, and is populated purely so
// drift (a password changed by anyone other than this resource) is visible in
// `terraform plan` instead of going unnoticed.
type UserPasswordTarget struct {
	Exists     bool   `json:"exists"`
	UserDN     string `json:"user_dn"`
	UserGUID   string `json:"user_guid"`
	PwdLastSet string `json:"pwd_last_set"`
}

func SetUserPassword(ctx context.Context, c *client.Client, user string, password string) (*UserPasswordTarget, error) {
	script, err := buildScript(c, map[string]any{
		"user":     strings.TrimSpace(user),
		"password": password,
	}, commonScript, userPasswordSet)
	if err != nil {
		return nil, err
	}

	var result UserPasswordTarget
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadUserPasswordTarget(ctx context.Context, c *client.Client, user string) (*UserPasswordTarget, error) {
	script, err := buildScript(c, map[string]any{
		"user": strings.TrimSpace(user),
	}, commonScript, userPasswordRead)
	if err != nil {
		return nil, err
	}

	var result UserPasswordTarget
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
