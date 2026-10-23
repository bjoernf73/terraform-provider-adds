terraform {
  required_version = ">= 1.6.0"

  required_providers {
    adlc = {
      source  = "bjoernf73/adlc"
      version = "0.0.0-ci"
    }
  }
}

provider "adlc" {
  transport            = var.transport
  host                 = var.host
  port                 = var.port
  username             = var.username
  password             = var.password
  winrm_use_tls        = var.winrm_use_tls
  winrm_auth           = var.winrm_auth
  winrm_kerberos_realm = var.winrm_kerberos_realm
  insecure             = var.insecure
  powershell_path      = var.powershell_path
  timeout_seconds      = var.timeout_seconds
}

data "adlc_domain" "current" {}

resource "adlc_organizational_unit" "smoke" {
  path           = var.ou_path
  description    = var.ou_description
  delete_subtree = true
}

resource "adlc_group" "smoke" {
  name         = var.group_name
  path         = adlc_organizational_unit.smoke.path
  description  = var.ou_description
  display_name = "${var.group_name} (CI)"
  info         = "Created by the terraform-provider-adlc pipeline."
  managed_by   = "Administrator"
  category     = "Security"
  scope        = var.group_scope
}

output "id" {
  value = adlc_organizational_unit.smoke.id
}

output "distinguished_name" {
  value = adlc_organizational_unit.smoke.distinguished_name
}

output "name" {
  value = adlc_organizational_unit.smoke.name
}

output "group_id" {
  value = adlc_group.smoke.id
}

output "group_distinguished_name" {
  value = adlc_group.smoke.distinguished_name
}

output "group_sid" {
  value = adlc_group.smoke.sid
}

output "group_managed_by_dn" {
  value = adlc_group.smoke.managed_by_dn
}

# Delegates computer management on the OU to the group stored inside it, which is the
# dependency shape that a nested acl attribute could not express without a cycle.
# Constructor 6: object_type + inheritance + inherited_object_type.
resource "adlc_access_rule" "smoke" {
  target                = adlc_organizational_unit.smoke.distinguished_name
  trustee               = adlc_group.smoke.sid
  rights                = ["CreateChild", "DeleteChild"]
  access                = "Allow"
  object_type           = "computer"
  inherited_object_type = "organizationalUnit"
  inheritance           = "Descendents"
}

output "access_rule_id" {
  value = adlc_access_rule.smoke.id
}

output "access_rule_trustee_sid" {
  value = adlc_access_rule.smoke.trustee_sid
}

# A second group, nested into the first, exercises group membership.
resource "adlc_group" "smoke_member" {
  name  = "${var.group_name}-member"
  path  = adlc_organizational_unit.smoke.path
  scope = "Global"
}

resource "adlc_group_member" "smoke" {
  group  = adlc_group.smoke.id
  member = adlc_group.smoke_member.id
}

output "group_member_id" {
  value = adlc_group_member.smoke.id
}

# Security principal referenced by the "Domain - GPO2" backup's migration table (see
# adlc_backup_gpo.domain_gpo2 below); its restricted-groups setting resolves this by
# name in the target domain. Its name is a fixed literal (it must match the migration
# table), so only one transport job creates it - ssh and winrm run in parallel within
# the same pipeline, and sAMAccountName uniqueness is domain-wide, not per-OU.
resource "adlc_group" "right_adm_builtingroup_remotedesktopusers" {
  count = var.transport == "ssh" ? 1 : 0

  name     = "Right-ADM-BuiltinGroup-RemoteDesktopUsers"
  path     = adlc_organizational_unit.smoke.path
  category = "Security"
  scope    = "Global"
}

output "right_adm_builtingroup_remotedesktopusers_sid" {
  value = try(adlc_group.right_adm_builtingroup_remotedesktopusers[0].sid, null)
}

# GPO display names aren't namespaced per job like the OU/group/user fixtures above, so
# importing them from both parallel transport jobs would race the same way the group
# above did - only the ssh job imports these, matching that gating.

# Imported from a real GPMC backup (test/e2e/backup_gpo/Domain - GPO2); the migration
# table mixes same_as_source (resolve fresh by name) with an explicit destination built
# from the target domain, for the one entry we want redirected rather than re-resolved.
resource "adlc_backup_gpo" "domain_gpo2" {
  count = var.transport == "ssh" ? 1 : 0

  backup_name = "Domain - GPO2"
  path        = "${path.module}/backup_gpo"
  target_name = "Domain - GPO2"

  migrations = [
    { type = "Unknown", source = "Remote Desktop Users", same_as_source = true },
    {
      type        = "LocalGroup"
      source      = "Right-ADM-BuiltinGroup-RemoteDesktopUsers@utv.local"
      destination = "Right-ADM-BuiltinGroup-RemoteDesktopUsers@${data.adlc_domain.current.dns_root}"
    },
    { type = "UniversalGroup", source = "Enterprise Admins@utv.local", same_as_source = true },
    { type = "GlobalGroup", source = "Domain Admins@utv.local", same_as_source = true },
  ]

  depends_on = [adlc_group.right_adm_builtingroup_remotedesktopusers]
}

