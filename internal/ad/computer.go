package ad

import (
	"context"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	computerCommon   = "computer_common.ps1"
	computerEnsure   = "computer_ensure.ps1"
	computerRead     = "computer_read.ps1"
	computerUpdate   = "computer_update.ps1"
	computerDelete   = "computer_delete.ps1"
	computerDataRead = "computer_data_read.ps1"
)

type Computer struct {
	Name                            string   `json:"name"`
	SamAccountName                  string   `json:"sam_account_name"`
	SamAccountNameStripped          string   `json:"sam_account_name_stripped"`
	SamMatch                        bool     `json:"sam_match"`
	DNSHostName                     *string  `json:"dns_host_name"`
	Path                            string   `json:"path"`
	PathMatch                       bool     `json:"path_match"`
	ContainerDN                     string   `json:"container_dn"`
	DistinguishedName               string   `json:"distinguished_name"`
	GUID                            string   `json:"guid"`
	SID                             string   `json:"sid"`
	Description                     *string  `json:"description"`
	DisplayName                     *string  `json:"display_name"`
	Location                        *string  `json:"location"`
	UserPrincipalName               *string  `json:"user_principal_name"`
	ManagedBy                       string   `json:"managed_by"`
	ManagedByMatch                  bool     `json:"managed_by_match"`
	Enabled                         bool     `json:"enabled"`
	KerberosEncryptionType          []string `json:"kerberos_encryption_type"`
	ServicePrincipalNames           []string `json:"service_principal_names"`
	TrustedForDelegation            bool     `json:"trusted_for_delegation"`
	AccountNotDelegated             bool     `json:"account_not_delegated"`
	CompoundIdentitySupported       bool     `json:"compound_identity_supported"`
	OperatingSystem                 *string  `json:"operating_system"`
	OperatingSystemVersion          *string  `json:"operating_system_version"`
	ProtectedFromAccidentalDeletion bool     `json:"protected_from_accidental_deletion"`
	Exists                          bool     `json:"exists"`
}

// ComputerInput carries the reconcilable attributes of a computer account.
type ComputerInput struct {
	Name                            string
	SamAccountName                  *string
	DNSHostName                     *string
	Path                            string
	Description                     *string
	DisplayName                     *string
	Location                        *string
	UserPrincipalName               *string
	ManagedBy                       string
	Enabled                         bool
	KerberosEncryptionType          []string
	ServicePrincipalNames           []string
	TrustedForDelegation            bool
	AccountNotDelegated             bool
	CompoundIdentitySupported       bool
	ProtectedFromAccidentalDeletion bool
}

func (i ComputerInput) payload() map[string]any {
	var sam *string
	if i.SamAccountName != nil {
		trimmed := strings.TrimSpace(*i.SamAccountName)
		sam = &trimmed
	}

	return map[string]any{
		"name":                               strings.TrimSpace(i.Name),
		"sam_account_name":                   sam,
		"dns_host_name":                      i.DNSHostName,
		"path":                               NormalizePath(i.Path),
		"description":                        i.Description,
		"display_name":                       i.DisplayName,
		"location":                           i.Location,
		"user_principal_name":                i.UserPrincipalName,
		"managed_by":                         strings.TrimSpace(i.ManagedBy),
		"enabled":                            i.Enabled,
		"kerberos_encryption_type":           i.KerberosEncryptionType,
		"service_principal_names":            i.ServicePrincipalNames,
		"trusted_for_delegation":             i.TrustedForDelegation,
		"account_not_delegated":              i.AccountNotDelegated,
		"compound_identity_supported":        i.CompoundIdentitySupported,
		"protected_from_accidental_deletion": i.ProtectedFromAccidentalDeletion,
	}
}

func EnsureComputer(ctx context.Context, c *client.Client, input ComputerInput) (*Computer, error) {
	script, err := buildScript(c, input.payload(), commonScript, computerCommon, computerEnsure)
	if err != nil {
		return nil, err
	}

	var result Computer
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadComputer(ctx context.Context, c *client.Client, guid string, input ComputerInput) (*Computer, error) {
	payload := input.payload()
	payload["guid"] = guid

	script, err := buildScript(c, payload, commonScript, computerCommon, computerRead)
	if err != nil {
		return nil, err
	}

	var result Computer
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func UpdateComputer(ctx context.Context, c *client.Client, guid string, input ComputerInput) (*Computer, error) {
	payload := input.payload()
	payload["guid"] = guid

	script, err := buildScript(c, payload, commonScript, computerCommon, computerUpdate)
	if err != nil {
		return nil, err
	}

	var result Computer
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ReadComputerByIdentity looks up an existing computer account by any AD identity
// (distinguished name, objectGUID, SID, DOMAIN\name or sAMAccountName). It errors when
// no matching account exists; the data source treats a missing computer as a hard failure.
func ReadComputerByIdentity(ctx context.Context, c *client.Client, identity string) (*Computer, error) {
	script, err := buildScript(c, map[string]any{
		"identity": strings.TrimSpace(identity),
	}, commonScript, computerCommon, computerDataRead)
	if err != nil {
		return nil, err
	}

	var result Computer
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteComputer(ctx context.Context, c *client.Client, guid string) error {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, computerCommon, computerDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}
