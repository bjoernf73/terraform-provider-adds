resource "adds_json_gpo" "domain_gpo5" {
  path        = "${path.module}/json_gpo/Domain - GPO5.json"
  target_name = "Domain - GPO5"

  # Free-text ####key#### tokens the export couldn't classify automatically (a domain
  # FQDN embedded in a script argument, for example) - keys are bare names, the ####
  # delimiters are implied. Security principals are resolved automatically by name and
  # never need an entry here.
  replacements = {
    DomainFQDN = data.adds_domain.current.dns_root
  }
}
