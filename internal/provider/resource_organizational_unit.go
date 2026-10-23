package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                = &organizationalUnitResource{}
	_ resource.ResourceWithConfigure   = &organizationalUnitResource{}
	_ resource.ResourceWithImportState = &organizationalUnitResource{}
)

func NewOrganizationalUnitResource() resource.Resource {
	return &organizationalUnitResource{}
}

type organizationalUnitResource struct {
	client *client.Client
}

type organizationalUnitResourceModel struct {
	ID                         types.String `tfsdk:"id"`
	Path                       types.String `tfsdk:"path"`
	Description                types.String `tfsdk:"description"`
	DeleteSubtree              types.Bool   `tfsdk:"delete_subtree"`
	DistinguishedName          types.String `tfsdk:"distinguished_name"`
	Name                       types.String `tfsdk:"name"`
	CreatedOrganizationalUnits types.List   `tfsdk:"created_organizational_units"`
}

func (r *organizationalUnitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organizational_unit"
}

func (r *organizationalUnitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Active Directory organizational unit by executing PowerShell on a remote Windows host.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier. Equals the organizational unit distinguished name.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownUnlessPathChanges{},
				},
			},
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "OU path relative to the domain root. Use slash-delimited segments such as `Servers/Windows`, a relative DN such as `OU=Servers`, or a full DN. Slash segments are always OU names, even when one matches the domain name. Changing the path moves and/or renames the OU in place, so every child object—managed or not—moves with it and the OU keeps its GUID, GPO links and ACLs.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OU description.",
			},
			"delete_subtree": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Recursively delete child objects when destroying this resource.",
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the organizational unit.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownUnlessPathChanges{},
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Leaf organizational unit name.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownUnlessPathChanges{},
				},
			},
			"created_organizational_units": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Distinguished names of ancestor OUs this resource created because they did not already exist. They are removed on destroy, deepest first, but only while empty. Pre-existing OUs in the path are never recorded here and are left untouched on destroy.",
				PlanModifiers: []planmodifier.List{
					listUseStateForUnknownUnlessPathChanges{},
				},
			},
		},
	}
}

