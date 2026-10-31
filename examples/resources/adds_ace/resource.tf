resource "adds_organizational_unit" "servers" {
  path = "Contoso/Servers"
}

resource "adds_group" "server_admins" {
  name  = "Server Admins"
  path  = "Contoso/Groups"
  scope = "DomainLocal"
}

# Create and delete computer objects in child OUs.
resource "adds_ace" "create_delete_computers" {
  target                = adds_organizational_unit.servers.distinguished_name
  trustee               = adds_group.server_admins.sid
  rights                = ["CreateChild", "DeleteChild"]
  object_type           = "computer"
  inherited_object_type = "organizationalUnit"
  inheritance           = "Descendents"
}

# Write any property on the computer objects themselves.
resource "adds_ace" "write_computer_properties" {
  target                = adds_organizational_unit.servers.distinguished_name
  trustee               = adds_group.server_admins.sid
  rights                = ["WriteProperty"]
  object_type           = "All"
  inherited_object_type = "computer"
  inheritance           = "Descendents"
}

# Built-in principals do not need to be managed by Terraform.
resource "adds_ace" "authenticated_read" {
  target      = adds_organizational_unit.servers.distinguished_name
  trustee     = "Authenticated Users"
  rights      = ["GenericRead"]
  inheritance = "All"
}

# Deny an extended right.
resource "adds_ace" "deny_password_reset" {
  target                = adds_organizational_unit.servers.distinguished_name
  trustee               = "Authenticated Users"
  rights                = ["ExtendedRight"]
  access                = "Deny"
  object_type           = "Reset Password"
  inherited_object_type = "user"
  inheritance           = "Descendents"
}
