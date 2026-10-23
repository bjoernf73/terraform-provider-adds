# Look up an existing user that Terraform does not manage, for example to use them as the
# manager or owner of objects this configuration creates.
data "adds_user" "department_head" {
  identity = "jsmith"
}

resource "adds_group" "department" {
  name       = "Department Staff"
  path       = "Contoso/Groups"
  scope      = "Global"
  managed_by = data.adds_user.department_head.distinguished_name
}

output "department_head" {
  value = {
    distinguished_name = data.adds_user.department_head.distinguished_name
    sid                = data.adds_user.department_head.sid
    enabled            = data.adds_user.department_head.enabled
  }
}
