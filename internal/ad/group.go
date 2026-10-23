package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	groupCommon         = "group_common.ps1"
	groupEnsure         = "group_ensure.ps1"
	groupRead           = "group_read.ps1"
	groupUpdate         = "group_update.ps1"
	groupDelete         = "group_delete.ps1"
	groupDataRead       = "group_data_read.ps1"
	groupScopePreflight = "group_scope_preflight.ps1"
)

type Group struct {
	Name                            string  `json:"name"`
	SamAccountName                  string  `json:"sam_account_name"`
	Description                     *string `json:"description"`
	DisplayName                     *string `json:"display_name"`
	Mail                            *string `json:"mail"`
	Info                            *string `json:"info"`
	Homepage                        *string `json:"homepage"`
	ManagedBy                       string  `json:"managed_by"`
	ManagedByMatch                  bool    `json:"managed_by_match"`
	ManagerCanUpdateMembership      bool    `json:"manager_can_update_membership"`
	ProtectedFromAccidentalDeletion bool    `json:"protected_from_accidental_deletion"`
	Category                        string  `json:"category"`
	Scope                           string  `json:"scope"`
	Path                            string  `json:"path"`
	PathMatch                       bool    `json:"path_match"`
	ContainerDN                     string  `json:"container_dn"`
	DistinguishedName               string  `json:"distinguished_name"`
	GUID                            string  `json:"guid"`
	SID                             string  `json:"sid"`
	Exists                          bool    `json:"exists"`
}

// GroupInput carries the reconcilable attributes of a group.
type GroupInput struct {
	Name                            string
	SamAccountName                  string
	Path                            string
	Description                     *string
	DisplayName                     *string
	Mail                            *string
	Info                            *string
	Homepage                        *string
	ManagedBy                       string
	ManagerCanUpdateMembership      bool
	ProtectedFromAccidentalDeletion bool
	Category                        string
	Scope                           string
}

func (i GroupInput) payload() map[string]any {
	return map[string]any{
		"name":                               strings.TrimSpace(i.Name),
		"sam_account_name":                   strings.TrimSpace(i.SamAccountName),
		"path":                               NormalizePath(i.Path),
		"description":                        i.Description,
		"display_name":                       i.DisplayName,
		"mail":                               i.Mail,
		"info":                               i.Info,
		"homepage":                           i.Homepage,
		"managed_by":                         strings.TrimSpace(i.ManagedBy),
		"manager_can_update_membership":      i.ManagerCanUpdateMembership,
		"protected_from_accidental_deletion": i.ProtectedFromAccidentalDeletion,
		"category":                           i.Category,
		"scope":                              i.Scope,
	}
}

func EnsureGroup(ctx context.Context, c *client.Client, input GroupInput) (*Group, error) {
	script, err := buildScript(c, input.payload(), commonScript, groupCommon, groupEnsure)
	if err != nil {
		return nil, err
	}

	var result Group
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ReadGroupByIdentity looks up an existing group by any AD identity (distinguished name,
// objectGUID, SID, DOMAIN\name or sAMAccountName). It errors when no matching group exists.
func ReadGroupByIdentity(ctx context.Context, c *client.Client, identity string) (*Group, error) {
	script, err := buildScript(c, map[string]any{
		"identity": strings.TrimSpace(identity),
	}, commonScript, groupCommon, groupDataRead)
	if err != nil {
		return nil, err
	}

	var result Group
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GroupScopeBlocker is a related group whose nesting prevents a scope conversion step.
// Relation is "memberOf" (a group this group belongs to) or "member" (a group this group
// contains).
type GroupScopeBlocker struct {
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	Relation string `json:"relation"`
}

// GroupScopePreflight reports whether a scope conversion is possible given the group's
// current nesting, without attempting the change.
type GroupScopePreflight struct {
	CanConvert bool                `json:"can_convert"`
	FromScope  string              `json:"from_scope"`
	ToScope    string              `json:"to_scope"`
	Blocking   []GroupScopeBlocker `json:"blocking"`
}

// PreflightGroupScopeConversion inspects the group's memberOf and member sets and evaluates
// each step of the current -> desired scope conversion (stepping through Universal where AD
// requires it) against AD's nesting rules. It is best-effort: a clean result does not
// guarantee the change succeeds, and the apply still enforces the rules.
func PreflightGroupScopeConversion(ctx context.Context, c *client.Client, guid, currentScope, desiredScope string) (*GroupScopePreflight, error) {
	script, err := buildScript(c, map[string]any{
		"guid":          guid,
		"current_scope": currentScope,
		"desired_scope": desiredScope,
	}, commonScript, groupCommon, groupScopePreflight)
	if err != nil {
		return nil, err
	}

	var result GroupScopePreflight
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadGroup(ctx context.Context, c *client.Client, guid string, path string, managedBy string) (*Group, error) {
	script, err := buildScript(c, map[string]any{
		"guid":       guid,
		"path":       path,
		"managed_by": managedBy,
	}, commonScript, groupCommon, groupRead)
	if err != nil {
		return nil, err
	}

	var result Group
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func UpdateGroup(ctx context.Context, c *client.Client, guid string, input GroupInput) (*Group, error) {
	payload := input.payload()
	payload["guid"] = guid

	script, err := buildScript(c, payload, commonScript, groupCommon, groupUpdate)
	if err != nil {
		return nil, err
	}

	var result Group
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteGroup(ctx context.Context, c *client.Client, guid string) error {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, groupCommon, groupDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}
