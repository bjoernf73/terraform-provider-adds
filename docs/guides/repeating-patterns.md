---
page_title: "Repeating object patterns across systems"
subcategory: "Guides"
description: |-
  Using for_each and modules to apply the same Active Directory structure to many
  systems without duplicating configuration.
---

# Repeating object patterns across systems

A common Active Directory layout gives every system its own subtree, with the same
standard objects inside each one:

```
OU=Systems,DC=domain,DC=com
  OU=SystemA
    OU=Servers
    OU=Groups
    OU=ServiceAccounts
  OU=SystemB
    OU=Servers
    ...
```

Only the system name differs. Terraform expresses this with `for_each`, so the structure
is written once and the list of systems is the only thing that grows.

## The direct form

`for_each` on a single resource creates one instance per element:

```hcl
resource "adds_organizational_unit" "system" {
  for_each = toset(["SystemA", "SystemB", "SystemC"])

  path        = "Systems/${each.value}"
  description = "Root OU for ${each.value}"
}
```

The instances are addressed by key:

```
adds_organizational_unit.system["SystemA"]
adds_organizational_unit.system["SystemB"]
adds_organizational_unit.system["SystemC"]
```

This works well for a single resource. For a *set* of objects per system, loop a module
instead.

## Looping a module

Put the per-system pattern in a module:

```hcl
# modules/system/variables.tf
variable "system" {
  type        = string
  description = "Name of the system."
}

variable "scope" {
  type        = string
  description = "Scope of the system's admin group."
  default     = "DomainLocal"
}
```

```hcl
# modules/system/main.tf
resource "adds_organizational_unit" "root" {
  path        = "Systems/${var.system}"
  description = "Root OU for ${var.system}"
}

resource "adds_organizational_unit" "sub" {
  for_each = toset(["Servers", "Groups", "ServiceAccounts"])

  path = "Systems/${var.system}/${each.value}"
}

resource "adds_group" "admins" {
  name             = "${var.system}-Admins"
  sam_account_name = "${upper(var.system)}-ADMINS"
  path             = adds_organizational_unit.sub["Groups"].path
  description      = "Administrators of ${var.system}"
  scope            = var.scope
}

resource "adds_ace" "manage_computers" {
  target                = adds_organizational_unit.sub["Servers"].distinguished_name
  trustee               = adds_group.admins.sid
  rights                = ["CreateChild", "DeleteChild"]
  object_type           = "computer"
  inheritance           = "Descendents"
  inherited_object_type = "organizationalUnit"
}
```

```hcl
# modules/system/outputs.tf
output "admins_id" {
  value = adds_group.admins.id
}

output "root_dn" {
  value = adds_organizational_unit.root.distinguished_name
}
```

Then loop the module itself:

```hcl
# main.tf
module "system" {
  source   = "./modules/system"
  for_each = toset(["SystemA", "SystemB", "SystemC"])

  system = each.value
}
```

Adding a system is now a one-line change. Module outputs are addressed the same way:

```hcl
output "system_a_admins" {
  value = module.system["SystemA"].admins_id
}
```

## Use for_each, not count

`count` also repeats a resource, but instances are keyed by **index**:

```hcl
# Don't do this.
resource "adds_organizational_unit" "system" {
  count = length(var.systems)
  path  = "Systems/${var.systems[count.index]}"
}
```

Remove `SystemB` from the middle of that list and every later element shifts down one
index. Terraform compares by address, so `adds_organizational_unit.system[2]` changing
from `SystemC` to something else is read as "this resource changed" — and for an OU whose
`path` requires replacement, that means **destroying and recreating** objects that were
never meant to be touched.

With `for_each` the key is the system name, so removing `SystemB` removes exactly
`["SystemB"]` and leaves the others untouched.

~> Prefer `for_each` for anything that represents a named directory object. The only
reasonable use of `count` is a simple on/off toggle: `count = var.enabled ? 1 : 0`.

## When systems are not identical

As soon as one system needs something different, switch from a set to a map. The keys
become the system names and the values carry per-system settings:

```hcl
locals {
  systems = {
    SystemA = {
      scope  = "DomainLocal"
      owners = ["alice"]
    }
    SystemB = {
      scope  = "Global"
      owners = ["bob", "carol"]
    }
    SystemC = {
      scope  = "DomainLocal"
      owners = []
    }
  }
}

module "system" {
  source   = "./modules/system"
  for_each = local.systems

  system = each.key
  scope  = each.value.scope
}
```

With a map, `each.key` is the system name and `each.value` is its configuration object.
With `toset()`, `each.key` and `each.value` are both the string itself.

## Nested loops

`for_each` cannot be nested directly — a resource has only one `for_each`. To create a
resource for every combination of two things, such as every owner of every system, build
a flat map first and loop that:

```hcl
locals {
  memberships = merge([
    for system, config in local.systems : {
      for owner in config.owners :
      "${system}/${owner}" => {
        system = system
        owner  = owner
      }
    }
  ]...)
}

resource "adds_group_member" "owners" {
  for_each = local.memberships

  group  = module.system[each.value.system].admins_id
  member = each.value.owner
}
```

This produces keys such as `SystemA/alice` and `SystemB/bob`, so each membership has a
stable address that does not move when another system or owner is added.

-> Note the `...` after the list in `merge([...]...)`. That is the expansion symbol, which
turns a single list argument into the separate arguments `merge` expects. Leaving it out
is a common and confusing error.

`setproduct` is an alternative when every combination applies, rather than a per-key list:

```hcl
locals {
  standard_groups = ["Admins", "Operators", "Readers"]

  system_groups = {
    for pair in setproduct(keys(local.systems), local.standard_groups) :
    "${pair[0]}/${pair[1]}" => {
      system = pair[0]
      group  = pair[1]
    }
  }
}

resource "adds_group" "standard" {
  for_each = local.system_groups

  name  = "${each.value.system}-${each.value.group}"
  path  = "Systems/${each.value.system}/Groups"
  scope = "DomainLocal"
}
```

## Keys are part of the resource address

Because the key appears in the address, **changing a key destroys and recreates** that
instance. In this module:

```hcl
resource "adds_organizational_unit" "sub" {
  for_each = toset(["Servers", "Groups", "ServiceAccounts"])
  path     = "Systems/${var.system}/${each.value}"
}
```

renaming `"Servers"` to `"Server"` removes `sub["Servers"]` and creates `sub["Server"]` —
deleting the OU and everything Terraform knows about beneath it. Treat the key strings as
stable identifiers.

If a rename is genuinely intended and the object should survive, move the state entry
rather than letting Terraform destroy it:

```hcl
moved {
  from = adds_organizational_unit.sub["Servers"]
  to   = adds_organizational_unit.sub["Server"]
}
```

## Putting it together

```hcl
locals {
  systems = {
    SystemA = { scope = "DomainLocal", owners = ["alice"] }
    SystemB = { scope = "Global", owners = ["bob", "carol"] }
  }

  memberships = merge([
    for system, config in local.systems : {
      for owner in config.owners :
      "${system}/${owner}" => { system = system, owner = owner }
    }
  ]...)
}

module "system" {
  source   = "./modules/system"
  for_each = local.systems

  system = each.key
  scope  = each.value.scope
}

resource "adds_group_member" "owners" {
  for_each = local.memberships

  group  = module.system[each.value.system].admins_id
  member = each.value.owner
}
```

Two systems, each with four OUs, an admin group and a delegation, plus three memberships —
from a single structural definition and a map of names.
