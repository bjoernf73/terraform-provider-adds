package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	aclCommon = "acl_common.ps1"
	aclEnsure = "acl_ensure.ps1"
	aclRead   = "acl_read.ps1"
	aclDelete = "acl_delete.ps1"
)

// ACLAce is a single explicit ACE in an authoritative DACL, expressed with the same
// vocabulary as adds_access_rule.
type ACLAce struct {
	Trustee             string   `json:"trustee"`
	Access              string   `json:"access"`
	Rights              []string `json:"rights"`
	ObjectType          string   `json:"object_type"`
	InheritedObjectType string   `json:"inherited_object_type"`
	Inheritance         string   `json:"inheritance"`
}

// ACL is the authoritative DACL of a directory object.
type ACL struct {
	Exists             bool     `json:"exists"`
	TargetDN           string   `json:"target_dn"`
	InheritanceBlocked bool     `json:"inheritance_blocked"`
	ACEs               []ACLAce `json:"aces"`
}

// ACLInput describes the desired authoritative DACL of a directory object.
type ACLInput struct {
	Target             string
	InheritanceBlocked bool
	IgnoreAdminCount1  bool
	ACEs               []ACLAce
}

func (i ACLInput) payload() map[string]any {
	aces := make([]map[string]any, 0, len(i.ACEs))
	for _, ace := range i.ACEs {
		aces = append(aces, map[string]any{
			"trustee":               strings.TrimSpace(ace.Trustee),
			"access":                ace.Access,
			"rights":                ace.Rights,
			"object_type":           strings.TrimSpace(ace.ObjectType),
			"inherited_object_type": strings.TrimSpace(ace.InheritedObjectType),
			"inheritance":           strings.TrimSpace(ace.Inheritance),
		})
	}

	return map[string]any{
		"target":               strings.TrimSpace(i.Target),
		"inheritance_blocked":  i.InheritanceBlocked,
		"ignore_admin_count_1": i.IgnoreAdminCount1,
		"aces":                 aces,
	}
}

func EnsureACL(ctx context.Context, c *client.Client, input ACLInput) (*ACL, error) {
	return runACLScript(ctx, c, input, aclEnsure)
}

func ReadACL(ctx context.Context, c *client.Client, input ACLInput) (*ACL, error) {
	return runACLScript(ctx, c, input, aclRead)
}

func DeleteACL(ctx context.Context, c *client.Client, input ACLInput) error {
	script, err := buildScript(c, input.payload(), commonScript, accessRuleCommon, aclCommon, aclDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

func runACLScript(ctx context.Context, c *client.Client, input ACLInput, body string) (*ACL, error) {
	script, err := buildScript(c, input.payload(), commonScript, accessRuleCommon, aclCommon, body)
	if err != nil {
		return nil, err
	}

	var result ACL
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
