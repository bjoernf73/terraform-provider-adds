package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                = &userResource{}
	_ resource.ResourceWithConfigure   = &userResource{}
	_ resource.ResourceWithImportState = &userResource{}
)

func NewUserResource() resource.Resource {
	return &userResource{}
}

type userResource struct {
	client *client.Client
}

type userResourceModel struct {
	ID                              types.String `tfsdk:"id"`
	Name                            types.String `tfsdk:"name"`
	SamAccountName                  types.String `tfsdk:"sam_account_name"`
	UserPrincipalName               types.String `tfsdk:"user_principal_name"`
	Path                            types.String `tfsdk:"path"`
	Description                     types.String `tfsdk:"description"`
	DisplayName                     types.String `tfsdk:"display_name"`
	GivenName                       types.String `tfsdk:"given_name"`
	Surname                         types.String `tfsdk:"surname"`
	Initials                        types.String `tfsdk:"initials"`
	OtherName                       types.String `tfsdk:"other_name"`
	Email                           types.String `tfsdk:"email"`
	Office                          types.String `tfsdk:"office"`
	OfficePhone                     types.String `tfsdk:"office_phone"`
	HomePhone                       types.String `tfsdk:"home_phone"`
	MobilePhone                     types.String `tfsdk:"mobile_phone"`
	Fax                             types.String `tfsdk:"fax"`
	HomePage                        types.String `tfsdk:"home_page"`
	StreetAddress                   types.String `tfsdk:"street_address"`
	POBox                           types.String `tfsdk:"po_box"`
	City                            types.String `tfsdk:"city"`
	State                           types.String `tfsdk:"state"`
	PostalCode                      types.String `tfsdk:"postal_code"`
	Country                         types.String `tfsdk:"country"`
	Company                         types.String `tfsdk:"company"`
	Department                      types.String `tfsdk:"department"`
	Division                        types.String `tfsdk:"division"`
	Organization                    types.String `tfsdk:"organization"`
	EmployeeID                      types.String `tfsdk:"employee_id"`
	EmployeeNumber                  types.String `tfsdk:"employee_number"`
	Title                           types.String `tfsdk:"title"`
	HomeDirectory                   types.String `tfsdk:"home_directory"`
	HomeDrive                       types.String `tfsdk:"home_drive"`
	LogonWorkstations               types.String `tfsdk:"logon_workstations"`
	ScriptPath                      types.String `tfsdk:"script_path"`
	ProfilePath                     types.String `tfsdk:"profile_path"`
	AccountExpirationDate           types.String `tfsdk:"account_expiration_date"`
	Manager                         types.String `tfsdk:"manager"`
	ManagerDN                       types.String `tfsdk:"manager_dn"`
	Enabled                         types.Bool   `tfsdk:"enabled"`
	PasswordNeverExpires            types.Bool   `tfsdk:"password_never_expires"`
	CannotChangePassword            types.Bool   `tfsdk:"cannot_change_password"`
	SmartCardLogonRequired          types.Bool   `tfsdk:"smart_card_logon_required"`
	TrustedForDelegation            types.Bool   `tfsdk:"trusted_for_delegation"`
	ProtectedFromAccidentalDeletion types.Bool   `tfsdk:"protected_from_accidental_deletion"`
	DistinguishedName               types.String `tfsdk:"distinguished_name"`
	SID                             types.String `tfsdk:"sid"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computedString := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	boolDefaultFalse := booldefault.StaticBool(false)

	optional := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, MarkdownDescription: desc}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Active Directory user account by executing PowerShell on a remote Windows host.\n\n" +
			"This resource owns the account's `enabled` state. Enabling an account that has no usable " +
			"password sets a throwaway strong random password (never stored in state) purely so the account " +
			"can be enabled; a password-management resource can later set the real password without changing " +
			"the enabled state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier. Equals the user `objectGUID`, which is stable across renames and moves.",
				PlanModifiers:       computedString,
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "User name (the `CN`). Changing this renames the user in place.",
			},
			"sam_account_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Pre-Windows 2000 logon name. Unique domain-wide, maximum 20 characters.",
				Validators: []validator.String{
					maxLength(20),
				},
			},
			"user_principal_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "User principal name, for example `jdoe@contoso.local`.",
			},
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Container holding the user. Accepts a slash-delimited path relative to the domain root (for example `Users` or `Contoso/Users`), a relative distinguished name such as `CN=Users`, or a full distinguished name. Slash segments are always OU names. Changing this moves the user.",
			},
			"description":     optional("Description."),
			"display_name":    optional("Display name."),
			"given_name":      optional("Given (first) name."),
			"surname":         optional("Surname (last name)."),
			"initials":        optional("Initials."),
			"other_name":      optional("Middle name (`middleName`)."),
			"email":           optional("Email address (`mail`)."),
			"office":          optional("Office location (`physicalDeliveryOfficeName`)."),
			"office_phone":    optional("Office telephone number."),
			"home_phone":      optional("Home telephone number."),
			"mobile_phone":    optional("Mobile telephone number (`mobile`)."),
			"fax":             optional("Fax number."),
			"home_page":       optional("Web page address (`wWWHomePage`)."),
			"street_address":  optional("Street address."),
			"po_box":          optional("Post office box."),
			"city":            optional("Town or city (`l`)."),
			"state":           optional("State or province (`st`)."),
			"postal_code":     optional("Postal or ZIP code."),
			"country":         optional("Country, as a 2-letter ISO 3166 code."),
			"company":         optional("Company."),
			"department":      optional("Department."),
			"division":        optional("Division."),
			"organization":    optional("Organization (`o`)."),
			"employee_id":     optional("Employee ID."),
			"employee_number": optional("Employee number."),
			"title":           optional("Job title."),
			"home_directory":  optional("Home directory UNC path."),
			"home_drive":      optional("Drive letter mapped to `home_directory`, for example `H:`."), "logon_workstations": optional("Comma-separated NetBIOS names of workstations the user may log on from. Empty means any workstation."),
			"script_path":  optional("Logon script path."),
			"profile_path": optional("Roaming profile path."),
			"account_expiration_date": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Date the account expires, as `YYYY-MM-DD`. Omit for an account that never expires. " +
					"AD stores this as a timestamp, so the value read back may shift by the local UTC offset.",
			}, "manager": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Manager of this user. Accepts a distinguished name, `objectGUID`, SID, " +
					"`DOMAIN\\name` or `sAMAccountName`. The resolved distinguished name is published as `manager_dn`.",
			},
			"manager_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved distinguished name of `manager`.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: boolDefaultFalse,
				MarkdownDescription: "Whether the account is enabled. This resource enforces the value: enabling an " +
					"account with no usable password sets a throwaway strong random password (never stored in state) " +
					"so it can be enabled. A password-management resource may later set the real password.",
			},
			"password_never_expires": schema.BoolAttribute{
				Optional: true, Computed: true, Default: boolDefaultFalse,
				MarkdownDescription: "Exempt the account's password from the domain's maximum password age.",
			},
			"cannot_change_password": schema.BoolAttribute{
				Optional: true, Computed: true, Default: boolDefaultFalse,
				MarkdownDescription: "Prevent the user from changing their own password.",
			},
			"smart_card_logon_required": schema.BoolAttribute{
				Optional: true, Computed: true, Default: boolDefaultFalse,
				MarkdownDescription: "Require a smart card for interactive logon.",
			},
			"trusted_for_delegation": schema.BoolAttribute{
				Optional: true, Computed: true, Default: boolDefaultFalse,
				MarkdownDescription: "Trust the account for Kerberos delegation.",
			},
			"protected_from_accidental_deletion": schema.BoolAttribute{
				Optional: true, Computed: true, Default: boolDefaultFalse,
				MarkdownDescription: "Protect the user from accidental deletion. This is not a stored attribute: it adds Deny access control entries for `Everyone` on `Delete` and `DeleteTree`.",
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the user. Changes when the user is renamed or moved.",
			},
			"sid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Security identifier of the user.",
				PlanModifiers:       computedString,
			},
		},
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := ad.EnsureUser(ctx, r.client, userInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create user", err.Error())
		return
	}

	state := userState(plan, user)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := ad.ReadUser(ctx, r.client, state.ID.ValueString(), state.Path.ValueString(), state.Manager.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read user", err.Error())
		return
	}

	if !user.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, userState(state, user))...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userResourceModel
	var state userResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := ad.UpdateUser(ctx, r.client, state.ID.ValueString(), userInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update user", err.Error())
		return
	}

	updated := userState(plan, user)
	resp.Diagnostics.Append(resp.State.Set(ctx, updated)...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteUser(ctx, r.client, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete user", err.Error())
	}
}

func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func userInput(model userResourceModel) ad.UserInput {
	return ad.UserInput{
		Name:                            model.Name.ValueString(),
		SamAccountName:                  model.SamAccountName.ValueString(),
		UserPrincipalName:               model.UserPrincipalName.ValueString(),
		Path:                            model.Path.ValueString(),
		Description:                     optionalString(model.Description),
		DisplayName:                     optionalString(model.DisplayName),
		GivenName:                       optionalString(model.GivenName),
		Surname:                         optionalString(model.Surname),
		Initials:                        optionalString(model.Initials),
		OtherName:                       optionalString(model.OtherName),
		Email:                           optionalString(model.Email),
		Office:                          optionalString(model.Office),
		OfficePhone:                     optionalString(model.OfficePhone),
		HomePhone:                       optionalString(model.HomePhone),
		MobilePhone:                     optionalString(model.MobilePhone),
		Fax:                             optionalString(model.Fax),
		HomePage:                        optionalString(model.HomePage),
		StreetAddress:                   optionalString(model.StreetAddress),
		POBox:                           optionalString(model.POBox),
		City:                            optionalString(model.City),
		State:                           optionalString(model.State),
		PostalCode:                      optionalString(model.PostalCode),
		Country:                         optionalString(model.Country),
		Company:                         optionalString(model.Company),
		Department:                      optionalString(model.Department),
		Division:                        optionalString(model.Division),
		Organization:                    optionalString(model.Organization),
		EmployeeID:                      optionalString(model.EmployeeID),
		EmployeeNumber:                  optionalString(model.EmployeeNumber),
		Title:                           optionalString(model.Title),
		HomeDirectory:                   optionalString(model.HomeDirectory),
		HomeDrive:                       optionalString(model.HomeDrive),
		LogonWorkstations:               optionalString(model.LogonWorkstations),
		ScriptPath:                      optionalString(model.ScriptPath),
		ProfilePath:                     optionalString(model.ProfilePath),
		AccountExpirationDate:           optionalString(model.AccountExpirationDate),
		Manager:                         model.Manager.ValueString(),
		Enabled:                         model.Enabled.ValueBool(),
		PasswordNeverExpires:            model.PasswordNeverExpires.ValueBool(),
		CannotChangePassword:            model.CannotChangePassword.ValueBool(),
		SmartCardLogonRequired:          model.SmartCardLogonRequired.ValueBool(),
		TrustedForDelegation:            model.TrustedForDelegation.ValueBool(),
		ProtectedFromAccidentalDeletion: model.ProtectedFromAccidentalDeletion.ValueBool(),
	}
}

// userState keeps the configured path/manager spelling when they resolve to the same
// value, because a slash path or identity string and a distinguished name can denote the
// same place.
func userState(model userResourceModel, user *ad.User) userResourceModel {
	path := types.StringValue(user.Path)
	if user.PathMatch && !model.Path.IsNull() && !model.Path.IsUnknown() {
		path = model.Path
	}

	manager := types.StringNull()
	if user.Manager != "" {
		manager = types.StringValue(user.Manager)
		if user.ManagerMatch && !model.Manager.IsNull() && !model.Manager.IsUnknown() {
			manager = model.Manager
		}
	}

	managerDN := types.StringNull()
	if user.Manager != "" {
		managerDN = types.StringValue(user.Manager)
	}

	return userResourceModel{
		ID:                              types.StringValue(user.GUID),
		Name:                            types.StringValue(user.Name),
		SamAccountName:                  types.StringValue(user.SamAccountName),
		UserPrincipalName:               types.StringValue(user.UserPrincipalName),
		Path:                            path,
		Description:                     stringPointerToTerraform(user.Description),
		DisplayName:                     stringPointerToTerraform(user.DisplayName),
		GivenName:                       stringPointerToTerraform(user.GivenName),
		Surname:                         stringPointerToTerraform(user.Surname),
		Initials:                        stringPointerToTerraform(user.Initials),
		OtherName:                       stringPointerToTerraform(user.OtherName),
		Email:                           stringPointerToTerraform(user.Email),
		Office:                          stringPointerToTerraform(user.Office),
		OfficePhone:                     stringPointerToTerraform(user.OfficePhone),
		HomePhone:                       stringPointerToTerraform(user.HomePhone),
		MobilePhone:                     stringPointerToTerraform(user.MobilePhone),
		Fax:                             stringPointerToTerraform(user.Fax),
		HomePage:                        stringPointerToTerraform(user.HomePage),
		StreetAddress:                   stringPointerToTerraform(user.StreetAddress),
		POBox:                           stringPointerToTerraform(user.POBox),
		City:                            stringPointerToTerraform(user.City),
		State:                           stringPointerToTerraform(user.State),
		PostalCode:                      stringPointerToTerraform(user.PostalCode),
		Country:                         stringPointerToTerraform(user.Country),
		Company:                         stringPointerToTerraform(user.Company),
		Department:                      stringPointerToTerraform(user.Department),
		Division:                        stringPointerToTerraform(user.Division),
		Organization:                    stringPointerToTerraform(user.Organization),
		EmployeeID:                      stringPointerToTerraform(user.EmployeeID),
		EmployeeNumber:                  stringPointerToTerraform(user.EmployeeNumber),
		Title:                           stringPointerToTerraform(user.Title),
		HomeDirectory:                   stringPointerToTerraform(user.HomeDirectory),
		HomeDrive:                       stringPointerToTerraform(user.HomeDrive),
		LogonWorkstations:               stringPointerToTerraform(user.LogonWorkstations),
		ScriptPath:                      stringPointerToTerraform(user.ScriptPath),
		ProfilePath:                     stringPointerToTerraform(user.ProfilePath),
		AccountExpirationDate:           stringPointerToTerraform(user.AccountExpirationDate),
		Manager:                         manager,
		ManagerDN:                       managerDN,
		Enabled:                         types.BoolValue(user.Enabled),
		PasswordNeverExpires:            types.BoolValue(user.PasswordNeverExpires),
		CannotChangePassword:            types.BoolValue(user.CannotChangePassword),
		SmartCardLogonRequired:          types.BoolValue(user.SmartCardLogonRequired),
		TrustedForDelegation:            types.BoolValue(user.TrustedForDelegation),
		ProtectedFromAccidentalDeletion: types.BoolValue(user.ProtectedFromAccidentalDeletion),
		DistinguishedName:               types.StringValue(user.DistinguishedName),
		SID:                             types.StringValue(user.SID),
	}
}
