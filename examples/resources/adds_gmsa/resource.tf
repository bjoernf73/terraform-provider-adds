resource "adds_gmsa" "websvc" {
  name          = "websvc-gmsa"
  dns_host_name = "websvc.contoso.local"
  path          = "Managed Service Accounts"
  description   = "gMSA for the internal web service farm"

  # Hosts allowed to retrieve the managed password. Usually the computer accounts
  # of the servers running the service, or a group that contains them.
  principals_allowed_to_retrieve_managed_password = [
    adds_group.web_servers.sid,
  ]

  service_principal_names = [
    "HTTP/websvc.contoso.local",
    "HTTP/websvc",
  ]

  kerberos_encryption_type       = ["AES128", "AES256"]
  managed_password_interval_days = 30
  enabled                        = true
}

resource "adds_group" "web_servers" {
  name  = "Web Servers"
  path  = "Contoso/Groups"
  scope = "Global"
}
