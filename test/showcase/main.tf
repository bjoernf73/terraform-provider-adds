terraform {
  required_version = ">= 1.6.0"

  required_providers {
    adds = {
      source  = "bjoernf73/adds"
      version = "0.0.0-ci"
    }
  }
}

provider "adds" {
  transport     = "winrm"
  host          = var.host
  port          = var.port
  username      = var.username
  password      = var.password
  winrm_use_tls = true
  winrm_auth    = var.winrm_auth
  # False validates the listener certificate; true is only for self-signed certificates.
  insecure        = var.insecure
  powershell_path = var.powershell_path
  timeout_seconds = var.timeout_seconds
}

data "adds_domain" "current" {}

# Fixed names: this configuration is applied repeatedly without a persisted state and
# without a destroy, so every run must reconcile the same objects rather than create
# new ones.

resource "adds_organizational_unit" "root" {
  path        = var.showcase_path
  description = "terraform-provider-adds showcase, applied over WinRM/HTTPS"
}

resource "adds_organizational_unit" "child" {
  for_each = toset(["Servers", "Groups", "ServiceAccounts", "Workstations", "Staff"])

  path        = "${var.showcase_path}/${each.value}"
  description = "${each.value} for the showcase"
}

# Every settable property populated, so the result can be inspected in ADUC.
resource "adds_group" "admins" {
  name             = "adds-showcase-admins"
  sam_account_name = "ADDS-SHOWCASE-ADM"
  path             = adds_organizational_unit.child["Groups"].path
  description      = "Showcase administrators"
  display_name     = "ADDS Showcase Administrators"
  mail             = "adds-showcase-admins@${data.adds_domain.current.dns_root}"
  info             = "Every property on this group is set by Terraform."
  homepage         = "https://example.invalid/adds-showcase"
  managed_by       = "Administrator"
  category         = "Security"
  scope            = "DomainLocal"

  manager_can_update_membership      = true
  protected_from_accidental_deletion = true
}

resource "adds_group" "operators" {
  name         = "adds-showcase-operators"
  path         = adds_organizational_unit.child["Groups"].path
  description  = "Showcase operators"
  display_name = "ADDS Showcase Operators"
  category     = "Security"
  scope        = "Global"
}

# Every settable property populated; adds_user enables it via a throwaway bootstrap password,
# and adds_user_password sets the real password afterwards.
resource "adds_user" "showcase" {
  name                = "ADDS Showcase User"
  sam_account_name    = "adds-showcase-user"
  user_principal_name = "adds-showcase-user@${data.adds_domain.current.dns_root}"
  path                = adds_organizational_unit.root.path

  given_name              = "ADDS"
  surname                 = "Showcase"
  display_name            = "ADDS Showcase User"
  initials                = "DS"
  other_name              = "Terraform"
  description             = "Every property on this user is set by Terraform."
  email                   = "adds-showcase-user@${data.adds_domain.current.dns_root}"
  office                  = "Remote"
  office_phone            = "+1 555 0100"
  home_phone              = "+1 555 0101"
  mobile_phone            = "+1 555 0102"
  fax                     = "+1 555 0103"
  home_page               = "https://example.invalid/adds-showcase-user"
  street_address          = "1 Showcase Way"
  po_box                  = "PO Box 1"
  city                    = "Showcase City"
  state                   = "Showcase State"
  postal_code             = "00000"
  country                 = "US"
  company                 = "Contoso"
  department              = "Platform Engineering"
  division                = "Engineering"
  organization            = "Contoso"
  employee_id             = "E-0001"
  employee_number         = "0001"
  title                   = "Showcase Engineer"
  home_directory          = "\\\\fileserver\\home\\adds-showcase-user"
  home_drive              = "H:"
  logon_workstations      = "WORKSTATION1,WORKSTATION2"
  script_path             = "logon.bat"
  profile_path            = "\\\\fileserver\\profiles\\adds-showcase-user"
  account_expiration_date = "2099-12-31"
  manager                 = "Administrator"

  # Enabled on first apply: with no password set yet, adds_user sets a throwaway random
  # password (never stored in state) so Active Directory allows the account to be enabled.
  # adds_user_password.showcase then sets the real password without changing the enabled state.
  enabled                            = true
  password_never_expires             = true
  cannot_change_password             = false
  smart_card_logon_required          = false
  trusted_for_delegation             = false
  protected_from_accidental_deletion = true
}

