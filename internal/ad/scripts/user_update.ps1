$domainDN = Get-DomainDN
$serverParams = Get-ServerParams
$user = Get-UserByIdentity ([string]$payload.guid)

$setParams = Get-UserSetParams
Set-ADUser -Identity $user.DistinguishedName @setParams @serverParams -ErrorAction Stop

Set-UserEnabledState $user.DistinguishedName ([bool]$payload.enabled)

# -cne (case-sensitive): a case-only sAMAccountName change must still apply.
if ([string]$user.SamAccountName -cne [string]$payload.sam_account_name) {
    Set-ADUser -Identity $user.DistinguishedName -SamAccountName ([string]$payload.sam_account_name) @serverParams -ErrorAction Stop
}

# -cne (case-sensitive): a case-only userPrincipalName change must still apply.
if ([string]$user.UserPrincipalName -cne [string]$payload.user_principal_name) {
    Set-ADUser -Identity $user.DistinguishedName -UserPrincipalName ([string]$payload.user_principal_name) @serverParams -ErrorAction Stop
}

$user = Get-UserByIdentity ([string]$payload.guid)

# Move before rename so the rename targets the final container. Protection is lifted
# first if needed, since a Deny on Delete also blocks moves and renames.
$targetContainerDN = Convert-PathToDN ([string]$payload.path) $domainDN
$needsMove = ((Get-ParentDN $user.DistinguishedName) -ne $targetContainerDN)
# -cne (case-sensitive): a case-only rename must apply; -ne is case-insensitive.
$needsRename = ([string]$user.Name -cne [string]$payload.name)

if (($needsMove -or $needsRename) -and [bool]$user.ProtectedFromAccidentalDeletion) {
    Set-ADObject -Identity $user.DistinguishedName -ProtectedFromAccidentalDeletion $false @serverParams -ErrorAction Stop
    $user = Get-UserByIdentity ([string]$payload.guid)
}

if ($needsMove) {
    Move-ADObject -Identity $user.DistinguishedName -TargetPath $targetContainerDN @serverParams -ErrorAction Stop
    $user = Get-UserByIdentity ([string]$payload.guid)
}

if ($needsRename) {
    Rename-ADObject -Identity $user.DistinguishedName -NewName ([string]$payload.name) @serverParams -ErrorAction Stop
    $user = Get-UserByIdentity ([string]$payload.guid)
}

Sync-UserProtection $user.DistinguishedName ([bool]$user.ProtectedFromAccidentalDeletion)
$user = Get-UserByIdentity ([string]$payload.guid)

Get-UserResult $user $domainDN | ConvertTo-Json -Compress
