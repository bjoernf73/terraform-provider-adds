resource "adds_organizational_unit" "servers" {
  path = "Contoso/Servers"
}

resource "adds_group" "server_admins" {
  name  = "Server Admins"
  path  = "Contoso/Groups"
  scope = "DomainLocal"
}

# Authoritative DACL for the OU. This resource owns every explicit ACE on the OU - any
# explicit ACE not declared below is removed on apply - while inherited ACEs and the
# always-preserved "Protect from accidental deletion" ACE are left in place. Set
# inheritance_blocked = true to additionally break inheritance.
resource "adds_acl" "servers" {
  target = adds_organizational_unit.servers.distinguished_name

  # Server admins can create and delete computer objects in the OU.
  ace {
    trustee               = adds_group.server_admins.sid
    rights                = ["CreateChild", "DeleteChild"]
    object_type           = "computer"
    inherited_object_type = "organizationalUnit"
    inheritance           = "Descendents"
  }

  # ... and write any property on those computer objects.
  ace {
    trustee               = adds_group.server_admins.sid
    rights                = ["WriteProperty"]
    object_type           = "All"
    inherited_object_type = "computer"
    inheritance           = "Descendents"
  }

  # Authenticated users may read the subtree.
  ace {
    trustee     = "Authenticated Users"
    rights      = ["GenericRead"]
    inheritance = "All"
  }

  # Deny resetting user passwords in the subtree.
  ace {
    trustee               = "Authenticated Users"
    access                = "Deny"
    rights                = ["ExtendedRight"]
    object_type           = "Reset Password"
    inherited_object_type = "user"
    inheritance           = "Descendents"
  }
}