resource "adds_group" "announcements" {
  name         = "adds-showcase-announcements"
  path         = adds_organizational_unit.child["Groups"].path
  description  = "Showcase distribution group"
  display_name = "ADDS Showcase Announcements"
  mail         = "adds-showcase@${data.adds_domain.current.dns_root}"
  category     = "Distribution"
  scope        = "Universal"
}

# Security principal referenced by the "Domain - GPO1" backup's migration table (see
# adds_backup_gpo.domain_gpo1 below); the GPO's restricted groups setting resolves this
# by name in the target domain.
resource "adds_group" "right_dc_ura_sesystemprofileprivilege" {
  name     = "Right-DC-URA-SeSystemProfilePrivilege"
  path     = adds_organizational_unit.child["Groups"].path
  category = "Security"
  scope    = "Global"
}

resource "adds_group" "right_dc_ura_seshutdownprivilege" {
  name     = "Right-DC-URA-SeShutdownPrivilege"
  path     = adds_organizational_unit.child["Groups"].path
  category = "Security"
  scope    = "Global"
}

# Security principals referenced by the json_gpo fixtures under test/showcase/json_gpo/
# (see dry.module.ad's ####Replace[DOMAIN\Name] token convention): these GPOs resolve
# each name automatically on import, so the groups just need to exist by name here.
# Right-DC-URA-SeSystemProfilePrivilege above is already one of these; the rest follow.
locals {
  json_gpo_principal_groups = toset([
    "Right-DC-BuiltinGroup-AccessControlAssistanceOperators",
    "Right-DC-BuiltinGroup-BackupOperators",
    "Right-DC-BuiltinGroup-CertificateServiceDCOMAccess",
    "Right-DC-BuiltinGroup-CryptographicOperators",
    "Right-DC-BuiltinGroup-DistributedCOMUsers",
    "Right-DC-BuiltinGroup-EventLogReaders",
    "Right-DC-BuiltinGroup-PerformanceLogUsers",
    "Right-DC-BuiltinGroup-PerformanceMonitorUsers",
    "Right-DC-BuiltinGroup-RemoteDesktopUsers",
    "Right-DC-BuiltinGroup-RemoteManagementUsers",
    "Right-DC-BuiltinGroup-Replicator",
    "Right-DC-BuiltinGroup-TerminalServerLicenseServers",
    "Right-DC-BuiltinGroup-WindowsAuthorizationAccessGroup",
    "Right-DC-URA-SeAssignPrimaryTokenPrivilege",
    "Right-DC-URA-SeDelegateSessionUserImpersonatePrivilege",
    "Right-DC-URA-SeIncreaseQuotaPrivilege",
    "Right-DC-URA-SeIncreaseWorkingSetPrivilege",
    "Right-DC-URA-SeRelabelPrivilege",
    "Right-DC-URA-SeRemoteShutdownPrivilege",
    "Right-DC-URA-SeSecurityPrivilege",
  ])
}

resource "adds_group" "json_gpo_principals" {
  for_each = local.json_gpo_principal_groups

  name     = each.value
  path     = adds_organizational_unit.child["Groups"].path
  category = "Security"
  scope    = "Global"
}

