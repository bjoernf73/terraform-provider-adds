resource "adlc_user" "jdoe" {
  name                = "John Doe"
  sam_account_name    = "jdoe"
  user_principal_name = "jdoe@contoso.local"
  path                = "Contoso/Users"
  given_name          = "John"
  surname             = "Doe"

  # adlc_user owns the enabled state: with no password yet, it sets a throwaway random
  # one (never stored in state) so the account can be enabled. adlc_user_password below
  # replaces the password without changing the enabled state.
  enabled = true
}

# Generates a random password and sets it on the account, then pushes it to Vault.
# Nothing in this provider talks to Vault: the vault_kv_secret_v2 resource below is
# part of the separate hashicorp/vault provider.
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

# Reuses a password already stored in Vault, without ever writing it to Terraform
# state. Requires Terraform 1.11+. Bump password_wo_version to rotate the password —
# password_wo itself cannot be diffed, since it is never persisted anywhere.
data "vault_kv_secret_v2" "jsmith" {
  mount = "secret"
  name  = "ad/users/jsmith"
}

resource "adlc_user" "jsmith" {
  name                = "Jane Smith"
  sam_account_name    = "jsmith"
  user_principal_name = "jsmith@contoso.local"
  path                = "Contoso/Users"
  enabled             = true
}

resource "adlc_user_password" "jsmith" {
  user                = adlc_user.jsmith.id
  password_wo         = data.vault_kv_secret_v2.jsmith.data["password"]
  password_wo_version = 1
}

# Forces the password to be regenerated whenever var.rotation_marker changes, without
# touching any other attribute. The account's enabled state is left to adlc_user.
resource "adlc_user_password" "rotating" {
  user = adlc_user.jdoe.id

  keepers = {
    rotation = var.rotation_marker
  }
}
