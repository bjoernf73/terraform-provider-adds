package ad

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestUserReadLDAPParity creates a user exercising the userAccountControl flags, accountExpires,
// both ACL-derived fields (cannot_change_password, protected_from_accidental_deletion),
// manager_match and the full attribute set, then compares the LDAP read against the
// module-computed result from EnsureUser. Host-gated: set ADDS_HOST. Created under CN=Users and
// cleaned up. The account is left disabled so it needs no password.
func TestUserReadLDAPParity(t *testing.T) {
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
	name := fmt.Sprintf("adds-ldap-user-%d", suffix)
	sam := fmt.Sprintf("adds-u-%d", suffix%100000000)
	upn := sam + "@" + "example.test"

	input := UserInput{
		Name:                            name,
		SamAccountName:                  sam,
		UserPrincipalName:               upn,
		Path:                            "CN=Users",
		Description:                     p("user parity desc"),
		DisplayName:                     p(name + " disp"),
		GivenName:                       p("Giv"),
		Surname:                         p("Sur"),
		Initials:                        p("G.S."),
		OtherName:                       p("Mid"),
		Email:                           p(sam + "@example.test"),
		Office:                          p("Office1"),
		OfficePhone:                     p("555-1000"),
		HomePhone:                       p("555-2000"),
		MobilePhone:                     p("555-3000"),
		Fax:                             p("555-4000"),
		HomePage:                        p("https://example.test/home"),
		StreetAddress:                   p("1 Test St"),
		POBox:                           p("PO 99"),
		City:                            p("Testville"),
		State:                           p("TS"),
		PostalCode:                      p("12345"),
		Country:                         p("US"),
		Company:                         p("TestCo"),
		Department:                      p("TestDept"),
		Division:                        p("TestDiv"),
		Organization:                    p("TestOrg"),
		EmployeeID:                      p("EID-1"),
		EmployeeNumber:                  p("EN-1"),
		Title:                           p("Tester"),
		HomeDirectory:                   p(`\\srv\home`),
		HomeDrive:                       p("H:"),
		LogonWorkstations:               p("WS1,WS2"),
		ScriptPath:                      p("logon.bat"),
		ProfilePath:                     p(`\\srv\profile`),
		AccountExpirationDate:           p("2030-01-01"),
		Manager:                         "Administrator",
		Enabled:                         false,
		PasswordNeverExpires:            true,
		CannotChangePassword:            true,
		SmartCardLogonRequired:          true,
		TrustedForDelegation:            false,
		ProtectedFromAccidentalDeletion: true,
	}

	oracle, err := EnsureUser(ctx, c, input)
	if err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	t.Cleanup(func() {
		if err := DeleteUser(context.Background(), c, oracle.GUID); err != nil {
			t.Errorf("cleanup DeleteUser(%s): %v", oracle.GUID, err)
		}
	})

	ldap, err := ReadUser(ctx, c, oracle.GUID, input.Path, input.Manager)
	if err != nil {
		t.Fatalf("ReadUser: %v", err)
	}
	if !ldap.Exists {
		t.Fatalf("LDAP read reported the user as missing")
	}

	strCases := []struct {
		name string
		got  string
		want string
	}{
		{"name", ldap.Name, oracle.Name},
		{"sam_account_name", ldap.SamAccountName, oracle.SamAccountName},
		{"user_principal_name", ldap.UserPrincipalName, oracle.UserPrincipalName},
		{"path", ldap.Path, oracle.Path},
		{"container_dn", ldap.ContainerDN, oracle.ContainerDN},
		{"distinguished_name", ldap.DistinguishedName, oracle.DistinguishedName},
		{"guid", ldap.GUID, oracle.GUID},
		{"sid", ldap.SID, oracle.SID},
		{"manager", ldap.Manager, oracle.Manager},
		{"description", deref(ldap.Description), deref(oracle.Description)},
		{"display_name", deref(ldap.DisplayName), deref(oracle.DisplayName)},
		{"given_name", deref(ldap.GivenName), deref(oracle.GivenName)},
		{"surname", deref(ldap.Surname), deref(oracle.Surname)},
		{"initials", deref(ldap.Initials), deref(oracle.Initials)},
		{"other_name", deref(ldap.OtherName), deref(oracle.OtherName)},
		{"email", deref(ldap.Email), deref(oracle.Email)},
		{"office", deref(ldap.Office), deref(oracle.Office)},
		{"office_phone", deref(ldap.OfficePhone), deref(oracle.OfficePhone)},
		{"home_phone", deref(ldap.HomePhone), deref(oracle.HomePhone)},
		{"mobile_phone", deref(ldap.MobilePhone), deref(oracle.MobilePhone)},
		{"fax", deref(ldap.Fax), deref(oracle.Fax)},
		{"home_page", deref(ldap.HomePage), deref(oracle.HomePage)},
		{"street_address", deref(ldap.StreetAddress), deref(oracle.StreetAddress)},
		{"po_box", deref(ldap.POBox), deref(oracle.POBox)},
		{"city", deref(ldap.City), deref(oracle.City)},
		{"state", deref(ldap.State), deref(oracle.State)},
		{"postal_code", deref(ldap.PostalCode), deref(oracle.PostalCode)},
		{"country", deref(ldap.Country), deref(oracle.Country)},
		{"company", deref(ldap.Company), deref(oracle.Company)},
		{"department", deref(ldap.Department), deref(oracle.Department)},
		{"division", deref(ldap.Division), deref(oracle.Division)},
		{"organization", deref(ldap.Organization), deref(oracle.Organization)},
		{"employee_id", deref(ldap.EmployeeID), deref(oracle.EmployeeID)},
		{"employee_number", deref(ldap.EmployeeNumber), deref(oracle.EmployeeNumber)},
		{"title", deref(ldap.Title), deref(oracle.Title)},
		{"home_directory", deref(ldap.HomeDirectory), deref(oracle.HomeDirectory)},
		{"home_drive", deref(ldap.HomeDrive), deref(oracle.HomeDrive)},
		{"logon_workstations", deref(ldap.LogonWorkstations), deref(oracle.LogonWorkstations)},
		{"script_path", deref(ldap.ScriptPath), deref(oracle.ScriptPath)},
		{"profile_path", deref(ldap.ProfilePath), deref(oracle.ProfilePath)},
		{"account_expiration_date", deref(ldap.AccountExpirationDate), deref(oracle.AccountExpirationDate)},
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
		{"path_match", ldap.PathMatch, oracle.PathMatch},
		{"manager_match", ldap.ManagerMatch, oracle.ManagerMatch},
		{"enabled", ldap.Enabled, oracle.Enabled},
		{"password_never_expires", ldap.PasswordNeverExpires, oracle.PasswordNeverExpires},
		{"cannot_change_password", ldap.CannotChangePassword, oracle.CannotChangePassword},
		{"smart_card_logon_required", ldap.SmartCardLogonRequired, oracle.SmartCardLogonRequired},
		{"trusted_for_delegation", ldap.TrustedForDelegation, oracle.TrustedForDelegation},
		{"protected_from_accidental_deletion", ldap.ProtectedFromAccidentalDeletion, oracle.ProtectedFromAccidentalDeletion},
	}
	for _, tc := range boolCases {
		if tc.got != tc.want {
			t.Errorf("%s: LDAP=%v module=%v", tc.name, tc.got, tc.want)
		}
	}
}
