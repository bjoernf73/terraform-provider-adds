# An authoritative ACL is imported by the target's distinguished name. Importing first
# captures every current explicit ACE, so you can review them before editing - this resource
# prunes any explicit ACE you do not keep in the configuration.
terraform import adds_acl.servers "OU=Servers,OU=Contoso,DC=contoso,DC=local"
