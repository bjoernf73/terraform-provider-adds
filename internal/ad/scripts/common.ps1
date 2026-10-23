# Helpers shared by every AD object type. Prefixed ahead of each operation script.
$ErrorActionPreference = 'Stop'

# stdout must carry nothing but the operation's JSON document. Silence the warning stream (for
# example the GroupPolicy module's WinPSCompatSession notice) so a stray warning cannot corrupt it.
$WarningPreference = 'SilentlyContinue'

# Keep ANSI escapes out of stderr so provider diagnostics stay readable.
if ($null -ne $PSStyle) {
    $PSStyle.OutputRendering = 'PlainText'
}

# The ActiveDirectory module is imported by buildScript (Go) only for operations that still need
# it; operations migrated to the LDAP helpers below run without it. See moduleFreeOperations.

function Get-ServerParams {
    $serverParams = @{}
    if ($null -ne $payload.domain_controller -and -not [string]::IsNullOrWhiteSpace([string]$payload.domain_controller)) {
        $serverParams['Server'] = [string]$payload.domain_controller
    }
    return $serverParams
}

# ---------------------------------------------------------------------------
# LDAP (System.DirectoryServices.Protocols) helpers
#
# S.DS.P ships in the base class library, so unlike the ActiveDirectory module it needs no
# import. That import costs ~600 ms per process, and the provider runs every operation in a
# fresh remote shell, so the cost is paid again on every single read and write. Reads served
# from these helpers skip it entirely. One connection is cached per process and reused.
# ---------------------------------------------------------------------------

$script:ADDSLdapConnection = $null

# The DC or domain to bind to, resolved without depending on any single environment variable.
# Preference: the configured domain_controller; then USERDNSDOMAIN (populated under WinRM but
# often absent in an SSH exec session); then the machine's domain membership read from the
# directory and, last, from the local network configuration. The domain forms let the DC
# locator resolve a reachable DC.
function Get-ADDSLdapServer {
    if ($null -ne $payload -and $null -ne $payload.domain_controller -and -not [string]::IsNullOrWhiteSpace([string]$payload.domain_controller)) {
        return [string]$payload.domain_controller
    }

    if (-not [string]::IsNullOrWhiteSpace($env:USERDNSDOMAIN)) {
        return [string]$env:USERDNSDOMAIN
    }

    try {
        return [System.DirectoryServices.ActiveDirectory.Domain]::GetComputerDomain().Name
    }
    catch {
    }

    try {
        return [System.DirectoryServices.ActiveDirectory.Domain]::GetCurrentDomain().Name
    }
    catch {
    }

    try {
        $dnsDomain = [System.Net.NetworkInformation.IPGlobalProperties]::GetIPGlobalProperties().DomainName
        if (-not [string]::IsNullOrWhiteSpace($dnsDomain)) {
            return $dnsDomain
        }
    }
    catch {
    }

    return $null
}

function Get-ADDSLdapConnection {
    if ($null -ne $script:ADDSLdapConnection) {
        return $script:ADDSLdapConnection
    }

    $server = Get-ADDSLdapServer
    if ([string]::IsNullOrWhiteSpace($server)) {
        throw "cannot determine a domain controller to bind to: set domain_controller or run on a domain-joined host"
    }

    $identifier = New-Object System.DirectoryServices.Protocols.LdapDirectoryIdentifier($server)
    $connection = New-Object System.DirectoryServices.Protocols.LdapConnection($identifier)
    $connection.AuthType = [System.DirectoryServices.Protocols.AuthType]::Negotiate
    $connection.SessionOptions.ProtocolVersion = 3
    # Sign and encrypt the LDAP traffic so the bind satisfies a DC configured to require
    # LDAP signing (the default hardening on current Windows Server).
    $connection.SessionOptions.Sealing = $true
    $connection.Bind()

    $script:ADDSLdapConnection = $connection
    return $connection
}

