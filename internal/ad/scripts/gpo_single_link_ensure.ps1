# Non-authoritative create/update of a single GPO link on a target, leaving every other link
# untouched (the counterpart to adds_gpo_links, which owns the whole ordered list). gPLink is one
# concatenated attribute, so this reads it, inserts/updates only this GPO's entry, and writes the
# whole string back - the same read-modify-write pattern adds_ace uses on a DACL.
$domainDN = Get-DomainDN
$targetDN = Convert-PathToDN ([string]$payload.target) $domainDN
$gpo = Resolve-ADDSGPOLink ([string]$payload.gpo) $domainDN

$flags = 0
if (-not [bool]$payload.enabled) { $flags = $flags -bor 1 }
if ([bool]$payload.enforced) { $flags = $flags -bor 2 }

$monitorOrder = [bool]$payload.monitor_order
$hasOrder = ($null -ne $payload.order)
$order = if ($hasOrder) { [int]$payload.order } else { 0 }

$entries = Get-ADDSRawGPLinks -TargetDN $targetDN
if ($null -eq $entries) {
    throw "target '$targetDN' was not found"
}
$entries = @($entries)

# Current 1-based position of this GPO's link, or 0 when it is not linked yet.
$existingIndex = 0
for ($i = 0; $i -lt $entries.Count; $i++) {
    if ($entries[$i].DN -ieq $gpo.DN) {
        $existingIndex = $i + 1
        break
    }
}

$remaining = @($entries | Where-Object { $_.DN -ine $gpo.DN })

# Decide where this link goes (1-based). Position is only asserted when inserting a new link, or
# when monitor_order keeps it pinned; an existing link with monitor_order = false keeps its slot so
# links other Terraform states add later do not cause drift.
if ($monitorOrder -and $hasOrder) {
    $insertAt = $order
}
elseif ($existingIndex -gt 0) {
    $insertAt = $existingIndex
}
elseif ($hasOrder) {
    $insertAt = $order
}
else {
    $insertAt = $remaining.Count + 1
}

if ($insertAt -lt 1) { $insertAt = 1 }
if ($insertAt -gt ($remaining.Count + 1)) { $insertAt = $remaining.Count + 1 }

$ours = [pscustomobject]@{ DN = $gpo.DN; Flags = $flags }
$final = New-Object System.Collections.Generic.List[object]
for ($i = 0; $i -lt $remaining.Count; $i++) {
    if (($i + 1) -eq $insertAt) { $final.Add($ours) }
    $final.Add($remaining[$i])
}
if ($insertAt -eq ($remaining.Count + 1)) { $final.Add($ours) }

$gpLink = ConvertTo-ADDSGPLinkString $final
Set-ADDSAttribute -DistinguishedName $targetDN -Name 'gPLink' -Values @($gpLink)

# Re-read so the reported order and resolved name reflect what AD actually stored.
$links = (Get-ADDSGPOLinks -TargetDN $targetDN -DomainDN $domainDN).links
$mine = $links | Where-Object { $_.gpo_guid -eq $gpo.Guid } | Select-Object -First 1

[pscustomobject]@{
    exists        = $true
    target_dn     = $targetDN
    gpo_guid      = $gpo.Guid
    gpo_name      = if ($mine) { $mine.gpo_name } else { $null }
    enabled       = (($flags -band 1) -eq 0)
    enforced      = (($flags -band 2) -ne 0)
    current_order = if ($mine) { [int]$mine.order } else { 0 }
} | ConvertTo-Json -Compress
