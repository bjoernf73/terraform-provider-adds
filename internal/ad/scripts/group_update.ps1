$domainDN = Get-DomainDN
$serverParams = Get-ServerParams
$group = Get-GroupByIdentity ([string]$payload.guid)

$targetContainerDN = Convert-PathToDN ([string]$payload.path) $domainDN
$needsMove = ((Get-ParentDN $group.DistinguishedName) -ne $targetContainerDN)
# -cne (case-sensitive): a case-only rename (DFMA -> dfma) must apply; -ne is case-insensitive.
$needsRename = ([string]$group.Name -cne [string]$payload.name)

# Accidental deletion protection denies Delete on the object, which also blocks moves.
if (($needsMove -or $needsRename) -and [bool]$group.ProtectedFromAccidentalDeletion) {
    Set-ADObject -Identity $group.DistinguishedName -ProtectedFromAccidentalDeletion $false @serverParams -ErrorAction Stop
    $group = Get-GroupByIdentity ([string]$payload.guid)
}

if ($needsMove) {
    Move-ADObject -Identity $group.DistinguishedName -TargetPath $targetContainerDN @serverParams -ErrorAction Stop
    $group = Get-GroupByIdentity ([string]$payload.guid)
}

if ($needsRename) {
    Rename-ADObject -Identity $group.DistinguishedName -NewName ([string]$payload.name) @serverParams -ErrorAction Stop
    $group = Get-GroupByIdentity ([string]$payload.guid)
}

# Runs last so protection is reapplied after any move or rename.
$group = Sync-GroupProperties $group

Get-GroupResult $group $domainDN | ConvertTo-Json -Compress
