$domainDN = Get-DomainDN

$guidBytes = ([guid]([string]$payload.guid)).ToByteArray()
$guidFilter = '(objectGUID=' + (ConvertTo-ADDSFilterBytes $guidBytes) + ')'
$attributes = @(
    'name', 'sAMAccountName', 'userPrincipalName', 'description', 'displayName', 'givenName',
    'sn', 'initials', 'middleName', 'mail', 'physicalDeliveryOfficeName', 'telephoneNumber',
    'homePhone', 'mobile', 'facsimileTelephoneNumber', 'wWWHomePage', 'streetAddress',
    'postOfficeBox', 'l', 'st', 'postalCode', 'c', 'company', 'department', 'division', 'o',
    'employeeID', 'employeeNumber', 'title', 'homeDirectory', 'homeDrive', 'userWorkstations',
    'scriptPath', 'profilePath', 'accountExpires', 'manager', 'userAccountControl',
    'objectGUID', 'objectSid'
)
$found = @(Search-ADDSEntries $domainDN $guidFilter ([System.DirectoryServices.Protocols.SearchScope]::Subtree) $attributes)

if ($found.Count -eq 0) {
    [pscustomobject]@{ exists = $false } | ConvertTo-Json -Compress
    return
}

$user = $found[0]
$dn = [string]$user.DistinguishedName
$containerDN = Get-ParentDN $dn
$managerDN = Get-ADDSString $user 'manager'

# Account-control flags live in the userAccountControl bit field.
$uac = [int64](Get-ADDSString $user 'userAccountControl')
$enabled = -not ($uac -band 0x2)
$passwordNeverExpires = [bool]($uac -band 0x10000)
$smartCardRequired = [bool]($uac -band 0x40000)
$trustedForDelegation = [bool]($uac -band 0x80000)

# accountExpires is a FILETIME; 0 and the max value both mean "never".
$accountExpirationDate = $null
$accountExpiresRaw = Get-ADDSString $user 'accountExpires'
if (-not [string]::IsNullOrWhiteSpace($accountExpiresRaw)) {
    $accountExpires = [int64]$accountExpiresRaw
    if ($accountExpires -ne 0 -and $accountExpires -ne 9223372036854775807) {
        $accountExpirationDate = [DateTime]::FromFileTime($accountExpires).ToString('yyyy-MM-dd')
    }
}

$pathMatch = $false
if ($null -ne $payload.path) {
    $pathMatch = ((Convert-PathToDN ([string]$payload.path) $domainDN) -eq $containerDN)
}

$managerMatch = $false
if (-not [string]::IsNullOrWhiteSpace([string]$payload.manager)) {
    $resolved = Resolve-ADDSPrincipal ([string]$payload.manager) @('distinguishedName')
    if ($null -ne $resolved -and [string]$resolved.DistinguishedName -eq [string]$managerDN) {
        $managerMatch = $true
    }
}

$security = Get-ADDSSecurityDescriptor $dn
$protected = Test-ADDSProtectedFromAccidentalDeletion $security
$cannotChangePassword = Test-ADDSCannotChangePassword $security

[pscustomobject]@{
    exists                             = $true
    name                               = Get-ADDSString $user 'name'
    sam_account_name                   = Get-ADDSString $user 'sAMAccountName'
    user_principal_name                = Get-ADDSString $user 'userPrincipalName'
    path                               = Convert-DNToPath $containerDN $domainDN
    path_match                         = $pathMatch
    container_dn                       = $containerDN
    distinguished_name                 = $dn
    guid                               = Get-ADDSGuid $user
    sid                                = Get-ADDSSid $user
    description                        = Get-ADDSString $user 'description'
    display_name                       = Get-ADDSString $user 'displayName'
    given_name                         = Get-ADDSString $user 'givenName'
    surname                            = Get-ADDSString $user 'sn'
    initials                           = Get-ADDSString $user 'initials'
    other_name                         = Get-ADDSString $user 'middleName'
    email                              = Get-ADDSString $user 'mail'
    office                             = Get-ADDSString $user 'physicalDeliveryOfficeName'
    office_phone                       = Get-ADDSString $user 'telephoneNumber'
    home_phone                         = Get-ADDSString $user 'homePhone'
    mobile_phone                       = Get-ADDSString $user 'mobile'
    fax                                = Get-ADDSString $user 'facsimileTelephoneNumber'
    home_page                          = Get-ADDSString $user 'wWWHomePage'
    street_address                     = Get-ADDSString $user 'streetAddress'
    po_box                             = Get-ADDSString $user 'postOfficeBox'
    city                               = Get-ADDSString $user 'l'
    state                              = Get-ADDSString $user 'st'
    postal_code                        = Get-ADDSString $user 'postalCode'
    country                            = Get-ADDSString $user 'c'
    company                            = Get-ADDSString $user 'company'
    department                         = Get-ADDSString $user 'department'
    division                           = Get-ADDSString $user 'division'
    organization                       = Get-ADDSString $user 'o'
    employee_id                        = Get-ADDSString $user 'employeeID'
    employee_number                    = Get-ADDSString $user 'employeeNumber'
    title                              = Get-ADDSString $user 'title'
    home_directory                     = Get-ADDSString $user 'homeDirectory'
    home_drive                         = Get-ADDSString $user 'homeDrive'
    logon_workstations                 = Get-ADDSString $user 'userWorkstations'
    script_path                        = Get-ADDSString $user 'scriptPath'
    profile_path                       = Get-ADDSString $user 'profilePath'
    account_expiration_date            = $accountExpirationDate
    manager                            = [string]$managerDN
    manager_match                      = $managerMatch
    enabled                            = $enabled
    password_never_expires             = $passwordNeverExpires
    cannot_change_password             = $cannotChangePassword
    smart_card_logon_required          = $smartCardRequired
    trusted_for_delegation             = $trustedForDelegation
    protected_from_accidental_deletion = $protected
} | ConvertTo-Json -Compress
