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
	_ datasource.DataSource              = &domainDataSource{}
	_ datasource.DataSourceWithConfigure = &domainDataSource{}
)

func NewDomainDataSource() datasource.DataSource {
	return &domainDataSource{}
}

type domainDataSource struct {
	client *client.Client
}

type domainDataSourceModel struct {
	ID                         types.String `tfsdk:"id"`
	DistinguishedName          types.String `tfsdk:"distinguished_name"`
	DNSRoot                    types.String `tfsdk:"dns_root"`
	NetBIOSName                types.String `tfsdk:"netbios_name"`
	SID                        types.String `tfsdk:"sid"`
	DomainMode                 types.String `tfsdk:"domain_mode"`
	Forest                     types.String `tfsdk:"forest"`
	UsersContainer             types.String `tfsdk:"users_container"`
	ComputersContainer         types.String `tfsdk:"computers_container"`
	DomainControllersContainer types.String `tfsdk:"domain_controllers_container"`
	PDCEmulator                types.String `tfsdk:"pdc_emulator"`
	InfrastructureMaster       types.String `tfsdk:"infrastructure_master"`
}

func (d *domainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (d *domainDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the Active Directory domain the provider is connected to. Useful for building " +
			"distinguished names without hardcoding the domain component.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the domain.",
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the domain, for example `DC=contoso,DC=local`.",
			},
			"dns_root": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS name of the domain, for example `contoso.local`.",
			},
			"netbios_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Pre-Windows 2000 domain name, for example `CONTOSO`.",
			},
			"sid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Security identifier of the domain.",
			},
			"domain_mode": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Domain functional level.",
			},
			"forest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS name of the forest root domain.",
			},
			"users_container": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the default users container.",
			},
			"computers_container": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the default computers container.",
			},
			"domain_controllers_container": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the domain controllers organizational unit.",
			},
			"pdc_emulator": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Host name of the PDC emulator.",
			},
			"infrastructure_master": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Host name of the infrastructure master.",
			},
		},
	}
}

func (d *domainDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *domainDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	domain, err := ad.ReadDomain(ctx, d.client)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read domain", err.Error())
		return
	}

	state := domainDataSourceModel{
		ID:                         types.StringValue(domain.DistinguishedName),
		DistinguishedName:          types.StringValue(domain.DistinguishedName),
		DNSRoot:                    types.StringValue(domain.DNSRoot),
		NetBIOSName:                types.StringValue(domain.NetBIOSName),
		SID:                        types.StringValue(domain.SID),
		DomainMode:                 types.StringValue(domain.DomainMode),
		Forest:                     types.StringValue(domain.Forest),
		UsersContainer:             types.StringValue(domain.UsersContainer),
		ComputersContainer:         types.StringValue(domain.ComputersContainer),
		DomainControllersContainer: types.StringValue(domain.DomainControllersContainer),
		PDCEmulator:                types.StringValue(domain.PDCEmulator),
		InfrastructureMaster:       types.StringValue(domain.InfrastructureMaster),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
