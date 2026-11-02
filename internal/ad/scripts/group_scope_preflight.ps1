$serverParams = Get-ServerParams
$current = [string]$payload.current_scope
$desired = [string]$payload.desired_scope

# Decodes the scope out of the groupType bit flag (0x2 Global, 0x4 DomainLocal, 0x8 Universal),
# avoiding a second lookup per related group.
function Get-ScopeFromGroupType($GroupType) {
    $bits = [int]$GroupType
    if ($bits -band 0x8) { return 'Universal' }
    if ($bits -band 0x4) { return 'DomainLocal' }
    if ($bits -band 0x2) { return 'Global' }
    return $null
}

function New-ScopeInfo([string]$Name, [string]$Scope, [string]$Relation) {
    return [pscustomobject]@{ name = $Name; scope = $Scope; relation = $Relation }
}

# Builds and emits the result in one object literal. $Blocking is cast with [object[]] rather
# than wrapped in @(): applying @() to a List[object] inside the hashtable literal throws
# "Argument types do not match" on PowerShell 7.6.
function Write-PreflightResult([bool]$CanConvert, $Blocking) {
    [pscustomobject]@{
        can_convert = $CanConvert
        from_scope  = $current
        to_scope    = $desired
        blocking    = [object[]]$Blocking
    } | ConvertTo-Json -Compress -Depth 5
}

if ($current -eq $desired) {
    Write-PreflightResult $true @()
    return
}

$group = $null
try {
    $group = Get-ADGroup -Identity ([string]$payload.guid) -Properties memberOf, member, GroupScope @serverParams -ErrorAction Stop
}
catch {
    if (Test-IsIdentityNotFound $_) {
        # Nothing to evaluate; apply will handle a missing object.
        Write-PreflightResult $true @()
        return
    }

    throw
}

# Scopes of the groups this group is a MEMBER OF, and of the group members it CONTAINS.
$parentScopes = [System.Collections.Generic.List[object]]::new()
foreach ($dn in @($group.memberOf)) {
    $parent = Get-ADObject -Identity $dn -Properties name, groupType @serverParams -ErrorAction Stop
    $parentScopes.Add((New-ScopeInfo $parent.Name (Get-ScopeFromGroupType $parent.groupType) 'memberOf'))
}

$memberScopes = [System.Collections.Generic.List[object]]::new()
foreach ($dn in @($group.member)) {
    $member = Get-ADObject -Identity $dn -Properties name, objectClass, groupType @serverParams -ErrorAction Stop
    if (@($member.objectClass) -contains 'group') {
        $memberScopes.Add((New-ScopeInfo $member.Name (Get-ScopeFromGroupType $member.groupType) 'member'))
    }
}

# Evaluate each conversion step against Active Directory's documented scope nesting rules.
# Universal -> DomainLocal has no member/membership constraint and is omitted.
$blocking = [System.Collections.Generic.List[object]]::new()
$from = $current
foreach ($to in (Get-GroupScopePath $current $desired)) {
    if ($from -eq 'Global' -and $to -eq 'Universal') {
        # A Universal group cannot be nested inside a Global group.
        foreach ($p in $parentScopes) {
            if ($p.scope -eq 'Global') { $blocking.Add($p) }
        }
    }
    elseif ($from -eq 'Universal' -and $to -eq 'Global') {
        # A Global group cannot contain a Universal member.
        foreach ($m in $memberScopes) {
            if ($m.scope -eq 'Universal') { $blocking.Add($m) }
        }
    }
    elseif ($from -eq 'DomainLocal' -and $to -eq 'Universal') {
        # A Universal group cannot contain a DomainLocal member.
        foreach ($m in $memberScopes) {
            if ($m.scope -eq 'DomainLocal') { $blocking.Add($m) }
        }
    }

    $from = $to
}

Write-PreflightResult ($blocking.Count -eq 0) $blocking
