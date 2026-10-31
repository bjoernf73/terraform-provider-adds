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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                   = &gpoLinkResource{}
	_ resource.ResourceWithConfigure      = &gpoLinkResource{}
	_ resource.ResourceWithImportState    = &gpoLinkResource{}
	_ resource.ResourceWithValidateConfig = &gpoLinkResource{}
)

func NewGPOLinkResource() resource.Resource {
	return &gpoLinkResource{}
}

type gpoLinkResource struct {
	client *client.Client
}

type gpoLinkResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Target       types.String `tfsdk:"target"`
	GPO          types.String `tfsdk:"gpo"`
	Enabled      types.Bool   `tfsdk:"enabled"`
	Enforced     types.Bool   `tfsdk:"enforced"`
	Order        types.Int64  `tfsdk:"order"`
	MonitorOrder types.Bool   `tfsdk:"monitor_order"`
	GPOGUID      types.String `tfsdk:"gpo_guid"`
	GPOName      types.String `tfsdk:"gpo_name"`
	TargetDN     types.String `tfsdk:"target_dn"`
	CurrentOrder types.Int64  `tfsdk:"current_order"`
}

func (r *gpoLinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gpo_link"
}

func (r *gpoLinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a **single** GPO link on an OU, domain or site, leaving every other link in place.\n\n" +
			"This resource is **non-authoritative**: unlike `adds_gpo_links` (which owns the whole ordered list), it only " +
			"inserts, updates or removes the one link it declares, so several independent Terraform configurations can each " +
			"link a GPO to the same target. Use it, not `adds_gpo_links`, on a target whose links are contributed by more " +
			"than one state — and do not point both at the same target, or the authoritative one will prune this link.\n\n" +
			"Precedence is 1-based, matching GPMC link order (`order = 1` is highest). By default order is **not** monitored: " +
			"`order` only chooses where a new link is inserted, after which links added by others never cause drift. Set " +
			"`monitor_order = true` to pin this link to `order` and reconcile it back if it moves.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier, `<target DN>|<GPO GUID>`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "OU, domain or site to link the GPO on. Accepts a slash-delimited path relative to the " +
					"domain root (for example `Servers` or `Contoso/Servers`), a relative distinguished name such as " +
					"`CN=Sites,CN=Configuration`, or a full distinguished name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"gpo": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "GPO to link, by GUID or display name. A GPO can be linked to a target at most once.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the link is enabled.",
			},
			"enforced": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether the link is enforced, so it cannot be blocked or overridden at a lower level.",
			},
			"order": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "Desired 1-based precedence slot (`1` = highest, applied first). Omit to append the link at " +
					"the bottom (lowest precedence). With `monitor_order = false` (the default) this only positions the link when " +
					"it is first created; afterwards the link keeps its slot and is never moved. With `monitor_order = true` the " +
					"link is kept at this slot and moved back if it drifts. Required when `monitor_order = true`.",
			},
			"monitor_order": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Whether to keep the link pinned at `order`. When `false` (default), position is set once on " +
					"create and then left alone, so links other configurations add never cause drift. When `true`, the link is " +
					"reconciled back to `order` on every apply — which can produce a permanent diff if another manager keeps moving it.",
			},
			"gpo_guid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved GUID of `gpo`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"gpo_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved display name of `gpo`.",
			},
			"target_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved distinguished name of `target`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"current_order": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The link's actual 1-based precedence position as read from Active Directory, which reflects links contributed by other configurations.",
			},
		},
	}
}

func (r *gpoLinkResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config gpoLinkResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Pinning a position needs a position to pin to.
	if config.MonitorOrder.ValueBool() && config.Order.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("order"),
			"Missing order",
			"order is required when monitor_order = true.",
		)
	}
}

func (r *gpoLinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *gpoLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gpoLinkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := ad.EnsureGPOLink(ctx, r.client, gpoLinkInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create GPO link", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, applyGPOLinkResult(plan, result))...)
}

func (r *gpoLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gpoLinkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := ad.ReadGPOLink(ctx, r.client, state.TargetDN.ValueString(), state.GPOGUID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GPO link", err.Error())
		return
	}

	if !result.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	state.TargetDN = types.StringValue(result.TargetDN)
	state.GPOGUID = types.StringValue(result.GPOGUID)
	state.GPOName = types.StringValue(result.GPOName)
	state.Enabled = types.BoolValue(result.Enabled)
	state.Enforced = types.BoolValue(result.Enforced)
	state.CurrentOrder = types.Int64Value(int64(result.CurrentOrder))
	state.ID = types.StringValue(gpoLinkID(result.TargetDN, result.GPOGUID))

	// When monitoring order, surface a drifted position by writing the actual order into `order`,
	// so Terraform plans to move it back. When not monitoring, leave `order` (the configured
	// insertion hint) untouched so links other configurations add never cause drift.
	if state.MonitorOrder.ValueBool() && !state.Order.IsNull() && int64(result.CurrentOrder) != state.Order.ValueInt64() {
		state.Order = types.Int64Value(int64(result.CurrentOrder))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *gpoLinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan gpoLinkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := ad.EnsureGPOLink(ctx, r.client, gpoLinkInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update GPO link", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, applyGPOLinkResult(plan, result))...)
}

func (r *gpoLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state gpoLinkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteGPOLink(ctx, r.client, state.TargetDN.ValueString(), state.GPOGUID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete GPO link", err.Error())
	}
}

func (r *gpoLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	target, gpoGUID, ok := strings.Cut(req.ID, "|")
	if !ok || target == "" || gpoGUID == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected '<target DN>|<GPO GUID>', for example "+
				"'OU=Servers,OU=Contoso,DC=contoso,DC=local|11111111-2222-3333-4444-555555555555'.",
		)
		return
	}

	gpoGUID = strings.ToLower(strings.Trim(gpoGUID, "{}"))

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), gpoLinkID(target, gpoGUID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target"), target)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target_dn"), target)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("gpo"), gpoGUID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("gpo_guid"), gpoGUID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("monitor_order"), false)...)
}

func gpoLinkInput(model gpoLinkResourceModel) ad.GPOLinkInput {
	var order *int64
	if !model.Order.IsNull() && !model.Order.IsUnknown() {
		value := model.Order.ValueInt64()
		order = &value
	}

	return ad.GPOLinkInput{
		Target:       model.Target.ValueString(),
		GPO:          model.GPO.ValueString(),
		Enabled:      model.Enabled.ValueBool(),
		Enforced:     model.Enforced.ValueBool(),
		Order:        order,
		MonitorOrder: model.MonitorOrder.ValueBool(),
	}
}

func applyGPOLinkResult(model gpoLinkResourceModel, result *ad.GPOLink) gpoLinkResourceModel {
	model.ID = types.StringValue(gpoLinkID(result.TargetDN, result.GPOGUID))
	model.TargetDN = types.StringValue(result.TargetDN)
	model.GPOGUID = types.StringValue(result.GPOGUID)
	model.GPOName = types.StringValue(result.GPOName)
	model.Enabled = types.BoolValue(result.Enabled)
	model.Enforced = types.BoolValue(result.Enforced)
	model.CurrentOrder = types.Int64Value(int64(result.CurrentOrder))

	return model
}

func gpoLinkID(targetDN, gpoGUID string) string {
	return targetDN + "|" + gpoGUID
}