# Reads the RootDSE (base scope, empty DN) and returns the requested operational attributes.
function Get-ADDSRootDSE([string[]]$Attributes) {
    $connection = Get-ADDSLdapConnection
    $request = New-Object System.DirectoryServices.Protocols.SearchRequest('', '(objectClass=*)', ([System.DirectoryServices.Protocols.SearchScope]::Base), $Attributes)
    $response = $connection.SendRequest($request)
    return $response.Entries[0]
}

# Base-reads a single entry by DN, returning $null when it does not exist.
function Get-ADDSEntry([string]$DistinguishedName, [string[]]$Attributes, [string]$Filter = '(objectClass=*)') {
    $connection = Get-ADDSLdapConnection
    $request = New-Object System.DirectoryServices.Protocols.SearchRequest($DistinguishedName, $Filter, ([System.DirectoryServices.Protocols.SearchScope]::Base), $Attributes)
    try {
        $response = $connection.SendRequest($request)
    }
    catch [System.DirectoryServices.Protocols.DirectoryOperationException] {
        if ($_.Exception.Response.ResultCode -eq [System.DirectoryServices.Protocols.ResultCode]::NoSuchObject) {
            return $null
        }
        throw
    }

    if ($response.Entries.Count -eq 0) {
        return $null
    }

    return $response.Entries[0]
}

# Searches under a base DN and returns the matching entries (possibly none).
function Search-ADDSEntries([string]$BaseDN, [string]$Filter, [System.DirectoryServices.Protocols.SearchScope]$Scope, [string[]]$Attributes) {
    $connection = Get-ADDSLdapConnection
    $request = New-Object System.DirectoryServices.Protocols.SearchRequest($BaseDN, $Filter, $Scope, $Attributes)
    $response = $connection.SendRequest($request)
    return $response.Entries
}

# Replaces an attribute's values via an LDAP modify. Passing no values (or only empty strings)
# clears the attribute: an LDAP replace with no values removes it.
function Set-ADDSAttribute([string]$DistinguishedName, [string]$Name, [string[]]$Values) {
    $connection = Get-ADDSLdapConnection
    $modification = New-Object System.DirectoryServices.Protocols.DirectoryAttributeModification
    $modification.Name = $Name
    $modification.Operation = [System.DirectoryServices.Protocols.DirectoryAttributeOperation]::Replace
    foreach ($value in @($Values)) {
        if (-not [string]::IsNullOrEmpty($value)) {
            [void]$modification.Add($value)
        }
    }
    $request = New-Object System.DirectoryServices.Protocols.ModifyRequest($DistinguishedName, $modification)
    [void]$connection.SendRequest($request)
}

# Reads a single string-valued attribute, or $null when absent.
function Get-ADDSString($Entry, [string]$Name) {
    if ($null -eq $Entry -or -not $Entry.Attributes.Contains($Name)) {
        return $null
    }

    return [string]$Entry.Attributes[$Name][0]
}

# Reads every value of a multi-valued string attribute as an array (empty when absent).
function Get-ADDSStrings($Entry, [string]$Name) {
    if ($null -eq $Entry -or -not $Entry.Attributes.Contains($Name)) {
        return @()
    }

    return @($Entry.Attributes[$Name].GetValues([string]))
}

# Converts the DC= components of a distinguished name to a dotted DNS name.
function ConvertFrom-DNToDnsName([string]$DistinguishedName) {
    $labels = @($DistinguishedName -split '(?<!\\),' | Where-Object { $_ -match '^\s*DC=' } | ForEach-Object { ($_ -replace '^\s*DC=', '').Trim() })
    return ($labels -join '.')
}

