# Every file under source_path is deployed recursively to NETLOGON/logon.
resource "adds_netlogon_files" "logon_scripts" {
  source_path = "${path.module}/netlogon"
  path        = "logon"
}
