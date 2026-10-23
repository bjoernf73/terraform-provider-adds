package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	groupMemberCommon = "group_member_common.ps1"
	groupMemberEnsure = "group_member_ensure.ps1"
	groupMemberRead   = "group_member_read.ps1"
	groupMemberDelete = "group_member_delete.ps1"
)

type GroupMember struct {
	Exists      bool   `json:"exists"`
	GroupDN     string `json:"group_dn"`
	GroupGUID   string `json:"group_guid"`
	MemberDN    string `json:"member_dn"`
	MemberGUID  string `json:"member_guid"`
	MemberSID   string `json:"member_sid"`
	MemberClass string `json:"member_class"`
}

func groupMemberPayload(group string, member string) map[string]any {
	return map[string]any{
		"group":  strings.TrimSpace(group),
		"member": strings.TrimSpace(member),
	}
}

func EnsureGroupMember(ctx context.Context, c *client.Client, group string, member string) (*GroupMember, error) {
	return runGroupMemberScript(ctx, c, group, member, groupMemberEnsure)
}

func ReadGroupMember(ctx context.Context, c *client.Client, group string, member string) (*GroupMember, error) {
	return runGroupMemberScript(ctx, c, group, member, groupMemberRead)
}

func DeleteGroupMember(ctx context.Context, c *client.Client, group string, member string) error {
	script, err := buildScript(c, groupMemberPayload(group, member), commonScript, groupMemberCommon, groupMemberDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}

func runGroupMemberScript(ctx context.Context, c *client.Client, group string, member string, body string) (*GroupMember, error) {
	script, err := buildScript(c, groupMemberPayload(group, member), commonScript, groupMemberCommon, body)
	if err != nil {
		return nil, err
	}

	var result GroupMember
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
