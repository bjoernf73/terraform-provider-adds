package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                = &kdsRootKeyResource{}
	_ resource.ResourceWithConfigure   = &kdsRootKeyResource{}
	_ resource.ResourceWithImportState = &kdsRootKeyResource{}
)

func NewKDSRootKeyResource() resource.Resource {
	return &kdsRootKeyResource{}
}

type kdsRootKeyResource struct {
	client *client.Client
}

type kdsRootKeyResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	EffectiveImmediately types.Bool   `tfsdk:"effective_immediately"`
	ForceReplication     types.Bool   `tfsdk:"force_replication"`
	KeyID                types.String `tfsdk:"key_id"`
	EffectiveTime        types.String `tfsdk:"effective_time"`
	CreationTime         types.String `tfsdk:"creation_time"`
	Created              types.Bool   `tfsdk:"created"`
}

func (r *kdsRootKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kds_root_key"
}

func (r *kdsRootKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computedString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Ensures a Key Distribution Service (KDS) root key exists in the forest, creating one only when none is present. " +
			"A KDS root key is the forest-wide prerequisite for group managed service accounts (`adds_gmsa`).\n\n" +
			"Removing this resource does **not** delete the key: Active Directory offers no supported way to remove a KDS root key, " +
			"and doing so would break every existing gMSA. Destroy only drops it from Terraform state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier. Equals `key_id`.",
				PlanModifiers:       computedString,
			},
			"effective_immediately": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "When this resource creates the key, backdate its effective time by 10 hours so gMSAs can be used immediately, " +
					"bypassing the replication safety window. This is sound only when every domain controller that will serve the gMSA " +
					"already holds the key — which in a single-DC forest is immediate, but in a multi-DC forest risks authentication " +
					"failures until replication converges. Ignored when an existing key is adopted; it only affects creation. Defaults to `false`.",
			},
			"force_replication": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "When this resource creates the key, force every other domain controller in the forest to replicate it immediately, " +
					"instead of waiting for normal replication. The key lives in the forest-wide Configuration partition, so this uses " +
					"`Sync-ADObject` to push it from the creating DC to every DC in every domain. Pair with `effective_immediately` to make " +
					"a new key usable across the forest at once. Requires the creating account to hold replication rights and network reachability " +
					"to every DC; the apply fails if any DC cannot be reached. Ignored when an existing key is adopted. Defaults to `false`.",
			},
			"key_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "GUID (`KeyId`) of the root key this resource created or adopted.",
				PlanModifiers:       computedString,
			},
			"effective_time": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "UTC timestamp at which the key becomes usable. A key is not usable until this time has passed on the serving domain controller.",
			},
			"creation_time": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "UTC timestamp at which the key was created.",
			},
			"created": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this resource created the key (`true`) or adopted an already-present one (`false`).",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *kdsRootKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *kdsRootKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan kdsRootKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := ad.EnsureKDSRootKey(ctx, r.client, plan.EffectiveImmediately.ValueBool(), plan.ForceReplication.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to ensure KDS root key", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, kdsRootKeyState(plan, key))...)
}

func (r *kdsRootKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state kdsRootKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	keyID := state.KeyID.ValueString()
	if keyID == "" {
		keyID = state.ID.ValueString()
	}

	key, err := ad.ReadKDSRootKey(ctx, r.client, keyID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read KDS root key", err.Error())
		return
	}

	if !key.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	// created is not knowable on read; keep the value recorded at creation.
	key.Created = state.Created.ValueBool()
	resp.Diagnostics.Append(resp.State.Set(ctx, kdsRootKeyState(state, key))...)
}

func (r *kdsRootKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan kdsRootKeyResourceModel
	var state kdsRootKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// effective_immediately only affects creation, so an in-place change touches nothing in AD.
	// Re-read the adopted key to keep the computed attributes fresh.
	keyID := state.KeyID.ValueString()
	key, err := ad.ReadKDSRootKey(ctx, r.client, keyID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read KDS root key", err.Error())
		return
	}

	if !key.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	key.Created = state.Created.ValueBool()
	resp.Diagnostics.Append(resp.State.Set(ctx, kdsRootKeyState(plan, key))...)
}

// Delete removes the resource from Terraform state only. Active Directory has no supported
// way to remove a KDS root key, and removing one breaks every existing gMSA, so nothing is
// done on the directory.
func (r *kdsRootKeyResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *kdsRootKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func kdsRootKeyState(model kdsRootKeyResourceModel, key *ad.KDSRootKey) kdsRootKeyResourceModel {
	return kdsRootKeyResourceModel{
		ID:                   types.StringValue(key.KeyID),
		EffectiveImmediately: model.EffectiveImmediately,
		ForceReplication:     model.ForceReplication,
		KeyID:                types.StringValue(key.KeyID),
		EffectiveTime:        types.StringValue(key.EffectiveTime),
		CreationTime:         types.StringValue(key.CreationTime),
		Created:              types.BoolValue(key.Created),
	}
}
