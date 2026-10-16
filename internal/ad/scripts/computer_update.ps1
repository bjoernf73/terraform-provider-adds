$domainDN = Get-DomainDN
$serverParams = Get-ServerParams
$computer = Get-ComputerByIdentity ([string]$payload.guid)

$setParams = Get-ComputerSetParams
Set-ADComputer -Identity $computer.DistinguishedName @setParams @serverParams -ErrorAction Stop
$computer = Get-ComputerByIdentity ([string]$payload.guid)

Set-ComputerMultiValued $computer.DistinguishedName
$computer = Get-ComputerByIdentity ([string]$payload.guid)

# The sAMAccountName can be changed independently of the CN; AD keeps the trailing '$'.
if (-not [string]::IsNullOrWhiteSpace([string]$payload.sam_account_name)) {
    $desiredSam = Get-StrippedSam ([string]$payload.sam_account_name)
    if ((Get-StrippedSam ([string]$computer.SamAccountName)) -ine $desiredSam) {
        Set-ADComputer -Identity $computer.DistinguishedName -SamAccountName ($desiredSam + '$') @serverParams -ErrorAction Stop
        $computer = Get-ComputerByIdentity ([string]$payload.guid)
    }
}

# Move before rename so the rename targets the final container. Protection is lifted first
# if needed, since a Deny on Delete also blocks moves and renames.
$targetContainerDN = Convert-PathToDN ([string]$payload.path) $domainDN
$needsMove = ((Get-ParentDN $computer.DistinguishedName) -ne $targetContainerDN)
# -cne (case-sensitive): a case-only rename must apply; -ne is case-insensitive.
$needsRename = ([string]$computer.Name -cne [string]$payload.name)

if (($needsMove -or $needsRename) -and [bool]$computer.ProtectedFromAccidentalDeletion) {
    Set-ADObject -Identity $computer.DistinguishedName -ProtectedFromAccidentalDeletion $false @serverParams -ErrorAction Stop
    $computer = Get-ComputerByIdentity ([string]$payload.guid)
}

if ($needsMove) {
    Move-ADObject -Identity $computer.DistinguishedName -TargetPath $targetContainerDN @serverParams -ErrorAction Stop
    $computer = Get-ComputerByIdentity ([string]$payload.guid)
}

if ($needsRename) {
    Rename-ADObject -Identity $computer.DistinguishedName -NewName ([string]$payload.name) @serverParams -ErrorAction Stop
    $computer = Get-ComputerByIdentity ([string]$payload.guid)
}

Sync-ComputerProtection $computer.DistinguishedName ([bool]$computer.ProtectedFromAccidentalDeletion)
$computer = Get-ComputerByIdentity ([string]$payload.guid)

Get-ComputerResult $computer $domainDN | ConvertTo-Json -Compress -Depth 5
