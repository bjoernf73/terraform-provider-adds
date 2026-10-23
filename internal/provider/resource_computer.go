package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                = &computerResource{}
	_ resource.ResourceWithConfigure   = &computerResource{}
	_ resource.ResourceWithImportState = &computerResource{}
)

func NewComputerResource() resource.Resource {
	return &computerResource{}
}

type computerResource struct {
	client *client.Client
}

type computerResourceModel struct {
	ID                              types.String `tfsdk:"id"`
	Name                            types.String `tfsdk:"name"`
	SamAccountName                  types.String `tfsdk:"sam_account_name"`
	DNSHostName                     types.String `tfsdk:"dns_host_name"`
	Path                            types.String `tfsdk:"path"`
	Description                     types.String `tfsdk:"description"`
	DisplayName                     types.String `tfsdk:"display_name"`
	Location                        types.String `tfsdk:"location"`
	UserPrincipalName               types.String `tfsdk:"user_principal_name"`
	ManagedBy                       types.String `tfsdk:"managed_by"`
	ManagedByDN                     types.String `tfsdk:"managed_by_dn"`
	Enabled                         types.Bool   `tfsdk:"enabled"`
	KerberosEncryptionType          types.Set    `tfsdk:"kerberos_encryption_type"`
	ServicePrincipalNames           types.Set    `tfsdk:"service_principal_names"`
	TrustedForDelegation            types.Bool   `tfsdk:"trusted_for_delegation"`
	AccountNotDelegated             types.Bool   `tfsdk:"account_not_delegated"`
	CompoundIdentitySupported       types.Bool   `tfsdk:"compound_identity_supported"`
	OperatingSystem                 types.String `tfsdk:"operating_system"`
	OperatingSystemVersion          types.String `tfsdk:"operating_system_version"`
	ProtectedFromAccidentalDeletion types.Bool   `tfsdk:"protected_from_accidental_deletion"`
	DistinguishedName               types.String `tfsdk:"distinguished_name"`
	SID                             types.String `tfsdk:"sid"`
}

func (r *computerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_computer"
}

func (r *computerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computedString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Active Directory computer account by executing PowerShell on a remote Windows host.\n\n" +
			"This pre-stages or reconciles the directory object for a machine; it does not join a host to the domain. " +
			"Attributes populated by a joined machine, such as `operating_system`, are read back but not managed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier. Equals the account `objectGUID`, which is stable across renames and moves.",
				PlanModifiers:       computedString,
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Computer name (the `CN`). Changing this renames the account in place.",
			},
			"sam_account_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Pre-Windows 2000 logon name. Defaults to `name`. Active Directory appends a `$`, so a trailing `$` you supply is ignored and the remainder must be at most 15 characters. Both `web01` and `web01$` refer to the same account.",
				Validators: []validator.String{
					gmsaSamAccountName(15),
				},
			},
			"dns_host_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Fully qualified DNS host name of the machine, for example `web01.contoso.local`.",
			},
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Container holding the account. Accepts a slash-delimited path relative to the domain root (for example `Computers` or `Contoso/Servers`), a relative distinguished name such as `CN=Computers`, or a full distinguished name. Slash segments are always OU names. Changing this moves the account.",
			},
			"description":         schema.StringAttribute{Optional: true, MarkdownDescription: "Description."},
			"display_name":        schema.StringAttribute{Optional: true, MarkdownDescription: "Display name (`displayName`)."},
			"location":            schema.StringAttribute{Optional: true, MarkdownDescription: "Physical location of the machine (`location`)."},
			"user_principal_name": schema.StringAttribute{Optional: true, MarkdownDescription: "User principal name of the account."},
			"managed_by": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Owner of this computer object (`managedBy`). Accepts a distinguished name, `objectGUID`, SID, " +
					"`DOMAIN\\name` or `sAMAccountName`. The resolved distinguished name is published as `managed_by_dn`.",
			},
			"managed_by_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved distinguished name of `managed_by`.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Whether the account is enabled.",
			},
			"kerberos_encryption_type": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Kerberos encryption types the account supports. Any of `None`, `DES`, `RC4`, `AES128`, `AES256`. " +
					"Omit to leave the Active Directory default in place.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				Validators: []validator.Set{
					setValuesOneOf("None", "DES", "RC4", "AES128", "AES256"),
				},
			},
			"service_principal_names": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Service principal names registered on the account, for example `HOST/web01.contoso.local`. This set is authoritative: names not listed are removed.",
			},
			"trusted_for_delegation": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Trust the account for unconstrained Kerberos delegation.",
			},
			"account_not_delegated": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Mark the account as sensitive and not able to be delegated.",
			},
			"compound_identity_supported": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Advertise support for Kerberos armoring / compound identity (Dynamic Access Control).",
			},
			"operating_system": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system reported by the joined machine (`operatingSystem`). Empty until a host joins with this account.",
			},
			"operating_system_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system version reported by the joined machine (`operatingSystemVersion`). Empty until a host joins with this account.",
			},
			"protected_from_accidental_deletion": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Protect the account from accidental deletion. This is not a stored attribute: it adds Deny access control entries for `Everyone` on `Delete` and `DeleteTree`.",
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the account. Changes when the account is renamed or moved.",
			},
			"sid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Security identifier of the account.",
				PlanModifiers:       computedString,
			},
		},
	}
}