# Imported from a real GPMC backup (test/showcase/backup_gpo/Domain - GPO1); the
# migration table entries map security principals baked into the backup by name, so
# they resolve fresh in this domain instead of carrying over the source's SIDs.
resource "adds_backup_gpo" "domain_gpo1" {
  backup_name = "Domain - GPO1"
  path        = "${path.module}/backup_gpo"
  target_name = "Domain - ${data.adds_domain.current.netbios_name} - GPO1"

  migrations = [
    { type = "GlobalGroup", source = "Right-DC-URA-SeSystemProfilePrivilege@utv.local", destination = "${data.adds_domain.current.netbios_name}\\Right-DC-URA-SeSystemProfilePrivilege" },
    { type = "GlobalGroup", source = "Right-DC-URA-SeShutdownPrivilege@utv.local", destination = "${data.adds_domain.current.netbios_name}\\Right-DC-URA-SeShutdownPrivilege" },
  ]

  depends_on = [
    adds_group.right_dc_ura_sesystemprofileprivilege,
    adds_group.right_dc_ura_seshutdownprivilege,
  ]
}

# Imported as-is: no migration table for "Domain - GPO4".
resource "adds_backup_gpo" "domain_gpo4" {
  backup_name = "Domain - GPO4"
  path        = "${path.module}/backup_gpo"
  target_name = "Domain - ${data.adds_domain.current.netbios_name} - GPO4"
}

# Real DoD STIG baselines and custom Domain/DC hardening GPOs, exported from a separate
# source domain as JSON (test/showcase/json_gpo/*.json). Every security principal they
# reference resolves automatically by name, via adds_group.right_dc_ura_sesystemprofileprivilege
# and .json_gpo_principals above; json_gpo_replacements below is only for the handful of
# free-text ####key#### tokens (a "migtable" in the same sense as backup_gpo's migrations,
# just plain text instead of typed entries) that automatic resolution can't cover.
locals {
  json_gpo_files = fileset("${path.module}/json_gpo", "*.json")

  json_gpo_replacements = {
    "Domain - Domain Policy - v0r2.json" = {
      DomainFQDN = data.adds_domain.current.dns_root
      DomainNB   = data.adds_domain.current.netbios_name
    }
  }
}

resource "adds_json_gpo" "imports" {
  for_each = local.json_gpo_files

  path         = "${path.module}/json_gpo/${each.value}"
  target_name  = trimsuffix(each.value, ".json")
  replacements = lookup(local.json_gpo_replacements, each.value, {})

  depends_on = [
    adds_group.json_gpo_principals,
    adds_group.right_dc_ura_sesystemprofileprivilege,
  ]
}

# Links two GPOs onto the Servers OU. Authoritative for that OU's links, so it exercises the
# create path (writing gPLink over LDAP) for both, precedence order (domain_gpo1 first), and an
# enforced link. The OU holds no other links, so re-applies stay idempotent.
resource "adds_gpo_links" "servers" {
  target = adds_organizational_unit.child["Servers"].path

  links = [
    { gpo = adds_backup_gpo.domain_gpo1.id },
    { gpo = adds_backup_gpo.domain_gpo4.id, enforced = true },
  ]
}

# SINGLE link on the Workstations OU. Deliberately one entry so the read path returns a lone
# GPO link: PowerShell's ConvertTo-Json emits a single item as an object, not a one-element
# array, so this guards the provider's "coerce scalar to list" handling against the common
# single-vs-many serialization bug.
resource "adds_gpo_links" "workstations" {
  target = adds_organizational_unit.child["Workstations"].path

  links = [
    { gpo = adds_json_gpo.imports["DoD Windows 11 Computer STIG v1r2.json"].id },
  ]
}

# MULTIPLE json_gpo links on the Staff OU, in precedence order, with one enforced. Pairs with
# the single-link case above so the same read/update code is exercised for both a scalar and
# an array result.
resource "adds_gpo_links" "staff" {
  target = adds_organizational_unit.child["Staff"].path

  links = [
    { gpo = adds_json_gpo.imports["DoD Google Chrome STIG Computer v2r7.json"].id },
    { gpo = adds_json_gpo.imports["DoD Microsoft Edge STIG Computer v1r6.json"].id, enforced = true },
    { gpo = adds_json_gpo.imports["DoD Microsoft Defender Antivirus STIG Computer v2r4.json"].id, enabled = false },
  ]
}

