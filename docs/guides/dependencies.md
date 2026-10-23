---
page_title: "Dependencies and ordering"
subcategory: "Guides"
description: |-
  How Terraform decides the order objects are created in, why a literal name can race,
  and how to break dependency cycles.
---

# Dependencies and ordering

Active Directory objects refer to each other constantly: a group is managed by another
group, an access rule grants rights to a group on an organizational unit, a membership
joins two groups. Terraform has to create them in an order where each reference already
exists.

It works this out from **references between resources**, not from the order resources
appear in a file. Understanding where references exist — and where they only appear to —
explains most ordering failures.

## Order in the file means nothing

```hcl
resource "adds_group" "a" {
  name       = "GroupA"
  path       = "Contoso/Groups"
  managed_by = adds_group.b.id
}

resource "adds_group" "b" {
  name = "GroupB"
  path = "Contoso/Groups"
}
```

`adds_group.b` is created first, despite being written second, because
`adds_group.a` refers to it. Moving the blocks around, or splitting them across files,
changes nothing.

Terraform applies the reverse order on destroy, so `GroupA` is removed before `GroupB`.

## A literal name creates no dependency

This is the same configuration with one character changed, and it is a race:

```hcl
resource "adds_group" "a" {
  name       = "GroupA"
  path       = "Contoso/Groups"
  managed_by = "GroupB" # a string, not a reference
}

resource "adds_group" "b" {
  name = "GroupB"
  path = "Contoso/Groups"
}
```

Terraform sees no relationship between the two, and it applies up to 10 resources
concurrently, so `GroupA` may be created before `GroupB` exists. The apply fails while
resolving the manager:

```
Error: Unable to create group

  no directory object found for identity 'GroupB'
```

The confusing part is that running `terraform apply` again usually succeeds — `GroupB`
exists by then — which makes a deterministic modelling error look like a flaky provider.

~> If an apply fails with `no directory object found for identity`, and a re-run fixes
it, the cause is almost always a literal name where a reference belongs.

### Fixing it

Prefer a reference, which also keeps working if the referenced object is renamed:

```hcl
managed_by = adds_group.b.id
```

When the value genuinely has to stay a literal — because the object is not managed by
this configuration, or it would create a cycle — declare the edge explicitly:

```hcl
resource "adds_group" "a" {
  name       = "GroupA"
  managed_by = "GroupB"

  depends_on = [adds_group.b]
}
```

`depends_on` is the tool of last resort: it is invisible to anyone reading the attribute,
and it is easy to forget when the configuration changes later.

## Which attributes take identities

Every attribute that names another directory object behaves this way:

| Attribute | Accepts |
| --- | --- |
| `adds_group.managed_by` | DN, `objectGUID`, SID, `DOMAIN\name`, `sAMAccountName` |
| `adds_group_member.group` | the same forms |
| `adds_group_member.member` | the same forms |
| `adds_access_rule.trustee` | the same forms, plus well-known names |
| `adds_access_rule.target` | a path or DN, see [Paths and distinguished names](paths) |

Because all of them accept plain strings, all of them can silently lose an edge. Useful
attributes to reference instead:

```hcl
group   = adds_group.admins.id                              # objectGUID
trustee = adds_group.admins.sid                             # SID
target  = adds_organizational_unit.servers.distinguished_name
member  = adds_group.operators.id
```

## Objects outside the configuration

A literal is correct when the object is not managed by Terraform:

```hcl
resource "adds_access_rule" "authenticated_read" {
  target  = adds_organizational_unit.servers.distinguished_name
  trustee = "Authenticated Users" # well-known, always exists
  rights  = ["GenericRead"]
}

resource "adds_group_member" "service_account" {
  group  = adds_group.admins.id
  member = "svc-backup" # created by another process
}
```

There is nothing to order against, so no edge is needed. The object simply has to exist
when the apply runs, otherwise resolution fails with the same error as above.

## Containers must exist first

`adds_group` and `adds_access_rule` require their container or target to exist;
only `adds_organizational_unit` creates missing parents along its path. Referencing the
OU rather than repeating its path gets both the correct value and the ordering:

```hcl
resource "adds_organizational_unit" "groups" {
  path = "Contoso/Groups"
}

resource "adds_group" "admins" {
  name = "Admins"
  path = adds_organizational_unit.groups.path # edge, and no duplicated string
}
```

Writing `path = "Contoso/Groups"` in both places produces the same directory layout when
it works, but nothing guarantees the OU is created first.

## Cycles

Terraform refuses to plan a configuration whose references form a loop:

```
Error: Cycle: adds_group.a, adds_group.b
```

Two groups managing each other is the obvious case:

```hcl
resource "adds_group" "a" {
  managed_by = adds_group.b.id
}

resource "adds_group" "b" {
  managed_by = adds_group.a.id # cycle
}
```

Break it by making one side a literal with an explicit dependency:

```hcl
resource "adds_group" "a" {
  name       = "GroupA"
  managed_by = adds_group.b.id
}

resource "adds_group" "b" {
  name       = "GroupB"
  managed_by = "GroupA"
  depends_on = [adds_group.a]
}
```

This works because `managed_by` is reconciled after the object is created, so `GroupA`
exists by the time `GroupB` is configured. Mutual management is unusual, though — a cycle
is often a sign that the relationship should be modelled differently.

### The cycle that is not obvious

Delegating rights on an OU to a group that lives inside that same OU looks circular but
is not, provided the access rule is its own resource:

```hcl
resource "adds_organizational_unit" "servers" {
  path = "Contoso/Servers"
}

resource "adds_group" "server_admins" {
  name = "Server Admins"
  path = adds_organizational_unit.servers.path # group depends on OU
}

resource "adds_access_rule" "delegate" {
  target  = adds_organizational_unit.servers.distinguished_name
  trustee = adds_group.server_admins.sid # rule depends on both
  rights  = ["CreateChild", "DeleteChild"]
}
```

The graph is `OU → group → rule`, with no loop. Had permissions been an attribute of the
organizational unit, the OU would have depended on the group and the group on the OU, and
this everyday delegation would have been impossible to express. That is one of the
reasons access rules are a separate resource — see
[Access rules and delegation](access-rules).

## Inspecting the graph

To see what Terraform thinks depends on what:

```sh
terraform graph | dot -Tsvg > graph.svg
```

Or, for a specific resource, check what a plan says it must create first:

```sh
terraform plan -target=adds_group.admins
```

If an object you expected to be created first is missing from that plan, the edge is
missing too.
