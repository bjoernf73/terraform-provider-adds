# Authoritative reconcile of a directory object's DACL: the declared ACEs become the exact
# set of explicit, non-inherited ACEs on the target, except the "Protect from accidental
# deletion" ACE (always kept) and inherited ACEs (owned by parents). Order is irrelevant -
# ACEs are matched by identity, not position, so Active Directory's own ACE ordering never
# produces a diff. Serves both create and update.
$domainDN = Get-DomainDN
$targetDN = Convert-PathToDN ([string]$payload.target) $domainDN

$ignoreAdminCount1 = $false
if ($null -ne $payload.ignore_admin_count_1) {
    $ignoreAdminCount1 = [bool]$payload.ignore_admin_count_1
}
Assert-NotAdminCountProtected $targetDN $ignoreAdminCount1

$inheritanceBlocked = $false
if ($null -ne $payload.inheritance_blocked) {
    $inheritanceBlocked = [bool]$payload.inheritance_blocked
}

$aclPath = Get-ADObjectAclPath $targetDN
$acl = Get-Acl -Path $aclPath -ErrorAction Stop

$contexts = @()
foreach ($ace in @($payload.aces)) {
    $contexts += New-AclAceContext $ace $targetDN
}

# Break or restore inheritance first. preserveInheritance = $false: when breaking, the
# inherited ACEs are dropped rather than copied down as explicit ACEs, so afterwards only
# the declared (and accidental-deletion) ACEs remain.
$acl.SetAccessRuleProtection($inheritanceBlocked, $false)

# Prune every explicit ACE that is neither declared nor the accidental-deletion ACE, and
# drop declared ACEs whose rights drifted so they are re-added cleanly below.
$explicit = @($acl.Access | Where-Object { -not $_.IsInherited })
foreach ($ace in $explicit) {
    if (Test-IsAccidentalDeletionAce $ace) {
        continue
    }

    $match = $null
    foreach ($context in $contexts) {
        if ($context.Satisfied) {
            continue
        }
        if (Test-AccessRuleKey $ace $context) {
            $match = $context
            break
        }
    }

    if ($null -eq $match) {
        $acl.RemoveAccessRuleSpecific($ace)
        continue
    }

    if ([int]$ace.ActiveDirectoryRights -ne (Get-AclAceMask $match.Rights)) {
        $acl.RemoveAccessRuleSpecific($ace)
    }
    else {
        $match.Satisfied = $true
    }
}

foreach ($context in $contexts) {
    if ($context.Satisfied) {
        continue
    }
    $acl.AddAccessRule((New-ADAccessRule $context))
}

Set-Acl -Path $aclPath -AclObject $acl -ErrorAction Stop

# Echo the declared ACEs (in declared order) so state carries the configured spelling; the
# rights were written exactly as given.
$resultAces = New-Object System.Collections.Generic.List[object]
foreach ($context in $contexts) {
    $resultAces.Add((New-AclResultAce $context $context.Rights))
}

[pscustomobject]@{
    exists              = $true
    target_dn           = $targetDN
    inheritance_blocked = $inheritanceBlocked
    aces                = @($resultAces)
} | ConvertTo-Json -Depth 6 -Compress
