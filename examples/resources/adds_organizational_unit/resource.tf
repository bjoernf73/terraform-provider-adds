resource "adds_organizational_unit" "servers" {
  path        = "Contoso/Servers/Windows"
  description = "Windows server OU"
}

# Parent OUs are created on demand, so this creates nothing new.
resource "adds_organizational_unit" "linux" {
  path        = "Contoso/Servers/Linux"
  description = "Linux server OU"
}
