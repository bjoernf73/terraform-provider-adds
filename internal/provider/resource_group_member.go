package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                = &groupMemberResource{}
	_ resource.ResourceWithConfigure   = &groupMemberResource{}
	_ resource.ResourceWithImportState = &groupMemberResource{}
)

func NewGroupMemberResource() resource.Resource {
	return &groupMemberResource{}
}

type groupMemberResource struct {
	client *client.Client
}

type groupMemberResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Group       types.String `tfsdk:"group"`
	Member      types.String `tfsdk:"member"`
	GroupDN     types.String `tfsdk:"group_dn"`
	MemberDN    types.String `tfsdk:"member_dn"`
	MemberSID   types.String `tfsdk:"member_sid"`
	MemberClass types.String `tfsdk:"member_class"`
}

func (r *groupMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_member"
}

func (r *groupMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computed := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Adds a single member to an Active Directory group.\n\n" +
			"This resource is **non-authoritative**: it manages one membership and ignores every " +
			"other member of the group. Use one resource per membership.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier, formed as `<group objectGUID>/<member objectGUID>`.",
				PlanModifiers:       computed,
			},
			"group": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Group to add the member to. Accepts a distinguished name, `objectGUID`, SID, `DOMAIN\\name` or `sAMAccountName`.",
				PlanModifiers:       replace,
			},
			"member": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Member to add. Accepts a distinguished name, `objectGUID`, SID, `DOMAIN\\name` or `sAMAccountName`. May be a user, group, computer or service account.",
				PlanModifiers:       replace,
			},
			"group_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the group.",
				PlanModifiers:       computed,
			},
			"member_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the member.",
				PlanModifiers:       computed,
			},
			"member_sid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Security identifier of the member.",
				PlanModifiers:       computed,
			},
			"member_class": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Object class of the member, for example `user`, `group` or `computer`.",
				PlanModifiers:       computed,
			},
		},
	}
}

func (r *groupMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	addsClient, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}

	r.client = addsClient
}

func (r *groupMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	membership, err := ad.EnsureGroupMember(ctx, r.client, plan.Group.ValueString(), plan.Member.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to add group member", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, groupMemberState(plan, membership))...)
}

func (r *groupMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	membership, err := ad.ReadGroupMember(ctx, r.client, state.Group.ValueString(), state.Member.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read group member", err.Error())
		return
	}

	if !membership.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, groupMemberState(state, membership))...)
}

// Update is unreachable: both inputs require replacement.
func (r *groupMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *groupMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteGroupMember(ctx, r.client, state.Group.ValueString(), state.Member.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to remove group member", err.Error())
	}
}

func (r *groupMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	group, member, found := strings.Cut(req.ID, "/")
	if !found {
		resp.Diagnostics.AddError(
			"Invalid import id",
			fmt.Sprintf("Expected \"<group>/<member>\", got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group"), group)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("member"), member)...)
}

func groupMemberState(model groupMemberResourceModel, membership *ad.GroupMember) groupMemberResourceModel {
	model.ID = types.StringValue(membership.GroupGUID + "/" + membership.MemberGUID)
	model.GroupDN = types.StringValue(membership.GroupDN)
	model.MemberDN = types.StringValue(membership.MemberDN)
	model.MemberSID = types.StringValue(membership.MemberSID)
	model.MemberClass = types.StringValue(membership.MemberClass)

	return model
}
