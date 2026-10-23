# User helpers. Requires common.ps1.

# Attributes read directly and reconciled through native Set-ADUser parameters. AD
# cmdlets clear a string attribute when $null is passed, so no separate -Clear/-Replace
# bookkeeping is needed the way group's extra attributes required.
$script:UserProperties = @(
    'Description', 'DisplayName', 'GivenName', 'Surname', 'Initials', 'OtherName',
    'EmailAddress', 'Office', 'OfficePhone', 'HomePhone', 'MobilePhone', 'Fax', 'HomePage',
    'StreetAddress', 'POBox', 'City', 'State', 'PostalCode', 'Country',
    'Company', 'Department', 'Division', 'Organization', 'EmployeeID', 'EmployeeNumber', 'Title',
    'HomeDirectory', 'HomeDrive', 'Manager', 'LogonWorkstations', 'ScriptPath', 'ProfilePath',
    'AccountExpirationDate',
    'Enabled', 'PasswordNeverExpires', 'CannotChangePassword', 'SmartcardLogonRequired', 'TrustedForDelegation',
    'ProtectedFromAccidentalDeletion',
    'DistinguishedName', 'Name', 'SamAccountName', 'UserPrincipalName', 'ObjectGUID', 'SID'
)

function Get-UserByIdentity([string]$Identity) {
    $serverParams = Get-ServerParams
    return Get-ADUser -Identity $Identity -Properties $script:UserProperties @serverParams -ErrorAction Stop
}

function Get-UserOrNull([string]$Identity) {
    try {
        return Get-UserByIdentity $Identity
    }
    catch {
        if (Test-IsIdentityNotFound $_) {
            return $null
        }

        throw
    }
}

function Get-UserResult($User, [string]$DomainDN) {
    $containerDN = Get-ParentDN $User.DistinguishedName

    $pathMatch = $false
    if ($null -ne $payload.path) {
        $pathMatch = ((Convert-PathToDN ([string]$payload.path) $DomainDN) -eq $containerDN)
    }

    $managerDN = [string]$User.Manager
    $managerMatch = $false
    if (-not [string]::IsNullOrWhiteSpace([string]$payload.manager)) {
        try {
            $managerMatch = ((Resolve-ADPrincipal ([string]$payload.manager)).distinguishedName -eq $managerDN)
        }
        catch {
            $managerMatch = $false
        }
    }

    return [pscustomobject]@{
        exists                             = $true
        name                               = $User.Name
        sam_account_name                   = $User.SamAccountName
        user_principal_name                = $User.UserPrincipalName
        path                               = Convert-DNToPath $containerDN $DomainDN
        path_match                         = $pathMatch
        container_dn                       = $containerDN
        distinguished_name                 = $User.DistinguishedName
        guid                               = [string]$User.ObjectGUID
        sid                                = [string]$User.SID
        description                        = $User.Description
        display_name                       = $User.DisplayName
        given_name                         = $User.GivenName
        surname                            = $User.Surname
        initials                           = $User.Initials
        other_name                         = $User.OtherName
        email                              = $User.EmailAddress
        office                             = $User.Office
        office_phone                       = $User.OfficePhone
        home_phone                         = $User.HomePhone
        mobile_phone                       = $User.MobilePhone
        fax                                = $User.Fax
        home_page                          = $User.HomePage
        street_address                     = $User.StreetAddress
        po_box                             = $User.POBox
        city                               = $User.City
        state                              = $User.State
        postal_code                        = $User.PostalCode
        country                            = $User.Country
        company                            = $User.Company
        department                         = $User.Department
        division                           = $User.Division
        organization                       = $User.Organization
        employee_id                        = $User.EmployeeID
        employee_number                    = $User.EmployeeNumber
        title                              = $User.Title
        home_directory                     = $User.HomeDirectory
        home_drive                         = $User.HomeDrive
        logon_workstations                 = $User.LogonWorkstations
        script_path                        = $User.ScriptPath
        profile_path                       = $User.ProfilePath
        account_expiration_date            = if ($null -ne $User.AccountExpirationDate) { $User.AccountExpirationDate.ToString('yyyy-MM-dd') } else { $null }
        manager                            = $managerDN
        manager_match                      = $managerMatch
        enabled                            = [bool]$User.Enabled
        password_never_expires            = [bool]$User.PasswordNeverExpires
        cannot_change_password            = [bool]$User.CannotChangePassword
        smart_card_logon_required         = [bool]$User.SmartcardLogonRequired
        trusted_for_delegation            = [bool]$User.TrustedForDelegation
        protected_from_accidental_deletion = [bool]$User.ProtectedFromAccidentalDeletion
    }
}

