---
page_title: "Group scope conversions"
subcategory: "Guides"
description: |-
  What happens when an adds_group's scope changes between Global, DomainLocal and
  Universal, which conversions Active Directory allows, and the nesting rules that can
  still block a change.
---

# Group scope conversions

Every Active Directory group has a **scope** that controls where it can be used and what it
can contain:

| Scope | May contain | May be used for permissions |
| --- | --- | --- |
| `Global` | Accounts and other `Global` groups from the **same domain** | Anywhere in the forest |
| `DomainLocal` | Accounts, `Global` and `Universal` groups from **any domain**, plus `DomainLocal` groups from the same domain | **Only** in the group's own domain |
| `Universal` | Accounts, `Global` and `Universal` groups from **any domain** in the forest | Anywhere in the forest |

Changing `scope` on an `adds_group` is reconciled **in place** — the group keeps its
`objectGUID`, SID, membership and access control entries. Terraform never replaces the group
to change its scope.

## The conversion matrix

Active Directory does not allow every scope to convert directly to every other. The table
below shows what a single conversion step requires:

| From \ To | `Global` | `DomainLocal` | `Universal` |
| --- | --- | --- | --- |
| `Global` | — | **not direct** | direct |
| `DomainLocal` | **not direct** | — | direct |
| `Universal` | direct | direct | — |

The only forbidden pair is `Global` ⇄ `DomainLocal`. `Universal` sits in the middle: it can
convert to or from either of the other two.

## What the provider does

Because `Global` and `DomainLocal` cannot convert directly, the provider performs the change
in two steps through `Universal`:

```
Global      ->  Universal  ->  DomainLocal
DomainLocal ->  Universal  ->  Global
```

This is automatic. A plan that changes

```hcl
resource "adds_group" "readers" {
  name  = "Readers"
  path  = "Contoso/Groups"
  scope = "DomainLocal" # was "Global"
}
```

converts `Global → Universal → DomainLocal` in a single apply, with no replacement and no
manual intermediate step.

## Nesting rules that can still block a conversion

A scope is not just a label — it constrains what a group may contain and which groups it may
belong to. Each conversion step must leave the group **and** every group it touches in a
valid state, so a change can fail even though the scope itself is convertible. The rules that
matter in practice:

- **`Global → Universal`** fails if the group is a member of another `Global` group. A
  `Universal` group cannot be nested inside a `Global` group.
- **`Universal → Global`** fails if the group **contains** a `Universal` group as a member. A
  `Global` group cannot contain `Universal` members.
- **`Universal → DomainLocal`** is normally allowed; a `DomainLocal` group accepts the widest
  range of members.
- **`DomainLocal → Universal`** fails if the group **contains** a `DomainLocal` group as a
  member. A `Universal` group cannot contain `DomainLocal` members.

Because `Global ⇄ DomainLocal` goes through `Universal`, **both** the relevant rules apply:
for example `Global → DomainLocal` must first satisfy the `Global → Universal` rule (no
`Global` parent group) and then the `Universal → DomainLocal` step.

When a step violates one of these rules, Active Directory rejects it. The provider
**pre-checks** a scope change at plan time: before any change is attempted it reads the
group's `memberOf` and `member` sets, evaluates every step of the conversion, and fails the
plan with an error naming the specific groups that block it, for example:

```
Group scope conversion blocked by nesting: Active Directory cannot convert this group from
Global to DomainLocal. The conversion must pass through Universal, and a step is blocked by
group nesting:
  - this group is a member of: Corp Admins (Global)
```

The preflight is best-effort and uses Active Directory's documented rules; the apply still
enforces them, so a conversion that slips past the preflight is still rejected by the
directory. If a conversion is blocked, resolve the offending membership — remove the
incompatible member or leave the incompatible parent group (for example with
`adds_group_member`) — and plan again.

## Related

- [`adds_group` resource](../resources/group.md)
- [`adds_group_member` resource](../resources/group_member.md) — manage the memberships that
  the nesting rules above depend on