# Reads the packed Version watermark from a GPO's SYSVOL GPT.ini. $SysvolPath is the
# gPCFileSysPath UNC (also exposed as a Gpo object's .Path). Returns 0 when the file or the
# Version line is absent. The low word is the computer version, the high word the user version.
function Get-ADDSGptIniVersion([string]$SysvolPath) {
    if ([string]::IsNullOrWhiteSpace($SysvolPath)) {
        return [int64]0
    }

    $gptIni = Join-Path $SysvolPath 'GPT.ini'
    if (-not (Test-Path -LiteralPath $gptIni)) {
        return [int64]0
    }

    foreach ($line in [System.IO.File]::ReadAllLines($gptIni)) {
        if ($line -match '^\s*Version\s*=\s*(\d+)') {
            return [int64]$Matches[1]
        }
    }

    return [int64]0
}

# Maps msDS-Behavior-Version to the ADDomainMode token the ActiveDirectory module reports, so
# state carries the same value whichever read path produced it.
function Convert-DomainModeFromBehaviorVersion($Version) {
    switch ([int]$Version) {
        0 { 'Windows2000Domain' }
        1 { 'Windows2003InterimDomain' }
        2 { 'Windows2003Domain' }
        3 { 'Windows2008Domain' }
        4 { 'Windows2008R2Domain' }
        5 { 'Windows2012Domain' }
        6 { 'Windows2012R2Domain' }
        7 { 'Windows2016Domain' }
        default { 'Unknown' }
    }
}

# fSMORoleOwner points at an NTDS Settings object; its parent server object carries the
# dNSHostName that the ActiveDirectory module reports for the role holder.
function Resolve-ADDSFsmoHost([string]$NtdsSettingsDN) {
    if ([string]::IsNullOrWhiteSpace($NtdsSettingsDN)) {
        return $null
    }

    $serverDN = Get-ParentDN $NtdsSettingsDN
    $entry = Get-ADDSEntry -DistinguishedName $serverDN -Attributes @('dNSHostName')
    return Get-ADDSString $entry 'dNSHostName'
}

# Decodes an entry's objectSid to its SDDL string, or $null when absent.
function Get-ADDSSid($Entry) {
    if ($null -eq $Entry -or -not $Entry.Attributes.Contains('objectSid')) {
        return $null
    }

    $bytes = $Entry.Attributes['objectSid'].GetValues([byte[]])[0]
    return (New-Object System.Security.Principal.SecurityIdentifier($bytes, 0)).Value
}

# Decodes an entry's objectGUID to its canonical string, or $null when absent.
function Get-ADDSGuid($Entry) {
    if ($null -eq $Entry -or -not $Entry.Attributes.Contains('objectGUID')) {
        return $null
    }

    $bytes = $Entry.Attributes['objectGUID'].GetValues([byte[]])[0]
    return ([guid]::new($bytes)).ToString()
}

# Escapes a byte array as the backslash-hex form an LDAP filter needs for binary attributes.
function ConvertTo-ADDSFilterBytes([byte[]]$Bytes) {
    return (($Bytes | ForEach-Object { '\{0:x2}' -f $_ }) -join '')
}

# Reads an object's security descriptor (owner/group/DACL, never the SACL, so no privilege is
# required) as a parsed ActiveDirectorySecurity, or $null when the object has none.
function Get-ADDSSecurityDescriptor([string]$DistinguishedName) {
    $connection = Get-ADDSLdapConnection
    $request = New-Object System.DirectoryServices.Protocols.SearchRequest($DistinguishedName, '(objectClass=*)', ([System.DirectoryServices.Protocols.SearchScope]::Base), @('nTSecurityDescriptor'))
    $masks = [System.DirectoryServices.Protocols.SecurityMasks]::Owner -bor [System.DirectoryServices.Protocols.SecurityMasks]::Group -bor [System.DirectoryServices.Protocols.SecurityMasks]::Dacl
    [void]$request.Controls.Add((New-Object System.DirectoryServices.Protocols.SecurityDescriptorFlagControl($masks)))
    try {
        $response = $connection.SendRequest($request)
    }
    catch [System.DirectoryServices.Protocols.DirectoryOperationException] {
        if ($_.Exception.Response.ResultCode -eq [System.DirectoryServices.Protocols.ResultCode]::NoSuchObject) {
            return $null
        }
        throw
    }

    if ($response.Entries.Count -eq 0 -or -not $response.Entries[0].Attributes.Contains('ntsecuritydescriptor')) {
        return $null
    }

    $bytes = $response.Entries[0].Attributes['nTSecurityDescriptor'].GetValues([byte[]])[0]
    $security = New-Object System.DirectoryServices.ActiveDirectorySecurity
    $security.SetSecurityDescriptorBinaryForm($bytes)
    return $security
}

