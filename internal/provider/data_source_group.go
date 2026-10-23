package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ datasource.DataSource              = &groupDataSource{}
	_ datasource.DataSourceWithConfigure = &groupDataSource{}
)

func NewGroupDataSource() datasource.DataSource {
	return &groupDataSource{}
}

type groupDataSource struct {
	client *client.Client
}

type groupDataSourceModel struct {
	Identity          types.String `tfsdk:"identity"`
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	SamAccountName    types.String `tfsdk:"sam_account_name"`
	Description       types.String `tfsdk:"description"`
	DisplayName       types.String `tfsdk:"display_name"`
	Mail              types.String `tfsdk:"mail"`
	Info              types.String `tfsdk:"info"`
	Homepage          types.String `tfsdk:"homepage"`
	ManagedBy         types.String `tfsdk:"managed_by"`
	Category          types.String `tfsdk:"category"`
	Scope             types.String `tfsdk:"scope"`
	Path              types.String `tfsdk:"path"`
	DistinguishedName types.String `tfsdk:"distinguished_name"`
	SID               types.String `tfsdk:"sid"`
}

func (d *groupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *groupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an existing Active Directory group. Use it to reference groups that are not managed by " +
			"this Terraform configuration, for example to obtain a `sid` for an access rule or a GPO security filter.",
		Attributes: map[string]schema.Attribute{
			"identity": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Identity of the group to look up. Accepts a distinguished name, `objectGUID`, SID, " +
					"`DOMAIN\\name` or `sAMAccountName`. Reading fails if no group matches.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Account `objectGUID`, stable across renames and moves.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Group name (the `CN`).",
			},
			"sam_account_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Pre-Windows 2000 group name.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description.",
			},
			"display_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Display name (`displayName`).",
			},
			"mail": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Email address (`mail`).",
			},
			"info": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Notes (`info`).",
			},
			"homepage": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Web page address (`wWWHomePage`).",
			},
			"managed_by": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the group's owner (`managedBy`).",
			},
			"category": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Group category: `Security` or `Distribution`.",
			},
			"scope": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Group scope: `DomainLocal`, `Global` or `Universal`.",
			},
			"path": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Slash-delimited path of the container holding the group, relative to the domain root.",
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the group.",
			},
			"sid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Security identifier of the group.",
			},
		},
	}
}

func (d *groupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	addsClient, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}

	d.client = addsClient
}

func (d *groupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config groupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := ad.ReadGroupByIdentity(ctx, d.client, config.Identity.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read group", err.Error())
		return
	}

	state := groupDataSourceModel{
		Identity:          config.Identity,
		ID:                types.StringValue(group.GUID),
		Name:              types.StringValue(group.Name),
		SamAccountName:    types.StringValue(group.SamAccountName),
		Description:       stringPointerToTerraform(group.Description),
		DisplayName:       stringPointerToTerraform(group.DisplayName),
		Mail:              stringPointerToTerraform(group.Mail),
		Info:              stringPointerToTerraform(group.Info),
		Homepage:          stringPointerToTerraform(group.Homepage),
		ManagedBy:         types.StringValue(group.ManagedBy),
		Category:          types.StringValue(group.Category),
		Scope:             types.StringValue(group.Scope),
		Path:              types.StringValue(group.Path),
		DistinguishedName: types.StringValue(group.DistinguishedName),
		SID:               types.StringValue(group.SID),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
