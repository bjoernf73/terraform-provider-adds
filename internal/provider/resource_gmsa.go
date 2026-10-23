package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                = &gmsaResource{}
	_ resource.ResourceWithConfigure   = &gmsaResource{}
	_ resource.ResourceWithImportState = &gmsaResource{}
)

func NewGMSAResource() resource.Resource {
	return &gmsaResource{}
}

type gmsaResource struct {
	client *client.Client
}

type gmsaResourceModel struct {
	ID                              types.String `tfsdk:"id"`
	Name                            types.String `tfsdk:"name"`
	SamAccountName                  types.String `tfsdk:"sam_account_name"`
	DNSHostName                     types.String `tfsdk:"dns_host_name"`
	Path                            types.String `tfsdk:"path"`
	Description                     types.String `tfsdk:"description"`
	DisplayName                     types.String `tfsdk:"display_name"`
	HomePage                        types.String `tfsdk:"home_page"`
	Enabled                         types.Bool   `tfsdk:"enabled"`
	KerberosEncryptionType          types.Set    `tfsdk:"kerberos_encryption_type"`
	ManagedPasswordIntervalDays     types.Int64  `tfsdk:"managed_password_interval_days"`
	PrincipalsAllowedToRetrieve     types.Set    `tfsdk:"principals_allowed_to_retrieve_managed_password"`
	PrincipalsAllowedToRetrieveDNs  types.Set    `tfsdk:"principals_allowed_to_retrieve_managed_password_dns"`
	PrincipalsAllowedToDelegate     types.Set    `tfsdk:"principals_allowed_to_delegate_to_account"`
	PrincipalsAllowedToDelegateDNs  types.Set    `tfsdk:"principals_allowed_to_delegate_to_account_dns"`
	ServicePrincipalNames           types.Set    `tfsdk:"service_principal_names"`
	TrustedForDelegation            types.Bool   `tfsdk:"trusted_for_delegation"`
	AccountNotDelegated             types.Bool   `tfsdk:"account_not_delegated"`
	CompoundIdentitySupported       types.Bool   `tfsdk:"compound_identity_supported"`
	AccountExpirationDate           types.String `tfsdk:"account_expiration_date"`
	ProtectedFromAccidentalDeletion types.Bool   `tfsdk:"protected_from_accidental_deletion"`
	DistinguishedName               types.String `tfsdk:"distinguished_name"`
	SID                             types.String `tfsdk:"sid"`
}

func (r *gmsaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gmsa"
}

func (r *gmsaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computedString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Active Directory group managed service account (gMSA) by executing PowerShell on a remote Windows host.\n\n" +
			"The password is generated and rotated by the Key Distribution Service (KDS), not managed here, so the account can be created enabled. " +
			"A KDS root key must already exist in the forest.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier. Equals the account `objectGUID`, which is stable across renames and moves.",
				PlanModifiers:       computedString,
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Account name (the `CN`). Changing this renames the account in place.",
			},
			"sam_account_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Pre-Windows 2000 logon name. Defaults to `name`. Active Directory appends a `$`, so a trailing `$` you supply is ignored and the remainder must be at most 19 characters. Both `my-gmsa` and `my-gmsa$` refer to the same account.",
				Validators: []validator.String{
					gmsaSamAccountName(19),
				},
			},
			"dns_host_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Fully qualified DNS host name the account authenticates as, for example `websvc.contoso.local`. Required for a gMSA.",
			},
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Container holding the account. Accepts a slash-delimited path relative to the domain root (for example `Managed Service Accounts` or `Contoso/Service Accounts`), a relative distinguished name such as `CN=Managed Service Accounts`, or a full distinguished name. Slash segments are always OU names. Changing this moves the account.",
			},
			"description":  schema.StringAttribute{Optional: true, MarkdownDescription: "Description."},
			"display_name": schema.StringAttribute{Optional: true, MarkdownDescription: "Display name (`displayName`)."},
			"home_page":    schema.StringAttribute{Optional: true, MarkdownDescription: "Web page address (`wWWHomePage`)."},
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
			"managed_password_interval_days": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "How often, in days, the KDS rotates the managed password. Fixed at creation; changing it replaces the account. Defaults to 30.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"principals_allowed_to_retrieve_managed_password": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Principals allowed to retrieve the managed password, usually the computer accounts or a group of hosts that run the service. " +
					"Each accepts a distinguished name, `objectGUID`, SID, `DOMAIN\\name` or `sAMAccountName`. This set is authoritative: principals not listed are removed. Resolved distinguished names are published as `principals_allowed_to_retrieve_managed_password_dns`.",
			},
			"principals_allowed_to_retrieve_managed_password_dns": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Resolved distinguished names of `principals_allowed_to_retrieve_managed_password`.",
			},
			"principals_allowed_to_delegate_to_account": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Principals permitted resource-based constrained delegation to this account (`msDS-AllowedToActOnBehalfOfOtherIdentity`). " +
					"Each accepts a distinguished name, `objectGUID`, SID, `DOMAIN\\name` or `sAMAccountName`. This set is authoritative. Resolved distinguished names are published as `principals_allowed_to_delegate_to_account_dns`.",
			},
			"principals_allowed_to_delegate_to_account_dns": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Resolved distinguished names of `principals_allowed_to_delegate_to_account`.",
			},
			"service_principal_names": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Service principal names registered on the account, for example `HTTP/websvc.contoso.local`. This set is authoritative: names not listed are removed.",
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
			"account_expiration_date": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Date the account expires, as `YYYY-MM-DD`. Omit for an account that never expires. " +
					"AD stores this as a timestamp, so the value read back may shift by the local UTC offset.",
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

