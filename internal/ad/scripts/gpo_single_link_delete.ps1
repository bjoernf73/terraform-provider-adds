# Removes only this resource's GPO link from the target, preserving every other link and leaving
# gPOptions (block-inheritance) untouched. A no-op when the target or the link is already gone.
$domainDN = Get-DomainDN
$targetDN = [string]$payload.target_dn
$gpoGuid = ([string]$payload.gpo_guid).ToLower()

$entries = Get-ADDSRawGPLinks -TargetDN $targetDN
if ($null -ne $entries) {
    $entries = @($entries)
    $remaining = @($entries | Where-Object { (Get-ADDSGPOGuidFromDN $_.DN) -ne $gpoGuid })
    if ($remaining.Count -ne $entries.Count) {
        $gpLink = ConvertTo-ADDSGPLinkString $remaining
        Set-ADDSAttribute -DistinguishedName $targetDN -Name 'gPLink' -Values @($gpLink)
    }
}

[pscustomobject]@{ exists = $false } | ConvertTo-Json -Compress
