# Authoritative ACL helpers. Requires common.ps1 and access_rule_common.ps1, whose
# Resolve-TrusteeSid, Resolve-ADGuid, Test-AccessRuleKey, New-ADAccessRule and
# Get-NormalizedGuid this file reuses so a declared ACE resolves and compares exactly the
# same way a single adds_access_rule does.

# Builds an access-rule context (same shape Get-AccessRuleContext produces) from one ACE
# object taken from $payload.aces, resolving the trustee and object-type names to a SID and
# GUIDs. Source keeps the raw input so the configured spelling can be echoed back into state.
function New-AclAceContext($Ace, [string]$TargetDN) {
    $inheritance = $null
    if (-not [string]::IsNullOrWhiteSpace([string]$Ace.inheritance)) {
        $inheritance = [System.DirectoryServices.ActiveDirectorySecurityInheritance][string]$Ace.inheritance
    }

    $access = [string]$Ace.access
    if ([string]::IsNullOrWhiteSpace($access)) {
        $access = 'Allow'
    }

    $context = [pscustomobject]@{
        TargetDN                = $TargetDN
        Sid                     = Resolve-TrusteeSid ([string]$Ace.trustee)
        Access                  = $access
        ObjectTypeGuid          = Resolve-ADGuid ([string]$Ace.object_type)
        InheritedObjectTypeGuid = Resolve-ADGuid ([string]$Ace.inherited_object_type)
        Inheritance             = $inheritance
        Rights                  = @($Ace.rights | ForEach-Object { [string]$_ })
        Source                  = $Ace
        Satisfied               = $false
    }

    if ($null -ne $context.InheritedObjectTypeGuid -and $null -eq $context.Inheritance) {
        throw 'inherited_object_type requires inheritance to be set'
    }

    return $context
}

function Get-AclAceMask($Rights) {
    $joined = @($Rights | ForEach-Object { [string]$_ }) -join ', '
    return [int][System.DirectoryServices.ActiveDirectoryRights]$joined
}

# True for the explicit Deny / Everyone (S-1-1-0) / Delete+DeleteTree ACE that "Protect
# object from accidental deletion" adds. Authoritative reconcile must always leave it in
# place and must never report it as drift, so it is excluded from both pruning and state.
function Test-IsAccidentalDeletionAce($Ace) {
    if ($Ace.AccessControlType -ne [System.Security.AccessControl.AccessControlType]::Deny) {
        return $false
    }

    $sid = $Ace.IdentityReference
    if ($sid -isnot [System.Security.Principal.SecurityIdentifier]) {
        $sid = $sid.Translate([System.Security.Principal.SecurityIdentifier])
    }
    if ($sid.Value -ne 'S-1-1-0') {
        return $false
    }

    $delete = [System.DirectoryServices.ActiveDirectoryRights]::Delete
    $deleteTree = [System.DirectoryServices.ActiveDirectoryRights]::DeleteTree
    $rights = $Ace.ActiveDirectoryRights
    return (($rights -band $delete) -eq $delete) -and (($rights -band $deleteTree) -eq $deleteTree)
}

# Reverses Resolve-TrusteeSid for an ACE discovered on the object but not declared in the
# configuration (drift). Best effort: falls back to the raw SID when it cannot be translated.
function ConvertFrom-AclSid([string]$Sid) {
    if ([string]::IsNullOrWhiteSpace($Sid)) {
        return $Sid
    }

    try {
        return ([System.Security.Principal.SecurityIdentifier]$Sid).Translate([System.Security.Principal.NTAccount]).Value
    }
    catch {
        return $Sid
    }
}

# Reverses Resolve-ADGuid for a drifted ACE: looks the GUID up as a schema class/attribute,
# then as an extended right. Guid.Empty means "not object specific" and maps to an empty
# string (an omitted object_type). Falls back to the GUID string when nothing matches.
function ConvertFrom-AclGuid([guid]$Guid) {
    if ($Guid -eq [guid]::Empty) {
        return ''
    }

    $serverParams = Get-ServerParams
    $rootDSE = Get-ADRootDSE @serverParams -ErrorAction Stop

    $octet = (($Guid.ToByteArray()) | ForEach-Object { '\{0:x2}' -f $_ }) -join ''
    $schema = @(Get-ADObject -SearchBase $rootDSE.SchemaNamingContext -LDAPFilter "(schemaIDGUID=$octet)" -Properties lDAPDisplayName @serverParams -ErrorAction SilentlyContinue)
    if ($schema.Count -gt 0) {
        return [string]$schema[0].lDAPDisplayName
    }

    $right = @(Get-ADObject -SearchBase $rootDSE.ConfigurationNamingContext -LDAPFilter "(&(objectClass=controlAccessRight)(rightsGuid=$($Guid.ToString())))" -Properties displayName @serverParams -ErrorAction SilentlyContinue)
    if ($right.Count -gt 0) {
        return [string]$right[0].displayName
    }

    return $Guid.ToString()
}

# State record for a declared ACE, echoing the configured field spelling so a stable ACE
# never diffs. Rights come from the caller: the configured list when the effective mask is
# unchanged, or the live rights when it drifted.
function New-AclResultAce($Context, $Rights) {
    return [ordered]@{
        trustee               = [string]$Context.Source.trustee
        access                = $Context.Access
        rights                = @($Rights | ForEach-Object { [string]$_ })
        object_type           = Get-OptionalString $Context.Source.object_type
        inherited_object_type = Get-OptionalString $Context.Source.inherited_object_type
        inheritance           = Get-OptionalString $Context.Source.inheritance
    }
}

# State record for an undeclared ACE found on the object (drift). Reverse-resolves the SID
# and GUIDs to names so it is shown in the same vocabulary as the configuration; these
# entries only persist until the next apply prunes them.
function New-AclExtraAce($Ace) {
    $sid = $Ace.IdentityReference
    if ($sid -isnot [System.Security.Principal.SecurityIdentifier]) {
        $sid = $sid.Translate([System.Security.Principal.SecurityIdentifier])
    }

    $objectType = ConvertFrom-AclGuid (Get-NormalizedGuid $Ace.ObjectType)
    $inheritedObjectType = ConvertFrom-AclGuid (Get-NormalizedGuid $Ace.InheritedObjectType)
    $inheritance = [string]$Ace.InheritanceType

    return [ordered]@{
        trustee               = ConvertFrom-AclSid $sid.Value
        access                = [string]$Ace.AccessControlType
        rights                = @($Ace.ActiveDirectoryRights.ToString() -split ',\s*')
        object_type           = Get-OptionalString $objectType
        inherited_object_type = Get-OptionalString $inheritedObjectType
        inheritance           = if ($inheritance -eq 'None') { $null } else { $inheritance }
    }
}