func (r *gmsaResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *gmsaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gmsaResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := gmsaInput(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	gmsa, err := ad.EnsureGMSA(ctx, r.client, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create gMSA", err.Error())
		return
	}

	state, diags := gmsaState(ctx, plan, gmsa)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *gmsaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gmsaResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := gmsaInput(ctx, state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	gmsa, err := ad.ReadGMSA(ctx, r.client, state.ID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read gMSA", err.Error())
		return
	}

	if !gmsa.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	newState, diags := gmsaState(ctx, state, gmsa)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *gmsaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan gmsaResourceModel
	var state gmsaResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := gmsaInput(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	gmsa, err := ad.UpdateGMSA(ctx, r.client, state.ID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update gMSA", err.Error())
		return
	}

	newState, diags := gmsaState(ctx, plan, gmsa)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *gmsaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state gmsaResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteGMSA(ctx, r.client, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete gMSA", err.Error())
	}
}

func (r *gmsaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func gmsaInput(ctx context.Context, model gmsaResourceModel, diags *diag.Diagnostics) ad.GMSAInput {
	input := ad.GMSAInput{
		Name:                        model.Name.ValueString(),
		SamAccountName:              optionalString(model.SamAccountName),
		DNSHostName:                 model.DNSHostName.ValueString(),
		Path:                        model.Path.ValueString(),
		Description:                 optionalString(model.Description),
		DisplayName:                 optionalString(model.DisplayName),
		HomePage:                    optionalString(model.HomePage),
		Enabled:                     model.Enabled.ValueBool(),
		KerberosEncryptionType:      stringSet(ctx, model.KerberosEncryptionType, diags),
		ManagedPasswordIntervalDays: optionalInt64(model.ManagedPasswordIntervalDays),
		PrincipalsAllowedToRetrieveManagedPassword: stringSet(ctx, model.PrincipalsAllowedToRetrieve, diags),
		PrincipalsAllowedToDelegateToAccount:       stringSet(ctx, model.PrincipalsAllowedToDelegate, diags),
		ServicePrincipalNames:                      stringSet(ctx, model.ServicePrincipalNames, diags),
		TrustedForDelegation:                       model.TrustedForDelegation.ValueBool(),
		AccountNotDelegated:                        model.AccountNotDelegated.ValueBool(),
		CompoundIdentitySupported:                  model.CompoundIdentitySupported.ValueBool(),
		AccountExpirationDate:                      optionalString(model.AccountExpirationDate),
		ProtectedFromAccidentalDeletion:            model.ProtectedFromAccidentalDeletion.ValueBool(),
	}

	return input
}

// gmsaState keeps the configured spelling of path, sam_account_name and the two principal
// sets when they resolve to the same value, because an identity string and a distinguished
// name can denote the same principal.
func gmsaState(ctx context.Context, model gmsaResourceModel, gmsa *ad.GMSA) (gmsaResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	path := types.StringValue(gmsa.Path)
	if gmsa.PathMatch && !model.Path.IsNull() && !model.Path.IsUnknown() {
		path = model.Path
	}

	sam := types.StringValue(gmsa.SamAccountNameStripped)
	if gmsa.SamMatch && !model.SamAccountName.IsNull() && !model.SamAccountName.IsUnknown() {
		sam = model.SamAccountName
	}

	retrieve, d := principalSetState(ctx, model.PrincipalsAllowedToRetrieve, gmsa.PrincipalsAllowedToRetrieveManagedPassword, gmsa.PrincipalsAllowedToRetrieveMatch)
	diags.Append(d...)
	retrieveDNs, d := types.SetValueFrom(ctx, types.StringType, orEmptyStrings(gmsa.PrincipalsAllowedToRetrieveManagedPassword))
	diags.Append(d...)

	delegate, d := principalSetState(ctx, model.PrincipalsAllowedToDelegate, gmsa.PrincipalsAllowedToDelegateToAccount, gmsa.PrincipalsAllowedToDelegateMatch)
	diags.Append(d...)
	delegateDNs, d := types.SetValueFrom(ctx, types.StringType, orEmptyStrings(gmsa.PrincipalsAllowedToDelegateToAccount))
	diags.Append(d...)

	kerberos, d := types.SetValueFrom(ctx, types.StringType, orEmptyStrings(gmsa.KerberosEncryptionType))
	diags.Append(d...)

	spns, d := optionalStringSetState(ctx, model.ServicePrincipalNames, gmsa.ServicePrincipalNames)
	diags.Append(d...)

	return gmsaResourceModel{
		ID:                              types.StringValue(gmsa.GUID),
		Name:                            types.StringValue(gmsa.Name),
		SamAccountName:                  sam,
		DNSHostName:                     types.StringValue(gmsa.DNSHostName),
		Path:                            path,
		Description:                     stringPointerToTerraform(gmsa.Description),
		DisplayName:                     stringPointerToTerraform(gmsa.DisplayName),
		HomePage:                        stringPointerToTerraform(gmsa.HomePage),
		Enabled:                         types.BoolValue(gmsa.Enabled),
		KerberosEncryptionType:          kerberos,
		ManagedPasswordIntervalDays:     types.Int64Value(gmsa.ManagedPasswordIntervalDays),
		PrincipalsAllowedToRetrieve:     retrieve,
		PrincipalsAllowedToRetrieveDNs:  retrieveDNs,
		PrincipalsAllowedToDelegate:     delegate,
		PrincipalsAllowedToDelegateDNs:  delegateDNs,
		ServicePrincipalNames:           spns,
		TrustedForDelegation:            types.BoolValue(gmsa.TrustedForDelegation),
		AccountNotDelegated:             types.BoolValue(gmsa.AccountNotDelegated),
		CompoundIdentitySupported:       types.BoolValue(gmsa.CompoundIdentitySupported),
		AccountExpirationDate:           stringPointerToTerraform(gmsa.AccountExpirationDate),
		ProtectedFromAccidentalDeletion: types.BoolValue(gmsa.ProtectedFromAccidentalDeletion),
		DistinguishedName:               types.StringValue(gmsa.DistinguishedName),
		SID:                             types.StringValue(gmsa.SID),
	}, diags
}

// principalSetState keeps the configured identities when they resolve to the current set,
// otherwise reports the resolved distinguished names so drift is visible.
func principalSetState(ctx context.Context, configured types.Set, resolvedDNs []string, matches bool) (types.Set, diag.Diagnostics) {
	if matches && !configured.IsNull() && !configured.IsUnknown() {
		return configured, nil
	}
	if matches && len(resolvedDNs) == 0 {
		return configured, nil
	}
	return types.SetValueFrom(ctx, types.StringType, orEmptyStrings(resolvedDNs))
}

// optionalStringSetState preserves a null configuration when the account holds no values,
// and otherwise reflects the stored values verbatim.
func optionalStringSetState(ctx context.Context, configured types.Set, actual []string) (types.Set, diag.Diagnostics) {
	if configured.IsNull() && len(actual) == 0 {
		return configured, nil
	}
	return types.SetValueFrom(ctx, types.StringType, orEmptyStrings(actual))
}

func stringSet(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	var values []string
	diags.Append(set.ElementsAs(ctx, &values, false)...)
	return values
}

func optionalInt64(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	result := value.ValueInt64()
	return &result
}