# Nested membership.
resource "adds_group_member" "operators_in_admins" {
  group  = adds_group.admins.id
  member = adds_group.operators.id
}

# Replication sites and their subnets live in the forest-wide Configuration partition.
# Two sites exercise single-subnet (primary) vs multi-subnet (secondary) assignment.
# Names avoid "s13" (already present in the forest); subnets stay in 172.16.0.0/16, in a
# high /24 block kept clear of the per-pipeline e2e subnets below .250.
resource "adds_site" "primary" {
  name        = "adds-showcase-primary"
  description = "Showcase primary replication site"
}

resource "adds_site" "secondary" {
  name        = "adds-showcase-secondary"
  description = "Showcase secondary replication site"
}

# Single subnet on the primary site (single-item read path).
resource "adds_subnet" "primary" {
  name        = "172.16.250.0/24"
  site        = adds_site.primary.name
  description = "Showcase primary subnet"
  location    = "Showcase DC / Rack 1"
}

# Multiple subnets on the secondary site (array read path).
resource "adds_subnet" "secondary" {
  for_each = toset(["172.16.251.0/24", "172.16.252.0/24"])

  name = each.value
  site = adds_site.secondary.name
}

# Constructor 1: rights on the object itself.
resource "adds_access_rule" "constructor1_full_control" {
  target  = adds_organizational_unit.root.distinguished_name
  trustee = adds_group.admins.sid
  rights  = ["GenericAll"]
}

# Constructor 2: inherited by every descendant.
resource "adds_access_rule" "constructor2_read_all" {
  target      = adds_organizational_unit.root.distinguished_name
  trustee     = adds_group.operators.sid
  rights      = ["GenericRead"]
  inheritance = "All"
}

# Constructor 3: inherited by one class of descendant.
resource "adds_access_rule" "constructor3_manage_users" {
  target                = adds_organizational_unit.root.distinguished_name
  trustee               = adds_group.admins.sid
  rights                = ["GenericAll"]
  inheritance           = "Descendents"
  inherited_object_type = "user"
}

# Constructor 4: one class of child object, this object only.
resource "adds_access_rule" "constructor4_create_computers_here" {
  target      = adds_organizational_unit.child["Servers"].distinguished_name
  trustee     = adds_group.operators.sid
  rights      = ["CreateChild", "DeleteChild"]
  object_type = "computer"
}

# Constructor 5: one class of child object, propagated.
resource "adds_access_rule" "constructor5_create_computers_below" {
  target      = adds_organizational_unit.child["Servers"].distinguished_name
  trustee     = adds_group.admins.sid
  rights      = ["CreateChild", "DeleteChild"]
  object_type = "computer"
  inheritance = "Descendents"
}

# Constructor 6: an extended right, on one class, propagated.
resource "adds_access_rule" "constructor6_reset_passwords" {
  target                = adds_organizational_unit.child["ServiceAccounts"].distinguished_name
  trustee               = adds_group.admins.sid
  rights                = ["ExtendedRight"]
  object_type           = "Reset Password"
  inheritance           = "Descendents"
  inherited_object_type = "user"
}

# A property set, which is a control access right used with ReadProperty/WriteProperty.
resource "adds_access_rule" "personal_information" {
  target                = adds_organizational_unit.child["ServiceAccounts"].distinguished_name
  trustee               = adds_group.operators.sid
  rights                = ["ReadProperty", "WriteProperty"]
  object_type           = "Personal Information"
  inheritance           = "Descendents"
  inherited_object_type = "user"
}

# A Deny entry.
resource "adds_access_rule" "deny_delete_tree" {
  target      = adds_organizational_unit.child["Servers"].distinguished_name
  trustee     = "Authenticated Users"
  rights      = ["DeleteTree"]
  access      = "Deny"
  inheritance = "All"
}

