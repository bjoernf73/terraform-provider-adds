# Look up an existing organizational unit that Terraform does not manage, for example one
# created during the domain build, and place new objects inside it.
data "adds_organizational_unit" "servers" {
  path = "Contoso/Servers"
}

resource "adds_group" "server_admins" {
  name  = "Server Admins"
  path  = data.adds_organizational_unit.servers.distinguished_name
  scope = "DomainLocal"
}

output "servers_ou" {
  value = {
    distinguished_name = data.adds_organizational_unit.servers.distinguished_name
    name               = data.adds_organizational_unit.servers.name
  }
}
