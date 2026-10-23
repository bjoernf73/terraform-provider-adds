package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	accessRuleCommon = "access_rule_common.ps1"
	accessRuleEnsure = "access_rule_ensure.ps1"
	accessRuleRead   = "access_rule_read.ps1"
	accessRuleDelete = "access_rule_delete.ps1"
)

type AccessRule struct {
	Exists                  bool     `json:"exists"`
	TargetDN                string   `json:"target_dn"`
	TrusteeSID              string   `json:"trustee_sid"`
	Access                  string   `json:"access"`
	Rights                  []string `json:"rights"`
	RightsMask              int64    `json:"rights_mask"`
	RightsMatch             bool     `json:"rights_match"`
	ObjectTypeGUID          string   `json:"object_type_guid"`
	InheritedObjectTypeGUID string   `json:"inherited_object_type_guid"`
	Inheritance             string   `json:"inheritance"`
}

// AccessRuleInput describes a single ACE on a directory object.
type AccessRuleInput struct {
	Target              string
	Trustee             string
	Rights              []string
	Access              string
	ObjectType          string
	InheritedObjectType string
	Inheritance         string
	IgnoreAdminCount1   bool
}

func (i AccessRuleInput) payload() map[string]any {
	return map[string]any{
		"target":                strings.TrimSpace(i.Target),
		"trustee":               strings.TrimSpace(i.Trustee),
		"rights":                i.Rights,
		"access":                i.Access,
		"object_type":           strings.TrimSpace(i.ObjectType),
		"inherited_object_type": strings.TrimSpace(i.InheritedObjectType),
		"inheritance":           strings.TrimSpace(i.Inheritance),
		"ignore_admin_count_1":  i.IgnoreAdminCount1,
	}
}

func EnsureAccessRule(ctx context.Context, c *client.Client, input AccessRuleInput) (*AccessRule, error) {
	return runAccessRuleScript(ctx, c, input, accessRuleEnsure)
}

func ReadAccessRule(ctx context.Context, c *client.Client, input AccessRuleInput) (*AccessRule, error) {
	return runAccessRuleScript(ctx, c, input, accessRuleRead)
}

func DeleteAccessRule(ctx context.Context, c *client.Client, input AccessRuleInput) error {
	script, err := buildScript(c, input.payload(), commonScript, accessRuleCommon, accessRuleDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

func runAccessRuleScript(ctx context.Context, c *client.Client, input AccessRuleInput, body string) (*AccessRule, error) {
	script, err := buildScript(c, input.payload(), commonScript, accessRuleCommon, body)
	if err != nil {
		return nil, err
	}

	var result AccessRule
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
