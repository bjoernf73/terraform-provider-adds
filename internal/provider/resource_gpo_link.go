package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                   = &gpoLinksResource{}
	_ resource.ResourceWithConfigure      = &gpoLinksResource{}
	_ resource.ResourceWithImportState    = &gpoLinksResource{}
	_ resource.ResourceWithValidateConfig = &gpoLinksResource{}
)

func NewGPOLinksResource() resource.Resource {
	return &gpoLinksResource{}
}

type gpoLinksResource struct {
	client *client.Client
}

type gpoLinkEntryModel struct {
	GPO      types.String `tfsdk:"gpo"`
	Enabled  types.Bool   `tfsdk:"enabled"`
	Enforced types.Bool   `tfsdk:"enforced"`
	GPOGUID  types.String `tfsdk:"gpo_guid"`
	GPOName  types.String `tfsdk:"gpo_name"`
}

var gpoLinkEntryAttrTypes = map[string]attr.Type{
	"gpo":      types.StringType,
	"enabled":  types.BoolType,
	"enforced": types.BoolType,
	"gpo_guid": types.StringType,
	"gpo_name": types.StringType,
}

type gpoLinksResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Target           types.String `tfsdk:"target"`
	BlockInheritance types.Bool   `tfsdk:"block_inheritance"`
	Links            types.List   `tfsdk:"links"`
	TargetDN         types.String `tfsdk:"target_dn"`
}

func (r *gpoLinksResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gpo_links"
}

func (r *gpoLinksResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the full, ordered set of GPO links on a single OU, domain or site, plus whether it " +
			"blocks inheritance from above. This resource is **authoritative** for `target`: any link present in Active " +
			"Directory but not listed in `links` is removed.\n\n" +
			"`links` order is significant: the first entry has the highest precedence (`Set-GPLink -Order 1`), matching how " +
			"GPMC displays link order.\n\n" +
			"When a target's links are contributed by more than one Terraform configuration, use the non-authoritative " +
			"`adds_gpo_link` (singular) instead, which manages one link without touching the others.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier. Equals `target_dn`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "OU, domain or site to manage links on. Accepts a slash-delimited path relative to the " +
					"domain root (for example `Servers` or `Contoso/Servers`), a relative distinguished name such as `CN=Sites,CN=Configuration`, " +
					"or a full distinguished name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"block_inheritance": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Block inheritance of GPOs linked above `target` in the Group Policy hierarchy " +
					"(`Set-GPInheritance -IsBlocked`). Links enforced above `target` still apply regardless of this setting.",
			},
			"links": schema.ListNestedAttribute{
				Required: true,
				MarkdownDescription: "GPO links on `target`, in precedence order (first entry wins conflicts). Any link on " +
					"`target` not listed here is removed.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"gpo": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "GPO to link, by GUID or display name.",
						},
						"enabled": schema.BoolAttribute{
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(true),
							MarkdownDescription: "Whether the link is enabled (`Set-GPLink -LinkEnabled`).",
						},
						"enforced": schema.BoolAttribute{
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(false),
							MarkdownDescription: "Whether the link is enforced, so it cannot be blocked or overridden at a lower level (`Set-GPLink -Enforced`).",
						},
						"gpo_guid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Resolved GUID of `gpo`.",
						},
						"gpo_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Resolved display name of `gpo`.",
						},
					},
				},
			},
			"target_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of `target`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *gpoLinksResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config gpoLinksResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Links.IsUnknown() || config.Links.IsNull() {
		return
	}

	var entries []gpoLinkEntryModel
	resp.Diagnostics.Append(config.Links.ElementsAs(ctx, &entries, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.GPO.IsUnknown() || entry.GPO.IsNull() {
			continue
		}

		key := strings.ToUpper(strings.TrimSpace(entry.GPO.ValueString()))
		if seen[key] {
			resp.Diagnostics.AddAttributeError(
				path.Root("links"),
				"Duplicate GPO link",
				fmt.Sprintf("%q appears more than once in links; each GPO can only be linked to target once.", entry.GPO.ValueString()),
			)
			continue
		}
		seen[key] = true
	}
}

