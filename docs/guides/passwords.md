---
page_title: "User passwords and secret storage"
subcategory: "Guides"
description: |-
  How adlc_user_password sets an initial password, and why storing it in Vault is a
  separate provider's job.
---

# User passwords and secret storage

`adlc_user` deliberately has no password attribute. `adlc_user_password` is a separate
resource that sets a user's initial password — generated randomly, or supplied from
somewhere else — and this guide covers both that resource and where it stops.

## Why a separate resource

A password's lifecycle does not match an account's. The account's attributes
(`department`, `title`, group memberships, and so on) are reconciled on every plan; a
password must never be silently reset just because a plan ran. Keeping them in one
resource would mean either re-setting the password on unrelated changes, or teaching
`adlc_user` special-case logic to avoid it. A separate, create-only resource sidesteps
this entirely: every attribute of `adlc_user_password` requires replacement, so it either
does nothing or deliberately sets a new password — never a silent side effect.

## Why this provider does not talk to Vault

Terraform already has a mature `hashicorp/vault` provider with proper authentication
(AppRole, Kubernetes, token, and more) and resources such as `vault_kv_secret_v2` for
exactly this purpose. Building Vault access into `adlc_user_password` would mean:

- duplicating Vault's authentication handling inside this provider,
- giving this provider credentials to a second system beyond Active Directory, and
- coupling two otherwise independent systems' release cycles together.

Instead, `adlc_user_password` does one thing: it sets a password on an AD account and
returns it as a sensitive output. Getting that value into Vault — or anywhere else — is
composition, done with the `vault` provider in the same configuration.

## Generating a password and storing it in Vault

```hcl
resource "adlc_user" "jdoe" {
  name                = "John Doe"
  sam_account_name    = "jdoe"
  user_principal_name = "jdoe@contoso.local"
  path                = "Contoso/Users"

  # Left false: adlc_user_password enables the account once a password exists.
  enabled = false
}

resource "adlc_user_password" "jdoe" {
  user = adlc_user.jdoe.id
}

resource "vault_kv_secret_v2" "jdoe" {
  mount = "secret"
  name  = "ad/users/jdoe"
  data_json = jsonencode({
    username = adlc_user.jdoe.sam_account_name
    password = adlc_user_password.jdoe.password
  })
}
```

`adlc_user_password.jdoe.password` is marked sensitive on both sides of that reference,
so it never appears in plan output — but it is present in Terraform state, on both the
`adlc_user_password` resource and the `vault_kv_secret_v2` resource. State encryption
and access control are yours to manage, the same as for any other sensitive value
Terraform handles; this is not specific to passwords.

## Reusing a password from Vault without storing it in state

`password` is a normal Terraform attribute: whatever value it holds — generated or
supplied — is written to state. If that is not acceptable for a password sourced from
Vault, use `password_wo` instead. It is a
[write-only attribute](https://developer.hashicorp.com/terraform/language/resources/ephemeral/write-only)
(Terraform 1.11+): Terraform never writes its value to a plan file or to state, on
either side of the reference.

```hcl
data "vault_kv_secret_v2" "jsmith" {
  mount = "secret"
  name  = "ad/users/jsmith"
}

resource "adlc_user" "jsmith" {
  name                = "Jane Smith"
  sam_account_name    = "jsmith"
  user_principal_name = "jsmith@contoso.local"
  path                = "Contoso/Users"
  enabled             = false
}

resource "adlc_user_password" "jsmith" {
  user                = adlc_user.jsmith.id
  password_wo         = data.vault_kv_secret_v2.jsmith.data["password"]
  password_wo_version = 1
}
```

`password_wo` and `password` are mutually exclusive, and `password_wo` requires
`password_wo_version` to be set. This pairing exists because Terraform has nothing to
diff a write-only value against — it is never stored — so it cannot tell on its own that
`password_wo` changed. `password_wo_version` is an ordinary attribute that *is* stored;
changing it is what actually triggers replacement (and therefore re-reads `password_wo`
from config and re-sets the password). Changing `password_wo` alone, without also
changing `password_wo_version`, has no effect.

Because `password_wo` is never stored, the resource's own `password` output stays null
in this mode — there is no value left anywhere in Terraform's control for it to expose.

A plain `password` can also be supplied this way (skipping generation while still
storing the value in state); use `password_wo` only when the value must never be
persisted.

## Enabling the account

`adlc_user.enabled` fully owns the account's enabled state; `adlc_user_password` never
touches it. This keeps the two concerns independent:

- Set `adlc_user.enabled = true` to enable the account. If it has no usable password yet,
  the `adlc_user` resource sets a throwaway strong random password (never stored in state)
  purely so Active Directory allows it to be enabled.
- Add an `adlc_user_password` resource to set the real password. It replaces whatever
  password is in place but leaves the enabled state alone, so an account that is already
  enabled stays enabled and its chosen password is not overwritten on the next apply.
- Set `adlc_user.enabled = false` (the default) to keep the account disabled, even when an
  `adlc_user_password` resource manages its password.

## Rotation

`adlc_user_password` never rotates a password on its own; every attribute requires
replacement, so nothing changes unless the configuration does. To force a deliberate
rotation, change a value in `keepers`:

```hcl
resource "adlc_user_password" "jdoe" {
  user = adlc_user.jdoe.id

  keepers = {
    rotation = var.rotation_marker
  }
}
```

Changing `var.rotation_marker` replaces the resource: a new password is generated (or the
new `password` value applied), and `vault_kv_secret_v2.jdoe` picks up the new value on the
same apply, since it references `adlc_user_password.jdoe.password` directly.

## Forcing a password change at next logon

Set `change_password_at_next_logon = true` to require the user to choose a new password the
first time they sign in (this sets AD's `pwdLastSet` to 0). The typical use is a personal
account: Terraform generates a strong initial password, you hand it to the user (for example
through Vault), and they must replace it at first logon.

```hcl
resource "adlc_user_password" "jdoe" {
  user                          = adlc_user.jdoe.id
  change_password_at_next_logon = true
}
```

This flag is fire-and-forget. Active Directory clears it as soon as the user changes the
password, so there is nothing stable to reconcile against; the resource sets it when it sets
the password and never reads it back. Leave it `false` (the default) for automation accounts
that keep using the stored password. It cannot be combined with
`adlc_user.password_never_expires` or `cannot_change_password` on the same account, because
Active Directory rejects that combination.

## Detecting drift without resetting anything

`password_last_set` mirrors AD's `pwdLastSet` for `user`. It is refreshed on every plan
and carries no plan modifiers, so if the account's password is changed by anyone or
anything else — a helpdesk reset, self-service, another script — `terraform plan` shows
`password_last_set` changing even though nothing in the configuration did. That is
Terraform's only way to surface that the password Terraform thinks it set is no longer
the one in effect.

Nothing acts on that drift automatically: there is no plan modifier tying
`password_last_set` to replacement, so it never triggers a reset. Deciding whether to
respond (e.g. by rotating via `keepers` or `password_wo_version`) is left to you.

## What this resource does not do

- **It cannot read the password back.** Active Directory has no attribute that exposes a
  password, so `Read` only confirms the account still exists; the password in state is
  never compared against — and can never be verified against — the live account.
- **It never re-sets a password Terraform did not just create.** Every attribute requires
  replacement, so there is no "update" path that could silently reset a live password.
- **`terraform destroy` changes nothing on the account.** Destroying this resource only
  stops Terraform from tracking the password; the account keeps whatever password and
  enabled state it had. Disabling or resetting an account you no longer want is
  `adlc_user`'s job, not this resource's.
