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
	_ datasource.DataSource              = &computerDataSource{}
	_ datasource.DataSourceWithConfigure = &computerDataSource{}
)

func NewComputerDataSource() datasource.DataSource {
	return &computerDataSource{}
}

type computerDataSource struct {
	client *client.Client
}

type computerDataSourceModel struct {
	Identity                        types.String `tfsdk:"identity"`
	ID                              types.String `tfsdk:"id"`
	Name                            types.String `tfsdk:"name"`
	SamAccountName                  types.String `tfsdk:"sam_account_name"`
	DNSHostName                     types.String `tfsdk:"dns_host_name"`
	Path                            types.String `tfsdk:"path"`
	DistinguishedName               types.String `tfsdk:"distinguished_name"`
	SID                             types.String `tfsdk:"sid"`
	Description                     types.String `tfsdk:"description"`
	DisplayName                     types.String `tfsdk:"display_name"`
	Location                        types.String `tfsdk:"location"`
	UserPrincipalName               types.String `tfsdk:"user_principal_name"`
	ManagedBy                       types.String `tfsdk:"managed_by"`
	Enabled                         types.Bool   `tfsdk:"enabled"`
	KerberosEncryptionType          types.Set    `tfsdk:"kerberos_encryption_type"`
	ServicePrincipalNames           types.Set    `tfsdk:"service_principal_names"`
	TrustedForDelegation            types.Bool   `tfsdk:"trusted_for_delegation"`
	AccountNotDelegated             types.Bool   `tfsdk:"account_not_delegated"`
	CompoundIdentitySupported       types.Bool   `tfsdk:"compound_identity_supported"`
	OperatingSystem                 types.String `tfsdk:"operating_system"`
	OperatingSystemVersion          types.String `tfsdk:"operating_system_version"`
	ProtectedFromAccidentalDeletion types.Bool   `tfsdk:"protected_from_accidental_deletion"`
}

func (d *computerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_computer"
}

func (d *computerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an existing Active Directory computer account. Use it to reference machines that are " +
			"not managed by this Terraform configuration, for example when adding their accounts to a gMSA's " +
			"`principals_allowed_to_retrieve_managed_password`.",
		Attributes: map[string]schema.Attribute{
			"identity": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Identity of the computer to look up. Accepts a distinguished name, `objectGUID`, SID, " +
					"`DOMAIN\\name` or `sAMAccountName` (with or without the trailing `$`). Reading fails if no account matches.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Account `objectGUID`, stable across renames and moves.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Computer name (the `CN`).",
			},
			"sam_account_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Pre-Windows 2000 logon name, including the trailing `$`.",
			},
			"dns_host_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fully qualified DNS host name of the machine.",
			},
			"path": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Slash-delimited path of the container holding the account, relative to the domain root.",
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the account.",
			},
			"sid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Security identifier of the account.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description.",
			},
			"display_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Display name (`displayName`).",
			},
			"location": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Physical location of the machine (`location`).",
			},
			"user_principal_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "User principal name of the account.",
			},
			"managed_by": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the account's owner (`managedBy`).",
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the account is enabled.",
			},
			"kerberos_encryption_type": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Kerberos encryption types the account supports. Any of `None`, `DES`, `RC4`, `AES128`, `AES256`.",
			},
			"service_principal_names": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Service principal names registered on the account.",
			},
			"trusted_for_delegation": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the account is trusted for unconstrained Kerberos delegation.",
			},
			"account_not_delegated": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the account is marked sensitive and cannot be delegated.",
			},
			"compound_identity_supported": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the account advertises Kerberos armoring / compound identity support.",
			},
			"operating_system": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system reported by the joined machine (`operatingSystem`).",
			},
			"operating_system_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system version reported by the joined machine (`operatingSystemVersion`).",
			},
			"protected_from_accidental_deletion": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the account carries Deny access control entries protecting it from accidental deletion.",
			},
		},
	}
}

func (d *computerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *computerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config computerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	computer, err := ad.ReadComputerByIdentity(ctx, d.client, config.Identity.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read computer", err.Error())
		return
	}

	kerberos, diags := types.SetValueFrom(ctx, types.StringType, computer.KerberosEncryptionType)
	resp.Diagnostics.Append(diags...)
	spns, diags := types.SetValueFrom(ctx, types.StringType, computer.ServicePrincipalNames)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := computerDataSourceModel{
		Identity:                        config.Identity,
		ID:                              types.StringValue(computer.GUID),
		Name:                            types.StringValue(computer.Name),
		SamAccountName:                  types.StringValue(computer.SamAccountName),
		DNSHostName:                     stringPointerToTerraform(computer.DNSHostName),
		Path:                            types.StringValue(computer.Path),
		DistinguishedName:               types.StringValue(computer.DistinguishedName),
		SID:                             types.StringValue(computer.SID),
		Description:                     stringPointerToTerraform(computer.Description),
		DisplayName:                     stringPointerToTerraform(computer.DisplayName),
		Location:                        stringPointerToTerraform(computer.Location),
		UserPrincipalName:               stringPointerToTerraform(computer.UserPrincipalName),
		ManagedBy:                       types.StringValue(computer.ManagedBy),
		Enabled:                         types.BoolValue(computer.Enabled),
		KerberosEncryptionType:          kerberos,
		ServicePrincipalNames:           spns,
		TrustedForDelegation:            types.BoolValue(computer.TrustedForDelegation),
		AccountNotDelegated:             types.BoolValue(computer.AccountNotDelegated),
		CompoundIdentitySupported:       types.BoolValue(computer.CompoundIdentitySupported),
		OperatingSystem:                 stringPointerToTerraform(computer.OperatingSystem),
		OperatingSystemVersion:          stringPointerToTerraform(computer.OperatingSystemVersion),
		ProtectedFromAccidentalDeletion: types.BoolValue(computer.ProtectedFromAccidentalDeletion),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
