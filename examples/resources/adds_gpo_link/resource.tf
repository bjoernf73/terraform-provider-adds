resource "adds_organizational_unit" "servers" {
  path = "Contoso/Servers"
}

# A single, non-authoritative link. Other GPOs linked to the same OU - including ones linked
# by a different Terraform configuration - are left untouched.
resource "adds_gpo_link" "baseline" {
  target = adds_organizational_unit.servers.distinguished_name
  gpo    = "Server Baseline"

  # Insert at the top on create. With monitor_order = false (the default) the link keeps its
  # slot afterwards, so links added later by other configurations never cause drift.
  order = 1
}

# A system-specific link contributed by, for example, a per-system module. It appends itself
# below whatever is already linked without disturbing the baseline above.
resource "adds_gpo_link" "system_specific" {
  target   = adds_organizational_unit.servers.distinguished_name
  gpo      = "App Server Settings"
  enforced = true
}

# Pin a link to a fixed slot and keep it there, reconciling it back if anything moves it.
# This can produce a permanent diff if another manager fights it for the slot.
resource "adds_gpo_link" "always_first" {
  target        = adds_organizational_unit.servers.distinguished_name
  gpo           = "Security Mandatory"
  order         = 1
  monitor_order = true
}
