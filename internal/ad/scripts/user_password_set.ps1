$user = Resolve-ADPrincipal ([string]$payload.user)
$serverParams = Get-ServerParams

$securePassword = ConvertTo-SecureString -String ([string]$payload.password) -AsPlainText -Force
Set-ADAccountPassword -Identity $user.distinguishedName -Reset -NewPassword $securePassword @serverParams -ErrorAction Stop

# Fire-and-forget: forces pwdLastSet to 0 so the user must change the password at next logon.
# AD clears this once they do, so it is set here and never read back or reconciled.
if ([bool]$payload.change_password_at_next_logon) {
    Set-ADUser -Identity $user.distinguishedName -ChangePasswordAtLogon $true @serverParams -ErrorAction Stop
}

[pscustomobject]@{
    exists        = $true
    user_dn       = $user.distinguishedName
    user_guid     = [string]$user.objectGUID
    pwd_last_set  = Get-PasswordLastSet $user.distinguishedName
} | ConvertTo-Json -Compress
