$domainDN = Get-DomainDN
$serverParams = Get-ServerParams
$currentDN = [string]$payload.distinguished_name
$ou = Get-LeafOrganizationalUnit $currentDN

$segments = @([string]$payload.path -split '/' | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne '' })
if ($segments.Count -eq 0) {
    throw 'path must contain at least one OU segment'
}

$leafName = $segments[-1]
$parentSegments = @()
if ($segments.Count -gt 1) {
    $parentSegments = @($segments[0..($segments.Count - 2)])
}

# Distinguished names of ancestor OUs this update creates for the new path. The leaf is
# excluded because it is tracked separately via distinguished_name; pre-existing OUs are
# never added.
$createdOUs = New-Object System.Collections.Generic.List[string]

$targetParentDN = $domainDN
foreach ($segment in $parentSegments) {
    $currentAncestorDN = 'OU=' + $segment + ',' + $targetParentDN

    $found = $true
    try {
        Get-ADObject -Identity $currentAncestorDN @serverParams -ErrorAction Stop | Out-Null
    }
    catch {
        if (-not (Test-IsIdentityNotFound $_)) {
            throw
        }

        $found = $false
    }

    if (-not $found) {
        try {
            New-ADOrganizationalUnit -Name $segment -Path $targetParentDN @serverParams -ErrorAction Stop | Out-Null
            $createdOUs.Add($currentAncestorDN)
        }
        catch {
            # A concurrent apply may have created it between the check and now; treat as pre-existing.
            if (-not (Test-IsIdentityAlreadyExists $_)) {
                throw
            }
        }
    }

    $targetParentDN = $currentAncestorDN
}

$needsMove = ((Get-ParentDN $currentDN) -ne $targetParentDN)
# -cne (case-sensitive): a case-only rename must apply; -ne is case-insensitive.
$needsRename = ([string]$ou.Name -cne $leafName)

# Accidental deletion protection denies Delete on the object, which also blocks moves. Record
# the prior state so it can be restored after the move or rename.
$wasProtected = [bool]$ou.ProtectedFromAccidentalDeletion
if (($needsMove -or $needsRename) -and $wasProtected) {
    Set-ADOrganizationalUnit -Identity $currentDN -ProtectedFromAccidentalDeletion:$false @serverParams -ErrorAction Stop
    $ou = Get-LeafOrganizationalUnit $currentDN
}

if ($needsMove) {
    Move-ADObject -Identity $currentDN -TargetPath $targetParentDN @serverParams -ErrorAction Stop
    $currentDN = 'OU=' + $ou.Name + ',' + $targetParentDN
    $ou = Get-LeafOrganizationalUnit $currentDN
}

if ($needsRename) {
    Rename-ADObject -Identity $currentDN -NewName $leafName @serverParams -ErrorAction Stop
    $currentDN = 'OU=' + $leafName + ',' + $targetParentDN
    $ou = Get-LeafOrganizationalUnit $currentDN
}

# Restore protection to the state it had before the move or rename.
if (($needsMove -or $needsRename) -and $wasProtected) {
    Set-ADOrganizationalUnit -Identity $currentDN -ProtectedFromAccidentalDeletion:$true @serverParams -ErrorAction Stop
    $ou = Get-LeafOrganizationalUnit $currentDN
}

if ($null -eq $payload.description -or [string]::IsNullOrWhiteSpace([string]$payload.description)) {
    Set-ADOrganizationalUnit -Identity $currentDN -Clear Description @serverParams -ErrorAction Stop
}
else {
    Set-ADOrganizationalUnit -Identity $currentDN -Description ([string]$payload.description) @serverParams -ErrorAction Stop
}

# Ancestor OUs this resource previously created that the move may have left empty. Remove
# them deepest first while empty; keep any that still hold other managed objects so a
# branch in use is never destroyed.
$remainingCreated = New-Object System.Collections.Generic.List[string]
$previousCreated = @()
if (($payload.PSObject.Properties.Name -contains 'created_organizational_units') -and ($null -ne $payload.created_organizational_units)) {
    $previousCreated = @($payload.created_organizational_units)
}

if ($previousCreated.Count -gt 0) {
    $ordered = @($previousCreated)
    [array]::Reverse($ordered)
    foreach ($dn in $ordered) {
        # A parent shared with the new path must survive; never delete a current ancestor.
        if ($currentDN.EndsWith(',' + $dn)) {
            $remainingCreated.Add($dn)
            continue
        }

        $exists = $true
        try {
            Get-ADObject -Identity $dn @serverParams -ErrorAction Stop | Out-Null
        }
        catch {
            if (Test-IsIdentityNotFound $_) {
                $exists = $false
            }
            else {
                throw
            }
        }

        if (-not $exists) {
            continue
        }

        $children = @(Get-ADObject -SearchBase $dn -SearchScope OneLevel -LDAPFilter '(objectClass=*)' @serverParams -ErrorAction Stop)
        if ($children.Count -gt 0) {
            $remainingCreated.Add($dn)
            continue
        }

        Set-ADOrganizationalUnit -Identity $dn -ProtectedFromAccidentalDeletion:$false @serverParams -ErrorAction Stop
        Remove-ADOrganizationalUnit -Identity $dn -Confirm:$false @serverParams -ErrorAction Stop
    }
}

# Preserve a stable order: kept previous ancestors first (as tracked), then newly created.
$finalCreated = New-Object System.Collections.Generic.List[string]
foreach ($dn in $previousCreated) {
    if ($remainingCreated.Contains($dn) -and -not $finalCreated.Contains($dn)) {
        $finalCreated.Add($dn)
    }
}
foreach ($dn in $createdOUs) {
    if (-not $finalCreated.Contains($dn)) {
        $finalCreated.Add($dn)
    }
}

$ou = Get-LeafOrganizationalUnit $currentDN
[pscustomobject]@{
    exists = $true
    path = Convert-DNToPath $ou.DistinguishedName $domainDN
    description = $ou.Description
    distinguished_name = $ou.DistinguishedName
    name = $ou.Name
    created_organizational_units = @($finalCreated)
} | ConvertTo-Json -Compress
