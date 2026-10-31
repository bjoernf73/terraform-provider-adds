# Removes the ACEs this resource declared (matched by identity, never the accidental-deletion
# ACE) and, when this resource had broken inheritance, restores it. Inherited ACEs and any
# ACE this resource did not own are left untouched.
$domainDN = Get-DomainDN
$targetDN = Convert-PathToDN ([string]$payload.target) $domainDN

try {
    $aclPath = Get-ADObjectAclPath $targetDN
    $acl = Get-Acl -Path $aclPath -ErrorAction Stop
}
catch {
    if ((Test-IsIdentityNotFound $_) -or ($_.Exception -is [System.Management.Automation.ItemNotFoundException])) {
        [pscustomobject]@{ deleted = $false; exists = $false } | ConvertTo-Json -Compress
        return
    }

    throw
}

$contexts = @()
foreach ($ace in @($payload.aces)) {
    $contexts += New-AclAceContext $ace $targetDN
}

$changed = $false
$explicit = @($acl.Access | Where-Object { -not $_.IsInherited })
foreach ($ace in $explicit) {
    if (Test-IsAccidentalDeletionAce $ace) {
        continue
    }
    foreach ($context in $contexts) {
        if (Test-AccessRuleKey $ace $context) {
            $acl.RemoveAccessRuleSpecific($ace)
            $changed = $true
            break
        }
    }
}

$inheritanceBlocked = $false
if ($null -ne $payload.inheritance_blocked) {
    $inheritanceBlocked = [bool]$payload.inheritance_blocked
}
if ($inheritanceBlocked -and $acl.AreAccessRulesProtected) {
    # This resource broke inheritance; re-enable it so the object returns to inheriting.
    $acl.SetAccessRuleProtection($false, $false)
    $changed = $true
}

if ($changed) {
    Set-Acl -Path $aclPath -AclObject $acl -ErrorAction Stop
}

[pscustomobject]@{ deleted = $changed; exists = $false } | ConvertTo-Json -Compress
