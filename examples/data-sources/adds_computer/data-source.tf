# Look up an existing computer account that Terraform does not manage, for example a
# server that was domain-joined by another process.
data "adds_computer" "app_server" {
  identity = "APP01$"
}

# Reference the looked-up account where a security principal is expected, such as the
# hosts allowed to retrieve a gMSA's managed password.
resource "adds_gmsa" "app_svc" {
  name          = "app-svc-gmsa"
  dns_host_name = "app-svc.contoso.local"
  path          = "Managed Service Accounts"

  principals_allowed_to_retrieve_managed_password = [
    data.adds_computer.app_server.sid,
  ]
}

output "app_server" {
  value = {
    distinguished_name = data.adds_computer.app_server.distinguished_name
    sid                = data.adds_computer.app_server.sid
    operating_system   = data.adds_computer.app_server.operating_system
  }
}
