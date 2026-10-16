# Group helpers. Requires common.ps1.

# 'Manager can update membership list' is not an attribute: it is an Allow ACE granting
# the managedBy principal WriteProperty on the member attribute. The GUID is fixed by the
# base schema in every forest.
$script:MemberAttributeGuid = [guid]'bf9679c0-0de6-11d0-a285-00aa003049e2'

function Test-ManagerCanUpdateMembership([string]$GroupDN, [string]$ManagerDN) {
    if ([string]::IsNullOrWhiteSpace($ManagerDN)) {
        return $false
    }

    $managerSid = (Resolve-ADPrincipal $ManagerDN).objectSid.Value
    $acl = Get-Acl -Path (Get-ADObjectAclPath $GroupDN) -ErrorAction Stop

    foreach ($ace in @($acl.Access)) {
        if (Test-MemberWriteAce $ace $managerSid) {
            return $true
        }
    }

    return $false
}

function Test-MemberWriteAce($Ace, [string]$Sid) {
    if ($Ace.IsInherited -or $Ace.ObjectType -ne $script:MemberAttributeGuid) {
        return $false
    }

    if ([string]$Ace.AccessControlType -ne 'Allow') {
        return $false
    }

    if (-not ($Ace.ActiveDirectoryRights.ToString() -match 'WriteProperty')) {
        return $false
    }

    $aceSid = $Ace.IdentityReference
    if ($aceSid -isnot [System.Security.Principal.SecurityIdentifier]) {
        $aceSid = $aceSid.Translate([System.Security.Principal.SecurityIdentifier])
    }

    return ($aceSid.Value -eq $Sid)
}

function Set-ManagerCanUpdateMembership([string]$GroupDN, [string]$ManagerDN, [bool]$Enabled, [string]$PreviousManagerDN) {
    $aclPath = Get-ADObjectAclPath $GroupDN
    $acl = Get-Acl -Path $aclPath -ErrorAction Stop
    $changed = $false

    # Revoke from a manager that is being replaced or cleared.
    $staleDN = $PreviousManagerDN
    if (-not [string]::IsNullOrWhiteSpace($staleDN) -and $staleDN -ne $ManagerDN) {
        $staleSid = (Resolve-ADPrincipal $staleDN).objectSid.Value
        foreach ($ace in @($acl.Access)) {
            if (Test-MemberWriteAce $ace $staleSid) {
                $acl.RemoveAccessRuleSpecific($ace)
                $changed = $true
            }
        }
    }

    if (-not [string]::IsNullOrWhiteSpace($ManagerDN)) {
        $managerSid = (Resolve-ADPrincipal $ManagerDN).objectSid.Value
        $present = $false
        foreach ($ace in @($acl.Access)) {
            if (Test-MemberWriteAce $ace $managerSid) {
                if ($Enabled) {
                    $present = $true
                }
                else {
                    $acl.RemoveAccessRuleSpecific($ace)
                    $changed = $true
                }
            }
        }

        if ($Enabled -and -not $present) {
            $identity = [System.Security.Principal.IdentityReference]([System.Security.Principal.SecurityIdentifier]$managerSid)
            $rule = New-Object System.DirectoryServices.ActiveDirectoryAccessRule(
                $identity,
                [System.DirectoryServices.ActiveDirectoryRights]::WriteProperty,
                [System.Security.AccessControl.AccessControlType]::Allow,
                $script:MemberAttributeGuid
            )
            $acl.AddAccessRule($rule)
            $changed = $true
        }
    }

    if ($changed) {
        Set-Acl -Path $aclPath -AclObject $acl -ErrorAction Stop
    }
}

# Optional single-valued string attributes, mapped from payload key to LDAP attribute.
$script:GroupAttributeMap = @(
    @{ Key = 'description'; Attribute = 'description' }
    @{ Key = 'display_name'; Attribute = 'displayName' }
    @{ Key = 'mail'; Attribute = 'mail' }
    @{ Key = 'info'; Attribute = 'info' }
    @{ Key = 'homepage'; Attribute = 'wWWHomePage' }
)

function Get-GroupResult($Group, [string]$DomainDN) {
    $containerDN = Get-ParentDN $Group.DistinguishedName

    # Lets the provider keep the configured spelling of path when it resolves to the
    # same container; a DN and a slash path can denote the same place.
    $pathMatch = $false
    if ($null -ne $payload.path) {
        $pathMatch = ((Convert-PathToDN ([string]$payload.path) $DomainDN) -eq $containerDN)
    }

    # Same idea for managed_by, which may be configured as a name but stored as a DN.
    $managedByMatch = $false
    if (-not [string]::IsNullOrWhiteSpace([string]$payload.managed_by)) {
        try {
            $managedByMatch = ((Resolve-ADPrincipal ([string]$payload.managed_by)).distinguishedName -eq [string]$Group.ManagedBy)
        }
        catch {
            $managedByMatch = $false
        }
    }

    return [pscustomobject]@{
        exists                             = $true
        name                               = $Group.Name
        sam_account_name                   = $Group.SamAccountName
        description                        = $Group.Description
        display_name                       = $Group.DisplayName
        mail                               = $Group.mail
        info                               = $Group.info
        homepage                           = $Group.wWWHomePage
        managed_by                         = [string]$Group.ManagedBy
        managed_by_match                   = $managedByMatch
        manager_can_update_membership      = (Test-ManagerCanUpdateMembership $Group.DistinguishedName ([string]$Group.ManagedBy))
        protected_from_accidental_deletion = [bool]$Group.ProtectedFromAccidentalDeletion
        category                           = [string]$Group.GroupCategory
        scope                              = [string]$Group.GroupScope
        path                               = Convert-DNToPath $containerDN $DomainDN
        path_match                         = $pathMatch
        container_dn                       = $containerDN
        distinguished_name                 = $Group.DistinguishedName
        guid                               = [string]$Group.ObjectGUID
        sid                                = [string]$Group.SID
    }
}

