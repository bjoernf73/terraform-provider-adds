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
	_ datasource.DataSource              = &userDataSource{}
	_ datasource.DataSourceWithConfigure = &userDataSource{}
)

func NewUserDataSource() datasource.DataSource {
	return &userDataSource{}
}

type userDataSource struct {
	client *client.Client
}

type userDataSourceModel struct {
	Identity              types.String `tfsdk:"identity"`
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	SamAccountName        types.String `tfsdk:"sam_account_name"`
	UserPrincipalName     types.String `tfsdk:"user_principal_name"`
	Path                  types.String `tfsdk:"path"`
	DistinguishedName     types.String `tfsdk:"distinguished_name"`
	SID                   types.String `tfsdk:"sid"`
	Description           types.String `tfsdk:"description"`
	DisplayName           types.String `tfsdk:"display_name"`
	GivenName             types.String `tfsdk:"given_name"`
	Surname               types.String `tfsdk:"surname"`
	Initials              types.String `tfsdk:"initials"`
	OtherName             types.String `tfsdk:"other_name"`
	Email                 types.String `tfsdk:"email"`
	Office                types.String `tfsdk:"office"`
	OfficePhone           types.String `tfsdk:"office_phone"`
	HomePhone             types.String `tfsdk:"home_phone"`
	MobilePhone           types.String `tfsdk:"mobile_phone"`
	Fax                   types.String `tfsdk:"fax"`
	HomePage              types.String `tfsdk:"home_page"`
	StreetAddress         types.String `tfsdk:"street_address"`
	POBox                 types.String `tfsdk:"po_box"`
	City                  types.String `tfsdk:"city"`
	State                 types.String `tfsdk:"state"`
	PostalCode            types.String `tfsdk:"postal_code"`
	Country               types.String `tfsdk:"country"`
	Company               types.String `tfsdk:"company"`
	Department            types.String `tfsdk:"department"`
	Division              types.String `tfsdk:"division"`
	Organization          types.String `tfsdk:"organization"`
	EmployeeID            types.String `tfsdk:"employee_id"`
	EmployeeNumber        types.String `tfsdk:"employee_number"`
	Title                 types.String `tfsdk:"title"`
	HomeDirectory         types.String `tfsdk:"home_directory"`
	HomeDrive             types.String `tfsdk:"home_drive"`
	LogonWorkstations     types.String `tfsdk:"logon_workstations"`
	ScriptPath            types.String `tfsdk:"script_path"`
	ProfilePath           types.String `tfsdk:"profile_path"`
	AccountExpirationDate types.String `tfsdk:"account_expiration_date"`
	Manager               types.String `tfsdk:"manager"`
	Enabled               types.Bool   `tfsdk:"enabled"`
	PasswordNeverExpires  types.Bool   `tfsdk:"password_never_expires"`
	CannotChangePassword  types.Bool   `tfsdk:"cannot_change_password"`
	SmartCardLogon        types.Bool   `tfsdk:"smart_card_logon_required"`
	TrustedForDelegation  types.Bool   `tfsdk:"trusted_for_delegation"`
}

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{Computed: true, MarkdownDescription: desc}
}

func computedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{Computed: true, MarkdownDescription: desc}
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an existing Active Directory user. Use it to reference users that are not managed by " +
			"this Terraform configuration, for example as a `manager`, a `managed_by` owner, or a principal in an access rule.",
		Attributes: map[string]schema.Attribute{
			"identity": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Identity of the user to look up. Accepts a distinguished name, `objectGUID`, SID, " +
					"`DOMAIN\\name`, `sAMAccountName` or `userPrincipalName`. Reading fails if no user matches.",
			},
			"id":                        computedString("Account `objectGUID`, stable across renames and moves."),
			"name":                      computedString("User name (the `CN`)."),
			"sam_account_name":          computedString("Pre-Windows 2000 logon name."),
			"user_principal_name":       computedString("User principal name."),
			"path":                      computedString("Slash-delimited path of the container holding the user, relative to the domain root."),
			"distinguished_name":        computedString("Distinguished name of the user."),
			"sid":                       computedString("Security identifier of the user."),
			"description":               computedString("Description."),
			"display_name":              computedString("Display name (`displayName`)."),
			"given_name":                computedString("First name (`givenName`)."),
			"surname":                   computedString("Last name (`sn`)."),
			"initials":                  computedString("Initials."),
			"other_name":                computedString("Middle name (`middleName`)."),
			"email":                     computedString("Email address (`mail`)."),
			"office":                    computedString("Office location (`physicalDeliveryOfficeName`)."),
			"office_phone":              computedString("Office phone number."),
			"home_phone":                computedString("Home phone number."),
			"mobile_phone":              computedString("Mobile phone number."),
			"fax":                       computedString("Fax number."),
			"home_page":                 computedString("Web page address (`wWWHomePage`)."),
			"street_address":            computedString("Street address."),
			"po_box":                    computedString("Post office box."),
			"city":                      computedString("City (`l`)."),
			"state":                     computedString("State or province (`st`)."),
			"postal_code":               computedString("Postal code."),
			"country":                   computedString("Country (`c`)."),
			"company":                   computedString("Company."),
			"department":                computedString("Department."),
			"division":                  computedString("Division."),
			"organization":              computedString("Organization (`o`)."),
			"employee_id":               computedString("Employee ID."),
			"employee_number":           computedString("Employee number."),
			"title":                     computedString("Job title."),
			"home_directory":            computedString("Home directory path."),
			"home_drive":                computedString("Home drive letter."),
			"logon_workstations":        computedString("Workstations the user may log on to."),
			"script_path":               computedString("Logon script path."),
			"profile_path":              computedString("Roaming profile path."),
			"account_expiration_date":   computedString("Date the account expires, as `YYYY-MM-DD`. Empty when the account never expires."),
			"manager":                   computedString("Distinguished name of the user's manager."),
			"enabled":                   computedBool("Whether the account is enabled."),
			"password_never_expires":    computedBool("Whether the password never expires."),
			"cannot_change_password":    computedBool("Whether the user is prevented from changing their password."),
			"smart_card_logon_required": computedBool("Whether smart card logon is required."),
			"trusted_for_delegation":    computedBool("Whether the account is trusted for unconstrained Kerberos delegation."),
		},
	}
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config userDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := ad.ReadUserByIdentity(ctx, d.client, config.Identity.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read user", err.Error())
		return
	}

	state := userDataSourceModel{
		Identity:              config.Identity,
		ID:                    types.StringValue(user.GUID),
		Name:                  types.StringValue(user.Name),
		SamAccountName:        types.StringValue(user.SamAccountName),
		UserPrincipalName:     types.StringValue(user.UserPrincipalName),
		Path:                  types.StringValue(user.Path),
		DistinguishedName:     types.StringValue(user.DistinguishedName),
		SID:                   types.StringValue(user.SID),
		Description:           stringPointerToTerraform(user.Description),
		DisplayName:           stringPointerToTerraform(user.DisplayName),
		GivenName:             stringPointerToTerraform(user.GivenName),
		Surname:               stringPointerToTerraform(user.Surname),
		Initials:              stringPointerToTerraform(user.Initials),
		OtherName:             stringPointerToTerraform(user.OtherName),
		Email:                 stringPointerToTerraform(user.Email),
		Office:                stringPointerToTerraform(user.Office),
		OfficePhone:           stringPointerToTerraform(user.OfficePhone),
		HomePhone:             stringPointerToTerraform(user.HomePhone),
		MobilePhone:           stringPointerToTerraform(user.MobilePhone),
		Fax:                   stringPointerToTerraform(user.Fax),
		HomePage:              stringPointerToTerraform(user.HomePage),
		StreetAddress:         stringPointerToTerraform(user.StreetAddress),
		POBox:                 stringPointerToTerraform(user.POBox),
		City:                  stringPointerToTerraform(user.City),
		State:                 stringPointerToTerraform(user.State),
		PostalCode:            stringPointerToTerraform(user.PostalCode),
		Country:               stringPointerToTerraform(user.Country),
		Company:               stringPointerToTerraform(user.Company),
		Department:            stringPointerToTerraform(user.Department),
		Division:              stringPointerToTerraform(user.Division),
		Organization:          stringPointerToTerraform(user.Organization),
		EmployeeID:            stringPointerToTerraform(user.EmployeeID),
		EmployeeNumber:        stringPointerToTerraform(user.EmployeeNumber),
		Title:                 stringPointerToTerraform(user.Title),
		HomeDirectory:         stringPointerToTerraform(user.HomeDirectory),
		HomeDrive:             stringPointerToTerraform(user.HomeDrive),
		LogonWorkstations:     stringPointerToTerraform(user.LogonWorkstations),
		ScriptPath:            stringPointerToTerraform(user.ScriptPath),
		ProfilePath:           stringPointerToTerraform(user.ProfilePath),
		AccountExpirationDate: stringPointerToTerraform(user.AccountExpirationDate),
		Manager:               types.StringValue(user.Manager),
		Enabled:               types.BoolValue(user.Enabled),
		PasswordNeverExpires:  types.BoolValue(user.PasswordNeverExpires),
		CannotChangePassword:  types.BoolValue(user.CannotChangePassword),
		SmartCardLogon:        types.BoolValue(user.SmartCardLogonRequired),
		TrustedForDelegation:  types.BoolValue(user.TrustedForDelegation),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
