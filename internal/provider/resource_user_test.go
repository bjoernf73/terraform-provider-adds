package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccUserResource_enabledCreate verifies that a user configured with enabled = true is
// brought into the enabled state by the resource itself: it sets a throwaway bootstrap
// password on the otherwise passwordless account so Active Directory allows enabling it.
// A second plan-only step guards idempotency (no perpetual enabled drift).
func TestAccUserResource_enabledCreate(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)
	ou := "adlc-acc-user-" + suffix
	name := "acc-u-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfigEnabled(ou, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("adlc_user.test", "name", name),
					resource.TestCheckResourceAttr("adlc_user.test", "sam_account_name", name),
					resource.TestCheckResourceAttr("adlc_user.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("adlc_user.test", "id"),
					resource.TestCheckResourceAttrSet("adlc_user.test", "distinguished_name"),
					resource.TestCheckResourceAttrSet("adlc_user.test", "sid"),
				),
			},
			{
				Config:   testAccUserConfigEnabled(ou, name),
				PlanOnly: true,
			},
		},
	})
}

func testAccUserConfigEnabled(ou, name string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "adlc_organizational_unit" "test" {
  path           = %[1]q
  description    = "adlc user acceptance test"
  delete_subtree = true
}

resource "adlc_user" "test" {
  name                = %[2]q
  sam_account_name    = %[2]q
  user_principal_name = "%[2]s@example.test"
  path                = adlc_organizational_unit.test.path
  enabled             = true
}
`, ou, name)
}
