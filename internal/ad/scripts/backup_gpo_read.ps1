# Reads the GPO's version watermark from AD only. backup_gpo lets Import-GPO own SYSVOL and GPT.ini,
# so the resource tracks just the groupPolicyContainer's versionNumber: the computer version is the
# low word, the user version the high word.
$domainDN = Get-DomainDN
$guid = [string]$payload.guid
$gpoDN = "CN={$guid},CN=Policies,CN=System,$domainDN"

$entry = Get-ADDSEntry -DistinguishedName $gpoDN -Attributes @('displayName', 'versionNumber', 'flags') -Filter '(objectClass=groupPolicyContainer)'

if ($null -eq $entry) {
    [pscustomobject]@{
        exists              = $false
        guid                = $null
        name                = $null
        distinguished_name  = $null
        domain              = $null
        status              = $null
        version_number      = 0
        computer_ad_version = 0
        user_ad_version     = 0
    } | ConvertTo-Json -Compress
    return
}

$versionNumber = [int64](Get-ADDSString $entry 'versionNumber')
$computerAdVersion = $versionNumber -band 0xFFFF
$userAdVersion = ($versionNumber -shr 16) -band 0xFFFF

# The flags attribute maps directly to the GpoStatus enum the GroupPolicy module reports.
switch ([int](Get-ADDSString $entry 'flags')) {
    1 { $status = 'UserSettingsDisabled' }
    2 { $status = 'ComputerSettingsDisabled' }
    3 { $status = 'AllSettingsDisabled' }
    default { $status = 'AllSettingsEnabled' }
}

[pscustomobject]@{
    exists              = $true
    guid                = $guid
    name                = Get-ADDSString $entry 'displayName'
    distinguished_name  = $gpoDN
    domain              = ConvertFrom-DNToDnsName $domainDN
    status              = $status
    version_number      = $versionNumber
    computer_ad_version = $computerAdVersion
    user_ad_version     = $userAdVersion
} | ConvertTo-Json -Compress
