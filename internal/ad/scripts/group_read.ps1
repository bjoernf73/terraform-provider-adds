$domainDN = Get-DomainDN

$guidBytes = ([guid]([string]$payload.guid)).ToByteArray()
$guidFilter = '(objectGUID=' + (ConvertTo-ADDSFilterBytes $guidBytes) + ')'
$attributes = @('name', 'sAMAccountName', 'description', 'displayName', 'mail', 'info', 'wWWHomePage', 'managedBy', 'groupType', 'objectGUID', 'objectSid')
$found = @(Search-ADDSEntries $domainDN $guidFilter ([System.DirectoryServices.Protocols.SearchScope]::Subtree) $attributes)

if ($found.Count -eq 0) {
    [pscustomobject]@{
        exists = $false
    } | ConvertTo-Json -Compress
    return
}

$group = $found[0]
$dn = [string]$group.DistinguishedName
$containerDN = Get-ParentDN $dn
$managedBy = Get-ADDSString $group 'managedBy'

# groupType is a bit flag: scope in the low bits, 0x80000000 marks a security group.
$groupType = [int64](Get-ADDSString $group 'groupType')
if ($groupType -band 0x8) { $scope = 'Universal' }
elseif ($groupType -band 0x4) { $scope = 'DomainLocal' }
else { $scope = 'Global' }
if ($groupType -band 0x80000000) { $category = 'Security' } else { $category = 'Distribution' }

# path and managed_by may be configured in a different but equivalent spelling; report whether
# the stored value matches so the provider can keep the user's spelling.
$pathMatch = $false
if ($null -ne $payload.path) {
    $pathMatch = ((Convert-PathToDN ([string]$payload.path) $domainDN) -eq $containerDN)
}

$managedByMatch = $false
if (-not [string]::IsNullOrWhiteSpace([string]$payload.managed_by)) {
    $resolved = Resolve-ADDSPrincipal ([string]$payload.managed_by) @('distinguishedName')
    if ($null -ne $resolved -and [string]$resolved.DistinguishedName -eq [string]$managedBy) {
        $managedByMatch = $true
    }
}

$security = Get-ADDSSecurityDescriptor $dn
$protected = Test-ADDSProtectedFromAccidentalDeletion $security

$managerCanUpdate = $false
if (-not [string]::IsNullOrWhiteSpace([string]$managedBy)) {
    $managerSid = Get-ADDSSid (Get-ADDSEntry -DistinguishedName ([string]$managedBy) -Attributes @('objectSid'))
    $managerCanUpdate = Test-ADDSMemberWriteGranted $security $managerSid
}

[pscustomobject]@{
    exists                             = $true
    name                               = Get-ADDSString $group 'name'
    sam_account_name                   = Get-ADDSString $group 'sAMAccountName'
    description                        = Get-ADDSString $group 'description'
    display_name                       = Get-ADDSString $group 'displayName'
    mail                               = Get-ADDSString $group 'mail'
    info                               = Get-ADDSString $group 'info'
    homepage                           = Get-ADDSString $group 'wWWHomePage'
    managed_by                         = [string]$managedBy
    managed_by_match                   = $managedByMatch
    manager_can_update_membership      = $managerCanUpdate
    protected_from_accidental_deletion = $protected
    category                           = $category
    scope                              = $scope
    path                               = Convert-DNToPath $containerDN $domainDN
    path_match                         = $pathMatch
    container_dn                       = $containerDN
    distinguished_name                 = $dn
    guid                               = Get-ADDSGuid $group
    sid                                = Get-ADDSSid $group
} | ConvertTo-Json -Compress
