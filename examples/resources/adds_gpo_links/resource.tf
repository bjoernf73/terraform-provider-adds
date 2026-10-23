resource "adds_organizational_unit" "servers" {
  path = "Contoso/Servers"
}

resource "adds_backup_gpo" "baseline_workstation" {
  backup_name = "Baseline Workstation"
  path        = "${path.module}/backup_gpo"
  target_name = "Baseline Workstation"
}

resource "adds_backup_gpo" "screen_lock" {
  backup_name = "Screen Lock"
  path        = "${path.module}/backup_gpo"
  target_name = "Screen Lock"
}

# Authoritative for every GPO link on this OU: any link present in AD but not listed
# here is removed. Order matters - the first entry has the highest precedence.
resource "adds_gpo_links" "servers" {
  target            = adds_organizational_unit.servers.path
  block_inheritance = true

  links = [
    {
      gpo = adds_backup_gpo.screen_lock.id
    },
    {
      gpo      = adds_backup_gpo.baseline_workstation.id
      enforced = true
    },
  ]
}
