package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccComputerResource_basic(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)
	ou := "adds-acc-comp-" + suffix
	name := "acc-c-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccComputerConfigBasic(ou, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("adds_computer.test", "name", name),
					resource.TestCheckResourceAttr("adds_computer.test", "sam_account_name", name),
					resource.TestCheckResourceAttr("adds_computer.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("adds_computer.test", "id"),
					resource.TestCheckResourceAttrSet("adds_computer.test", "distinguished_name"),
					resource.TestCheckResourceAttrSet("adds_computer.test", "sid"),
				),
			},
			{
				ResourceName:      "adds_computer.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccComputerResource_update(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)
	ou := "adds-acc-comp-" + suffix
	name := "acc-c-" + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccComputerConfigFull(ou, name, "first description", "Oslo", false, []string{"HOST/" + name}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("adds_computer.test", "description", "first description"),
					resource.TestCheckResourceAttr("adds_computer.test", "location", "Oslo"),
					resource.TestCheckResourceAttr("adds_computer.test", "enabled", "false"),
					resource.TestCheckResourceAttr("adds_computer.test", "service_principal_names.#", "1"),
					resource.TestCheckResourceAttr("adds_computer.test", "kerberos_encryption_type.#", "2"),
					resource.TestCheckResourceAttrSet("adds_computer.test", "managed_by_dn"),
				),
			},
			{
				Config: testAccComputerConfigFull(ou, name, "second description", "Bergen", true, []string{"HOST/" + name, "HOST/" + name + ".contoso.local"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("adds_computer.test", "description", "second description"),
					resource.TestCheckResourceAttr("adds_computer.test", "location", "Bergen"),
					resource.TestCheckResourceAttr("adds_computer.test", "enabled", "true"),
					resource.TestCheckResourceAttr("adds_computer.test", "service_principal_names.#", "2"),
				),
			},
		},
	})
}

func testAccComputerConfigBasic(ou, name string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "adds_organizational_unit" "test" {
  path           = %[1]q
  description    = "adds computer acceptance test"
  delete_subtree = true
}

resource "adds_computer" "test" {
  name = %[2]q
  path = adds_organizational_unit.test.path
}
`, ou, name)
}

func testAccComputerConfigFull(ou, name, description, location string, enabled bool, spns []string) string {
	quoted := make([]string, len(spns))
	for i, spn := range spns {
		quoted[i] = fmt.Sprintf("%q", spn)
	}

	return testAccProviderConfig() + fmt.Sprintf(`
resource "adds_organizational_unit" "test" {
  path           = %[1]q
  description    = "adds computer acceptance test"
  delete_subtree = true
}

resource "adds_group" "owner" {
  name  = "%[2]s-owner"
  path  = adds_organizational_unit.test.path
  scope = "Global"
}

resource "adds_computer" "test" {
  name        = %[2]q
  path        = adds_organizational_unit.test.path
  description = %[3]q
  location    = %[4]q
  enabled     = %[5]t
  managed_by  = adds_group.owner.distinguished_name

  service_principal_names  = [%[6]s]
  kerberos_encryption_type = ["AES128", "AES256"]
}
`, ou, name, description, location, enabled, joinTerraformList(quoted))
}

func joinTerraformList(items []string) string {
	result := ""
	for i, item := range items {
		if i > 0 {
			result += ", "
		}
		result += item
	}
	return result
}
