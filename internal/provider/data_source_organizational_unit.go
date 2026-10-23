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
	_ datasource.DataSource              = &organizationalUnitDataSource{}
	_ datasource.DataSourceWithConfigure = &organizationalUnitDataSource{}
)

func NewOrganizationalUnitDataSource() datasource.DataSource {
	return &organizationalUnitDataSource{}
}

type organizationalUnitDataSource struct {
	client *client.Client
}

type organizationalUnitDataSourceModel struct {
	ID                              types.String `tfsdk:"id"`
	Path                            types.String `tfsdk:"path"`
	Name                            types.String `tfsdk:"name"`
	Description                     types.String `tfsdk:"description"`
	DistinguishedName               types.String `tfsdk:"distinguished_name"`
	ProtectedFromAccidentalDeletion types.Bool   `tfsdk:"protected_from_accidental_deletion"`
}

func (d *organizationalUnitDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organizational_unit"
}

func (d *organizationalUnitDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an existing Active Directory organizational unit. Use it to reference an OU that is not " +
			"managed by this Terraform configuration, for example as the target of an access rule or the parent of a new object.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Path of the organizational unit to look up. Accepts a slash-delimited path relative to the " +
					"domain root (for example `Contoso/Servers`), a relative distinguished name, or a full distinguished name. " +
					"Reading fails if no OU matches.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the organizational unit.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name (the `OU` RDN) of the organizational unit.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description.",
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the organizational unit.",
			},
			"protected_from_accidental_deletion": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the organizational unit is protected from accidental deletion.",
			},
		},
	}
}

func (d *organizationalUnitDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *organizationalUnitDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config organizationalUnitDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ou, err := ad.ReadOrganizationalUnitByPath(ctx, d.client, config.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read organizational unit", err.Error())
		return
	}

	state := organizationalUnitDataSourceModel{
		ID:                              types.StringValue(ou.DistinguishedName),
		Path:                            types.StringValue(ou.Path),
		Name:                            types.StringValue(ou.Name),
		Description:                     stringPointerToTerraform(ou.Description),
		DistinguishedName:               types.StringValue(ou.DistinguishedName),
		ProtectedFromAccidentalDeletion: types.BoolValue(ou.ProtectedFromAccidentalDeletion),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
