$user = Resolve-ADPrincipal ([string]$payload.user)
$serverParams = Get-ServerParams

$securePassword = ConvertTo-SecureString -String ([string]$payload.password) -AsPlainText -Force
Set-ADAccountPassword -Identity $user.distinguishedName -Reset -NewPassword $securePassword @serverParams -ErrorAction Stop

[pscustomobject]@{
    exists        = $true
    user_dn       = $user.distinguishedName
    user_guid     = [string]$user.objectGUID
    pwd_last_set  = Get-PasswordLastSet $user.distinguishedName
} | ConvertTo-Json -Compress
