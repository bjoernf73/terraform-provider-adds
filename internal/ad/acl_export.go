package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const aclExportRead = "acl_export_read.ps1"

// ACLExportAce is one explicit ACE of an exported object, in the same vocabulary as adds_acl.
type ACLExportAce struct {
	Trustee             string   `json:"trustee"`
	Access              string   `json:"access"`
	Rights              []string `json:"rights"`
	ObjectType          string   `json:"object_type"`
	InheritedObjectType string   `json:"inherited_object_type"`
	Inheritance         string   `json:"inheritance"`
}

// ACLExportObject is one exported object's explicit DACL plus its inheritance-blocked flag.
type ACLExportObject struct {
	DistinguishedName  string         `json:"distinguished_name"`
	Path               string         `json:"path"`
	ObjectClass        string         `json:"object_class"`
	InheritanceBlocked bool           `json:"inheritance_blocked"`
	ACEs               []ACLExportAce `json:"aces"`
}

type ACLExport struct {
	Objects []ACLExportObject `json:"objects"`
}

// ACLExportInput selects which objects to export. ObjectTypes filters on structural class name
// (empty = all); Recursive descends below Path; IncludeEmpty keeps objects with no explicit ACEs.
type ACLExportInput struct {
	Path         string
	Recursive    bool
	ObjectTypes  []string
	IncludeEmpty bool
}

func (i ACLExportInput) payload() map[string]any {
	return map[string]any{
		"path":          strings.TrimSpace(i.Path),
		"recursive":     i.Recursive,
		"object_types":  i.ObjectTypes,
		"include_empty": i.IncludeEmpty,
	}
}

func ExportACL(ctx context.Context, c *client.Client, input ACLExportInput) (*ACLExport, error) {
	script, err := buildScript(c, input.payload(), commonScript, accessRuleCommon, aclCommon, aclExportRead)
	if err != nil {
		return nil, err
	}

	var result ACLExport
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
