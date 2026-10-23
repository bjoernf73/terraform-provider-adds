# Access rules are imported with a composite id:
#   <target DN>|<trustee SID>|<access>|<object type GUID>|<inherited object type GUID>|<inheritance>
# Use 00000000-0000-0000-0000-000000000000 for an unset GUID and an empty segment for unset inheritance.
terraform import adds_access_rule.create_delete_computers \
  "OU=Servers,OU=Contoso,DC=contoso,DC=local|S-1-5-21-1234567890-1234567890-1234567890-1234|Allow|bf967a86-0de6-11d0-a285-00aa003049e2|bf967aa5-0de6-11d0-a285-00aa003049e2|Descendents"