func (r *organizationalUnitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *organizationalUnitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan organizationalUnitResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ou, err := ad.EnsureOrganizationalUnit(ctx, r.client, plan.Path.ValueString(), optionalString(plan.Description))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create organizational unit", err.Error())
		return
	}

	createdOUs, diags := types.ListValueFrom(ctx, types.StringType, orEmptyStrings(ou.CreatedOrganizationalUnits))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := organizationalUnitResourceModel{
		ID:                         types.StringValue(ou.DistinguishedName),
		Path:                       types.StringValue(ou.Path),
		DeleteSubtree:              plan.DeleteSubtree,
		DistinguishedName:          types.StringValue(ou.DistinguishedName),
		Name:                       types.StringValue(ou.Name),
		Description:                stringPointerToTerraform(ou.Description),
		CreatedOrganizationalUnits: createdOUs,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *organizationalUnitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationalUnitResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	distinguishedName := state.DistinguishedName.ValueString()
	if distinguishedName == "" {
		distinguishedName = state.ID.ValueString()
	}

	ou, err := ad.ReadOrganizationalUnit(ctx, r.client, distinguishedName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read organizational unit", err.Error())
		return
	}

	if !ou.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(ou.DistinguishedName)
	state.Path = types.StringValue(ou.Path)
	state.DistinguishedName = types.StringValue(ou.DistinguishedName)
	state.Name = types.StringValue(ou.Name)
	state.Description = stringPointerToTerraform(ou.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *organizationalUnitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan organizationalUnitResourceModel
	var state organizationalUnitResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var createdOUs []string
	if !state.CreatedOrganizationalUnits.IsNull() && !state.CreatedOrganizationalUnits.IsUnknown() {
		resp.Diagnostics.Append(state.CreatedOrganizationalUnits.ElementsAs(ctx, &createdOUs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	ou, err := ad.UpdateOrganizationalUnit(ctx, r.client, state.DistinguishedName.ValueString(), plan.Path.ValueString(), optionalString(plan.Description), createdOUs)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update organizational unit", err.Error())
		return
	}

	createdOUList, diags := types.ListValueFrom(ctx, types.StringType, orEmptyStrings(ou.CreatedOrganizationalUnits))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	newState := organizationalUnitResourceModel{
		ID:                         types.StringValue(ou.DistinguishedName),
		Path:                       types.StringValue(ou.Path),
		DeleteSubtree:              plan.DeleteSubtree,
		DistinguishedName:          types.StringValue(ou.DistinguishedName),
		Name:                       types.StringValue(ou.Name),
		Description:                stringPointerToTerraform(ou.Description),
		CreatedOrganizationalUnits: createdOUList,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *organizationalUnitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationalUnitResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteSubtree := false
	if !state.DeleteSubtree.IsNull() && !state.DeleteSubtree.IsUnknown() {
		deleteSubtree = state.DeleteSubtree.ValueBool()
	}

	var createdOUs []string
	if !state.CreatedOrganizationalUnits.IsNull() && !state.CreatedOrganizationalUnits.IsUnknown() {
		resp.Diagnostics.Append(state.CreatedOrganizationalUnits.ElementsAs(ctx, &createdOUs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if err := ad.DeleteOrganizationalUnit(ctx, r.client, state.DistinguishedName.ValueString(), deleteSubtree, createdOUs); err != nil {
		resp.Diagnostics.AddError("Unable to delete organizational unit", err.Error())
	}
}

func (r *organizationalUnitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func optionalString(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	result := value.ValueString()
	return &result
}

func stringPointerToTerraform(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func orEmptyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// ouPathChanges reports whether the planned OU path differs from the prior state path. It is
// conservative: any ambiguity (unknown/null values) counts as a change so dependent computed
// attributes fall back to "known after apply" rather than a stale value.
func ouPathChanges(ctx context.Context, plan tfsdk.Plan, state tfsdk.State) bool {
	var planPath types.String
	var statePath types.String
	plan.GetAttribute(ctx, path.Root("path"), &planPath)
	state.GetAttribute(ctx, path.Root("path"), &statePath)

	if planPath.IsNull() || planPath.IsUnknown() || statePath.IsNull() || statePath.IsUnknown() {
		return true
	}

	return ad.NormalizePath(planPath.ValueString()) != ad.NormalizePath(statePath.ValueString())
}

// useStateForUnknownUnlessPathChanges copies the prior state value into the plan (like
// UseStateForUnknown) but only while the path is unchanged. When the path changes the OU is
// moved/renamed, so the value must resolve to "known after apply" to avoid an inconsistent
// result after apply.
type useStateForUnknownUnlessPathChanges struct{}

func (m useStateForUnknownUnlessPathChanges) Description(_ context.Context) string {
	return "Use prior state for unknown values unless the path attribute changes."
}

func (m useStateForUnknownUnlessPathChanges) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m useStateForUnknownUnlessPathChanges) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || !resp.PlanValue.IsUnknown() {
		return
	}
	if ouPathChanges(ctx, req.Plan, req.State) {
		return
	}
	resp.PlanValue = req.StateValue
}

// listUseStateForUnknownUnlessPathChanges is the list-typed counterpart used for
// created_organizational_units.
type listUseStateForUnknownUnlessPathChanges struct{}

func (m listUseStateForUnknownUnlessPathChanges) Description(_ context.Context) string {
	return "Use prior state for unknown values unless the path attribute changes."
}

func (m listUseStateForUnknownUnlessPathChanges) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m listUseStateForUnknownUnlessPathChanges) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.StateValue.IsNull() || !resp.PlanValue.IsUnknown() {
		return
	}
	if ouPathChanges(ctx, req.Plan, req.State) {
		return
	}
	resp.PlanValue = req.StateValue
}