func (r *gpoLinksResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *gpoLinksResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gpoLinksResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *gpoLinksResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gpoLinksResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var priorLinks []gpoLinkEntryModel
	resp.Diagnostics.Append(state.Links.ElementsAs(ctx, &priorLinks, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := ad.ReadGPOLinks(ctx, r.client, state.TargetDN.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GPO links", err.Error())
		return
	}

	if !result.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	links, diags := gpoLinksFromResult(ctx, priorLinks, result)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ID = types.StringValue(result.TargetDN)
	state.TargetDN = types.StringValue(result.TargetDN)
	state.BlockInheritance = types.BoolValue(result.BlockInheritance)
	state.Links = links

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *gpoLinksResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan gpoLinksResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *gpoLinksResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state gpoLinksResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteGPOLinks(ctx, r.client, state.TargetDN.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete GPO links", err.Error())
	}
}

func (r *gpoLinksResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("target_dn"), req, resp)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ensureAndSet resolves the plan's links, applies them (create and update share this: see
// EnsureGPOLinks), and writes the result back, keeping each entry's original `gpo` string.
func (r *gpoLinksResource) ensureAndSet(ctx context.Context, plan gpoLinksResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	var planLinks []gpoLinkEntryModel
	diags.Append(plan.Links.ElementsAs(ctx, &planLinks, false)...)
	if diags.HasError() {
		return
	}

	entries := make([]ad.GPOLinkEntry, 0, len(planLinks))
	for _, l := range planLinks {
		entries = append(entries, ad.GPOLinkEntry{
			GPO:      l.GPO.ValueString(),
			Enabled:  l.Enabled.ValueBool(),
			Enforced: l.Enforced.ValueBool(),
		})
	}

	result, err := ad.EnsureGPOLinks(ctx, r.client, ad.GPOLinksInput{
		Target:           plan.Target.ValueString(),
		BlockInheritance: plan.BlockInheritance.ValueBool(),
		Links:            entries,
	})
	if err != nil {
		diags.AddError("Unable to set GPO links", err.Error())
		return
	}

	links, listDiags := gpoLinksFromEnsure(ctx, planLinks, result)
	diags.Append(listDiags...)
	if diags.HasError() {
		return
	}

	plan.ID = types.StringValue(result.TargetDN)
	plan.TargetDN = types.StringValue(result.TargetDN)
	plan.BlockInheritance = types.BoolValue(result.BlockInheritance)
	plan.Links = links

	diags.Append(state.Set(ctx, &plan)...)
}

// gpoLinksFromEnsure zips the plan's links with the ensure result by position: Ensure
// requests them in that exact order and removes everything else, so the lengths match.
func gpoLinksFromEnsure(ctx context.Context, planLinks []gpoLinkEntryModel, result *ad.GPOLinks) (types.List, diag.Diagnostics) {
	models := make([]gpoLinkEntryModel, 0, len(result.Links))
	for i, link := range result.Links {
		gpo := types.StringValue(link.GPOGUID)
		if i < len(planLinks) {
			gpo = planLinks[i].GPO
		}

		models = append(models, gpoLinkEntryModel{
			GPO:      gpo,
			Enabled:  types.BoolValue(link.Enabled),
			Enforced: types.BoolValue(link.Enforced),
			GPOGUID:  types.StringValue(link.GPOGUID),
			GPOName:  types.StringValue(link.GPOName),
		})
	}

	return types.ListValueFrom(ctx, types.ObjectType{AttrTypes: gpoLinkEntryAttrTypes}, models)
}

// gpoLinksFromResult matches by GUID rather than position, since Read() must reflect
// whatever links actually exist, in whatever order, including drift Ensure never saw.
func gpoLinksFromResult(ctx context.Context, priorLinks []gpoLinkEntryModel, result *ad.GPOLinks) (types.List, diag.Diagnostics) {
	original := make(map[string]types.String, len(priorLinks))
	for _, l := range priorLinks {
		if !l.GPOGUID.IsNull() {
			original[strings.ToUpper(l.GPOGUID.ValueString())] = l.GPO
		}
	}

	models := make([]gpoLinkEntryModel, 0, len(result.Links))
	for _, link := range result.Links {
		gpo := types.StringValue(link.GPOGUID)
		if o, ok := original[strings.ToUpper(link.GPOGUID)]; ok {
			gpo = o
		}

		models = append(models, gpoLinkEntryModel{
			GPO:      gpo,
			Enabled:  types.BoolValue(link.Enabled),
			Enforced: types.BoolValue(link.Enforced),
			GPOGUID:  types.StringValue(link.GPOGUID),
			GPOName:  types.StringValue(link.GPOName),
		})
	}

	return types.ListValueFrom(ctx, types.ObjectType{AttrTypes: gpoLinkEntryAttrTypes}, models)
}
