variable "domain" {
  default = "contoso.com"
}

# Export a GPO from a reference environment, e.g.:
#   Backup-GPO -Name "Baseline Workstation" -Path .\backup_gpo
# This creates .\backup_gpo\Baseline Workstation\{<guid>}\... - the folder name
# ("Baseline Workstation" here) is backup_name below, and its parent is path.
resource "adds_backup_gpo" "baseline_workstation" {
  backup_name = "Baseline Workstation"
  path        = "${path.module}/backup_gpo"
  target_name = "Baseline Workstation"

  # Remaps security principals and UNC paths baked into the backup when it was
  # exported from a different domain or forest.
  migrations = [
    {
      source      = "SOURCEDOMAIN\\GPO-Filter-Servers"
      destination = "CONTOSO\\GPO-Filter-Servers"
      type        = "GlobalGroup"
    },
    {
      source      = "\\\\sourcedomain.local\\netlogon\\scripts"
      destination = "\\\\${var.domain}\\netlogon\\scripts"
      type        = "UNCPath"
    },
  ]
}