# A well-known principal, and a relative DN target outside the showcase OU.
resource "adds_access_rule" "read_computers_container" {
  target      = "CN=Computers"
  trustee     = adds_group.operators.sid
  rights      = ["ReadProperty"]
  object_type = "All"
  inheritance = "Descendents"
}

# Forest-wide prerequisite for group managed service accounts. effective_immediately
# backdates the key by 10 hours so the gMSAs below can be created without waiting for the
# replication safety window; sound here because the CI target is a single-DC forest. Destroy
# only drops it from Terraform state, since a KDS root key cannot be removed from AD.
resource "adds_kds_root_key" "showcase" {
  effective_immediately = true
}

# Pre-staged computer accounts (New-ADComputer creates them disabled). They exist to be
# referenced as gMSA principals and to be read back through the adds_computer data source; a
# real host would later domain-join under the same name.
resource "adds_computer" "web" {
  for_each = toset(["web01", "web02"])

  name = each.value
  path = adds_organizational_unit.child["Servers"].path
}

# A computer with every settable property populated, so the result can be inspected in ADUC.
resource "adds_computer" "app01" {
  name             = "app01"
  sam_account_name = "app01"
  dns_host_name    = "app01.${data.adds_domain.current.dns_root}"
  path             = adds_organizational_unit.child["Servers"].path

  description         = "Showcase application server, every property set by Terraform."
  display_name        = "APP01"
  location            = "Showcase DC / Rack 1"
  user_principal_name = "host/app01.${data.adds_domain.current.dns_root}"
  managed_by          = adds_group.admins.distinguished_name

  kerberos_encryption_type = ["AES128", "AES256"]
  service_principal_names = [
    "HOST/app01.${data.adds_domain.current.dns_root}",
    "HOST/app01",
  ]

  enabled                            = false
  trusted_for_delegation             = false
  account_not_delegated              = true
  compound_identity_supported        = true
  protected_from_accidental_deletion = true
}

# Reads a pre-staged account back through the data source, proving computers that a
# configuration does not manage can still be referenced by identity.
data "adds_computer" "app01" {
  identity = adds_computer.app01.distinguished_name
}

# Minimal gMSA: required attributes plus a single group principal.
resource "adds_gmsa" "minimal" {
  name          = "adds-svc-min"
  dns_host_name = "adds-svc-min.${data.adds_domain.current.dns_root}"
  path          = adds_organizational_unit.child["ServiceAccounts"].path

  principals_allowed_to_retrieve_managed_password = [
    adds_group.operators.sid,
  ]

  depends_on = [adds_kds_root_key.showcase]
}

# Full gMSA: every settable property populated, with computer accounts and a group as the
# principals allowed to retrieve the managed password. This is the case that exercises the
# msDS-SupportedEncryptionTypes read path.
resource "adds_gmsa" "full" {
  name             = "adds-svc-full"
  sam_account_name = "adds-svc-full"
  dns_host_name    = "adds-svc-full.${data.adds_domain.current.dns_root}"
  path             = adds_organizational_unit.child["ServiceAccounts"].path

  description  = "Showcase gMSA with every property set by Terraform."
  display_name = "ADDS Showcase Service Account"
  home_page    = "https://example.invalid/adds-svc-full"

  principals_allowed_to_retrieve_managed_password = concat(
    [for c in adds_computer.web : c.sid],
    [adds_computer.app01.sid, adds_group.operators.sid],
  )

  principals_allowed_to_delegate_to_account = [
    adds_computer.app01.sid,
  ]

  service_principal_names = [
    "HTTP/adds-svc-full.${data.adds_domain.current.dns_root}",
    "HTTP/adds-svc-full",
  ]

  kerberos_encryption_type       = ["AES128", "AES256"]
  managed_password_interval_days = 30

  enabled                            = true
  trusted_for_delegation             = false
  account_not_delegated              = true
  compound_identity_supported        = true
  protected_from_accidental_deletion = true

  depends_on = [adds_kds_root_key.showcase]
}

