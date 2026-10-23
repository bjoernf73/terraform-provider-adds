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
	_ datasource.DataSource              = &gpoDataSource{}
	_ datasource.DataSourceWithConfigure = &gpoDataSource{}
)

func NewGPODataSource() datasource.DataSource {
	return &gpoDataSource{}
}

type gpoDataSource struct {
	client *client.Client
}

type gpoDataSourceModel struct {
	Identity              types.String `tfsdk:"identity"`
	ID                    types.String `tfsdk:"id"`
	GUID                  types.String `tfsdk:"guid"`
	Name                  types.String `tfsdk:"name"`
	DistinguishedName     types.String `tfsdk:"distinguished_name"`
	Domain                types.String `tfsdk:"domain"`
	Status                types.String `tfsdk:"status"`
	Description           types.String `tfsdk:"description"`
	CreationTime          types.String `tfsdk:"creation_time"`
	ModificationTime      types.String `tfsdk:"modification_time"`
	ComputerADVersion     types.Int64  `tfsdk:"computer_ad_version"`
	ComputerSysvolVersion types.Int64  `tfsdk:"computer_sysvol_version"`
	UserADVersion         types.Int64  `tfsdk:"user_ad_version"`
	UserSysvolVersion     types.Int64  `tfsdk:"user_sysvol_version"`
}

func (d *gpoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gpo"
}

func (d *gpoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an existing Group Policy Object. Use it to reference a GPO that this Terraform " +
			"configuration did not create, for example to link it (`adds_gpo_links`), set its permissions " +
			"(`adds_gpo_permission`), or attach a security or WMI filter.",
		Attributes: map[string]schema.Attribute{
			"identity": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Identity of the GPO to look up. Accepts the GPO display name or its GUID. " +
					"Reading fails if no GPO matches.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "GPO GUID.",
			},
			"guid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "GPO GUID.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Display name of the GPO.",
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the GPO object under `CN=Policies,CN=System`.",
			},
			"domain": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS name of the domain the GPO belongs to.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "GPO status, for example `AllSettingsEnabled` or `AllSettingsDisabled`.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description.",
			},
			"creation_time": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp, in round-trip (`o`) format.",
			},
			"modification_time": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Last modification timestamp, in round-trip (`o`) format.",
			},
			"computer_ad_version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Computer configuration version in the directory.",
			},
			"computer_sysvol_version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Computer configuration version in SYSVOL.",
			},
			"user_ad_version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "User configuration version in the directory.",
			},
			"user_sysvol_version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "User configuration version in SYSVOL.",
			},
		},
	}
}

func (d *gpoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *gpoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config gpoDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	gpo, err := ad.ReadGPOByIdentity(ctx, d.client, config.Identity.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GPO", err.Error())
		return
	}

	state := gpoDataSourceModel{
		Identity:              config.Identity,
		ID:                    types.StringValue(gpo.GUID),
		GUID:                  types.StringValue(gpo.GUID),
		Name:                  types.StringValue(gpo.Name),
		DistinguishedName:     types.StringValue(gpo.DistinguishedName),
		Domain:                types.StringValue(gpo.Domain),
		Status:                types.StringValue(gpo.Status),
		Description:           stringPointerToTerraform(gpo.Description),
		CreationTime:          stringPointerToTerraform(gpo.CreationTime),
		ModificationTime:      stringPointerToTerraform(gpo.ModificationTime),
		ComputerADVersion:     types.Int64Value(gpo.ComputerADVersion),
		ComputerSysvolVersion: types.Int64Value(gpo.ComputerSysvolVersion),
		UserADVersion:         types.Int64Value(gpo.UserADVersion),
		UserSysvolVersion:     types.Int64Value(gpo.UserSysvolVersion),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
