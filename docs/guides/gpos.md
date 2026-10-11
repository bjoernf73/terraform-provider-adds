---
page_title: "Managing Group Policy Objects"
subcategory: "Guides"
description: |-
  The two ways this provider imports GPOs, how migration/token replacement works for
  each, and how links, ACLs and drift are handled.
---

# Managing Group Policy Objects

This provider has two independent ways to get a GPO into Active Directory. Both import
into an ordinary GPO by name and both re-import in place on every apply, preserving the
target's GUID and any links or ACLs already on it - neither one ever deletes and
recreates the GPO.

| | `adlc_backup_gpo` | `adlc_json_gpo` |
| --- | --- | --- |
| Source | A `Backup-GPO` folder (or the GPMC UI's backup) | A JSON description |
| Underlying mechanism | `Import-GPO` | Rewrites SYSVOL files directly, using an embedded copy of Microsoft's `GPRegistryPolicyParser` for `Registry.pol` |
| Migrating principals/paths | An explicit, typed `migrations` list | Automatic, by name, for every SID the export found |
| Free-text substitution | Not supported | `replacements`, a plain string map |
| Coverage | Everything `Import-GPO` supports | Registry settings, security template, audit settings, comments, scripts, Group Policy Preferences - not links, ACLs or WMI filters |

### Exporting a live GPO

A live GPO can be exported in either format, regardless of whether it was created
manually, imported from JSON, or imported from a backup:

- [`adlc_json_gpo_export`](../data-sources/json_gpo_export.md) returns JSON for
  `adlc_json_gpo`, with portable security-principal tokens.
- [`adlc_backup_gpo_export`](../data-sources/backup_gpo_export.md) runs `Backup-GPO`
  and returns native GPMC backup files in a sensitive map of relative paths to base64
  contents. Use `local_sensitive_file` from `hashicorp/local` with `content_base64`
  to save these files under `<path>/<backup_name>/`, then use that folder with
  `adlc_backup_gpo` in a later configuration or apply. The import resource needs the
  local folder to exist during planning.

Backup exports create a new snapshot on every read, including a new backup GUID and
timestamps. The remote temporary directory is removed after export. Both formats store
policy contents in Terraform state, so protect that state as well as any saved files.
Exporting a backup does not manage links or generate a cross-domain migration table.

### Which PowerShell runs Group Policy operations

The `GroupPolicy` module is not native to PowerShell 7. Under `pwsh` it loads through the
Windows PowerShell Compatibility layer, which prints warnings and returns *deserialized*
objects, so nested properties can be lost. Scripts that import this module therefore
default to Windows PowerShell (`powershell.exe`), where it is native. This includes GPO
imports, the `adlc_gpo` data source, JSON and backup exports, permissions, security
filtering and WMI-filter assignment. LDAP-only GPO links and backup/JSON resource reads
use `powershell_path` instead (default `pwsh`).

Override this with the provider's `gpo_powershell_path` setting - for example set it to `pwsh`
to force the compatibility layer, or to a full path if `powershell.exe` is not on `PATH`. The
host must have the `GroupPolicy` module (RSAT Group Policy Management) available to whichever
executable runs these operations.

GPO links and permissions are managed by separate resources either way:
[`adlc_gpo_links`](../resources/gpo_links.md) manages where a GPO is linked;
[`adlc_gpo_permission`](../resources/gpo_permission.md) manages a named Group Policy
permission for one trustee; and [`adlc_gpo_security_filter`](../resources/gpo_security_filter.md)
authoritatively controls which principals may apply a GPO. [`adlc_access_rule`](../resources/access_rule.md)
is still appropriate for a raw ACL delegation on the GPO container itself (its distinguished name
is `CN={guid},CN=Policies,CN=System,<domain DN>`, published as `distinguished_name` on both
import resources), but it does not manage matching SYSVOL permissions and is not a substitute
for GPO security filtering.

### GPO permissions and security filtering

Use `adlc_gpo_permission` when Terraform should manage one trustee's standard Group Policy
permission (`GpoRead`, `GpoApply`, `GpoEdit`, or `GpoEditDeleteModifySecurity`). It uses
`Set-GPPermission`, which keeps the directory and SYSVOL permissions in agreement.

Use `adlc_gpo_security_filter` when the intent is that only a complete, declared set of users,
groups, or computers may apply the policy. It removes explicit `GpoApply` permissions from every
other principal, but retains `Authenticated Users` at `GpoRead`, matching GPMC's conventional
security-filtering behavior. It deliberately leaves editor and owner permissions unchanged.

```hcl
resource "adlc_gpo_security_filter" "server_baseline" {
  gpo        = adlc_json_gpo.server_baseline.target_name
  principals = [adlc_group.server_computers.distinguished_name]
}
```

## `adlc_backup_gpo`: importing a `Backup-GPO` folder

Each backup gets its own dedicated folder, matching `backup_name`, so the folder itself
is exactly what you'd pass to `Backup-GPO -Path`:

```powershell
Backup-GPO -Name "Domain - GPO1" -Path ".\backup_gpo\Domain - GPO1"
```

```hcl
resource "adlc_backup_gpo" "domain_gpo1" {
  backup_name = "Domain - GPO1"
  path        = "${path.module}/backup_gpo"
  target_name = "Domain - GPO1"
}
```

### Migrating principals across domains

`migrations` mirrors GPMC's own migration table concept, one entry per principal or UNC
path baked into the backup. `source` always reflects the *source* domain - it identifies
what's actually recorded in the backup and never changes based on where you're
importing to. Each entry needs exactly one of `destination` or `same_as_source`:

```hcl
migrations = [
  # Re-resolve "AnotherGroup" by name in the target domain, instead of a fixed value.
  { type = "GlobalGroup", source = "AnotherGroup@utv.local", same_as_source = true },

  # Or redirect explicitly, e.g. to a differently-named group in the target domain.
  { type = "LocalGroup", source = "Right-ADM-BuiltinGroup-RemoteDesktopUsers@utv.local",
    destination = "Right-ADM-BuiltinGroup-RemoteDesktopUsers@${data.adlc_domain.current.dns_root}" },
]
```

Whichever principals you reference must already exist, by name, in the target domain -
Terraform doesn't create them for you; a `adlc_group` alongside the import is the usual
pattern.

## `adlc_json_gpo`: importing a JSON description

```hcl
resource "adlc_json_gpo" "domain_gpo5" {
  path        = "${path.module}/json_gpo/Domain - GPO5.json"
  target_name = "Domain - GPO5"
}
```

Every security principal a JSON GPO references was recorded as a portable
`####Replace[DOMAIN\Name]` token when it was exported, and each one is resolved by name
in the target domain automatically - no `migrations` list to maintain. As with
`adlc_backup_gpo`, the referenced principals must already exist in the target domain.

### Free-text replacements

Some values in an export are free text an export can't classify automatically - a domain
FQDN embedded in a registry value, for example - marked with a `####key####` token of
their own. `replacements` fills these in; keys are bare names, with the `####` delimiters
implied - the same way Ansible implies double curly braces around a variable name:

```hcl
replacements = {
  DomainFQDN = data.adlc_domain.current.dns_root
}
```

### Exporting a GPO to JSON

`adlc_json_gpo_export` is the read-only mirror of the import: it reads a live GPO's
SYSVOL content and renders it as JSON, converting SIDs to `####Replace[DOMAIN\Name]`
tokens instead of resolving them from one. Pair it with the `local_file` resource from
the `hashicorp/local` provider to write the result to disk - the same way `Backup-GPO`'s
output isn't itself a Terraform concept for `adlc_backup_gpo`:

```hcl
data "adlc_json_gpo_export" "domain_gpo5" {
  name = "Domain - GPO5"
}

resource "local_file" "domain_gpo5" {
  filename = "${path.module}/json_gpo/Domain - GPO5.json"
  content  = data.adlc_json_gpo_export.domain_gpo5.json
}
```

Export, commit the resulting file, then `adlc_json_gpo` imports it elsewhere - a full
round trip between two domains without ever running GPMC by hand on the target.

## Detecting a changed source (the JSON file or backup folder)

Neither resource stores the backup folder or JSON file itself in Terraform state - only
a fingerprint, `content_hash`, computed from local disk every plan (not cached: it's
recomputed fresh each time, not just read back from state). `adlc_backup_gpo` hashes
every file under `path/backup_name/` plus `migrations`; `adlc_json_gpo` hashes the JSON
file's raw bytes plus `replacements`.

So if you edit the JSON file, or re-export a backup into the same `path`/`backup_name` -
`target_name` unchanged, nothing else in the `.tf` file touched either - the next
`terraform plan` reads the now-different file, computes a different hash, sees it
doesn't match what's recorded in state, and shows `content_hash` changing. That's what
triggers `Update()` (a re-import), independent of anything happening in Active Directory.

## Detecting drift made outside Terraform

A GPO exposes no content to diff against directly, so both resources detect edits made
outside Terraform (someone editing the GPO in GPMC) the same way: through its AD/SysVol
version counters, which GPMC increments on every settings change regardless of which
tool made it. Every plan re-checks the live version against the one recorded at the last
apply, and re-imports the backup or JSON to overwrite the drift when they no longer
match.