# Imported as-is: no migration table for "Domain - GPO3".
resource "adlc_backup_gpo" "domain_gpo3" {
  count = var.transport == "ssh" ? 1 : 0

  backup_name = "Domain - GPO3"
  path        = "${path.module}/backup_gpo"
  target_name = "Domain - GPO3"
}

data "adlc_json_gpo_export" "domain_gpo3" {
  count = var.transport == "ssh" ? 1 : 0

  name = adlc_backup_gpo.domain_gpo3[0].target_name

  lifecycle {
    postcondition {
      condition = (
        lower(self.id) == lower(adlc_backup_gpo.domain_gpo3[0].id) &&
        jsondecode(self.json).Name == adlc_backup_gpo.domain_gpo3[0].target_name
      )
      error_message = "GPO export must identify the imported Domain - GPO3 policy."
    }

    postcondition {
      condition = (
        jsondecode(self.json).ComputerSettingsEnabled == true &&
        jsondecode(self.json).UserSettingsEnabled == false
      )
      error_message = "GPO3 export must preserve computer-enabled and user-disabled settings."
    }

    postcondition {
      condition = alltrue([
        for line in concat(
          jsondecode(self.json).PolicySettings.GPOComments,
          jsondecode(self.json).PolicySettings.MachineComments,
          jsondecode(self.json).PolicySettings.UserComments,
        ) : can(tostring(line))
      ])
      error_message = "GPO export comments must be plain strings, not PowerShell metadata objects."
    }
  }
}

data "adlc_backup_gpo_export" "domain_gpo3" {
  count = var.transport == "ssh" ? 1 : 0

  name = adlc_backup_gpo.domain_gpo3[0].target_name

  lifecycle {
    postcondition {
      condition     = lower(self.id) == lower(adlc_backup_gpo.domain_gpo3[0].id)
      error_message = "Backup export must identify the imported Domain - GPO3 policy."
    }

    postcondition {
      condition = alltrue([
        for filename in ["Backup.xml", "gpreport.xml"] :
        anytrue([
          for exported_path in nonsensitive(keys(self.files)) :
          lower(exported_path) == lower("{${self.backup_id}}/${filename}")
        ])
      ])
      error_message = "GPO3 backup export must contain native GPMC backup metadata."
    }
  }
}

# Links the freshly imported GPO onto the smoke OU, exercising the create path (New-GPLink).
# ssh-gated to match the GPO imports it references.
resource "adlc_gpo_links" "smoke" {
  count = var.transport == "ssh" ? 1 : 0

  target = adlc_organizational_unit.smoke.path
  links = [
    {
      gpo = adlc_backup_gpo.domain_gpo3[0].id
    },
  ]
}

output "gpo_links_id" {
  value = try(adlc_gpo_links.smoke[0].id, null)
}

# Replication site and subnets (forest-wide Configuration partition). The parallel
# winrm/ssh jobs must not collide: the site name is namespaced by group_name (already
# pipeline- and transport-unique), but subnet CIDRs can't be. The /24 third octet is
# therefore derived from the pipeline id embedded in group_name plus a per-transport
# offset, kept inside 172.16.0.0/16 and below the showcase's fixed .250-.252 block.
locals {
  e2e_pipeline_id = try(tonumber(regex("adlc-ci-(\\d+)-", var.group_name)[0]), 0)
  e2e_subnet_base = (local.e2e_pipeline_id % 40) * 6 + (var.transport == "ssh" ? 3 : 0)
}

resource "adlc_site" "smoke" {
  name        = "${var.group_name}-site"
  description = var.ou_description
}

# Single subnet on the smoke site (single-item read path).
resource "adlc_subnet" "smoke" {
  name        = "172.16.${local.e2e_subnet_base}.0/24"
  site        = adlc_site.smoke.name
  description = var.ou_description
  location    = "CI/${var.transport}"
}

# Two more subnets so the single-vs-multiple read path is exercised on the same site.
resource "adlc_subnet" "smoke_extra" {
  for_each = toset([
    "172.16.${local.e2e_subnet_base + 1}.0/24",
    "172.16.${local.e2e_subnet_base + 2}.0/24",
  ])

  name = each.value
  site = adlc_site.smoke.name
}

output "site_id" {
  value = adlc_site.smoke.id
}

