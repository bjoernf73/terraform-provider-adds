$domainDN = Get-DomainDN
$targetDN = [string]$payload.target_dn

$result = Get-ADDSGPOLinks -TargetDN $targetDN -DomainDN $domainDN
if ($null -eq $result) {
    [pscustomobject]@{ exists = $false } | ConvertTo-Json -Compress
    return
}

$result | ConvertTo-Json -Compress -Depth 5