# True when the descriptor carries the ProtectedFromAccidentalDeletion ACE: an explicit Deny for
# Everyone (S-1-1-0) including Delete and DeleteTree, matching the ActiveDirectory module.
function Test-ADDSProtectedFromAccidentalDeletion($Security) {
    if ($null -eq $Security) {
        return $false
    }

    $everyone = 'S-1-1-0'
    $delete = [System.DirectoryServices.ActiveDirectoryRights]::Delete
    $deleteTree = [System.DirectoryServices.ActiveDirectoryRights]::DeleteTree
    foreach ($ace in $Security.GetAccessRules($true, $false, [System.Security.Principal.SecurityIdentifier])) {
        if ($ace.AccessControlType -ne [System.Security.AccessControl.AccessControlType]::Deny) { continue }
        if ([string]$ace.IdentityReference -ne $everyone) { continue }
        $rights = $ace.ActiveDirectoryRights
        if ((($rights -band $delete) -eq $delete) -and (($rights -band $deleteTree) -eq $deleteTree)) {
            return $true
        }
    }

    return $false
}

# True when the descriptor grants the SID an explicit Allow WriteProperty on the group member
# attribute - the ACE that 'manager can update membership' adds.
function Test-ADDSMemberWriteGranted($Security, [string]$Sid) {
    if ($null -eq $Security -or [string]::IsNullOrWhiteSpace($Sid)) {
        return $false
    }

    $memberGuid = [guid]'bf9679c0-0de6-11d0-a285-00aa003049e2'
    foreach ($ace in $Security.GetAccessRules($true, $false, [System.Security.Principal.SecurityIdentifier])) {
        if ($ace.IsInherited) { continue }
        if ($ace.ObjectType -ne $memberGuid) { continue }
        if ($ace.AccessControlType -ne [System.Security.AccessControl.AccessControlType]::Allow) { continue }
        if (-not ($ace.ActiveDirectoryRights.ToString() -match 'WriteProperty')) { continue }
        if ([string]$ace.IdentityReference -eq $Sid) { return $true }
    }

    return $false
}

# True when the descriptor denies the User-Change-Password control right to Everyone or Self,
# which is how 'user cannot change password' is stored. Matches the ActiveDirectory module.
function Test-ADDSCannotChangePassword($Security) {
    if ($null -eq $Security) {
        return $false
    }

    $changePassword = [guid]'ab721a53-1e2f-11d0-9819-00aa0040529b'
    $everyone = 'S-1-1-0'
    $self = 'S-1-5-10'
    foreach ($ace in $Security.GetAccessRules($true, $false, [System.Security.Principal.SecurityIdentifier])) {
        if ($ace.AccessControlType -ne [System.Security.AccessControl.AccessControlType]::Deny) { continue }
        if ($ace.ObjectType -ne $changePassword) { continue }
        $identity = [string]$ace.IdentityReference
        if ($identity -eq $everyone -or $identity -eq $self) { return $true }
    }

    return $false
}

