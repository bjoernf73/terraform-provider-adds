$domainDN = Get-DomainDN
$targetDN = Convert-PathToDN ([string]$payload.target) $domainDN

# Resolve every desired link to a canonical GPO DN plus its option flags, keeping the declared
# precedence order (first entry = highest precedence).
$entries = @()
foreach ($entry in @($payload.links)) {
    $gpo = Resolve-ADDSGPOLink ([string]$entry.gpo) $domainDN
    $flags = 0
    if (-not [bool]$entry.enabled) { $flags = $flags -bor 1 }
    if ([bool]$entry.enforced) { $flags = $flags -bor 2 }
    $entries += [pscustomobject]@{ DN = $gpo.DN; Flags = $flags }
}

# Authoritative: replace the whole gPLink with exactly the declared links (an empty string clears
# every link), and set gPOptions for block-inheritance.
$gpLink = ConvertTo-ADDSGPLinkString $entries
Set-ADDSAttribute -DistinguishedName $targetDN -Name 'gPLink' -Values @($gpLink)

$gpOptions = if ([bool]$payload.block_inheritance) { '1' } else { '0' }
Set-ADDSAttribute -DistinguishedName $targetDN -Name 'gPOptions' -Values @($gpOptions)

Get-ADDSGPOLinks -TargetDN $targetDN -DomainDN $domainDN | ConvertTo-Json -Compress -Depth 5
