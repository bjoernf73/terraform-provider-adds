# Export the explicit ACLs of an entire OU subtree and write them to a .tf file for review.
data "adds_acl_export" "servers" {
  path         = "Contoso/Servers"
  recursive    = true
  object_types = ["organizationalUnit", "container"]
}

# Persist the rendered adds_acl configuration. Requires the hashicorp/local provider.
resource "local_file" "servers_acls" {
  filename = "${path.module}/imported/servers-acls.tf"
  content  = data.adds_acl_export.servers.hcl
}

# Export a single object's ACL (no recursion).
data "adds_acl_export" "one_ou" {
  path = "Contoso/Servers"
}

# Export every object's ACL in the whole domain, using the domain root as the path.
data "adds_domain" "current" {}

data "adds_acl_export" "domain" {
  path      = data.adds_domain.current.domain_path
  recursive = true
}

output "domain_acl_hcl" {
  value = data.adds_acl_export.domain.hcl
}
