---
page_title: "Paths and distinguished names"
subcategory: "Guides"
description: |-
  How adds resolves object locations, so configurations avoid repeating the domain
  component of every distinguished name.
---

# Paths and distinguished names

Active Directory identifies objects by distinguished name, which is verbose, written
leaf-first, and carries the domain in every single value:

```
OU=Windows,OU=Servers,OU=Contoso,DC=contoso,DC=local
```

Written out in every resource, that is noise — and it hardcodes the domain, so the same
configuration cannot be applied to a test domain without editing every string.

The provider therefore accepts several forms wherever an object location is expected,
and appends the domain component itself.

## The forms

| Form | Example | Resolves to |
| --- | --- | --- |
| Slash path | `Contoso/Servers/Windows` | `OU=Windows,OU=Servers,OU=Contoso,DC=contoso,DC=local` |
| Relative DN | `CN=Computers` | `CN=Computers,DC=contoso,DC=local` |
| Relative DN, nested | `CN=Services,CN=Configuration` | `CN=Services,CN=Configuration,DC=contoso,DC=local` |
| Full DN | `OU=Servers,DC=contoso,DC=local` | used unchanged |
| Empty string | `""` | the domain root, `DC=contoso,DC=local` |

Which form is used is detected from the shape of the value:

1. Ends with the domain DN, or starts with `DC=` → already absolute, used as-is.
2. Contains no `/` and starts with an RDN prefix such as `CN=` or `OU=` → a DN relative
   to the domain root; the domain DN is appended.
3. Anything else → a slash path.

## Slash paths

A slash path reads in the natural direction, parent first, and every segment is assumed
to be an organizational unit below the connected domain root:

```hcl
resource "adds_organizational_unit" "windows" {
  path = "Contoso/Servers/Windows"
}
```

Slash segments are always OU names. If a segment happens to match the connected domain's
NetBIOS name or DNS label, it is still treated as an OU, not as a domain component. For
example, with the domain `Contoso.local`:

```text
Contoso/Servers       -> OU=Servers,OU=Contoso,DC=Contoso,DC=local
Contoso.local/Servers -> OU=Servers,OU=Contoso.local,DC=Contoso,DC=local
```

Use a relative or full DN when you need a non-OU RDN or want to spell out the domain:

```hcl
path = "OU=Servers"
path = "OU=Servers,DC=contoso,DC=local"
```

Nothing in the relative path names the domain, so the same module applies unchanged to
`contoso.local` and `test.contoso.local`.

~> `adds_organizational_unit` **creates missing parents** along the path. `adds_group`
and `adds_ace` do not — they expect the container to exist, and fail if it does
not.

## Implicitly created parent OUs

When a path names OUs that do not yet exist, `adds_organizational_unit` creates each
missing parent, not only the leaf. Every parent it actually creates is recorded on the
resource that created it — exposed as the read-only `created_organizational_units`
attribute — and removed again when that resource is destroyed. Parents that already
existed are never recorded and are never touched on destroy. In short: if the provider
created an OU, it owns its lifecycle; if it did not, it leaves it alone.

Recorded parents are removed **deepest first, and only while empty**. An OU is never
deleted while it still contains anything — including objects the provider did not create.
So if you remove an `adds_organizational_unit` from your configuration and one of the OUs
it originally created has since been populated (by hand, by another tool, or by a
resource whose path merely passes through it), that OU is kept, and only the empty parts
of the branch below the leaf are cleaned up. Nothing you did not ask the provider to
manage is destroyed as a side effect.

Because these implicit parents live inside the state of a single resource rather than as
separate resources, they cannot be addressed, imported, or moved individually. To keep
ownership explicit and predictable, **declare every OU in a path as its own
`adds_organizational_unit` resource** and build deeper paths from
`distinguished_name`:

```hcl
resource "adds_organizational_unit" "contoso" {
  path = "Contoso"
}

resource "adds_organizational_unit" "servers" {
  path = "${adds_organizational_unit.contoso.path}/Servers"
}

resource "adds_organizational_unit" "windows" {
  path = "${adds_organizational_unit.servers.path}/Windows"
}
```

Declaring each level makes every OU independently addressable, gives Terraform an explicit
create/destroy order through the dependency edges, and avoids a shared implicit parent
being left behind after a destroy.

## Moving or renaming an OU

Changing `path` on an existing `adds_organizational_unit` **moves and/or renames the OU in
place** — it is not destroyed and recreated. The OU keeps its GUID, and therefore keeps its
GPO links, ACLs and any other references that point at it. Every object it contains moves
with it, whether or not the provider manages it: child OUs, groups, users, computers, and
anything created by hand or another tool.

This matters because a rename is really two edits — the OU's own location and the paths of
everything beneath it — that must happen as one operation. Consider an OU created at
`Groups/Tier0` holding a couple of security groups whose own `path` is derived from it:

```hcl
resource "adds_organizational_unit" "tier0" {
  path = "Groups/Tier0" # later changed to "Tier0/Groups"
}

resource "adds_group" "admins" {
  name = "Tier0 Admins"
  path = adds_organizational_unit.tier0.path
}
```

