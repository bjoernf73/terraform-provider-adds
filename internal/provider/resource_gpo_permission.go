package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                = &gpoPermissionResource{}
	_ resource.ResourceWithConfigure   = &gpoPermissionResource{}
	_ resource.ResourceWithImportState = &gpoPermissionResource{}
)

func NewGPOPermissionResource() resource.Resource { return &gpoPermissionResource{} }

type gpoPermissionResource struct{ client *client.Client }

type gpoPermissionResourceModel struct {
	ID         types.String `tfsdk:"id"`
	GPO        types.String `tfsdk:"gpo"`
	Trustee    types.String `tfsdk:"trustee"`
	Permission types.String `tfsdk:"permission"`
	GPODN      types.String `tfsdk:"gpo_dn"`
	TrusteeSID types.String `tfsdk:"trustee_sid"`
}

func (r *gpoPermissionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gpo_permission"
}

func (r *gpoPermissionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computed := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one trustee's named Group Policy permission, using `Set-GPPermission`. This resource " +
			"updates both the GPO container and its SYSVOL policy folder.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier formed as `<gpo GUID>|<trustee SID>`.", PlanModifiers: computed},
			"gpo":     schema.StringAttribute{Required: true, MarkdownDescription: "GPO by GUID or display name.", PlanModifiers: replace},
			"trustee": schema.StringAttribute{Required: true, MarkdownDescription: "User, group, computer, SID, or `Authenticated Users` receiving the permission.", PlanModifiers: replace},
			"permission": schema.StringAttribute{
				Required: true, MarkdownDescription: "Named Group Policy permission level.",
				Validators: []validator.String{oneOf("GpoRead", "GpoApply", "GpoEdit", "GpoEditDeleteModifySecurity")},
			},
			"gpo_dn":      schema.StringAttribute{Computed: true, MarkdownDescription: "Distinguished name of the GPO container.", PlanModifiers: computed},
			"trustee_sid": schema.StringAttribute{Computed: true, MarkdownDescription: "Resolved SID of `trustee`.", PlanModifiers: computed},
		},
	}
}

func (r *gpoPermissionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
	}
}

func (r *gpoPermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gpoPermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if !resp.Diagnostics.HasError() {
		r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
	}
}

func (r *gpoPermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gpoPermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := ad.ReadGPOPermission(ctx, r.client, gpoPermissionInput(state))
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GPO permission", err.Error())
		return
	}
	if !result.Exists {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, gpoPermissionState(state, result))...)
}

func (r *gpoPermissionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan gpoPermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if !resp.Diagnostics.HasError() {
		r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
	}
}

func (r *gpoPermissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state gpoPermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := ad.DeleteGPOPermission(ctx, r.client, gpoPermissionInput(state)); err != nil {
		resp.Diagnostics.AddError("Unable to remove GPO permission", err.Error())
	}
}

func (r *gpoPermissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	gpo, trustee, found := strings.Cut(req.ID, "|")
	if !found {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected `<gpo>|<trustee>`, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("gpo"), gpo)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("trustee"), trustee)...)
}

func (r *gpoPermissionResource) ensureAndSet(ctx context.Context, plan gpoPermissionResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	result, err := ad.EnsureGPOPermission(ctx, r.client, gpoPermissionInput(plan))
	if err != nil {
		diags.AddError("Unable to set GPO permission", err.Error())
		return
	}

	diags.Append(state.Set(ctx, gpoPermissionState(plan, result))...)
}

func gpoPermissionInput(model gpoPermissionResourceModel) ad.GPOPermissionInput {
	return ad.GPOPermissionInput{GPO: model.GPO.ValueString(), Trustee: model.Trustee.ValueString(), Permission: model.Permission.ValueString()}
}

func gpoPermissionState(model gpoPermissionResourceModel, result *ad.GPOPermission) gpoPermissionResourceModel {
	model.ID = types.StringValue(result.GPOGUID + "|" + result.TrusteeSID)
	model.Permission = types.StringValue(result.Permission)
	model.GPODN = types.StringValue(result.GPODN)
	model.TrusteeSID = types.StringValue(result.TrusteeSID)
	return model
}
