package provider

import (
	"context"
	"fmt"
	"net/netip"

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
	_ resource.Resource                   = &subnetResource{}
	_ resource.ResourceWithConfigure      = &subnetResource{}
	_ resource.ResourceWithImportState    = &subnetResource{}
	_ resource.ResourceWithValidateConfig = &subnetResource{}
)

func NewSubnetResource() resource.Resource { return &subnetResource{} }

type subnetResource struct{ client *client.Client }

type subnetResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Site              types.String `tfsdk:"site"`
	Description       types.String `tfsdk:"description"`
	Location          types.String `tfsdk:"location"`
	DistinguishedName types.String `tfsdk:"distinguished_name"`
}

func (r *subnetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subnet"
}

func (r *subnetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computed := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a CIDR network assigned to an Active Directory replication site. Clients use their IP address and these subnet assignments to discover their local domain controllers.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Terraform identifier. Equals the subnet distinguished name.", PlanModifiers: computed},
			"name":               schema.StringAttribute{Required: true, MarkdownDescription: "Subnet in CIDR notation, for example `10.42.0.0/16`. Changing it replaces the subnet.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site":               schema.StringAttribute{Required: true, MarkdownDescription: "Site assigned to this subnet, by name or distinguished name. Changing it updates the assignment in place."},
			"description":        schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), MarkdownDescription: "Subnet description."},
			"location":           schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), MarkdownDescription: "Physical location of the subnet."},
			"distinguished_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Distinguished name of the replication subnet.", PlanModifiers: computed},
		},
	}
}

func (r *subnetResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config subnetResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Name.IsNull() || config.Name.IsUnknown() {
		return
	}
	prefix, err := netip.ParsePrefix(config.Name.ValueString())
	if err != nil || prefix != prefix.Masked() {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid subnet", "name must be a network CIDR with host bits clear, for example `10.42.0.0/16`.")
	}
}

func (r *subnetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *subnetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan subnetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *subnetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state subnetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := ad.ReadSubnet(ctx, r.client, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read AD subnet", err.Error())
		return
	}
	if !result.Exists {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, subnetState(result))...)
}

func (r *subnetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan subnetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.ensureAndSet(ctx, plan, &resp.State, &resp.Diagnostics)
}

func (r *subnetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state subnetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := ad.DeleteSubnet(ctx, r.client, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete AD subnet", err.Error())
	}
}

func (r *subnetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *subnetResource) ensureAndSet(ctx context.Context, plan subnetResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	subnet, err := ad.EnsureSubnet(ctx, r.client, ad.SubnetInput{
		Name:        plan.Name.ValueString(),
		Site:        plan.Site.ValueString(),
		Description: plan.Description.ValueString(),
		Location:    plan.Location.ValueString(),
	})
	if err != nil {
		diags.AddError("Unable to set AD subnet", err.Error())
		return
	}
	diags.Append(state.Set(ctx, subnetState(subnet))...)
}

func subnetState(subnet *ad.Subnet) subnetResourceModel {
	return subnetResourceModel{ID: types.StringValue(subnet.DistinguishedName), Name: types.StringValue(subnet.Name), Site: types.StringValue(subnet.SiteName), Description: types.StringValue(subnet.Description), Location: types.StringValue(subnet.Location), DistinguishedName: types.StringValue(subnet.DistinguishedName)}
}
