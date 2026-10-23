# One enterprise "system": its own OU under Systems, holding Groups, ServiceAccounts and
# Servers OUs, six groups, two server computer accounts, a gMSA per server, and a per-system
# JSON GPO linked onto the Servers OU. Invoked once per system short name by the showcase.

locals {
  system_path = "${var.systems_path}/${var.system}"
  servers     = toset(["prod", "test"])
}

resource "adds_organizational_unit" "system" {
  path        = local.system_path
  description = "System ${var.system}"
}

resource "adds_organizational_unit" "child" {
  for_each = toset(["Groups", "ServiceAccounts", "Servers"])

  path       = "${local.system_path}/${each.value}"
  depends_on = [adds_organizational_unit.system]
}

resource "adds_group" "group" {
  for_each = toset([for n in range(1, 7) : tostring(n)])

  name     = "ShowCase-${var.system}-Group${each.value}"
  path     = adds_organizational_unit.child["Groups"].path
  category = "Security"
  scope    = "Global"
}

resource "adds_computer" "server" {
  for_each = local.servers

  name = "server-${var.system}-${each.value}"
  # A computer sAMAccountName is capped at 15 characters (AD appends the trailing $); the CN
  # above can be longer, so derive a short, unique pre-Windows 2000 name.
  sam_account_name = "srv-${var.system}-${each.value}"
  path             = adds_organizational_unit.child["Servers"].path
}

resource "adds_gmsa" "gmsa" {
  for_each = local.servers

  name          = "gmsa-${var.system}-${each.value}"
  dns_host_name = "gmsa-${var.system}-${each.value}.${var.dns_root}"
  path          = adds_organizational_unit.child["ServiceAccounts"].path

  principals_allowed_to_retrieve_managed_password = [
    adds_computer.server[each.key].sid,
  ]
}

resource "adds_json_gpo" "common" {
  path        = "${path.module}/System-SYS-common.json"
  target_name = "System-${var.system}-common"

  replacements = {
    DomainNB = var.domain_netbios
    System   = var.system
  }

  # The GPO resolves ShowCase-<system>-GroupN principals by name on import, so the groups
  # must already exist.
  depends_on = [adds_group.group]
}

resource "adds_gpo_links" "servers" {
  target = adds_organizational_unit.child["Servers"].path

  links = [
    { gpo = adds_json_gpo.common.id },
  ]
}

output "system_ou_dn" {
  value = adds_organizational_unit.system.distinguished_name
}

output "gpo_id" {
  value = adds_json_gpo.common.id
}
