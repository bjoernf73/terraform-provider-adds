package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                   = &groupResource{}
	_ resource.ResourceWithConfigure      = &groupResource{}
	_ resource.ResourceWithImportState    = &groupResource{}
	_ resource.ResourceWithValidateConfig = &groupResource{}
	_ resource.ResourceWithModifyPlan     = &groupResource{}
)

func NewGroupResource() resource.Resource {
	return &groupResource{}
}

type groupResource struct {
	client *client.Client
}

type groupResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	SamAccountName    types.String `tfsdk:"sam_account_name"`
	Path              types.String `tfsdk:"path"`
	Description       types.String `tfsdk:"description"`
	DisplayName       types.String `tfsdk:"display_name"`
	Mail              types.String `tfsdk:"mail"`
	Info              types.String `tfsdk:"info"`
	Homepage          types.String `tfsdk:"homepage"`
	ManagedBy         types.String `tfsdk:"managed_by"`
	ManagedByDN       types.String `tfsdk:"managed_by_dn"`
	ManagerCanUpdate  types.Bool   `tfsdk:"manager_can_update_membership"`
	Protected         types.Bool   `tfsdk:"protected_from_accidental_deletion"`
	Category          types.String `tfsdk:"category"`
	Scope             types.String `tfsdk:"scope"`
	DistinguishedName types.String `tfsdk:"distinguished_name"`
	SID               types.String `tfsdk:"sid"`
}

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Active Directory group by executing PowerShell on a remote Windows host.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier. Equals the group `objectGUID`, which is stable across renames and moves.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Group name (the `CN`). Changing this renames the group in place.",
			},
			"sam_account_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Pre-Windows 2000 group name. Defaults to `name`. Maximum 20 characters.",
				Validators: []validator.String{
					maxLength(20),
				},
			},
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Container holding the group. Accepts a slash-delimited path relative to the domain root (for example `Groups` or `Contoso/Groups`), a relative distinguished name such as `CN=Users`, or a full distinguished name. Slash segments are always OU names. Changing this moves the group.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Group description.",
			},
			"display_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Display name (`displayName`).",
			},
			"mail": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Email address (`mail`), used for distribution groups and mail-enabled security groups.",
			},
			"info": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-form notes (`info`), shown as **Notes** in Active Directory Users and Computers.",
			},
			"homepage": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Web page address (`wWWHomePage`).",
			},
			"managed_by": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Owner of the group (`managedBy`). Accepts a distinguished name, `objectGUID`, SID, " +
					"`DOMAIN\\name` or `sAMAccountName`. The resolved distinguished name is published as `managed_by_dn`.",
			},
			"protected_from_accidental_deletion": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Protect the group from accidental deletion. This is not a stored attribute: it adds Deny access control entries for `Everyone` on `Delete` and `DeleteTree`.",
			},
			"manager_can_update_membership": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Allow the `managed_by` principal to change the membership list, the **Manager can update membership list** " +
					"checkbox in Active Directory Users and Computers. This is not a stored attribute: it adds an Allow access " +
					"control entry granting `WriteProperty` on the `member` attribute. Requires `managed_by`. Defaults to `false`, " +
					"which removes that entry for the manager; do not also manage it with `adds_access_rule`.",
			},
			"category": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("Security"),
				MarkdownDescription: "Group category: `Security` or `Distribution`.",
				Validators: []validator.String{
					oneOf("Security", "Distribution"),
				},
			},
			"scope": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("Global"),
				MarkdownDescription: "Group scope: `DomainLocal`, `Global` or `Universal`. The scope is reconciled in place; " +
					"Active Directory cannot convert `Global` directly to `DomainLocal` (or the reverse), so the provider " +
					"steps through `Universal` automatically. A conversion can still be rejected by Active Directory's " +
					"nesting rules — see the [group scope conversions guide](../guides/group-scope-conversions.md).",
				Validators: []validator.String{
					oneOf("DomainLocal", "Global", "Universal"),
				},
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the group. Changes when the group is renamed or moved.",
				PlanModifiers: []planmodifier.String{
					useGroupStateUnlessChanged{attributes: []string{"name", "path"}},
				},
			},
			"sid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Security identifier of the group.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"managed_by_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved distinguished name of `managed_by`.",
				PlanModifiers: []planmodifier.String{
					useGroupStateUnlessChanged{attributes: []string{"managed_by"}},
				},
			},
		},
	}
}

func (r *groupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config groupResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The checkbox grants rights to the managedBy principal, so there must be one.
	if config.ManagerCanUpdate.ValueBool() && config.ManagedBy.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("manager_can_update_membership"),
			"Missing managed_by",
			"manager_can_update_membership requires managed_by to be set.",
		)
	}
}

// ModifyPlan pre-checks a group scope change at plan time: it asks the domain controller
// whether the group's current nesting allows the conversion (stepping through Universal when
// required) and fails the plan with a named error if a step is blocked. The apply still
// enforces the rules, so this is an early, clearer failure rather than the only guard.
func (r *groupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Only updates matter: skip create (no prior state) and destroy (no plan).
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	if r.client == nil {
		return
	}

	var state, plan groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Scope.IsUnknown() || state.Scope.IsUnknown() || plan.Scope.ValueString() == state.Scope.ValueString() {
		return
	}

	preflight, err := ad.PreflightGroupScopeConversion(ctx, r.client, state.ID.ValueString(), state.Scope.ValueString(), plan.Scope.ValueString())
	if err != nil {
		// A preflight failure must not block planning on its own; apply still enforces the
		// scope rules, so surface the problem as a warning and continue.
		resp.Diagnostics.AddAttributeWarning(
			path.Root("scope"),
			"Could not pre-check group scope conversion",
			err.Error(),
		)
		return
	}

	if preflight.CanConvert {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("scope"),
		"Group scope conversion blocked by nesting",
		groupScopeConversionError(preflight),
	)
}

