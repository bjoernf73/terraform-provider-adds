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
	_ resource.Resource                = &gpoWMIFilterResource{}
	_ resource.ResourceWithConfigure   = &gpoWMIFilterResource{}
	_ resource.ResourceWithImportState = &gpoWMIFilterResource{}
)

func NewGPOWMIFilterResource() resource.Resource {
	return &gpoWMIFilterResource{}
}

type gpoWMIFilterResource struct {
	client *client.Client
}

type gpoWMIFilterResourceModel struct {
	ID            types.String `tfsdk:"id"`
	GPO           types.String `tfsdk:"gpo"`
	WMIFilter     types.String `tfsdk:"wmi_filter"`
	GPODN         types.String `tfsdk:"gpo_dn"`
	WMIFilterGUID types.String `tfsdk:"wmi_filter_guid"`
}

func (r *gpoWMIFilterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gpo_wmi_filter"
}

func (r *gpoWMIFilterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a WMI filter (`adds_wmi_filter`) to a GPO (`gPCWQLFilter`). A GPO can only have " +
			"one WMI filter at a time, so this resource is authoritative for the whole attribute.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved GUID of `gpo`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"gpo": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "GPO to assign the filter to, by GUID or display name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"wmi_filter": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "WMI filter to assign, by GUID or filter name. Changing this re-assigns the filter.",
			},
			"gpo_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of `gpo`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"wmi_filter_guid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved GUID of `wmi_filter`.",
			},
		},
	}
}

func (r *gpoWMIFilterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *gpoWMIFilterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gpoWMIFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *gpoWMIFilterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gpoWMIFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := ad.ReadGPOWMIFilter(ctx, r.client, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GPO WMI filter assignment", err.Error())
		return
	}

	if !result.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(result.GPOGUID)
	state.GPODN = types.StringValue(result.GPODN)
	state.WMIFilterGUID = types.StringValue(result.WMIFilterGUID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *gpoWMIFilterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan gpoWMIFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *gpoWMIFilterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state gpoWMIFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteGPOWMIFilter(ctx, r.client, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to remove GPO WMI filter assignment", err.Error())
	}
}

func (r *gpoWMIFilterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *gpoWMIFilterResource) ensureAndSet(ctx context.Context, plan gpoWMIFilterResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	result, err := ad.EnsureGPOWMIFilter(ctx, r.client, ad.GPOWMIFilterInput{
		GPO:       plan.GPO.ValueString(),
		WMIFilter: plan.WMIFilter.ValueString(),
	})
	if err != nil {
		diags.AddError("Unable to assign WMI filter to GPO", err.Error())
		return
	}

	plan.ID = types.StringValue(result.GPOGUID)
	plan.GPODN = types.StringValue(result.GPODN)
	plan.WMIFilterGUID = types.StringValue(result.WMIFilterGUID)

	diags.Append(state.Set(ctx, &plan)...)
}
