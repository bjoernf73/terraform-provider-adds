---
page_title: "Managing SYSVOL Files"
subcategory: "Guides"
description: |-
  Deploy NETLOGON scripts and Administrative Templates without replacing unrelated
  files in the shared SYSVOL directories.
---

# Managing SYSVOL Files

[`adds_netlogon_files`](../resources/netlogon_files.md) deploys a local directory
recursively below the domain's NETLOGON share. [`adds_administrative_templates`](../resources/administrative_templates.md)
deploys a local directory recursively to the Central Store root:
`SYSVOL/<domain>/Policies/PolicyDefinitions`.

Both resources are deliberately non-authoritative for their shared SYSVOL locations.
They write every file from `source_path`, but neither scans for nor deletes files that
Terraform did not previously record as its own. This means a vendor bundle, a script
managed by another configuration, or an administrator's file remains untouched.

## File ownership and deletion

The resource records each relative file path it writes. When a source file is removed,
the next apply removes only that previously recorded remote path. It then walks upward
and removes a parent directory only while it is empty. A directory containing any other
file or subdirectory is left intact.

Destroying the resource follows the same rule: it removes its recorded files and safely
prunes now-empty directories, without treating NETLOGON or the Central Store as a
Terraform-owned mirror.

## NETLOGON

Use `path` to select a relative destination under NETLOGON. The source directory can
have any nested layout, which is preserved on deployment:

```hcl
resource "adds_netlogon_files" "logon_scripts" {
  source_path = "${path.module}/netlogon"
  path        = "logon"
}
```

A source file at `netlogon/windows/map-drives.ps1` becomes
`NETLOGON/logon/windows/map-drives.ps1`.

## Administrative Templates

Administrative Templates always deploy directly to the Central Store root. There is no
relative destination attribute; organize vendor or language files within `source_path`
instead:

```hcl
resource "adds_administrative_templates" "central_store" {
  source_path = "${path.module}/policy_definitions"
}
```

Both resources compute a content fingerprint during planning. Adding, removing, or
editing a source file therefore produces a Terraform update. Remote modifications or
missing managed files are also detected during refresh and restored on the next apply.
