# Look up an existing Group Policy Object by display name (a GUID works too), for example
# one created in GPMC, and attach policy plumbing to it without importing it.
data "adds_gpo" "baseline" {
  identity = "Corp - Workstation Baseline"
}

resource "adds_gpo_links" "baseline_on_workstations" {
  target = "Contoso/Workstations"

  links = [
    { gpo = data.adds_gpo.baseline.guid },
  ]
}

output "baseline_gpo" {
  value = {
    guid               = data.adds_gpo.baseline.guid
    distinguished_name = data.adds_gpo.baseline.distinguished_name
    status             = data.adds_gpo.baseline.status
  }
}