# LDAP counterpart of Resolve-ADPrincipal: resolves a DN, GUID, SID, DOMAIN\name or sAMAccountName
# to an entry carrying the requested attributes, or $null when none matches.
function Resolve-ADDSPrincipal([string]$Identity, [string[]]$Attributes = @('distinguishedName', 'objectSid')) {
    if ($Identity -match '^(CN|OU|DC)=') {
        return Get-ADDSEntry -DistinguishedName $Identity -Attributes $Attributes
    }

    $domainDN = Get-DomainDN
    $parsedGuid = [guid]::Empty
    if ([guid]::TryParse($Identity, [ref]$parsedGuid)) {
        $filter = '(objectGUID=' + (ConvertTo-ADDSFilterBytes $parsedGuid.ToByteArray()) + ')'
    }
    elseif ($Identity -match '^S-\d-') {
        $sidObject = New-Object System.Security.Principal.SecurityIdentifier($Identity)
        $sidBytes = New-Object byte[] ($sidObject.BinaryLength)
        $sidObject.GetBinaryForm($sidBytes, 0)
        $filter = '(objectSid=' + (ConvertTo-ADDSFilterBytes $sidBytes) + ')'
    }
    else {
        $sam = $Identity
        if ($sam.Contains('\')) { $sam = $sam.Split('\')[-1] }
        $filter = '(sAMAccountName=' + (ConvertTo-LDAPFilterValue $sam) + ')'
    }

    $hits = @(Search-ADDSEntries $domainDN $filter ([System.DirectoryServices.Protocols.SearchScope]::Subtree) $Attributes)
    if ($hits.Count -gt 0) {
        return $hits[0]
    }

    return $null
}

function Get-DomainDN {
    $entry = Get-ADDSRootDSE @('defaultNamingContext')
    return [string]$entry.Attributes['defaultNamingContext'][0]
}

function Get-OptionalString($Value) {
    if ($null -eq $Value -or [string]::IsNullOrWhiteSpace([string]$Value)) {
        return $null
    }

    return [string]$Value
}

# A machine-account sAMAccountName always ends with '$'. Users may configure the account name
# with or without it; strip a single trailing '$' so both spellings resolve to the same account.
function Get-StrippedSam([string]$Value) {
    $trimmed = ([string]$Value).Trim()
    if ($trimmed.EndsWith('$')) {
        return $trimmed.Substring(0, $trimmed.Length - 1)
    }

    return $trimmed
}

# msDS-SupportedEncryptionTypes is a bit flag. Map it to the token set the AD account cmdlets
# accept so state carries a stable, human-readable value.
function Convert-EncryptionTypesToTokens($Value) {
    $bits = 0
    if ($null -ne $Value) {
        # Reading msDS-SupportedEncryptionTypes yields an ADPropertyValueCollection; unwrap the
        # first element before casting so both scalar and collection inputs convert cleanly.
        $scalar = $Value
        if ($scalar -is [System.Collections.IEnumerable] -and $scalar -isnot [string]) {
            $scalar = @($scalar)[0]
        }

        if ($null -ne $scalar) {
            $bits = [int]$scalar
        }
    }

    if ($bits -eq 0) {
        return @('None')
    }

    $tokens = New-Object System.Collections.Generic.List[string]
    if ($bits -band 0x3) { $tokens.Add('DES') }   # DES-CBC-CRC (0x1) or DES-CBC-MD5 (0x2)
    if ($bits -band 0x4) { $tokens.Add('RC4') }
    if ($bits -band 0x8) { $tokens.Add('AES128') }
    if ($bits -band 0x10) { $tokens.Add('AES256') }

    # Return the bare tokens; every caller re-wraps with @() to normalize to a JSON array.
    return $tokens.ToArray()
}

# Accepts a slash-delimited OU path relative to the domain root, a distinguished name
# relative to the domain root, or a full distinguished name. Slash segments default to
# OU= but may carry their own RDN prefix. Slash segments are always OUs; a segment that
# matches the domain name is still an OU name, not a domain component.
function Convert-PathToDN([string]$Path, [string]$DomainDN) {
    $trimmed = ([string]$Path).Trim()
    if ([string]::IsNullOrWhiteSpace($trimmed)) {
        return $DomainDN
    }

    if ($trimmed -match ([regex]::Escape($DomainDN) + '\s*$')) {
        return $trimmed
    }

    if ($trimmed -match '^\s*DC=') {
        return $trimmed
    }

    # A relative DN such as 'CN=Computers' or 'CN=Services,CN=Configuration'.
    if (($trimmed -notmatch '/') -and ($trimmed -match '^[A-Za-z]+=')) {
        return $trimmed.TrimEnd(',') + ',' + $DomainDN
    }

    $segments = @($trimmed -split '/' | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne '' })
    if ($segments.Count -eq 0) {
        return $DomainDN
    }

    [array]::Reverse($segments)
    $parts = $segments | ForEach-Object {
        if ($_ -match '^[A-Za-z]+=') { $_ } else { 'OU=' + $_ }
    }

    return ($parts -join ',') + ',' + $DomainDN
}

function Convert-DNToPath([string]$DistinguishedName, [string]$DomainDN) {
    $relativeDN = $DistinguishedName
    if ($relativeDN.EndsWith(',' + $DomainDN)) {
        $relativeDN = $relativeDN.Substring(0, $relativeDN.Length - ($DomainDN.Length + 1))
    }

    $segments = @($relativeDN -split ',' | ForEach-Object {
        if ($_.StartsWith('OU=')) {
            $_.Substring(3)
        }
        else {
            $_
        }
    })

    [array]::Reverse($segments)
    return ($segments -join '/')
}

function Get-ParentDN([string]$DistinguishedName) {
    $parts = $DistinguishedName -split '(?<!\\),'
    return ($parts[1..($parts.Count - 1)] -join ',')
}

function Test-IsIdentityNotFound([System.Management.Automation.ErrorRecord]$ErrorRecord) {
    if ($null -eq $ErrorRecord) {
        return $false
    }

    if ($ErrorRecord.CategoryInfo.Reason -eq 'ADIdentityNotFoundException') {
        return $true
    }

    if ($null -ne $ErrorRecord.Exception -and $ErrorRecord.Exception.Message -match 'Cannot find an object with identity') {
        return $true
    }

    return $false
}

function Test-IsIdentityAlreadyExists([System.Management.Automation.ErrorRecord]$ErrorRecord) {
    if ($null -eq $ErrorRecord) {
        return $false
    }

    if ($ErrorRecord.CategoryInfo.Reason -eq 'ADIdentityAlreadyExistsException') {
        return $true
    }

    if ($null -ne $ErrorRecord.Exception -and $ErrorRecord.Exception.Message -match 'already exists') {
        return $true
    }

    return $false
}

function ConvertTo-LDAPFilterValue([string]$Value) {
    # RFC 4515 escaping: these characters are otherwise filter syntax.
    $builder = New-Object System.Text.StringBuilder
    foreach ($char in $Value.ToCharArray()) {
        switch ($char) {
            '\' { [void]$builder.Append('\5c') }
            '*' { [void]$builder.Append('\2a') }
            '(' { [void]$builder.Append('\28') }
            ')' { [void]$builder.Append('\29') }
            "`0" { [void]$builder.Append('\00') }
            default { [void]$builder.Append($char) }
        }
    }

    return $builder.ToString()
}

# Resolves a distinguished name, GUID, SID, DOMAIN\name or sAMAccountName to a
# directory object carrying the identifiers the provider stores in state.
function Resolve-ADPrincipal([string]$Identity) {
    $serverParams = Get-ServerParams
    $properties = @('objectSid', 'objectGUID', 'distinguishedName', 'sAMAccountName', 'objectClass')

    $parsedGuid = [guid]::Empty
    if (($Identity -match '^(CN|OU|DC)=') -or [guid]::TryParse($Identity, [ref]$parsedGuid)) {
        return Get-ADObject -Identity $Identity -Properties $properties @serverParams -ErrorAction Stop
    }

    if ($Identity -match '^S-\d-') {
        $escapedSid = ConvertTo-LDAPFilterValue $Identity
        $bySid = @(Get-ADObject -LDAPFilter "(objectSid=$escapedSid)" -Properties $properties @serverParams -ErrorAction Stop)
        if ($bySid.Count -gt 0) {
            return $bySid[0]
        }

        throw "no directory object has SID '$Identity'"
    }

    $samAccountName = $Identity
    if ($samAccountName.Contains('\')) {
        $samAccountName = $samAccountName.Split('\')[-1]
    }

    $escaped = ConvertTo-LDAPFilterValue $samAccountName
    $byName = @(Get-ADObject -LDAPFilter "(sAMAccountName=$escaped)" -Properties $properties @serverParams -ErrorAction Stop)
    if ($byName.Count -gt 0) {
        return $byName[0]
    }

    throw "no directory object found for identity '$Identity'"
}

# Security descriptors are read and written through the AD: drive.
function Get-ADObjectAclPath([string]$DistinguishedName) {
    if ($null -eq (Get-PSDrive -Name 'AD' -ErrorAction SilentlyContinue)) {
        New-PSDrive -Name 'AD' -PSProvider 'ActiveDirectory' -Root '//RootDSE/' -ErrorAction Stop | Out-Null
    }

    # Pin the drive to the configured DC so ACL reads and writes hit the same replica.
    if ($null -ne $payload.domain_controller -and -not [string]::IsNullOrWhiteSpace([string]$payload.domain_controller)) {
        (Get-PSDrive -Name 'AD' -ErrorAction Stop).Server = [string]$payload.domain_controller
    }

    return "AD:\$DistinguishedName"
}

# adminCount is not a default property, so it must always be requested explicitly.
function Get-AdminCount([string]$DistinguishedName) {
    $serverParams = Get-ServerParams
    $object = Get-ADObject -Identity $DistinguishedName -Properties adminCount @serverParams -ErrorAction Stop

    if ($null -eq $object.adminCount) {
        return 0
    }

    return [int]$object.adminCount
}

# pwdLastSet is a raw FILETIME (100-ns intervals since 1601), not a DateTime, and is 0
# when no password has ever been set. Returned as an ISO 8601 string, or $null, so callers
# can detect a password changing (by anyone, anywhere) without ever seeing the password
# itself.
function Get-PasswordLastSet([string]$DistinguishedName) {
    $serverParams = Get-ServerParams
    $object = Get-ADObject -Identity $DistinguishedName -Properties pwdLastSet @serverParams -ErrorAction Stop

    $raw = [int64]$object.pwdLastSet
    if ($raw -le 0) {
        return $null
    }

    return [DateTime]::FromFileTimeUtc($raw).ToString('o')
}

# Objects protected by AdminSDHolder (adminCount = 1) have their ACL reset to match
# AdminSDHolder's on the next SDProp run, typically within an hour. Any explicit ACE this
# provider adds would silently disappear, so refuse unless the caller opts in.
function Assert-NotAdminCountProtected([string]$DistinguishedName, [bool]$IgnoreAdminCount1) {
    if ($IgnoreAdminCount1) {
        return
    }

    if ((Get-AdminCount $DistinguishedName) -ne 1) {
        return
    }

    throw "'$DistinguishedName' has adminCount=1 (protected by AdminSDHolder). Its ACL is periodically " +
        "reset by SDProp to match AdminSDHolder, so any ACE added here will be silently reverted. " +
        "Set ignore_admin_count_1 = true to proceed anyway."
}

# ---------------------------------------------------------------------------
# Event logging
#
# Every operation that changes AD, and every operation that fails, records an entry in a
# classic Windows event log named 'terraform-provider-adds' on the target host. Each
# operation script logs under its own source (the script file name without the .ps1
# extension) so entries can be filtered per operation. The log and each source are created
# on demand. The *-EventLog cmdlets were removed from PowerShell 7, so the .NET
# System.Diagnostics.EventLog API is used directly.
# ---------------------------------------------------------------------------

$script:ADDSEventLogName = 'terraform-provider-adds'

# Key names whose values must never be written to the event log in cleartext.
$script:ADDSSensitiveKeyPattern = 'password|passphrase|secret|credential|private_key|pfx|token'

# Ensures the classic event log and the given source both exist. CreateEventSource creates
# the log too when it is missing. Requires administrator rights on the target host and is a
# no-op once the source is registered.
function Register-ADDSEventSource([string]$Source) {
    if ([string]::IsNullOrWhiteSpace($Source)) {
        return
    }

    if ([System.Diagnostics.EventLog]::SourceExists($Source)) {
        return
    }

    [System.Diagnostics.EventLog]::CreateEventSource($Source, $script:ADDSEventLogName)
}

# Writes one entry to the provider event log, registering the source on first use. Logging
# must never mask the operation it describes, so permission or registration failures are
# swallowed.
function Write-ADDSEvent {
    param(
        [string]$Source,
        [System.Diagnostics.EventLogEntryType]$EntryType = [System.Diagnostics.EventLogEntryType]::Information,
        [string]$Message,
        [int]$EventId = 1000
    )

    if ([string]::IsNullOrWhiteSpace($Source)) {
        return
    }

    try {
        Register-ADDSEventSource $Source
        [System.Diagnostics.EventLog]::WriteEntry($Source, $Message, $EntryType, $EventId)
    }
    catch {
        # Best effort only: never let an event-log failure break the AD operation.
    }
}

# Renders $payload as JSON for inclusion in log entries, masking values whose key names
# look like secrets so passwords never reach the event log in cleartext.
function Get-ADDSInputText {
    if ($null -eq $payload) {
        return '<no input>'
    }

    try {
        $redacted = [ordered]@{}
        foreach ($property in $payload.PSObject.Properties) {
            if ($property.Name -match $script:ADDSSensitiveKeyPattern) {
                $redacted[$property.Name] = '***redacted***'
            }
            else {
                $redacted[$property.Name] = $property.Value
            }
        }

        return ([pscustomobject]$redacted | ConvertTo-Json -Depth 10 -Compress)
    }
    catch {
        return '<input could not be serialized>'
    }
}

# Logs a completed change together with the script input. Called automatically after a
# mutating operation body succeeds.
function Write-ADDSChange([string]$Source, [string]$Message) {
    $entry = "$Message`nInput: $(Get-ADDSInputText)"
    Write-ADDSEvent -Source $Source -EntryType ([System.Diagnostics.EventLogEntryType]::Information) -Message $entry -EventId 1000
}

# Logs a completed read/query together with the script input. Called automatically after a
# non-mutating operation body succeeds. Uses a distinct event ID (1002) so read traffic can
# be filtered apart from changes (1000) and failures (1001).
function Write-ADDSRead([string]$Source, [string]$Message) {
    $entry = "$Message`nInput: $(Get-ADDSInputText)"
    Write-ADDSEvent -Source $Source -EntryType ([System.Diagnostics.EventLogEntryType]::Information) -Message $entry -EventId 1002
}

# Logs a failed operation with the script input and the error message. Called automatically
# when an operation body throws.
function Write-ADDSFailure([string]$Source, [System.Management.Automation.ErrorRecord]$ErrorRecord) {
    $errorText = '<unknown error>'
    if ($null -ne $ErrorRecord) {
        $errorText = [string]$ErrorRecord.Exception.Message
        if (-not [string]::IsNullOrWhiteSpace([string]$ErrorRecord.ScriptStackTrace)) {
            $errorText += "`n" + [string]$ErrorRecord.ScriptStackTrace
        }
    }

    $entry = "Operation '$Source' failed.`nInput: $(Get-ADDSInputText)`nError: $errorText"
    Write-ADDSEvent -Source $Source -EntryType ([System.Diagnostics.EventLogEntryType]::Error) -Message $entry -EventId 1001
}
