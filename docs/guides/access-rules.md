---
page_title: "Access rules and delegation"
subcategory: "Guides"
description: |-
  How adds_ace maps onto Active Directory ACEs, including all six
  ActiveDirectoryAccessRule constructors.
---

# Access rules and delegation

`adds_ace` manages a single access control entry (ACE) in the DACL of an
Active Directory object. It is a separate resource rather than an attribute of the
objects it protects, for reasons covered in [Why a separate resource](#why-a-separate-resource).

~> `adds_ace` was named `adds_access_rule` until it was joined by the authoritative
[`adds_acl`](../resources/acl.md). The `adds_access_rule` type still works as a deprecated
alias; migrate with `terraform state mv adds_access_rule.<name> adds_ace.<name>`.

## The model

One resource is one ACE. `rights` is a set of `ActiveDirectoryRights` values that are
combined into a single entry using the flags enum, so this creates **one** ACE, not two:

```hcl
resource "adds_ace" "example" {
  target  = "Contoso/Servers"
  trustee = "Server Admins"
  rights  = ["CreateChild", "DeleteChild"]
}
```

An ACE is identified by everything except its rights: the target, the trustee SID, the
access type, the object type GUIDs and the inheritance flags. Changing any of those
replaces the resource; changing only `rights` updates it in place with a single write.

## Naming the target

`target` accepts three forms, so containers that are not organizational units are
reachable without spelling out the domain:

| Form | Example | Resolves to |
| --- | --- | --- |
| Slash path | `Contoso/Servers/Windows` | `OU=Windows,OU=Servers,OU=Contoso,DC=…` |
| Relative DN | `CN=Computers` | `CN=Computers,DC=…` |
| Relative DN, nested | `CN=Public Key Services,CN=Services,CN=Configuration` | appended with the domain DN |
| Full DN | `OU=Servers,DC=contoso,DC=local` | used unchanged |
| Empty | `""` | the domain root |

Slash segments default to `OU=`, but a segment may carry its own prefix when a branch
mixes containers and organizational units:

```hcl
target = "Contoso/CN=LegacyContainer"
```

The resolved value is published as `target_dn`.

```hcl
# The well-known Computers container, which is a CN and not an OU.
resource "adds_ace" "join_default_computers" {
  target      = "CN=Computers"
  trustee     = "Workstation Admins"
  rights      = ["CreateChild", "DeleteChild"]
  object_type = "computer"
}

# The PKI configuration container.
resource "adds_ace" "manage_pki" {
  target      = "CN=Public Key Services,CN=Services,CN=Configuration"
  trustee     = "PKI Admins"
  rights      = ["GenericAll"]
  inheritance = "All"
}

# The domain root.
resource "adds_ace" "replicate_directory_changes" {
  target      = ""
  trustee     = "Entra Connect"
  rights      = ["ExtendedRight"]
  object_type = "Replicating Directory Changes"
}
```

~> The relative form appends the **domain** DN. In a multi-domain forest the configuration
naming context belongs to the forest root, so `CN=…,CN=Configuration` only resolves
correctly from the forest root domain. Use a full DN elsewhere.

## The six constructors

`System.DirectoryServices.ActiveDirectoryAccessRule` has six constructors. Which one is
used depends on which of `object_type`, `inheritance` and `inherited_object_type` you
set:

| # | object_type | inheritance | inherited_object_type | Constructor |
| --- | --- | --- | --- | --- |
| 1 | – | – | – | `(identity, rights, type)` |
| 2 | – | set | – | `(identity, rights, type, inheritance)` |
| 3 | – | set | set | `(identity, rights, type, inheritance, inheritedObjectType)` |
| 4 | set | – | – | `(identity, rights, type, objectType)` |
| 5 | set | set | – | `(identity, rights, type, objectType, inheritance)` |
| 6 | set | set | set | `(identity, rights, type, objectType, inheritance, inheritedObjectType)` |

`inherited_object_type` without `inheritance` has no corresponding constructor and is
rejected during `terraform plan`.

### 1. Rights on the object itself

No object type, no inheritance. The ACE applies to the target and nothing else.

```hcl
resource "adds_ace" "full_control_on_ou" {
  target  = adds_organizational_unit.servers.distinguished_name
  trustee = "Server Admins"
  rights  = ["GenericAll"]
}
```

### 2. Rights inherited by descendants

Adding `inheritance` propagates the ACE. `All` covers the object and every descendant;
`Descendents` covers descendants only; `Children` covers immediate children.

```hcl
resource "adds_ace" "read_everything_below" {
  target      = adds_organizational_unit.servers.distinguished_name
  trustee     = "Authenticated Users"
  rights      = ["GenericRead"]
  inheritance = "All"
}
```

### 3. Rights on descendants of one class

`inherited_object_type` narrows inheritance to a single object class. Here, full control
over user objects anywhere below the OU — but not over the OU or any other class.

```hcl
resource "adds_ace" "manage_users" {
  target                = adds_organizational_unit.staff.distinguished_name
  trustee               = "Helpdesk"
  rights                = ["GenericAll"]
  inheritance           = "Descendents"
  inherited_object_type = "user"
}
```

### 4. Rights on one class of child object

`object_type` without inheritance. With `CreateChild` and `DeleteChild`, `object_type`
names the class that may be created or deleted — here, computer objects directly in this
OU only.

```hcl
resource "adds_ace" "join_computers" {
  target      = adds_organizational_unit.servers.distinguished_name
  trustee     = "Server Admins"
  rights      = ["CreateChild", "DeleteChild"]
  object_type = "computer"
}
```

### 5. Rights on one class, propagated

The same, but propagated to child OUs so the delegation survives future OU structure.

```hcl
resource "adds_ace" "join_computers_anywhere" {
  target      = adds_organizational_unit.servers.distinguished_name
  trustee     = "Server Admins"
  rights      = ["CreateChild", "DeleteChild"]
  object_type = "computer"
  inheritance = "Descendents"
}
```

### 6. One attribute, on one class, propagated

All three set. `object_type` is the attribute being written, `inherited_object_type` is
the class it is written on. This is the classic "let the helpdesk reset passwords on user
objects" delegation.

```hcl
resource "adds_ace" "reset_passwords" {
  target                = adds_organizational_unit.staff.distinguished_name
  trustee               = "Helpdesk"
  rights                = ["ExtendedRight"]
  object_type           = "Reset Password"
  inheritance           = "Descendents"
  inherited_object_type = "user"
}

resource "adds_ace" "write_description" {
  target                = adds_organizational_unit.staff.distinguished_name
  trustee               = "Helpdesk"
  rights                = ["ReadProperty", "WriteProperty"]
  object_type           = "description"
  inheritance           = "Descendents"
  inherited_object_type = "user"
}
```

## Object types

`object_type` and `inherited_object_type` accept:

| Kind | Example | Resolved from |
| --- | --- | --- |
| Schema class | `computer`, `user`, `organizationalUnit` | `lDAPDisplayName` in the schema NC |
| Attribute | `description`, `unicodePwd` | `lDAPDisplayName` in the schema NC |
| Property set | `Personal Information` | `displayName` of a `controlAccessRight` |
| Extended right | `Reset Password`, `Send As` | `displayName` of a `controlAccessRight` |
| Validated write | `Validated write to DNS host name` | `displayName` of a `controlAccessRight` |
| GUID | `bf967a86-0de6-11d0-a285-00aa003049e2` | used as-is |
| `All` | `All` | the empty GUID |

Lookups are targeted LDAP queries, not a full schema enumeration.

## Extended rights

Most `ActiveDirectoryRights` values map to a fixed permission bit. `ExtendedRight` does
not: it is a single bit that means "the operation named by `object_type`". Which
operations exist is defined by objects in
`CN=Extended-Rights,CN=Configuration,DC=…`, so the available set depends on the forest
— installing Exchange or extending the schema adds more.

```hcl
resource "adds_ace" "reset_passwords" {
  target                = adds_organizational_unit.staff.distinguished_name
  trustee               = "Helpdesk"
  rights                = ["ExtendedRight"]
  object_type           = "Reset Password" # which extended right
  inheritance           = "Descendents"
  inherited_object_type = "user" # on which class of object
}
```

~> **`ExtendedRight` without `object_type` grants *every* extended right** on the target,
including password resets and directory replication. Always name the specific right
unless that is genuinely what you intend.

### Commonly used extended rights

These exist in every Active Directory forest:

| `object_type` | Grants |
| --- | --- |
| `Reset Password` | Set a password without knowing the current one |
| `Change Password` | Change a password when the current one is supplied |
| `Unexpire Password` | Clear an expired password condition |
| `Enable Per User Reversibly Encrypted Password` | Toggle reversible encryption |
| `Allowed to Authenticate` | Authenticate against a computer in another forest |
| `Send As` / `Receive As` | Send or receive mail as the object |
| `Replicating Directory Changes` | Read replication data, as used by Entra Connect |
| `Replicating Directory Changes All` | Read replication data including secrets |
| `Manage Replication Topology` | Modify replication topology |
| `Reanimate Tombstones` | Restore deleted objects |
| `Update Password Not Required Bit` | Toggle `PASSWD_NOTREQD` |

### Three kinds of control access right

`CN=Extended-Rights` holds three different things, distinguished by the `validAccesses`
attribute. All three are named through `object_type`, but each pairs with a different
entry in `rights`:

| `validAccesses` | Kind | Use with |
| --- | --- | --- |
| `256` | Extended right | `rights = ["ExtendedRight"]` |
| `48` | Property set | `rights = ["ReadProperty"]` and/or `["WriteProperty"]` |
| `8` | Validated write | `rights = ["Self"]` |

A **property set** is a named group of attributes, which is how you delegate a coherent
set of fields without listing each one:

```hcl
resource "adds_ace" "edit_personal_information" {
  target                = adds_organizational_unit.staff.distinguished_name
  trustee               = "Helpdesk"
  rights                = ["ReadProperty", "WriteProperty"]
  object_type           = "Personal Information" # ~40 attributes: address, phone, ...
  inheritance           = "Descendents"
  inherited_object_type = "user"
}
```

Useful property sets: `Personal Information`, `Public Information`,
`General Information`, `Web Information`, `Membership`, `Account Restrictions`,
`Logon Information`, `Terminal Server License Server`.

A **validated write** permits a write that Active Directory itself validates, rather
than an unrestricted one:

```hcl
resource "adds_ace" "register_own_spn" {
  target                = adds_organizational_unit.servers.distinguished_name
  trustee               = "Server Admins"
  rights                = ["Self"]
  object_type           = "Validated write to service principal name"
  inheritance           = "Descendents"
  inherited_object_type = "computer"
}
```

### Discovering what a forest offers

Because the set is forest-specific, list it rather than guessing. Run against a domain
controller:

```powershell
Get-ADObject -SearchBase (Get-ADRootDSE).ConfigurationNamingContext `
    -LDAPFilter '(objectClass=controlAccessRight)' `
    -Properties displayName, rightsGuid, validAccesses |
  Sort-Object displayName |
  Format-Table displayName, validAccesses, rightsGuid
```

Filter to one kind with `(&(objectClass=controlAccessRight)(validAccesses=256))` for
extended rights, `48` for property sets, or `8` for validated writes.

The `displayName` is what `object_type` expects. If a name is ambiguous or contains
characters awkward in HCL, the `rightsGuid` can be used directly:

```hcl
object_type = "00299570-246d-11d0-a768-00aa006e0529" # Reset Password
```

## Trustees

`trustee` accepts a SID, a distinguished name, `DOMAIN\name`, a `sAMAccountName`, or a
well-known name such as `Authenticated Users` or `Everyone`. It is resolved to a SID
when the resource is applied and the SID is stored in state, so renaming a group does
not cause drift.

Because well-known principals resolve without being directory objects, they can be used
without declaring a `adds_group` for them:

```hcl
resource "adds_ace" "deny_everyone" {
  target      = adds_organizational_unit.secret.distinguished_name
  trustee     = "Everyone"
  rights      = ["GenericRead"]
  access      = "Deny"
  inheritance = "All"
}
```

~> **Deny ACEs take precedence over allow ACEs.** A `Deny` entry that matches a broad
principal such as `Everyone` will override delegations granted elsewhere.

## Redundant ACEs are canonicalized away

Windows canonicalizes a security descriptor when it is written. If one ACE for a trustee
is a strict subset of another ACE for the **same trustee and the same rights**, the
narrower one is folded away rather than stored twice. This is standard ACL behaviour, not
something the provider does.

The case that catches people out: `inheritance = "All"` already applies to the object
itself, not only its descendants. So granting the same trustee the same rights both with
and without inheritance is redundant, and only the broader entry survives:

```hcl
# These two rules target the same object and the same trustee with the same rights.
resource "adds_ace" "narrow" {
  target  = adds_organizational_unit.servers.distinguished_name
  trustee = adds_group.admins.sid
  rights  = ["GenericRead"]
}

resource "adds_ace" "broad" {
  target      = adds_organizational_unit.servers.distinguished_name
  trustee     = adds_group.admins.sid
  rights      = ["GenericRead"]
  inheritance = "All"
}
```

Applying this leaves only one ACE on the object. `narrow` will show as missing on the
next `terraform plan` and be recreated, only to be folded away again on the next apply —
a permanent, self-inflicted diff.

This only happens when **trustee, rights and access type all match** — different rights,
different trustees, or an `object_type`/`inherited_object_type` that narrows the scope to
one class all avoid it, since none of those pairs are true subsets of each other. Give
overlapping delegations for the same trustee a different `object_type`,
`inherited_object_type`, or trustee to keep them distinct.

## AdminSDHolder-protected objects

Active Directory marks highly privileged principals — members of `Domain Admins`,
`Enterprise Admins`, `Administrators`, and similar built-in groups — with `adminCount = 1`.
A background process (SDProp) runs roughly every hour and resets the ACL of every
`adminCount = 1` object to match the `AdminSDHolder` object, discarding any other ACE that
was added in the meantime — including ones added by Terraform.

`adds_ace` checks the target's `adminCount` before writing and refuses by
default:

```
Error: Unable to create access rule

'CN=Administrator,CN=Users,DC=contoso,DC=local' has adminCount=1 (protected by
AdminSDHolder). Its ACL is periodically reset by SDProp to match AdminSDHolder, so
any ACE added here will be silently reverted. Set ignore_admin_count_1 = true to
proceed anyway.
```

This is not restricted to well-known accounts: **any** security principal — a user, group,
service account, or computer — ends up with `adminCount = 1` once it is (or ever was) a
member of a protected group, and it is not cleared automatically when membership is
removed.

If you genuinely intend to delegate on a protected object — understanding that SDProp
will revert it on its own schedule — set `ignore_admin_count_1 = true`:

```hcl
resource "adds_ace" "temporary_delegation" {
  target                = "CN=Administrator,CN=Users,DC=contoso,DC=local"
  trustee                = adds_group.helpdesk.sid
  rights                 = ["ExtendedRight"]
  object_type            = "Reset Password"
  ignore_admin_count_1   = true
}
```

Expect this resource to show drift on a roughly hourly cadence once SDProp runs, since
the ACE will keep disappearing and Terraform will keep re-adding it. That drift is the
correct, expected signal that the delegation is fighting `AdminSDHolder` — the usual fix
is to delegate on a different, non-protected object instead of overriding the check, or 
if you're a rock'n'roll kinda guy - change the ACL on AdminSDHolder itself.

## What this resource does not touch

`adds_ace` is deliberately non-authoritative:

- **Inherited ACEs are never modified or removed.** Only explicit, non-inherited entries
  on the target are considered.
- **Inheritance is never enabled or disabled.** Blocking inheritance on an object is a
  destructive operation that must be performed deliberately, never as a side effect of
  adding a delegation.
- **ACEs belonging to other trustees are left alone**, including the defaults Active
  Directory creates.
- **Destroying the resource removes only the ACE it created.**

This means an object can carry a mix of Terraform-managed and externally-managed
permissions without either side reverting the other.

## Why a separate resource

Permissions are not modelled as an `acl` attribute on `adds_organizational_unit` and
friends. Four reasons:

1. **Partial ownership.** A nested attribute implies Terraform owns the whole DACL. Real
   objects carry inherited and default ACEs that no one wants to declare, and that change
   with schema updates.
2. **Unmanaged targets.** `target` is a plain DN, so rights can be granted on the domain
   root, `CN=Users`, or any OU that predates Terraform.
3. **Canonical ordering.** Windows reorders ACEs (deny before allow, explicit before
   inherited). A list-shaped attribute would produce permanent diffs.
4. **Dependency cycles.** Delegating rights on an OU to a group that lives inside that
   same OU is routine:

   ```hcl
   resource "adds_organizational_unit" "servers" {
     path = "Contoso/Servers"
   }

   resource "adds_group" "server_admins" {
     path = adds_organizational_unit.servers.path # group depends on OU
   }

   resource "adds_ace" "delegate" {
     target  = adds_organizational_unit.servers.distinguished_name
     trustee = adds_group.server_admins.sid # rule depends on both
     rights  = ["CreateChild", "DeleteChild"]
   }
   ```

   As an attribute of the OU, the OU would depend on the group and the group on the OU,
   and Terraform would refuse to plan. A third resource depending on both breaks the
   cycle.

## Rights reference

Common `ActiveDirectoryRights` values:

| Right | Meaning |
| --- | --- |
| `GenericAll` | Full control |
| `GenericRead` | Read permissions, properties and object list |
| `GenericWrite` | Write properties and validated writes |
| `CreateChild` / `DeleteChild` | Create or delete child objects of `object_type` |
| `ReadProperty` / `WriteProperty` | Read or write the attribute or property set named by `object_type` |
| `Delete` | Delete the target object itself |
| `DeleteTree` | Delete an object and its whole subtree |
| `ExtendedRight` | Exercise the extended right named by `object_type` — see [Extended rights](#extended-rights) |
| `ReadControl` / `WriteDacl` / `WriteOwner` | Read or modify the security descriptor |
| `Self` | Perform the validated write named by `object_type` |

-> Some combinations are rendered by .NET under a composite name — configuring
`ReadControl`, `ListChildren`, `ReadProperty` and `ListObject` reads back as
`GenericRead`. The provider compares the numeric access mask and keeps your spelling when
the effective rights are unchanged, so this does not produce a diff.
