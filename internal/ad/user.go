package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	userCommon   = "user_common.ps1"
	userEnsure   = "user_ensure.ps1"
	userRead     = "user_read.ps1"
	userUpdate   = "user_update.ps1"
	userDelete   = "user_delete.ps1"
	userDataRead = "user_data_read.ps1"
)

type User struct {
	Name                            string  `json:"name"`
	SamAccountName                  string  `json:"sam_account_name"`
	UserPrincipalName               string  `json:"user_principal_name"`
	Path                            string  `json:"path"`
	PathMatch                       bool    `json:"path_match"`
	ContainerDN                     string  `json:"container_dn"`
	DistinguishedName               string  `json:"distinguished_name"`
	GUID                            string  `json:"guid"`
	SID                             string  `json:"sid"`
	Description                     *string `json:"description"`
	DisplayName                     *string `json:"display_name"`
	GivenName                       *string `json:"given_name"`
	Surname                         *string `json:"surname"`
	Initials                        *string `json:"initials"`
	OtherName                       *string `json:"other_name"`
	Email                           *string `json:"email"`
	Office                          *string `json:"office"`
	OfficePhone                     *string `json:"office_phone"`
	HomePhone                       *string `json:"home_phone"`
	MobilePhone                     *string `json:"mobile_phone"`
	Fax                             *string `json:"fax"`
	HomePage                        *string `json:"home_page"`
	StreetAddress                   *string `json:"street_address"`
	POBox                           *string `json:"po_box"`
	City                            *string `json:"city"`
	State                           *string `json:"state"`
	PostalCode                      *string `json:"postal_code"`
	Country                         *string `json:"country"`
	Company                         *string `json:"company"`
	Department                      *string `json:"department"`
	Division                        *string `json:"division"`
	Organization                    *string `json:"organization"`
	EmployeeID                      *string `json:"employee_id"`
	EmployeeNumber                  *string `json:"employee_number"`
	Title                           *string `json:"title"`
	HomeDirectory                   *string `json:"home_directory"`
	HomeDrive                       *string `json:"home_drive"`
	LogonWorkstations               *string `json:"logon_workstations"`
	ScriptPath                      *string `json:"script_path"`
	ProfilePath                     *string `json:"profile_path"`
	AccountExpirationDate           *string `json:"account_expiration_date"`
	Manager                         string  `json:"manager"`
	ManagerMatch                    bool    `json:"manager_match"`
	Enabled                         bool    `json:"enabled"`
	PasswordNeverExpires            bool    `json:"password_never_expires"`
	CannotChangePassword            bool    `json:"cannot_change_password"`
	SmartCardLogonRequired          bool    `json:"smart_card_logon_required"`
	TrustedForDelegation            bool    `json:"trusted_for_delegation"`
	ProtectedFromAccidentalDeletion bool    `json:"protected_from_accidental_deletion"`
	Exists                          bool    `json:"exists"`
}

// UserInput carries the reconcilable attributes of a user.
type UserInput struct {
	Name                            string
	SamAccountName                  string
	UserPrincipalName               string
	Path                            string
	Description                     *string
	DisplayName                     *string
	GivenName                       *string
	Surname                         *string
	Initials                        *string
	OtherName                       *string
	Email                           *string
	Office                          *string
	OfficePhone                     *string
	HomePhone                       *string
	MobilePhone                     *string
	Fax                             *string
	HomePage                        *string
	StreetAddress                   *string
	POBox                           *string
	City                            *string
	State                           *string
	PostalCode                      *string
	Country                         *string
	Company                         *string
	Department                      *string
	Division                        *string
	Organization                    *string
	EmployeeID                      *string
	EmployeeNumber                  *string
	Title                           *string
	HomeDirectory                   *string
	HomeDrive                       *string
	LogonWorkstations               *string
	ScriptPath                      *string
	ProfilePath                     *string
	AccountExpirationDate           *string
	Manager                         string
	Enabled                         bool
	PasswordNeverExpires            bool
	CannotChangePassword            bool
	SmartCardLogonRequired          bool
	TrustedForDelegation            bool
	ProtectedFromAccidentalDeletion bool
}

// ReadUserByIdentity looks up an existing user by any AD identity (distinguished name,
// objectGUID, SID, DOMAIN\name, sAMAccountName or userPrincipalName). It errors when no
// matching user exists.
func ReadUserByIdentity(ctx context.Context, c *client.Client, identity string) (*User, error) {
	script, err := buildScript(c, map[string]any{
		"identity": strings.TrimSpace(identity),
	}, commonScript, userCommon, userDataRead)
	if err != nil {
		return nil, err
	}

	var result User
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (i UserInput) payload() map[string]any {
	return map[string]any{
		"name":                strings.TrimSpace(i.Name),
		"sam_account_name":    strings.TrimSpace(i.SamAccountName),
		"user_principal_name": strings.TrimSpace(i.UserPrincipalName),
		"path":                NormalizePath(i.Path),
		"description":         i.Description,
		"display_name":        i.DisplayName,
		"given_name":          i.GivenName,
		"surname":             i.Surname,
		"initials":            i.Initials,
		"other_name":          i.OtherName,
		"email":               i.Email,
		"office":              i.Office,
		"office_phone":        i.OfficePhone,
		"home_phone":          i.HomePhone,
		"mobile_phone":        i.MobilePhone,
		"fax":                 i.Fax,
		"home_page":           i.HomePage,
		"street_address":      i.StreetAddress,
		"po_box":              i.POBox,
		"city":                i.City,
		"state":               i.State,
		"postal_code":         i.PostalCode,
		"country":             i.Country,
		"company":             i.Company,
		"department":          i.Department,
		"division":            i.Division,
		"organization":        i.Organization,
		"employee_id":         i.EmployeeID,
		"employee_number":     i.EmployeeNumber,
		"title":               i.Title,
		"home_directory":      i.HomeDirectory,
		"home_drive":          i.HomeDrive, "logon_workstations": i.LogonWorkstations,
		"script_path":             i.ScriptPath,
		"profile_path":            i.ProfilePath,
		"account_expiration_date": i.AccountExpirationDate, "manager": strings.TrimSpace(i.Manager),
		"enabled":                            i.Enabled,
		"password_never_expires":             i.PasswordNeverExpires,
		"cannot_change_password":             i.CannotChangePassword,
		"smart_card_logon_required":          i.SmartCardLogonRequired,
		"trusted_for_delegation":             i.TrustedForDelegation,
		"protected_from_accidental_deletion": i.ProtectedFromAccidentalDeletion,
	}
}

func EnsureUser(ctx context.Context, c *client.Client, input UserInput) (*User, error) {
	script, err := buildScript(c, input.payload(), commonScript, userCommon, userEnsure)
	if err != nil {
		return nil, err
	}

	var result User
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadUser(ctx context.Context, c *client.Client, guid string, path string, manager string) (*User, error) {
	script, err := buildScript(c, map[string]any{
		"guid":    guid,
		"path":    path,
		"manager": manager,
	}, commonScript, userCommon, userRead)
	if err != nil {
		return nil, err
	}

	var result User
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func UpdateUser(ctx context.Context, c *client.Client, guid string, input UserInput) (*User, error) {
	payload := input.payload()
	payload["guid"] = guid

	script, err := buildScript(c, payload, commonScript, userCommon, userUpdate)
	if err != nil {
		return nil, err
	}

	var result User
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteUser(ctx context.Context, c *client.Client, guid string) error {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, userCommon, userDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}