output "site_distinguished_name" {
  value = adlc_site.smoke.distinguished_name
}

output "subnet_ids" {
  value = concat([adlc_subnet.smoke.id], [for s in adlc_subnet.smoke_extra : s.id])
}

output "backup_gpos" {
  value = {
    domain_gpo2 = try({ id = adlc_backup_gpo.domain_gpo2[0].id, dn = adlc_backup_gpo.domain_gpo2[0].distinguished_name }, null)
    domain_gpo3 = try({ id = adlc_backup_gpo.domain_gpo3[0].id, dn = adlc_backup_gpo.domain_gpo3[0].distinguished_name }, null)
  }
}

output "domain_dn" {
  value = data.adlc_domain.current.distinguished_name
}

output "domain_netbios_name" {
  value = data.adlc_domain.current.netbios_name
}

# A relative DN target: the well-known Computers container is a CN, not an OU.
# Constructor 5: object_type + inheritance, no inherited_object_type.
resource "adlc_access_rule" "smoke_container" {
  target      = "CN=Computers"
  trustee     = adlc_group.smoke.sid
  rights      = ["ReadProperty"]
  object_type = "All"
  inheritance = "Descendents"
}

output "access_rule_container_target_dn" {
  value = adlc_access_rule.smoke_container.target_dn
}

# The remaining four constructors from the access rules guide, exercised against the
# second group so every ACE key stays unique alongside adlc_access_rule.smoke above.

# Constructor 1: rights on the object itself. Uses a different trustee than constructor 2:
# the same trustee + same rights with inheritance=All would make this ACE a strict subset
# of that one (All already covers the object itself, not just descendants), and Windows
# canonicalizes the redundant narrower ACE away.
resource "adlc_access_rule" "constructor1_object_only" {
  target  = adlc_organizational_unit.smoke.distinguished_name
  trustee = adlc_group.smoke.sid
  rights  = ["GenericRead"]
}

# Constructor 2: rights inherited by every descendant.
resource "adlc_access_rule" "constructor2_all_descendants" {
  target      = adlc_organizational_unit.smoke.distinguished_name
  trustee     = adlc_group.smoke_member.sid
  rights      = ["GenericRead"]
  inheritance = "All"
}

# Constructor 3: rights inherited by one class of descendant.
resource "adlc_access_rule" "constructor3_one_class_descendants" {
  target                = adlc_organizational_unit.smoke.distinguished_name
  trustee               = adlc_group.smoke_member.sid
  rights                = ["GenericRead"]
  inheritance           = "Descendents"
  inherited_object_type = "user"
}

# Constructor 4: rights on one class of child object, this object only.
resource "adlc_access_rule" "constructor4_one_class_here" {
  target      = adlc_organizational_unit.smoke.distinguished_name
  trustee     = adlc_group.smoke_member.sid
  rights      = ["CreateChild", "DeleteChild"]
  object_type = "computer"
}

output "access_rule_constructors" {
  value = {
    "1_object_only"           = adlc_access_rule.constructor1_object_only.id
    "2_all_descendants"       = adlc_access_rule.constructor2_all_descendants.id
    "3_one_class_descendants" = adlc_access_rule.constructor3_one_class_descendants.id
    "4_one_class_here"        = adlc_access_rule.constructor4_one_class_here.id
    "5_one_class_propagated"  = adlc_access_rule.smoke_container.id
    "6_extended_right"        = adlc_access_rule.smoke.id
  }
}

resource "adlc_user" "smoke" {
  name = "${var.group_name}-user"
  # sAMAccountName has a hard 20 character limit; group_name already varies in length
  # by transport name and pipeline id, so truncate rather than risk "not a properly
  # formed account name" on a long combination.
  sam_account_name    = substr("${var.group_name}-usr", 0, 20)
  user_principal_name = "${substr("${var.group_name}-usr", 0, 20)}@${data.adlc_domain.current.dns_root}"
  path                = adlc_organizational_unit.smoke.path
  description         = var.ou_description
  display_name        = "${var.group_name} (CI)"
  given_name          = "CI"
  surname             = "Smoke"
  title               = "Smoke Test"
  manager             = "Administrator"
  enabled             = false
}

output "user_id" {
  value = adlc_user.smoke.id
}

output "user_distinguished_name" {
  value = adlc_user.smoke.distinguished_name
}

output "user_manager_dn" {
  value = adlc_user.smoke.manager_dn
}

resource "adlc_user_password" "smoke" {
  user   = adlc_user.smoke.id
  length = 20
}

output "user_password_id" {
  value = adlc_user_password.smoke.id
}

output "user_password_length" {
  value     = length(adlc_user_password.smoke.password)
  sensitive = true
}
