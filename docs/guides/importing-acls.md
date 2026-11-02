---
page_title: "Importing an existing domain's ACLs"
subcategory: "Guides"
description: |-
  Using adds_acl_export to lift an existing domain's delegation into Terraform as
  adds_acl configuration.
---

# Importing an existing domain's ACLs

The usual blocker to putting an established Active Directory under Terraform is not the
objects — it is the **permissions**. Years of delegation leave hundreds of explicit access
control entries scattered across OUs and containers, and recreating them by hand is
impractical and error-prone. The [`adds_acl_export`](../data-sources/acl_export.md) data
source reads those entries straight from the directory and renders them as
[`adds_acl`](../resources/acl.md) configuration you can review, keep, and replay.

It is the read side of [`adds_acl`](../resources/acl.md): where that resource *writes* an
authoritative DACL, this *reads* one back in the same vocabulary.

## What it captures

For each object in scope, the export records exactly what `adds_acl` manages:

- **Explicit ACEs only.** Inherited ACEs are ignored — they belong to a parent and are
  reproduced by that parent's own export, not duplicated here.
- **`inheritance_blocked`.** Whether the object breaks inheritance is captured, so the
  hierarchy's behaviour is reproduced and not just its entries.
- **Names, not raw identifiers.** Trustee SIDs resolve to `DOMAIN\name` (or a well-known
  name such as `Authenticated Users`), and object-type/inherited-object-type GUIDs resolve
  to their schema or extended-right names. Anything that cannot be resolved falls back to
  the raw SID or GUID, which `adds_acl` still accepts.

Two things are deliberately left out, because `adds_acl` handles them elsewhere:

- The **Protect object from accidental deletion** ACE (a `Deny` for `Everyone` on
  `Delete`/`DeleteTree`) — it is driven by each object resource's
  `protected_from_accidental_deletion`, not by the DACL.
- **Inherited** entries, as above.

## A single object

Point `path` at one object to export just its DACL:

```hcl
data "adds_acl_export" "servers_ou" {
  path = "Contoso/Servers"
}

output "hcl" {
  value = data.adds_acl_export.servers_ou.hcl
}
```

`path` takes the same forms as everywhere else in the provider — a slash path, a relative
distinguished name, a full DN, or `""` for the domain root (see
[Paths and distinguished names](paths)).

## A whole subtree

Set `recursive = true` to export every object below `path`:

```hcl
data "adds_acl_export" "servers_tree" {
  path      = "Contoso/Servers"
  recursive = true
}
```

The search is **paged**, so exporting a large branch — or the whole domain — is not
truncated by the directory's query size limit.

## The whole domain

Use the domain root as the path. The [`adds_domain`](../data-sources/domain.md) data source
now exposes the naming-context DNs for exactly this:

```hcl
data "adds_domain" "current" {}

data "adds_acl_export" "domain" {
  path      = data.adds_domain.current.domain_path # DC=contoso,DC=local
  recursive = true
}
```

`adds_domain` also publishes `configuration_path` and `schema_path`, so the configuration
and schema partitions can be exported the same way.

## Writing it to a file

A data source cannot write files itself, so pair it with the `local_file` resource from the
`hashicorp/local` provider:

```hcl
resource "local_file" "acls" {
  filename = "${path.module}/imported/servers-acls.tf"
  content  = data.adds_acl_export.servers_tree.hcl
}
```

Or render it once from the command line without a resource:

```sh
terraform apply
terraform output -raw hcl > imported-acls.tf
```

The structured form is available too, in `objects`, if you would rather template the output
yourself than use the ready-made `hcl`.

## Narrowing by object type

`object_types` restricts the export to specific Active Directory **structural class** names
(their `lDAPDisplayName`):

```hcl
data "adds_acl_export" "containers" {
  path         = ""
  recursive    = true
  object_types = ["organizationalUnit", "container"]
}
```

Matching is on the most-specific class, so `"user"` excludes computer objects (which also
carry `objectClass=user`) — ask for `"computer"` to get those. Omit `object_types` to export
every object type encountered.

~> Start narrow. An unfiltered recursive export from the domain root walks every object in
the domain and reads each one's security descriptor. Filter by `object_types`, or export a
branch at a time, before reaching for a full-domain sweep.

## Empty objects are skipped

By default the export omits objects that have **no explicit ACEs and do not block
inheritance** — there is nothing to replicate, and rendering an empty authoritative
`adds_acl` for them would *prune their DACL* on apply. Set `include_empty = true` only if you
specifically want a block emitted for every matched object.

## Portable targets

The `target` in each rendered block is the **portable** form, not the raw DN:

| Source object | Rendered `target` |
| --- | --- |
| An OU, for example `OU=Servers,OU=Contoso,DC=contoso,DC=local` | `"Contoso/Servers"` |
| A container or mixed path, for example `CN=Users,DC=contoso,DC=local` | `"CN=Users"` |
| The domain root | `""` |

Because the domain's `DC=` components are stripped, the same export **replays against another
domain** with the same relative structure — a lab rebuilt from production, or a second forest
mirroring the first. (The configuration partition is forest-wide, so exports from
`configuration_path` are only portable within a single-domain forest.)

## Applying the result

The rendered blocks are [`adds_acl`](../resources/acl.md) — **authoritative** resources. Read
the [`adds_acl`](../resources/acl.md) page before applying, and keep two cases in mind:

- **Replicating to a different (empty) domain.** Just `terraform apply`. Each block writes
  exactly the explicit ACEs it lists and sets `inheritance_blocked`. Trustee names resolve
  against the *target* domain, so the referenced users, groups and computers must exist
  there.
- **Adopting the domain you exported from.** `terraform import` each `adds_acl` first (by the
  object's DN), so Terraform records the current state rather than planning to recreate it.
  Without an import, the first apply still converges — the entries already match — but
  importing makes the plan a clean no-op and avoids a needless rewrite.

~> `adds_acl` owns the **whole** explicit DACL of its target. Do not also manage the same
object with [`adds_ace`](../resources/ace.md); the authoritative block would prune it. The
export captures every explicit ACE precisely so the generated `adds_acl` is complete.

## Review before you trust it

The export is a faithful snapshot, but treat the generated file as a starting point:

- Resource **labels are generated** from each object's path and de-duplicated; rename them to
  something meaningful for your configuration.
- An ACE whose trustee or object type could not be resolved appears as a **raw SID or GUID** —
  readable config benefits from replacing those with names where you can.
- Years of delegation accumulate **redundant or obsolete** entries. Importing them faithfully
  is the point, but it is also a good moment to prune what should not have been there.