func (r *computerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *computerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan computerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := computerInput(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	computer, err := ad.EnsureComputer(ctx, r.client, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create computer", err.Error())
		return
	}

	state, diags := computerState(ctx, plan, computer)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *computerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state computerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := computerInput(ctx, state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	computer, err := ad.ReadComputer(ctx, r.client, state.ID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read computer", err.Error())
		return
	}

	if !computer.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	newState, diags := computerState(ctx, state, computer)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *computerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan computerResourceModel
	var state computerResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := computerInput(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	computer, err := ad.UpdateComputer(ctx, r.client, state.ID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update computer", err.Error())
		return
	}

	newState, diags := computerState(ctx, plan, computer)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *computerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state computerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteComputer(ctx, r.client, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete computer", err.Error())
	}
}

func (r *computerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func computerInput(ctx context.Context, model computerResourceModel, diags *diag.Diagnostics) ad.ComputerInput {
	return ad.ComputerInput{
		Name:                            model.Name.ValueString(),
		SamAccountName:                  optionalString(model.SamAccountName),
		DNSHostName:                     optionalString(model.DNSHostName),
		Path:                            model.Path.ValueString(),
		Description:                     optionalString(model.Description),
		DisplayName:                     optionalString(model.DisplayName),
		Location:                        optionalString(model.Location),
		UserPrincipalName:               optionalString(model.UserPrincipalName),
		ManagedBy:                       model.ManagedBy.ValueString(),
		Enabled:                         model.Enabled.ValueBool(),
		KerberosEncryptionType:          stringSet(ctx, model.KerberosEncryptionType, diags),
		ServicePrincipalNames:           stringSet(ctx, model.ServicePrincipalNames, diags),
		TrustedForDelegation:            model.TrustedForDelegation.ValueBool(),
		AccountNotDelegated:             model.AccountNotDelegated.ValueBool(),
		CompoundIdentitySupported:       model.CompoundIdentitySupported.ValueBool(),
		ProtectedFromAccidentalDeletion: model.ProtectedFromAccidentalDeletion.ValueBool(),
	}
}

// computerState keeps the configured spelling of path, sam_account_name and managed_by when
// they resolve to the same value, because an identity string or a slash path and a
// distinguished name can denote the same object.
func computerState(ctx context.Context, model computerResourceModel, computer *ad.Computer) (computerResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	path := types.StringValue(computer.Path)
	if computer.PathMatch && !model.Path.IsNull() && !model.Path.IsUnknown() {
		path = model.Path
	}

	sam := types.StringValue(computer.SamAccountNameStripped)
	if computer.SamMatch && !model.SamAccountName.IsNull() && !model.SamAccountName.IsUnknown() {
		sam = model.SamAccountName
	}

	managedBy := types.StringNull()
	managedByDN := types.StringNull()
	if computer.ManagedBy != "" {
		managedBy = types.StringValue(computer.ManagedBy)
		if computer.ManagedByMatch && !model.ManagedBy.IsNull() && !model.ManagedBy.IsUnknown() {
			managedBy = model.ManagedBy
		}
		managedByDN = types.StringValue(computer.ManagedBy)
	}

	kerberos, d := types.SetValueFrom(ctx, types.StringType, orEmptyStrings(computer.KerberosEncryptionType))
	diags.Append(d...)

	spns, d := optionalStringSetState(ctx, model.ServicePrincipalNames, computer.ServicePrincipalNames)
	diags.Append(d...)

	return computerResourceModel{
		ID:                              types.StringValue(computer.GUID),
		Name:                            types.StringValue(computer.Name),
		SamAccountName:                  sam,
		DNSHostName:                     stringPointerToTerraform(computer.DNSHostName),
		Path:                            path,
		Description:                     stringPointerToTerraform(computer.Description),
		DisplayName:                     stringPointerToTerraform(computer.DisplayName),
		Location:                        stringPointerToTerraform(computer.Location),
		UserPrincipalName:               stringPointerToTerraform(computer.UserPrincipalName),
		ManagedBy:                       managedBy,
		ManagedByDN:                     managedByDN,
		Enabled:                         types.BoolValue(computer.Enabled),
		KerberosEncryptionType:          kerberos,
		ServicePrincipalNames:           spns,
		TrustedForDelegation:            types.BoolValue(computer.TrustedForDelegation),
		AccountNotDelegated:             types.BoolValue(computer.AccountNotDelegated),
		CompoundIdentitySupported:       types.BoolValue(computer.CompoundIdentitySupported),
		OperatingSystem:                 stringPointerToTerraform(computer.OperatingSystem),
		OperatingSystemVersion:          stringPointerToTerraform(computer.OperatingSystemVersion),
		ProtectedFromAccidentalDeletion: types.BoolValue(computer.ProtectedFromAccidentalDeletion),
		DistinguishedName:               types.StringValue(computer.DistinguishedName),
		SID:                             types.StringValue(computer.SID),
	}, diags
}
