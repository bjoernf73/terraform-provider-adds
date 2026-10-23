data "adds_backup_gpo_export" "defender" {
  name = "DoD Microsoft Defender Antivirus STIG Computer v2r4"
}

resource "local_sensitive_file" "defender_backup" {
  for_each = nonsensitive(toset(keys(data.adds_backup_gpo_export.defender.files)))

  filename       = "${path.module}/backup_gpo/${data.adds_backup_gpo_export.defender.name}/${each.key}"
  content_base64 = data.adds_backup_gpo_export.defender.files[each.key]
}