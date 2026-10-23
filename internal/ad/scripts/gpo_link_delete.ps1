$domainDN = Get-DomainDN
$targetDN = [string]$payload.target_dn

# Clear the links this resource owned and reset block-inheritance, but only if the target still
# exists (the OU/site may already be gone).
$entry = Get-ADDSEntry -DistinguishedName $targetDN -Attributes @('distinguishedName')
if ($null -ne $entry) {
    Set-ADDSAttribute -DistinguishedName $targetDN -Name 'gPLink' -Values @()
    Set-ADDSAttribute -DistinguishedName $targetDN -Name 'gPOptions' -Values @('0')
}

[pscustomobject]@{ exists = $false } | ConvertTo-Json -Compress
