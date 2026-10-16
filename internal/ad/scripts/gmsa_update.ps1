$domainDN = Get-DomainDN
$serverParams = Get-ServerParams
$gmsa = Get-GmsaByIdentity ([string]$payload.guid)

$setParams = Get-GmsaSetParams
Set-ADServiceAccount -Identity $gmsa.DistinguishedName @setParams @serverParams -ErrorAction Stop
$gmsa = Get-GmsaByIdentity ([string]$payload.guid)

Set-GmsaMultiValued $gmsa.DistinguishedName
$gmsa = Get-GmsaByIdentity ([string]$payload.guid)

# The sAMAccountName can be changed independently of the CN; AD keeps the trailing '$'.
$desiredSam = $null
if (-not [string]::IsNullOrWhiteSpace([string]$payload.sam_account_name)) {
    $desiredSam = Get-StrippedSam ([string]$payload.sam_account_name)
    if ((Get-StrippedSam ([string]$gmsa.SamAccountName)) -ine $desiredSam) {
        Set-ADServiceAccount -Identity $gmsa.DistinguishedName -SamAccountName ($desiredSam + '$') @serverParams -ErrorAction Stop
        $gmsa = Get-GmsaByIdentity ([string]$payload.guid)
    }
}

# Move before rename so the rename targets the final container. Protection is lifted first
# if needed, since a Deny on Delete also blocks moves and renames.
$targetContainerDN = Convert-PathToDN ([string]$payload.path) $domainDN
$needsMove = ((Get-ParentDN $gmsa.DistinguishedName) -ne $targetContainerDN)
# -cne (case-sensitive): a case-only rename must apply; -ne is case-insensitive.
$needsRename = ([string]$gmsa.Name -cne [string]$payload.name)

if (($needsMove -or $needsRename) -and [bool]$gmsa.ProtectedFromAccidentalDeletion) {
    Set-ADObject -Identity $gmsa.DistinguishedName -ProtectedFromAccidentalDeletion $false @serverParams -ErrorAction Stop
    $gmsa = Get-GmsaByIdentity ([string]$payload.guid)
}

if ($needsMove) {
    Move-ADObject -Identity $gmsa.DistinguishedName -TargetPath $targetContainerDN @serverParams -ErrorAction Stop
    $gmsa = Get-GmsaByIdentity ([string]$payload.guid)
}

if ($needsRename) {
    Rename-ADObject -Identity $gmsa.DistinguishedName -NewName ([string]$payload.name) @serverParams -ErrorAction Stop
    $gmsa = Get-GmsaByIdentity ([string]$payload.guid)
}

Sync-GmsaProtection $gmsa.DistinguishedName ([bool]$gmsa.ProtectedFromAccidentalDeletion)
$gmsa = Get-GmsaByIdentity ([string]$payload.guid)

Get-GmsaResult $gmsa $domainDN | ConvertTo-Json -Compress -Depth 5
