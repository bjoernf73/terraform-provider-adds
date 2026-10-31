# Reads the single GPO link this resource owns, identified by its GPO GUID on the target. Reports
# exists = false when the target is gone or the link has been removed, so Terraform recreates it.
$domainDN = Get-DomainDN
$targetDN = [string]$payload.target_dn
$gpoGuid = ([string]$payload.gpo_guid).ToLower()

$result = Get-ADDSGPOLinks -TargetDN $targetDN -DomainDN $domainDN
if ($null -eq $result) {
    [pscustomobject]@{ exists = $false } | ConvertTo-Json -Compress
    return
}

$mine = $result.links | Where-Object { $_.gpo_guid -eq $gpoGuid } | Select-Object -First 1
if ($null -eq $mine) {
    [pscustomobject]@{ exists = $false } | ConvertTo-Json -Compress
    return
}

[pscustomobject]@{
    exists        = $true
    target_dn     = $targetDN
    gpo_guid      = $mine.gpo_guid
    gpo_name      = $mine.gpo_name
    enabled       = $mine.enabled
    enforced      = $mine.enforced
    current_order = [int]$mine.order
} | ConvertTo-Json -Compress