// useGroupStateUnlessChanged keeps a computed value from state unless one of its source
// attributes changes, so unrelated updates do not replace resources that reference it.
type useGroupStateUnlessChanged struct {
	attributes []string
}

func (m useGroupStateUnlessChanged) Description(_ context.Context) string {
	return "Use prior state for unknown values unless a source attribute changes."
}

func (m useGroupStateUnlessChanged) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m useGroupStateUnlessChanged) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || !resp.PlanValue.IsUnknown() {
		return
	}

	for _, attribute := range m.attributes {
		var planned, prior types.String
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(attribute), &planned)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(attribute), &prior)...)
		if resp.Diagnostics.HasError() || planned.IsUnknown() || !planned.Equal(prior) {
			return
		}
	}

	resp.PlanValue = req.StateValue
}

func groupScopeConversionError(p *ad.GroupScopePreflight) string {
	var memberships, members []string
	for _, b := range p.Blocking {
		if b.Relation == "memberOf" {
			memberships = append(memberships, fmt.Sprintf("%s (%s)", b.Name, b.Scope))
		} else {
			members = append(members, fmt.Sprintf("%s (%s)", b.Name, b.Scope))
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Active Directory cannot convert this group from %s to %s.", p.FromScope, p.ToScope)
	if (p.FromScope == "Global" && p.ToScope == "DomainLocal") || (p.FromScope == "DomainLocal" && p.ToScope == "Global") {
		sb.WriteString(" The conversion must pass through Universal, and a step is blocked by group nesting:")
	} else {
		sb.WriteString(" The conversion is blocked by group nesting:")
	}
	if len(memberships) > 0 {
		fmt.Fprintf(&sb, "\n  - this group is a member of: %s", strings.Join(memberships, ", "))
	}
	if len(members) > 0 {
		fmt.Fprintf(&sb, "\n  - this group contains the group member(s): %s", strings.Join(members, ", "))
	}
	sb.WriteString("\n\nRemove the offending membership (for example with adds_group_member) or change those groups' " +
		"scope, then apply again. See the group scope conversions guide for the full matrix.")
	return sb.String()
}

func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := ad.EnsureGroup(ctx, r.client, groupInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create group", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, groupState(plan, group))...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := ad.ReadGroup(ctx, r.client, state.ID.ValueString(), state.Path.ValueString(), state.ManagedBy.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read group", err.Error())
		return
	}

	if !group.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, groupState(state, group))...)
}

func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupResourceModel
	var state groupResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := ad.UpdateGroup(ctx, r.client, state.ID.ValueString(), groupInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update group", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, groupState(plan, group))...)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteGroup(ctx, r.client, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete group", err.Error())
	}
}

func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func groupInput(model groupResourceModel) ad.GroupInput {
	samAccountName := model.SamAccountName.ValueString()
	if model.SamAccountName.IsNull() || model.SamAccountName.IsUnknown() {
		samAccountName = model.Name.ValueString()
	}

	return ad.GroupInput{
		Name:                            model.Name.ValueString(),
		SamAccountName:                  samAccountName,
		Path:                            model.Path.ValueString(),
		Description:                     optionalString(model.Description),
		DisplayName:                     optionalString(model.DisplayName),
		Mail:                            optionalString(model.Mail),
		Info:                            optionalString(model.Info),
		Homepage:                        optionalString(model.Homepage),
		ManagedBy:                       model.ManagedBy.ValueString(),
		ManagerCanUpdateMembership:      model.ManagerCanUpdate.ValueBool(),
		ProtectedFromAccidentalDeletion: model.Protected.ValueBool(),
		Category:                        model.Category.ValueString(),
		Scope:                           model.Scope.ValueString(),
	}
}

// groupState keeps the configured path when it resolves to the same container, because
// a slash path and a distinguished name can denote the same place. managed_by is kept
// for the same reason: it may be configured as a name but is stored as a DN.
func groupState(model groupResourceModel, group *ad.Group) groupResourceModel {
	path := types.StringValue(group.Path)
	if group.PathMatch && !model.Path.IsNull() && !model.Path.IsUnknown() {
		path = model.Path
	}

	managedBy := types.StringNull()
	if group.ManagedBy != "" {
		managedBy = types.StringValue(group.ManagedBy)
		if group.ManagedByMatch && !model.ManagedBy.IsNull() && !model.ManagedBy.IsUnknown() {
			managedBy = model.ManagedBy
		}
	}

	managedByDN := types.StringNull()
	if group.ManagedBy != "" {
		managedByDN = types.StringValue(group.ManagedBy)
	}

	return groupResourceModel{
		ID:                types.StringValue(group.GUID),
		Name:              types.StringValue(group.Name),
		SamAccountName:    types.StringValue(group.SamAccountName),
		Path:              path,
		Description:       stringPointerToTerraform(group.Description),
		DisplayName:       stringPointerToTerraform(group.DisplayName),
		Mail:              stringPointerToTerraform(group.Mail),
		Info:              stringPointerToTerraform(group.Info),
		Homepage:          stringPointerToTerraform(group.Homepage),
		ManagedBy:         managedBy,
		ManagedByDN:       managedByDN,
		ManagerCanUpdate:  types.BoolValue(group.ManagerCanUpdateMembership),
		Protected:         types.BoolValue(group.ProtectedFromAccidentalDeletion),
		Category:          types.StringValue(group.Category),
		Scope:             types.StringValue(group.Scope),
		DistinguishedName: types.StringValue(group.DistinguishedName),
		SID:               types.StringValue(group.SID),
	}
}
