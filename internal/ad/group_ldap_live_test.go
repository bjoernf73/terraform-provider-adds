package ad

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestGroupReadLDAPParity creates a group exercising every field the read reports - including the
// two ACL-derived ones (manager_can_update_membership, protected_from_accidental_deletion) and
// managed_by_match - then compares the LDAP read against the module-computed result returned by
// EnsureGroup. Host-gated: set ADDS_HOST. The group is created under CN=Users and cleaned up.
func TestGroupReadLDAPParity(t *testing.T) {
	c := liveADClient(t)
	ctx := context.Background()

	p := func(s string) *string { return &s }
	deref := func(sp *string) string {
		if sp == nil {
			return "<nil>"
		}
		return *sp
	}

	suffix := time.Now().UnixNano()
	name := fmt.Sprintf("adds-ldap-parity-%d", suffix)
	sam := fmt.Sprintf("adds-ldap-%d", suffix%1000000)

	input := GroupInput{
		Name:                            name,
		SamAccountName:                  sam,
		Path:                            "CN=Users",
		Description:                     p("LDAP parity test group"),
		DisplayName:                     p(name + " (parity)"),
		Mail:                            p(sam + "@example.test"),
		Info:                            p("created by TestGroupReadLDAPParity"),
		Homepage:                        p("https://example.test/" + sam),
		ManagedBy:                       "Administrator",
		ManagerCanUpdateMembership:      true,
		ProtectedFromAccidentalDeletion: true,
		Category:                        "Security",
		Scope:                           "Global",
	}

	oracle, err := EnsureGroup(ctx, c, input)
	if err != nil {
		t.Fatalf("EnsureGroup: %v", err)
	}
	t.Cleanup(func() {
		if err := DeleteGroup(context.Background(), c, oracle.GUID); err != nil {
			t.Errorf("cleanup DeleteGroup(%s): %v", oracle.GUID, err)
		}
	})

	ldap, err := ReadGroup(ctx, c, oracle.GUID, input.Path, input.ManagedBy)
	if err != nil {
		t.Fatalf("ReadGroup: %v", err)
	}

	if !ldap.Exists {
		t.Fatalf("LDAP read reported the group as missing")
	}

	strCases := []struct {
		name string
		got  string
		want string
	}{
		{"name", ldap.Name, oracle.Name},
		{"sam_account_name", ldap.SamAccountName, oracle.SamAccountName},
		{"description", deref(ldap.Description), deref(oracle.Description)},
		{"display_name", deref(ldap.DisplayName), deref(oracle.DisplayName)},
		{"mail", deref(ldap.Mail), deref(oracle.Mail)},
		{"info", deref(ldap.Info), deref(oracle.Info)},
		{"homepage", deref(ldap.Homepage), deref(oracle.Homepage)},
		{"managed_by", ldap.ManagedBy, oracle.ManagedBy},
		{"category", ldap.Category, oracle.Category},
		{"scope", ldap.Scope, oracle.Scope},
		{"path", ldap.Path, oracle.Path},
		{"container_dn", ldap.ContainerDN, oracle.ContainerDN},
		{"distinguished_name", ldap.DistinguishedName, oracle.DistinguishedName},
		{"guid", ldap.GUID, oracle.GUID},
		{"sid", ldap.SID, oracle.SID},
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
		{"managed_by_match", ldap.ManagedByMatch, oracle.ManagedByMatch},
		{"manager_can_update_membership", ldap.ManagerCanUpdateMembership, oracle.ManagerCanUpdateMembership},
		{"protected_from_accidental_deletion", ldap.ProtectedFromAccidentalDeletion, oracle.ProtectedFromAccidentalDeletion},
		{"path_match", ldap.PathMatch, oracle.PathMatch},
	}
	for _, tc := range boolCases {
		if tc.got != tc.want {
			t.Errorf("%s: LDAP=%v module=%v", tc.name, tc.got, tc.want)
		}
	}
}
