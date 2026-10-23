package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	gmsaCommon = "gmsa_common.ps1"
	gmsaEnsure = "gmsa_ensure.ps1"
	gmsaRead   = "gmsa_read.ps1"
	gmsaUpdate = "gmsa_update.ps1"
	gmsaDelete = "gmsa_delete.ps1"
)

type GMSA struct {
	Name                                       string   `json:"name"`
	SamAccountName                             string   `json:"sam_account_name"`
	SamAccountNameStripped                     string   `json:"sam_account_name_stripped"`
	SamMatch                                   bool     `json:"sam_match"`
	DNSHostName                                string   `json:"dns_host_name"`
	Path                                       string   `json:"path"`
	PathMatch                                  bool     `json:"path_match"`
	ContainerDN                                string   `json:"container_dn"`
	DistinguishedName                          string   `json:"distinguished_name"`
	GUID                                       string   `json:"guid"`
	SID                                        string   `json:"sid"`
	Description                                *string  `json:"description"`
	DisplayName                                *string  `json:"display_name"`
	HomePage                                   *string  `json:"home_page"`
	Enabled                                    bool     `json:"enabled"`
	KerberosEncryptionType                     []string `json:"kerberos_encryption_type"`
	ManagedPasswordIntervalDays                int64    `json:"managed_password_interval_days"`
	PrincipalsAllowedToRetrieveManagedPassword []string `json:"principals_allowed_to_retrieve_managed_password"`
	PrincipalsAllowedToRetrieveMatch           bool     `json:"principals_allowed_to_retrieve_managed_password_match"`
	PrincipalsAllowedToDelegateToAccount       []string `json:"principals_allowed_to_delegate_to_account"`
	PrincipalsAllowedToDelegateMatch           bool     `json:"principals_allowed_to_delegate_to_account_match"`
	ServicePrincipalNames                      []string `json:"service_principal_names"`
	TrustedForDelegation                       bool     `json:"trusted_for_delegation"`
	AccountNotDelegated                        bool     `json:"account_not_delegated"`
	CompoundIdentitySupported                  bool     `json:"compound_identity_supported"`
	AccountExpirationDate                      *string  `json:"account_expiration_date"`
	ProtectedFromAccidentalDeletion            bool     `json:"protected_from_accidental_deletion"`
	Exists                                     bool     `json:"exists"`
}

// GMSAInput carries the reconcilable attributes of a group managed service account.
type GMSAInput struct {
	Name                                       string
	SamAccountName                             *string
	DNSHostName                                string
	Path                                       string
	Description                                *string
	DisplayName                                *string
	HomePage                                   *string
	Enabled                                    bool
	KerberosEncryptionType                     []string
	ManagedPasswordIntervalDays                *int64
	PrincipalsAllowedToRetrieveManagedPassword []string
	PrincipalsAllowedToDelegateToAccount       []string
	ServicePrincipalNames                      []string
	TrustedForDelegation                       bool
	AccountNotDelegated                        bool
	CompoundIdentitySupported                  bool
	AccountExpirationDate                      *string
	ProtectedFromAccidentalDeletion            bool
}

func (i GMSAInput) payload() map[string]any {
	var sam *string
	if i.SamAccountName != nil {
		trimmed := strings.TrimSpace(*i.SamAccountName)
		sam = &trimmed
	}

	return map[string]any{
		"name":                           strings.TrimSpace(i.Name),
		"sam_account_name":               sam,
		"dns_host_name":                  strings.TrimSpace(i.DNSHostName),
		"path":                           NormalizePath(i.Path),
		"description":                    i.Description,
		"display_name":                   i.DisplayName,
		"home_page":                      i.HomePage,
		"enabled":                        i.Enabled,
		"kerberos_encryption_type":       i.KerberosEncryptionType,
		"managed_password_interval_days": i.ManagedPasswordIntervalDays,
		"principals_allowed_to_retrieve_managed_password": orEmptyStringSlice(i.PrincipalsAllowedToRetrieveManagedPassword),
		"principals_allowed_to_delegate_to_account":       orEmptyStringSlice(i.PrincipalsAllowedToDelegateToAccount),
		"service_principal_names":                         i.ServicePrincipalNames,
		"trusted_for_delegation":                          i.TrustedForDelegation,
		"account_not_delegated":                           i.AccountNotDelegated,
		"compound_identity_supported":                     i.CompoundIdentitySupported,
		"account_expiration_date":                         i.AccountExpirationDate,
		"protected_from_accidental_deletion":              i.ProtectedFromAccidentalDeletion,
	}
}

func orEmptyStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func EnsureGMSA(ctx context.Context, c *client.Client, input GMSAInput) (*GMSA, error) {
	script, err := buildScript(c, input.payload(), commonScript, gmsaCommon, gmsaEnsure)
	if err != nil {
		return nil, err
	}

	var result GMSA
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadGMSA(ctx context.Context, c *client.Client, guid string, input GMSAInput) (*GMSA, error) {
	payload := input.payload()
	payload["guid"] = guid

	script, err := buildScript(c, payload, commonScript, gmsaCommon, gmsaRead)
	if err != nil {
		return nil, err
	}

	var result GMSA
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func UpdateGMSA(ctx context.Context, c *client.Client, guid string, input GMSAInput) (*GMSA, error) {
	payload := input.payload()
	payload["guid"] = guid

	script, err := buildScript(c, payload, commonScript, gmsaCommon, gmsaUpdate)
	if err != nil {
		return nil, err
	}

	var result GMSA
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteGMSA(ctx context.Context, c *client.Client, guid string) error {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, gmsaCommon, gmsaDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}
