# A single GPO link is imported with a composite id: <target DN>|<GPO GUID>.
terraform import adds_gpo_link.baseline \
  "OU=Servers,OU=Contoso,DC=contoso,DC=local|11111111-2222-3333-4444-555555555555"
