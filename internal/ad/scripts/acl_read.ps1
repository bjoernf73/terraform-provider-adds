# Reads the authoritative view of a target's DACL. The declared ACEs (passed in as the prior
# state) are matched against the live descriptor by identity, so Active Directory's ACE
# ordering is ignored. Matched ACEs are echoed with the configured spelling - with rights
# refreshed only when the effective mask drifted - while any explicit ACE that is not declared
# (and is not the accidental-deletion ACE) is reported as drift so the next apply prunes it.
$domainDN = Get-DomainDN
$targetDN = Convert-PathToDN ([string]$payload.target) $domainDN

try {
    $aclPath = Get-ADObjectAclPath $targetDN
    $acl = Get-Acl -Path $aclPath -ErrorAction Stop
}
catch {
    if ((Test-IsIdentityNotFound $_) -or ($_.Exception -is [System.Management.Automation.ItemNotFoundException])) {
        [pscustomobject]@{ exists = $false } | ConvertTo-Json -Compress
        return
    }

    throw
}

$contexts = @()
foreach ($ace in @($payload.aces)) {
    $contexts += New-AclAceContext $ace $targetDN
}

$explicit = @($acl.Access | Where-Object { -not $_.IsInherited })
$consumed = New-Object 'System.Collections.Generic.HashSet[int]'
$resultAces = New-Object System.Collections.Generic.List[object]

# Declared ACEs first, in state order, so the stored list stays stable across reads.
foreach ($context in $contexts) {
    $liveIndex = -1
    for ($i = 0; $i -lt $explicit.Count; $i++) {
        if ($consumed.Contains($i)) {
            continue
        }
        if (Test-AccessRuleKey $explicit[$i] $context) {
            $liveIndex = $i
            break
        }
    }

    if ($liveIndex -lt 0) {
        # Declared but gone from AD: omit it so Terraform re-adds it on the next apply.
        continue
    }

    $consumed.Add($liveIndex) | Out-Null
    $live = $explicit[$liveIndex]

    $rights = $context.Rights
    if ([int]$live.ActiveDirectoryRights -ne (Get-AclAceMask $context.Rights)) {
        $rights = @($live.ActiveDirectoryRights.ToString() -split ',\s*')
    }

    $resultAces.Add((New-AclResultAce $context $rights))
}

# Remaining explicit ACEs are undeclared drift; surface them (minus accidental deletion).
for ($i = 0; $i -lt $explicit.Count; $i++) {
    if ($consumed.Contains($i)) {
        continue
    }
    if (Test-IsAccidentalDeletionAce $explicit[$i]) {
        continue
    }

    $resultAces.Add((New-AclExtraAce $explicit[$i]))
}

[pscustomobject]@{
    exists              = $true
    target_dn           = $targetDN
    inheritance_blocked = [bool]$acl.AreAccessRulesProtected
    aces                = @($resultAces)
} | ConvertTo-Json -Depth 6 -Compress