function Get-GroupByIdentity([string]$Identity) {
    $serverParams = Get-ServerParams
    $properties = @(
        'Description', 'DisplayName', 'DistinguishedName', 'Name', 'SamAccountName',
        'GroupCategory', 'GroupScope', 'ObjectGUID', 'SID', 'ManagedBy', 'mail', 'info',
        'wWWHomePage', 'ProtectedFromAccidentalDeletion'
    )

    return Get-ADGroup -Identity $Identity -Properties $properties @serverParams -ErrorAction Stop
}

# Returns the ordered list of scopes to pass through to reach $DesiredScope from
# $CurrentScope. Active Directory allows Universal<->Global and Universal<->DomainLocal
# directly, but not Global<->DomainLocal, which must hop through Universal.
function Get-GroupScopePath([string]$CurrentScope, [string]$DesiredScope) {
    if (($CurrentScope -eq 'Global' -and $DesiredScope -eq 'DomainLocal') -or
        ($CurrentScope -eq 'DomainLocal' -and $DesiredScope -eq 'Global')) {
        return @('Universal', $DesiredScope)
    }

    return @($DesiredScope)
}

# Converts a group's scope, stepping through Universal when AD forbids the direct change.
# A step can still fail on nesting rules (for example converting away from Universal while
# the group holds members of an incompatible scope); that error surfaces unchanged.
function Set-GroupScope([string]$Identity, [string]$CurrentScope, [string]$DesiredScope) {
    if ($CurrentScope -eq $DesiredScope) {
        return
    }

    $serverParams = Get-ServerParams
    foreach ($scope in (Get-GroupScopePath $CurrentScope $DesiredScope)) {
        Set-ADGroup -Identity $Identity -GroupScope $scope @serverParams -ErrorAction Stop
    }
}

# Reconciles everything that can change in place. Shared by ensure and update so the
# two paths cannot drift apart.
function Sync-GroupProperties($Group) {
    $serverParams = Get-ServerParams
    $identity = $Group.DistinguishedName
    $previousManagerDN = [string]$Group.ManagedBy

    $setParams = @{}
    # -cne (case-sensitive): a case-only sAMAccountName change must still apply.
    if ([string]$Group.SamAccountName -cne [string]$payload.sam_account_name) {
        $setParams['SamAccountName'] = [string]$payload.sam_account_name
    }
    if ([string]$Group.GroupCategory -ne [string]$payload.category) {
        $setParams['GroupCategory'] = [string]$payload.category
    }
    if ($setParams.Count -gt 0) {
        Set-ADGroup -Identity $identity @setParams @serverParams -ErrorAction Stop
    }

    # Scope changes are applied on their own because AD cannot convert Global directly to
    # DomainLocal (or back); Set-GroupScope steps through Universal when required.
    if ([string]$Group.GroupScope -ne [string]$payload.scope) {
        Set-GroupScope $identity ([string]$Group.GroupScope) ([string]$payload.scope)
    }

    $replace = @{}
    $clear = @()

    foreach ($mapping in $script:GroupAttributeMap) {
        $desired = [string]$payload.($mapping.Key)
        $current = [string]$Group.($mapping.Attribute)

        if ([string]::IsNullOrWhiteSpace($desired)) {
            if (-not [string]::IsNullOrWhiteSpace($current)) {
                $clear += $mapping.Attribute
            }
        }
        # -cne (case-sensitive): a case-only edit (e.g. description) must still apply.
        elseif ($current -cne $desired) {
            $replace[$mapping.Attribute] = $desired
        }
    }

    $managedBy = [string]$payload.managed_by
    $managedByDN = ''
    if ([string]::IsNullOrWhiteSpace($managedBy)) {
        if (-not [string]::IsNullOrWhiteSpace([string]$Group.ManagedBy)) {
            $clear += 'managedBy'
        }
    }
    else {
        $managedByDN = (Resolve-ADPrincipal $managedBy).distinguishedName
        if ([string]$Group.ManagedBy -ne $managedByDN) {
            $replace['managedBy'] = $managedByDN
        }
    }

    if ($replace.Count -gt 0) {
        Set-ADGroup -Identity $identity -Replace $replace @serverParams -ErrorAction Stop
    }
    if ($clear.Count -gt 0) {
        Set-ADGroup -Identity $identity -Clear $clear @serverParams -ErrorAction Stop
    }

    # Not a stored attribute: this adds or removes Deny ACEs for Everyone on Delete
    # and DeleteTree.
    $protected = $false
    if ($null -ne $payload.protected_from_accidental_deletion) {
        $protected = [bool]$payload.protected_from_accidental_deletion
    }
    if ([bool]$Group.ProtectedFromAccidentalDeletion -ne $protected) {
        Set-ADObject -Identity $identity -ProtectedFromAccidentalDeletion $protected @serverParams -ErrorAction Stop
    }

    $managerCanUpdate = $false
    if ($null -ne $payload.manager_can_update_membership) {
        $managerCanUpdate = [bool]$payload.manager_can_update_membership
    }
    Set-ManagerCanUpdateMembership $identity $managedByDN $managerCanUpdate $previousManagerDN

    return Get-GroupByIdentity $identity
}

function Get-GroupOrNull([string]$Identity) {
    try {
        return Get-GroupByIdentity $Identity
    }
    catch {
        if (Test-IsIdentityNotFound $_) {
            return $null
        }

        throw
    }
}
