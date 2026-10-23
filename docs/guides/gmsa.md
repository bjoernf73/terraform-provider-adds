---
page_title: "Group managed service accounts"
subcategory: "Guides"
description: |-
  How adds_gmsa manages Active Directory group managed service accounts, including the
  sAMAccountName rule, password retrieval, delegation and Kerberos settings.
---

# Group managed service accounts

A group managed service account (gMSA) is a directory principal whose password is generated
and rotated automatically by the domain's Key Distribution Service (KDS) rather than by an
administrator. Multiple hosts can share one gMSA, and each retrieves the current password on
demand. `adds_gmsa` manages the account object and every setting that governs who may use it.

```hcl
resource "adds_gmsa" "websvc" {
  name          = "websvc-gmsa"
  dns_host_name = "websvc.contoso.local"
  path          = "Managed Service Accounts"

  principals_allowed_to_retrieve_managed_password = [
    adds_group.web_servers.sid,
  ]

  service_principal_names = ["HTTP/websvc.contoso.local"]
}
```

~> A **KDS root key** must exist in the forest before any gMSA can be created. Manage it with
the [`adds_kds_root_key`](../resources/kds_root_key) resource, which creates one only when
none is present:

```hcl
resource "adds_kds_root_key" "root" {
  # Backdate the effective time by 10 hours so gMSAs work at once. Safe in a single-DC
  # forest; in production, leave false and wait for replication.
  effective_immediately = true

  # Push the new key to every DC in the forest immediately, rather than waiting for
  # normal replication. The key lives in the forest-wide Configuration partition.
  force_replication = true
}

resource "adds_gmsa" "websvc" {
  # ...
  depends_on = [adds_kds_root_key.root]
}
```

A newly created key is not effective for 10 hours by default (a replication safety window);
`effective_immediately` backdates it so it is usable straight away, while `force_replication`
uses `Sync-ADObject` to push the key object out to every domain controller in the forest at
once. Use both together to make a brand-new key usable everywhere immediately. Removing the
resource never deletes the key, since Active Directory has no supported way to remove one and
doing so would break every existing gMSA.

## The sAMAccountName rule

Active Directory stores a service account's pre-Windows 2000 logon name with a trailing `$`,
and the whole value is capped at 20 characters. A gMSA's `sam_account_name` is therefore
limited to **19 characters** before the `$` that AD appends automatically.

`adds_gmsa` accepts the name with or without the `$` and treats both spellings as the same
account:

```hcl
sam_account_name = "websvc"    # stored as WEBSVC$
sam_account_name = "websvc$"   # identical — the trailing $ is ignored
```

A trailing `$` you supply is stripped before the 19-character limit is checked, and your
configured spelling is preserved in state so neither form drifts. Omit `sam_account_name`
entirely to derive it from `name`.

## Password retrieval

The whole point of a gMSA is that a defined set of hosts, and only those hosts, may fetch its
password. That set is `principals_allowed_to_retrieve_managed_password`, and it is
**authoritative** — a principal removed from the list loses access on the next apply.

```hcl
principals_allowed_to_retrieve_managed_password = [
  adds_group.web_servers.sid,   # a group is easiest to maintain
  "SQL01$",                      # or individual computer accounts
]
```

Each entry accepts a distinguished name, `objectGUID`, SID, `DOMAIN\name` or `sAMAccountName`.
The resolved distinguished names are published as
`principals_allowed_to_retrieve_managed_password_dns`, and your configured spelling is kept in
the input attribute so a SID and its DN do not fight each other across plans.

Prefer a group over a raw list of computer accounts: add or remove a host by changing group
membership rather than editing the gMSA, and the delegation stays stable.

## Delegation

Two independent delegation controls are exposed:

- `principals_allowed_to_delegate_to_account` configures **resource-based constrained
  delegation** (`msDS-AllowedToActOnBehalfOfOtherIdentity`): the listed principals may
  impersonate users when calling this account's service. Like the retrieval list it is
  authoritative and its resolved DNs are published as
  `principals_allowed_to_delegate_to_account_dns`.
- `trusted_for_delegation` enables **unconstrained** Kerberos delegation and should be left
  `false` unless a legacy application genuinely requires it. `account_not_delegated` marks the
  account as sensitive so it can never be delegated at all.

## Kerberos and password rotation

`kerberos_encryption_type` is the set of supported encryption types, any of `None`, `DES`,
`RC4`, `AES128` and `AES256`. Omit it to leave the Active Directory default in place; set it
explicitly to force modern ciphers:

```hcl
kerberos_encryption_type = ["AES128", "AES256"]
```

`managed_password_interval_days` controls how often the KDS rotates the password. It is
**fixed at creation** — Active Directory offers no way to change it afterwards — so changing
this value replaces the account. It defaults to 30 days.

## Identity, moves and renames

Like the other account resources, the gMSA is tracked by `objectGUID`, exposed as `id`, so it
survives renames and moves:

- Changing `name` renames the account in place (the `CN`).
- Changing `path` moves it to another container.
- `distinguished_name` and `sid` are published as computed attributes.

`protected_from_accidental_deletion` adds the usual Deny ACEs on `Delete` and `DeleteTree`;
the provider lifts them automatically when it needs to move, rename or destroy the account.

## Importing

Import by `objectGUID`:

```shell
terraform import adds_gmsa.websvc "b3f2b9a0-1234-4a85-a7a3-9bcbb6a41d02"
```
