$domainDN = Get-DomainDN
$serverParams = Get-ServerParams
$containerDN = Convert-PathToDN ([string]$payload.path) $domainDN
$targetDN = 'CN=' + [string]$payload.name + ',' + $containerDN

$user = Get-UserOrNull $targetDN
$setParams = Get-UserSetParams

if ($null -eq $user) {
    # sAMAccountName and userPrincipalName are unique domain-wide; report conflicts clearly.
    $samAccountName = [string]$payload.sam_account_name
    $conflict = @(Get-ADUser -Filter 'SamAccountName -eq $samAccountName' @serverParams -ErrorAction Stop)
    if ($conflict.Count -gt 0) {
        throw "a user with sAMAccountName '$samAccountName' already exists at '$($conflict[0].DistinguishedName)'"
    }

    $newParams = @{
        Name              = [string]$payload.name
        SamAccountName    = $samAccountName
        UserPrincipalName = [string]$payload.user_principal_name
        Path              = $containerDN
    }

    # New-ADUser creates the account disabled (no AccountPassword is set here). Enabled is
    # reconciled separately below, deferring to a password resource when no password exists.
    New-ADUser @newParams @setParams @serverParams -ErrorAction Stop | Out-Null
    $user = Get-UserByIdentity $targetDN
}
else {
    # Adopt an existing user at the same DN and reconcile it to the configuration.
    Set-ADUser -Identity $user.DistinguishedName @setParams @serverParams -ErrorAction Stop

    # -cne (case-sensitive): a case-only userPrincipalName change must still apply.
    if ([string]$user.UserPrincipalName -cne [string]$payload.user_principal_name) {
        Set-ADUser -Identity $user.DistinguishedName -UserPrincipalName ([string]$payload.user_principal_name) @serverParams -ErrorAction Stop
    }

    $user = Get-UserByIdentity $user.DistinguishedName
}

Set-UserEnabledState $user.DistinguishedName ([bool]$payload.enabled)
$user = Get-UserByIdentity $user.DistinguishedName

Sync-UserProtection $user.DistinguishedName ([bool]$user.ProtectedFromAccidentalDeletion)
$user = Get-UserByIdentity $user.DistinguishedName

Get-UserResult $user $domainDN | ConvertTo-Json -Compress
