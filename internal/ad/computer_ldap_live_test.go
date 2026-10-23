package ad

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"
)

// TestComputerReadLDAPParity creates a computer account exercising the ACL field, the UAC flags,
// msDS-SupportedEncryptionTypes (encryption types plus the compound-identity bit), SPNs and the
// managed_by/sam/path match fields, then compares the LDAP read against the module-computed
// result from EnsureComputer. Host-gated: set ADDS_HOST. Created under CN=Computers, cleaned up.
func TestComputerReadLDAPParity(t *testing.T) {
	c := liveADClient(t)
	ctx := context.Background()

	p := func(s string) *string { return &s }
	deref := func(sp *string) string {
		if sp == nil {
			return "<nil>"
		}
		return *sp
	}
	sortedEqual := func(a, b []string) bool {
		a = append([]string(nil), a...)
		b = append([]string(nil), b...)
		sort.Strings(a)
		sort.Strings(b)
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	suffix := time.Now().UnixNano()
	name := fmt.Sprintf("ldapc-%d", suffix%100000000)

	input := ComputerInput{
		Name:                            name,
		SamAccountName:                  p(name),
		DNSHostName:                     p(name + ".example.test"),
		Path:                            "CN=Computers",
		Description:                     p("computer parity desc"),
		DisplayName:                     p(name + " disp"),
		Location:                        p("Rack 1"),
		UserPrincipalName:               p("host/" + name + ".example.test"),
		ManagedBy:                       "Administrator",
		Enabled:                         false,
		KerberosEncryptionType:          []string{"AES128", "AES256"},
		ServicePrincipalNames:           []string{"HOST/" + name, "HOST/" + name + ".example.test"},
		TrustedForDelegation:            false,
		AccountNotDelegated:             true,
		CompoundIdentitySupported:       true,
		ProtectedFromAccidentalDeletion: true,
	}

	oracle, err := EnsureComputer(ctx, c, input)
	if err != nil {
		t.Fatalf("EnsureComputer: %v", err)
	}
	t.Cleanup(func() {
		if err := DeleteComputer(context.Background(), c, oracle.GUID); err != nil {
			t.Errorf("cleanup DeleteComputer(%s): %v", oracle.GUID, err)
		}
	})

	ldap, err := ReadComputer(ctx, c, oracle.GUID, input)
	if err != nil {
		t.Fatalf("ReadComputer: %v", err)
	}
	if !ldap.Exists {
		t.Fatalf("LDAP read reported the computer as missing")
	}

	strCases := []struct {
		name string
		got  string
		want string
	}{
		{"name", ldap.Name, oracle.Name},
		{"sam_account_name", ldap.SamAccountName, oracle.SamAccountName},
		{"sam_account_name_stripped", ldap.SamAccountNameStripped, oracle.SamAccountNameStripped},
		{"dns_host_name", deref(ldap.DNSHostName), deref(oracle.DNSHostName)},
		{"path", ldap.Path, oracle.Path},
		{"container_dn", ldap.ContainerDN, oracle.ContainerDN},
		{"distinguished_name", ldap.DistinguishedName, oracle.DistinguishedName},
		{"guid", ldap.GUID, oracle.GUID},
		{"sid", ldap.SID, oracle.SID},
		{"description", deref(ldap.Description), deref(oracle.Description)},
		{"display_name", deref(ldap.DisplayName), deref(oracle.DisplayName)},
		{"location", deref(ldap.Location), deref(oracle.Location)},
		{"user_principal_name", deref(ldap.UserPrincipalName), deref(oracle.UserPrincipalName)},
		{"managed_by", ldap.ManagedBy, oracle.ManagedBy},
		{"operating_system", deref(ldap.OperatingSystem), deref(oracle.OperatingSystem)},
		{"operating_system_version", deref(ldap.OperatingSystemVersion), deref(oracle.OperatingSystemVersion)},
	}
	for _, tc := range strCases {
		if tc.got != tc.want {
			t.Errorf("%s: LDAP=%q module=%q", tc.name, tc.got, tc.want)
		}
	}

	boolCases := []struct {
		name string
		got  bool
		want bool
	}{
		{"sam_match", ldap.SamMatch, oracle.SamMatch},
		{"path_match", ldap.PathMatch, oracle.PathMatch},
		{"managed_by_match", ldap.ManagedByMatch, oracle.ManagedByMatch},
		{"enabled", ldap.Enabled, oracle.Enabled},
		{"trusted_for_delegation", ldap.TrustedForDelegation, oracle.TrustedForDelegation},
		{"account_not_delegated", ldap.AccountNotDelegated, oracle.AccountNotDelegated},
		{"compound_identity_supported", ldap.CompoundIdentitySupported, oracle.CompoundIdentitySupported},
		{"protected_from_accidental_deletion", ldap.ProtectedFromAccidentalDeletion, oracle.ProtectedFromAccidentalDeletion},
	}
	for _, tc := range boolCases {
		if tc.got != tc.want {
			t.Errorf("%s: LDAP=%v module=%v", tc.name, tc.got, tc.want)
		}
	}

	if !sortedEqual(ldap.KerberosEncryptionType, oracle.KerberosEncryptionType) {
		t.Errorf("kerberos_encryption_type: LDAP=%v module=%v", ldap.KerberosEncryptionType, oracle.KerberosEncryptionType)
	}
	if !sortedEqual(ldap.ServicePrincipalNames, oracle.ServicePrincipalNames) {
		t.Errorf("service_principal_names: LDAP=%v module=%v", ldap.ServicePrincipalNames, oracle.ServicePrincipalNames)
	}
}
