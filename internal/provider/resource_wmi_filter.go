package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                = &wmiFilterResource{}
	_ resource.ResourceWithConfigure   = &wmiFilterResource{}
	_ resource.ResourceWithImportState = &wmiFilterResource{}
)

func NewWMIFilterResource() resource.Resource {
	return &wmiFilterResource{}
}

type wmiFilterResource struct {
	client *client.Client
}

type wmiFilterQueryModel struct {
	Namespace types.String `tfsdk:"namespace"`
	Query     types.String `tfsdk:"query"`
}

var wmiFilterQueryAttrTypes = map[string]attr.Type{
	"namespace": types.StringType,
	"query":     types.StringType,
}

type wmiFilterResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	Queries           types.List   `tfsdk:"queries"`
	DistinguishedName types.String `tfsdk:"distinguished_name"`
}

func (r *wmiFilterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wmi_filter"
}

func (r *wmiFilterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a WMI filter (an `msWMI-Som` object). WMI filters are assigned to GPOs with " +
			"[`adds_gpo_wmi_filter`](../resources/gpo_wmi_filter.md).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "GUID of the WMI filter.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name of the WMI filter. Changing this replaces the filter.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Description of the WMI filter.",
			},
			"queries": schema.ListNestedAttribute{
				Required: true,
				MarkdownDescription: "WQL queries the filter evaluates. A target matches the filter only if every query " +
					"matches (GPMC ANDs them together).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"namespace": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString(`root\CIMv2`),
							MarkdownDescription: "WMI namespace the query runs against. Defaults to `root\\CIMv2`.",
						},
						"query": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "WQL query text, for example `SELECT * FROM Win32_OperatingSystem WHERE Version LIKE \"10.%\"`.",
						},
					},
				},
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the WMI filter object.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *wmiFilterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *wmiFilterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wmiFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *wmiFilterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wmiFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := ad.ReadWMIFilter(ctx, r.client, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read WMI filter", err.Error())
		return
	}

	if !result.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	newState, diags := wmiFilterModelFromResult(ctx, result)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *wmiFilterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan wmiFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *wmiFilterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wmiFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteWMIFilter(ctx, r.client, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete WMI filter", err.Error())
	}
}

func (r *wmiFilterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *wmiFilterResource) ensureAndSet(ctx context.Context, plan wmiFilterResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	var planQueries []wmiFilterQueryModel
	diags.Append(plan.Queries.ElementsAs(ctx, &planQueries, false)...)
	if diags.HasError() {
		return
	}

	queries := make([]ad.WMIFilterQuery, 0, len(planQueries))
	for _, q := range planQueries {
		queries = append(queries, ad.WMIFilterQuery{
			Namespace: q.Namespace.ValueString(),
			Query:     q.Query.ValueString(),
		})
	}

	result, err := ad.EnsureWMIFilter(ctx, r.client, ad.WMIFilterInput{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Queries:     queries,
	})
	if err != nil {
		diags.AddError("Unable to set WMI filter", err.Error())
		return
	}

	newState, listDiags := wmiFilterModelFromResult(ctx, result)
	diags.Append(listDiags...)
	if diags.HasError() {
		return
	}

	diags.Append(state.Set(ctx, &newState)...)
}

func wmiFilterModelFromResult(ctx context.Context, result *ad.WMIFilter) (wmiFilterResourceModel, diag.Diagnostics) {
	models := make([]wmiFilterQueryModel, 0, len(result.Queries))
	for _, q := range result.Queries {
		models = append(models, wmiFilterQueryModel{
			Namespace: types.StringValue(q.Namespace),
			Query:     types.StringValue(q.Query),
		})
	}

	queries, diags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: wmiFilterQueryAttrTypes}, models)
	if diags.HasError() {
		return wmiFilterResourceModel{}, diags
	}

	return wmiFilterResourceModel{
		ID:                types.StringValue(result.GUID),
		Name:              types.StringValue(result.Name),
		Description:       types.StringValue(result.Description),
		Queries:           queries,
		DistinguishedName: types.StringValue(result.DistinguishedName),
	}, diags
}
