package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                = &gpoSecurityFilterResource{}
	_ resource.ResourceWithConfigure   = &gpoSecurityFilterResource{}
	_ resource.ResourceWithImportState = &gpoSecurityFilterResource{}
)

func NewGPOSecurityFilterResource() resource.Resource { return &gpoSecurityFilterResource{} }

type gpoSecurityFilterResource struct{ client *client.Client }

type gpoSecurityFilterResourceModel struct {
	ID            types.String `tfsdk:"id"`
	GPO           types.String `tfsdk:"gpo"`
	Principals    types.Set    `tfsdk:"principals"`
	GPODN         types.String `tfsdk:"gpo_dn"`
	PrincipalSIDs types.Set    `tfsdk:"principal_sids"`
}

func (r *gpoSecurityFilterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gpo_security_filter"
}

func (r *gpoSecurityFilterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computed := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Authoritatively manages the principals allowed to apply one GPO. It removes `GpoApply` from every " +
			"other explicit trustee, while keeping `Authenticated Users` at `GpoRead` so the policy remains readable. It does not modify edit or owner permissions.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Resolved GUID of `gpo`.", PlanModifiers: computed},
			"gpo":            schema.StringAttribute{Required: true, MarkdownDescription: "GPO by GUID or display name.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"principals":     schema.SetAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Complete set of users, groups or computers allowed to apply the GPO. Each receives `GpoApply` (read plus apply)."},
			"gpo_dn":         schema.StringAttribute{Computed: true, MarkdownDescription: "Distinguished name of the GPO container.", PlanModifiers: computed},
			"principal_sids": schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Resolved SIDs of the current `GpoApply` trustees."},
		},
	}
}

func (r *gpoSecurityFilterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
	}
}

func (r *gpoSecurityFilterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gpoSecurityFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if !resp.Diagnostics.HasError() {
		r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
	}
}

func (r *gpoSecurityFilterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gpoSecurityFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := ad.ReadGPOSecurityFilter(ctx, r.client, gpoSecurityFilterInput(ctx, state, &resp.Diagnostics))
	if resp.Diagnostics.HasError() || err != nil {
		if err != nil {
			resp.Diagnostics.AddError("Unable to read GPO security filter", err.Error())
		}
		return
	}
	if !result.Exists {
		resp.State.RemoveResource(ctx)
		return
	}
	if !result.Matches {
		state.Principals, _ = types.SetValueFrom(ctx, types.StringType, result.PrincipalSIDs)
	}
	state.ID, state.GPODN = types.StringValue(result.GPOGUID), types.StringValue(result.GPODN)
	state.PrincipalSIDs, _ = types.SetValueFrom(ctx, types.StringType, result.PrincipalSIDs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *gpoSecurityFilterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan gpoSecurityFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if !resp.Diagnostics.HasError() {
		r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
	}
}

func (r *gpoSecurityFilterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state gpoSecurityFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	input := gpoSecurityFilterInput(ctx, state, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		if err := ad.DeleteGPOSecurityFilter(ctx, r.client, input); err != nil {
			resp.Diagnostics.AddError("Unable to remove GPO security filter", err.Error())
		}
	}
}

func (r *gpoSecurityFilterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("gpo"), req, resp)
}

func (r *gpoSecurityFilterResource) ensureAndSet(ctx context.Context, plan gpoSecurityFilterResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	input := gpoSecurityFilterInput(ctx, plan, diags)
	if diags.HasError() {
		return
	}
	result, err := ad.EnsureGPOSecurityFilter(ctx, r.client, input)
	if err != nil {
		diags.AddError("Unable to set GPO security filter", err.Error())
		return
	}
	plan.ID, plan.GPODN = types.StringValue(result.GPOGUID), types.StringValue(result.GPODN)
	plan.PrincipalSIDs, _ = types.SetValueFrom(ctx, types.StringType, result.PrincipalSIDs)
	diags.Append(state.Set(ctx, &plan)...)
}

func gpoSecurityFilterInput(ctx context.Context, model gpoSecurityFilterResourceModel, diags *diag.Diagnostics) ad.GPOSecurityFilterInput {
	var principals []string
	diags.Append(model.Principals.ElementsAs(ctx, &principals, false)...)
	return ad.GPOSecurityFilterInput{GPO: model.GPO.ValueString(), Principals: principals}
}