When you change the path to `Tier0/Groups`, the OU update runs first: it creates the new
parent (`OU=Tier0`), then moves and renames the leaf so it becomes `Tier0/Groups`, carrying
the two groups along. By the time the `adds_group` resources are reconciled they are already
in the right container, so their move is a no-op. Nothing is deleted, so there is never a
moment where AD is asked to remove a non-empty OU.

Because the move is in place, the change is a normal `~ update`, not a `-/+ replacement`, and
attributes derived from the OU (its `distinguished_name`, and the child groups' resolved
DNs) show as *known after apply* while the path is changing.

Any parent OUs the resource originally created that are left empty by the move are cleaned
up afterwards, deepest first and only while empty — the same rule that applies on destroy.
Pre-existing parents, and parents that still hold other objects, are left untouched.

## Relative distinguished names

Slash paths imply `OU=`, which covers most of a directory but not all of it. Several
important locations are containers (`CN=`) rather than organizational units:

- `CN=Users` and `CN=Computers`, the well-known default containers
- `CN=Managed Service Accounts`
- `CN=Public Key Services,CN=Services,CN=Configuration`, where PKI is configured
- `CN=System`, holding DNS and other infrastructure objects

For these, write the distinguished name but leave off the domain:

```hcl
resource "adds_group" "legacy_readers" {
  name  = "Legacy Readers"
  path  = "CN=Users"
  scope = "DomainLocal"
}

resource "adds_ace" "manage_pki" {
  target      = "CN=Public Key Services,CN=Services,CN=Configuration"
  trustee     = "PKI Admins"
  rights      = ["GenericAll"]
  inheritance = "All"
}
```

A relative DN is written leaf-first, exactly like a real distinguished name — only the
`DC=` components are omitted.

## Mixing the two

When a branch mixes organizational units and containers, a slash segment may carry its
own prefix. Only segments without one default to `OU=`:

```hcl
# OU=Contoso -> CN=LegacyContainer
target = "Contoso/CN=LegacyContainer"
```

## The domain root

An empty string means the domain root, which is where forest-wide delegations such as
directory replication are granted:

```hcl
resource "adds_ace" "replicate_directory_changes" {
  target      = ""
  trustee     = "Entra Connect"
  rights      = ["ExtendedRight"]
  object_type = "Replicating Directory Changes"
}
```

## Where each form is accepted

| Attribute | Meaning |
| --- | --- |
| `adds_organizational_unit.path` | The OU itself, with parents created as needed |
| `adds_group.path` | The container holding the group |
| `adds_ace.target` | The object the ACE is applied to |

Each resource also publishes the resolved distinguished name, which is what you pass to
anything that needs a real DN:

| Attribute | Contains |
| --- | --- |
| `adds_organizational_unit.distinguished_name` | DN of the OU |
| `adds_group.distinguished_name` | DN of the group |
| `adds_group.path` | Container, in the form it was configured |
| `adds_ace.target_dn` | Resolved DN of the target |

```hcl
resource "adds_ace" "delegate" {
  target  = adds_organizational_unit.servers.distinguished_name
  trustee = adds_group.admins.sid
  rights  = ["CreateChild"]
}
```

Referring to `distinguished_name` rather than repeating the path also creates the
dependency edge, so Terraform creates the OU before the ACE that needs it.

## Equivalent spellings do not cause drift

`CN=Users` and `CN=Users,DC=contoso,DC=local` denote the same container. The provider
compares **resolved** distinguished names rather than the strings you wrote, so either
spelling is stable across plans:

```
No changes. Your infrastructure matches the configuration.
```

A genuine move still produces a real diff, because the resolved DN actually differs.

## When you do need the domain

Use the `adds_domain` data source rather than a literal, so configurations stay portable:

```hcl
data "adds_domain" "current" {}

output "users_container" {
  value = data.adds_domain.current.users_container
}

resource "adds_group" "example" {
  name = "Example"
  path = data.adds_domain.current.computers_container
}
```

It also exposes `distinguished_name`, `dns_root`, `netbios_name` and
`domain_controllers_container`.

## Limits worth knowing

**A slash path cannot contain a literal `/`.** An organizational unit named `Sales/Marketing`
is legal in Active Directory but ambiguous as a path segment. Use the DN form for it:

```hcl
path = "OU=Sales/Marketing,OU=Contoso"
```

**Commas inside a name must be escaped in the DN form**, as in any distinguished name:

```hcl
path = "OU=Contoso\\, Inc,OU=Companies"
```

**Matching is case-insensitive**, as Active Directory is. `cn=users` and `CN=Users`
resolve identically, but the value is stored as written.

**The relative form appends the domain DN, not the forest root.** The configuration
naming context belongs to the forest root domain, so
`CN=Public Key Services,CN=Services,CN=Configuration` resolves correctly only when the
provider is connected to the forest root. From a child domain, give the full DN.
