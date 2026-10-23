data "adds_json_gpo_export" "domain_gpo5" {
  name = "Domain - GPO5"
}

resource "local_file" "domain_gpo5" {
  filename = "${path.module}/json_gpo/Domain - GPO5.json"
  content  = data.adds_json_gpo_export.domain_gpo5.json
}
