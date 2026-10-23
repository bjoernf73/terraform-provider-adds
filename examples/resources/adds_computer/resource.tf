resource "adds_computer" "web01" {
  name        = "web01"
  path        = "Contoso/Servers"
  description = "Pre-staged account for the web01 host"

  dns_host_name = "web01.contoso.local"
  location      = "Oslo DC1"
  managed_by    = adds_group.web_admins.distinguished_name

  service_principal_names = [
    "HOST/web01.contoso.local",
    "HOST/web01",
  ]

  kerberos_encryption_type = ["AES128", "AES256"]
  enabled                  = true
}

resource "adds_group" "web_admins" {
  name  = "Web Admins"
  path  = "Contoso/Groups"
  scope = "Global"
}
