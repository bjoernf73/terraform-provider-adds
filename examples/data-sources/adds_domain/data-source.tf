data "adds_domain" "current" {}

# Build container DNs without hardcoding the domain component.
resource "adds_group" "legacy_readers" {
  name  = "Legacy Readers"
  path  = data.adds_domain.current.users_container
  scope = "DomainLocal"
}

resource "adds_access_rule" "delegate_on_domain_root" {
  target      = data.adds_domain.current.distinguished_name
  trustee     = "Authenticated Users"
  rights      = ["GenericRead"]
  inheritance = "All"
}

output "domain" {
  value = {
    dns_root     = data.adds_domain.current.dns_root
    netbios_name = data.adds_domain.current.netbios_name
    mode         = data.adds_domain.current.domain_mode
  }
}