# Builds the Set-ADUser parameter table for every reconcilable attribute except identity
# (name/path/sam/upn), which the caller handles separately (create vs. rename/move).
function Get-UserSetParams {
    $manager = $null
    if (-not [string]::IsNullOrWhiteSpace([string]$payload.manager)) {
        $manager = (Resolve-ADPrincipal ([string]$payload.manager)).distinguishedName
    }

    $accountExpirationDate = $null
    if (-not [string]::IsNullOrWhiteSpace([string]$payload.account_expiration_date)) {
        $accountExpirationDate = [datetime]::Parse([string]$payload.account_expiration_date, [System.Globalization.CultureInfo]::InvariantCulture)
    }

    return @{
        Description           = (Get-OptionalString $payload.description)
        DisplayName            = (Get-OptionalString $payload.display_name)
        GivenName              = (Get-OptionalString $payload.given_name)
        Surname                = (Get-OptionalString $payload.surname)
        Initials               = (Get-OptionalString $payload.initials)
        OtherName              = (Get-OptionalString $payload.other_name)
        EmailAddress            = (Get-OptionalString $payload.email)
        Office                  = (Get-OptionalString $payload.office)
        OfficePhone             = (Get-OptionalString $payload.office_phone)
        HomePhone               = (Get-OptionalString $payload.home_phone)
        MobilePhone             = (Get-OptionalString $payload.mobile_phone)
        Fax                     = (Get-OptionalString $payload.fax)
        HomePage                = (Get-OptionalString $payload.home_page)
        StreetAddress           = (Get-OptionalString $payload.street_address)
        POBox                   = (Get-OptionalString $payload.po_box)
        City                    = (Get-OptionalString $payload.city)
        State                   = (Get-OptionalString $payload.state)
        PostalCode              = (Get-OptionalString $payload.postal_code)
        Country                 = (Get-OptionalString $payload.country)
        Company                 = (Get-OptionalString $payload.company)
        Department              = (Get-OptionalString $payload.department)
        Division                = (Get-OptionalString $payload.division)
        Organization            = (Get-OptionalString $payload.organization)
        EmployeeID              = (Get-OptionalString $payload.employee_id)
        EmployeeNumber          = (Get-OptionalString $payload.employee_number)
        Title                   = (Get-OptionalString $payload.title)
        HomeDirectory           = (Get-OptionalString $payload.home_directory)
        HomeDrive               = (Get-OptionalString $payload.home_drive)
        LogonWorkstations       = (Get-OptionalString $payload.logon_workstations)
        ScriptPath              = (Get-OptionalString $payload.script_path)
        ProfilePath             = (Get-OptionalString $payload.profile_path)
        AccountExpirationDate   = $accountExpirationDate
        Manager                 = $manager
        PasswordNeverExpires    = [bool]$payload.password_never_expires
        CannotChangePassword    = [bool]$payload.cannot_change_password
        SmartcardLogonRequired  = [bool]$payload.smart_card_logon_required
        TrustedForDelegation    = [bool]$payload.trusted_for_delegation
    }
}

# Generates a strong random password used only to bring an account into the enabled state
# when it has no usable password yet. It is never returned to Terraform or written to state;
# a password resource (adds_user_password) may replace it afterwards. One character from each
# class is guaranteed so AD's default complexity policy is met, then the result is shuffled.
function New-EnableBootstrapPassword([int]$Length = 25) {
    $lower = 'abcdefghijklmnopqrstuvwxyz'
    $upper = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'
    $digits = '0123456789'
    $symbols = '!@#$%^&*()-_=+[]{}<>?'
    $all = $lower + $upper + $digits + $symbols

    $bytes = [byte[]]::new($Length)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)

    $chars = [char[]]::new($Length)
    $chars[0] = $lower[$bytes[0] % $lower.Length]
    $chars[1] = $upper[$bytes[1] % $upper.Length]
    $chars[2] = $digits[$bytes[2] % $digits.Length]
    $chars[3] = $symbols[$bytes[3] % $symbols.Length]
    for ($i = 4; $i -lt $Length; $i++) {
        $chars[$i] = $all[$bytes[$i] % $all.Length]
    }

    $order = [byte[]]::new($Length)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($order)
    for ($i = $Length - 1; $i -gt 0; $i--) {
        $j = $order[$i] % ($i + 1)
        $tmp = $chars[$i]; $chars[$i] = $chars[$j]; $chars[$j] = $tmp
    }

    return ConvertTo-SecureString -String (-join $chars) -AsPlainText -Force
}

# Reconciles the account's Enabled state. adds_user fully owns the enabled flag and must be
# able to bring the account into the desired state on its own. Enabling an account that has
# no usable password fails the domain password policy; in that case set a throwaway strong
# random password (never surfaced to Terraform or stored in state) purely so the account can
# be enabled. A password resource (adds_user_password) may replace that password later, but
# must not change the enabled state. Any other failure re-throws.
function Set-UserEnabledState([string]$DistinguishedName, [bool]$Desired) {
    $serverParams = Get-ServerParams
    if (-not $Desired) {
        Set-ADUser -Identity $DistinguishedName -Enabled $false @serverParams -ErrorAction Stop
        return
    }

    try {
        Set-ADUser -Identity $DistinguishedName -Enabled $true @serverParams -ErrorAction Stop
    }
    catch {
        if ($_.Exception.Message -notmatch 'password does not meet') {
            throw
        }

        Set-ADAccountPassword -Identity $DistinguishedName -Reset -NewPassword (New-EnableBootstrapPassword) @serverParams -ErrorAction Stop
        Set-ADUser -Identity $DistinguishedName -Enabled $true @serverParams -ErrorAction Stop
    }
}

# Not a Set-ADUser parameter: adds or removes Deny ACEs for Everyone on Delete/DeleteTree.
function Sync-UserProtection([string]$DistinguishedName, [bool]$CurrentValue) {
    $serverParams = Get-ServerParams
    $desired = $false
    if ($null -ne $payload.protected_from_accidental_deletion) {
        $desired = [bool]$payload.protected_from_accidental_deletion
    }

    if ($CurrentValue -ne $desired) {
        Set-ADObject -Identity $DistinguishedName -ProtectedFromAccidentalDeletion $desired @serverParams -ErrorAction Stop
    }
}
