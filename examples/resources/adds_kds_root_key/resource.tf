# Ensure a KDS root key exists before creating any gMSA. Created once per forest.
resource "adds_kds_root_key" "root" {
  # In a single-DC lab, backdate the effective time so gMSAs work immediately.
  # Leave false (the default) in production multi-DC forests and wait for replication.
  effective_immediately = true

  # In a multi-DC forest, force the new key out to every DC at once instead of
  # waiting for normal replication. Combined with effective_immediately, the key is
  # usable across the whole forest right away.
  force_replication = true
}

resource "adds_gmsa" "websvc" {
  name          = "websvc-gmsa"
  dns_host_name = "websvc.contoso.local"
  path          = "Managed Service Accounts"

  principals_allowed_to_retrieve_managed_password = [
    adds_group.web_servers.sid,
  ]

  # Force creation order: the key must exist before the account.
  depends_on = [adds_kds_root_key.root]
}

resource "adds_group" "web_servers" {
  name  = "Web Servers"
  path  = "Contoso/Groups"
  scope = "Global"
}
