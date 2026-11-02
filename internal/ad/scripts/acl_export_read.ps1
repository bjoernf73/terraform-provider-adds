# Exports the explicit (non-inherited) DACL of one object, or every object below a path, in the
# shape adds_acl declares. The counterpart to acl_ensure.ps1: SIDs and GUIDs are reverse-resolved
# to names (New-AclExtraAce), the accidental-deletion ACE is excluded (adds_acl manages it
# separately), and inheritance_blocked is captured so the whole configuration can be replicated.
$domainDN = Get-DomainDN
$baseDN = Convert-PathToDN ([string]$payload.path) $domainDN

$includeEmpty = [bool]$payload.include_empty
$scope = if ([bool]$payload.recursive) {
    [System.DirectoryServices.Protocols.SearchScope]::Subtree
}
else {
    [System.DirectoryServices.Protocols.SearchScope]::Base
}

# Lower-cased structural-class allow-list. Empty means every object type.
$typeSet = @($payload.object_types | ForEach-Object { ([string]$_).ToLowerInvariant() })

if ($typeSet.Count -gt 0) {
    $clauses = ($typeSet | ForEach-Object { "(objectClass=$(ConvertTo-LDAPFilterValue $_))" }) -join ''
    $filter = "(|$clauses)"
}
else {
    $filter = '(objectClass=*)'
}

try {
    $entries = Search-ADDSExportEntries $baseDN $filter $scope @('objectClass', 'nTSecurityDescriptor')
}
catch [System.DirectoryServices.Protocols.DirectoryOperationException] {
    if ($_.Exception.Response.ResultCode -eq [System.DirectoryServices.Protocols.ResultCode]::NoSuchObject) {
        throw "path '$baseDN' was not found"
    }
    throw
}

$objects = [System.Collections.Generic.List[object]]::new()
foreach ($entry in $entries) {
    $classes = @($entry.Attributes['objectClass'].GetValues([string]))
    # The most-derived objectClass is the structural class; filter on it so 'user' excludes
    # computers (which also carry objectClass=user).
    $structural = $classes[$classes.Count - 1]
    if ($typeSet.Count -gt 0 -and ($typeSet -notcontains $structural.ToLowerInvariant())) {
        continue
    }

    if (-not $entry.Attributes.Contains('ntsecuritydescriptor')) {
        continue
    }
    $bytes = $entry.Attributes['nTSecurityDescriptor'].GetValues([byte[]])[0]
    $security = New-Object System.DirectoryServices.ActiveDirectorySecurity
    $security.SetSecurityDescriptorBinaryForm($bytes)

    $aces = [System.Collections.Generic.List[object]]::new()
    foreach ($ace in $security.GetAccessRules($true, $false, [System.Security.Principal.SecurityIdentifier])) {
        if (Test-IsAccidentalDeletionAce $ace) {
            continue
        }
        $aces.Add((New-AclExtraAce $ace))
    }

    $blocked = [bool]$security.AreAccessRulesProtected
    # Skip objects with nothing to replicate: an empty, non-protected DACL would render an empty
    # authoritative adds_acl, which prunes everything on apply. include_empty opts back in.
    if ($aces.Count -eq 0 -and -not $blocked -and -not $includeEmpty) {
        continue
    }

    $objects.Add([ordered]@{
        distinguished_name  = [string]$entry.DistinguishedName
        path                = ConvertFrom-DNToAclPath ([string]$entry.DistinguishedName) $domainDN
        object_class        = $structural
        inheritance_blocked = $blocked
        aces                = @($aces)
    })
}

[pscustomobject]@{ objects = @($objects) } | ConvertTo-Json -Depth 8 -Compress