# Read managed objects back through the read-only data sources, proving they resolve by
# identity exactly as an unmanaged object would. Each attribute reference defers the read
# until after the object exists.
data "adds_group" "admins_lookup" {
  identity = adds_group.admins.distinguished_name
}

data "adds_user" "showcase_lookup" {
  identity = adds_user.showcase.distinguished_name
}

data "adds_organizational_unit" "root_lookup" {
  path = adds_organizational_unit.root.path
}

data "adds_gpo" "domain_gpo4_lookup" {
  identity = adds_backup_gpo.domain_gpo4.target_name
}

data "adds_json_gpo_export" "domain_gpo4" {
  name = adds_backup_gpo.domain_gpo4.target_name

  lifecycle {
    postcondition {
      condition = (
        lower(self.id) == lower(adds_backup_gpo.domain_gpo4.id) &&
        jsondecode(self.json).Name == adds_backup_gpo.domain_gpo4.target_name
      )
      error_message = "GPO export must identify the imported Domain GPO4 policy."
    }

    postcondition {
      condition = (
        jsondecode(self.json).ComputerSettingsEnabled == true &&
        jsondecode(self.json).UserSettingsEnabled == false
      )
      error_message = "GPO4 export must preserve computer-enabled and user-disabled settings."
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

data "adds_backup_gpo_export" "defender" {
  name = adds_json_gpo.imports["DoD Microsoft Defender Antivirus STIG Computer v2r4.json"].target_name

  lifecycle {
    postcondition {
      condition     = lower(self.id) == lower(adds_json_gpo.imports["DoD Microsoft Defender Antivirus STIG Computer v2r4.json"].id)
      error_message = "Backup export must identify the Defender GPO imported from JSON."
    }

    postcondition {
      condition = alltrue([
        for filename in ["Backup.xml", "gpreport.xml", "DomainSysvol/GPO/Machine/Registry.pol"] :
        anytrue([
          for exported_path in nonsensitive(keys(self.files)) :
          lower(exported_path) == lower("{${self.backup_id}}/${filename}")
        ])
      ])
      error_message = "Defender backup export must contain GPMC metadata and the machine Registry.pol."
    }
  }
}

output "domain" {
  value = {
    distinguished_name = data.adds_domain.current.distinguished_name
    dns_root           = data.adds_domain.current.dns_root
    netbios_name       = data.adds_domain.current.netbios_name
    domain_mode        = data.adds_domain.current.domain_mode
  }
}

output "organizational_units" {
  value = merge(
    { root = adds_organizational_unit.root.distinguished_name },
    { for key, ou in adds_organizational_unit.child : key => ou.distinguished_name }
  )
}

output "groups" {
  value = {
    admins = {
      dn            = adds_group.admins.distinguished_name
      sid           = adds_group.admins.sid
      managed_by_dn = adds_group.admins.managed_by_dn
    }
    operators     = { dn = adds_group.operators.distinguished_name, sid = adds_group.operators.sid }
    announcements = { dn = adds_group.announcements.distinguished_name, sid = adds_group.announcements.sid }
    right_dc_ura_sesystemprofileprivilege = {
      dn  = adds_group.right_dc_ura_sesystemprofileprivilege.distinguished_name
      sid = adds_group.right_dc_ura_sesystemprofileprivilege.sid
    }
  }
}

output "user" {
  value = {
    dn         = adds_user.showcase.distinguished_name
    sid        = adds_user.showcase.sid
    manager_dn = adds_user.showcase.manager_dn
  }
}

# Sets a password each apply. keepers pins it to a fixed value so the same fixed-name
# showcase run does not generate a new password every time.
resource "adds_user_password" "showcase" {
  user   = adds_user.showcase.id
  length = 28

  keepers = {
    generation = "1"
  }
}

output "user_password_id" {
  value = adds_user_password.showcase.id
}

output "backup_gpos" {
  value = {
    domain_gpo1 = { id = adds_backup_gpo.domain_gpo1.id, dn = adds_backup_gpo.domain_gpo1.distinguished_name }
    domain_gpo4 = { id = adds_backup_gpo.domain_gpo4.id, dn = adds_backup_gpo.domain_gpo4.distinguished_name }
  }
}

output "json_gpos" {
  value = {
    for key, gpo in adds_json_gpo.imports : key => { id = gpo.id, dn = gpo.distinguished_name }
  }
}

output "gpo_links" {
  value = {
    servers_target_dn = adds_gpo_links.servers.target_dn
    servers_links     = adds_gpo_links.servers.links
    # Single-link OU: proves a lone GPO link round-trips (scalar-vs-array read path).
    workstations_target_dn = adds_gpo_links.workstations.target_dn
    workstations_links     = adds_gpo_links.workstations.links
    # Multi-link OU built from json_gpo imports.
    staff_target_dn = adds_gpo_links.staff.target_dn
    staff_links     = adds_gpo_links.staff.links
  }
}

output "sites" {
  value = {
    primary   = adds_site.primary.distinguished_name
    secondary = adds_site.secondary.distinguished_name
  }
}

output "subnets" {
  value = {
    # Single subnet on primary, map of subnets on secondary.
    primary   = adds_subnet.primary.distinguished_name
    secondary = { for cidr, s in adds_subnet.secondary : cidr => s.distinguished_name }
  }
}

output "kds_root_key" {
  value = {
    id      = adds_kds_root_key.showcase.id
    key_id  = adds_kds_root_key.showcase.key_id
    created = adds_kds_root_key.showcase.created
  }
}

output "computers" {
  value = merge(
    { for key, c in adds_computer.web : key => { dn = c.distinguished_name, sid = c.sid } },
    { app01 = { dn = adds_computer.app01.distinguished_name, sid = adds_computer.app01.sid } },
  )
}

output "computer_lookup" {
  value = {
    dn               = data.adds_computer.app01.distinguished_name
    sid              = data.adds_computer.app01.sid
    sam_account_name = data.adds_computer.app01.sam_account_name
    enabled          = data.adds_computer.app01.enabled
  }
}

output "gmsas" {
  value = {
    minimal = { dn = adds_gmsa.minimal.distinguished_name, sid = adds_gmsa.minimal.sid }
    full = {
      dn                    = adds_gmsa.full.distinguished_name
      sid                   = adds_gmsa.full.sid
      retrieve_password_dns = adds_gmsa.full.principals_allowed_to_retrieve_managed_password_dns
    }
  }
}

output "data_source_lookups" {
  value = {
    group_admins_sid       = data.adds_group.admins_lookup.sid
    user_showcase_sid      = data.adds_user.showcase_lookup.sid
    ou_root_dn             = data.adds_organizational_unit.root_lookup.distinguished_name
    gpo_domain_gpo4_guid   = data.adds_gpo.domain_gpo4_lookup.guid
    gpo_domain_gpo4_status = data.adds_gpo.domain_gpo4_lookup.status
  }
}

output "access_rules" {
  value = {
    full_control         = adds_access_rule.constructor1_full_control.id
    reset_passwords      = adds_access_rule.constructor6_reset_passwords.id
    personal_information = adds_access_rule.personal_information.id
    computers_container  = adds_access_rule.read_computers_container.target_dn
  }
}

output "access_rule_constructors" {
  value = {
    "1_object_only"           = adds_access_rule.constructor1_full_control.id
    "2_all_descendants"       = adds_access_rule.constructor2_read_all.id
    "3_one_class_descendants" = adds_access_rule.constructor3_manage_users.id
    "4_one_class_here"        = adds_access_rule.constructor4_create_computers_here.id
    "5_one_class_propagated"  = adds_access_rule.constructor5_create_computers_below.id
    "6_extended_right"        = adds_access_rule.constructor6_reset_passwords.id
  }
}
