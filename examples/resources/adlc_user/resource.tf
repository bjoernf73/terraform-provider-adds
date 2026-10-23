resource "adlc_user" "jdoe" {
  name                = "John Doe"
  sam_account_name    = "jdoe"
  user_principal_name = "jdoe@contoso.local"
  path                = "Contoso/Users"

  given_name   = "John"
  surname      = "Doe"
  display_name = "John Doe"
  email        = "john.doe@contoso.local"
  title        = "Systems Engineer"
  department   = "IT"
  company      = "Contoso"
  manager      = adlc_user.jsmith.id

  # Enabled on its own: with no password set, the resource sets a throwaway random
  # password (never stored in state) so the account can be enabled. A password-management
  # resource can set the real password afterwards without changing the enabled state.
  enabled = true
}

resource "adlc_user" "jsmith" {
  name                = "Jane Smith"
  sam_account_name    = "jsmith"
  user_principal_name = "jsmith@contoso.local"
  path                = "Contoso/Users"

  given_name   = "Jane"
  surname      = "Smith"
  display_name = "Jane Smith"
  title        = "IT Manager"
  department   = "IT"
  company      = "Contoso"
}
