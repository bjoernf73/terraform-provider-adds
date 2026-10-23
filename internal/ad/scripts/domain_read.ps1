$domainDN = Get-DomainDN

$rootDse = Get-ADDSRootDSE @('rootDomainNamingContext', 'configurationNamingContext')
$forestDN = [string]$rootDse.Attributes['rootDomainNamingContext'][0]
$configDN = [string]$rootDse.Attributes['configurationNamingContext'][0]

$domainEntry = Get-ADDSEntry -DistinguishedName $domainDN -Attributes @('objectSid', 'msDS-Behavior-Version', 'wellKnownObjects', 'fSMORoleOwner')

$sid = $null
if ($null -ne $domainEntry -and $domainEntry.Attributes.Contains('objectSid')) {
    $sidBytes = $domainEntry.Attributes['objectSid'].GetValues([byte[]])[0]
    $sid = (New-Object System.Security.Principal.SecurityIdentifier($sidBytes, 0)).Value
}

$behaviorVersion = $null
if ($null -ne $domainEntry -and $domainEntry.Attributes.Contains('msDS-Behavior-Version')) {
    $behaviorVersion = [string]$domainEntry.Attributes['msDS-Behavior-Version'][0]
}
$domainMode = Convert-DomainModeFromBehaviorVersion $behaviorVersion

# wellKnownObjects values are DN-with-binary: "B:32:<guidhex>:<dn>". Index them by GUID so the
# default containers resolve to wherever they actually live, even if they were redirected.
$wellKnown = @{}
if ($null -ne $domainEntry -and $domainEntry.Attributes.Contains('wellKnownObjects')) {
    foreach ($value in $domainEntry.Attributes['wellKnownObjects'].GetValues([string])) {
        $parts = $value -split ':', 4
        if ($parts.Count -eq 4) {
            $wellKnown[$parts[2].ToLowerInvariant()] = $parts[3]
        }
    }
}

$pdcEmulator = Resolve-ADDSFsmoHost (Get-ADDSString $domainEntry 'fSMORoleOwner')

$infraEntry = Get-ADDSEntry -DistinguishedName ('CN=Infrastructure,' + $domainDN) -Attributes @('fSMORoleOwner')
$infrastructureMaster = Resolve-ADDSFsmoHost (Get-ADDSString $infraEntry 'fSMORoleOwner')

# The NetBIOS name lives on the domain's crossRef in the Partitions container.
$netbios = $null
$escapedDomainDN = ConvertTo-LDAPFilterValue $domainDN
$crossRefs = Search-ADDSEntries ('CN=Partitions,' + $configDN) "(&(objectClass=crossRef)(nCName=$escapedDomainDN))" ([System.DirectoryServices.Protocols.SearchScope]::OneLevel) @('nETBIOSName')
if ($crossRefs.Count -gt 0) {
    $netbios = Get-ADDSString $crossRefs[0] 'nETBIOSName'
}

[pscustomobject]@{
    distinguished_name           = $domainDN
    dns_root                     = ConvertFrom-DNToDnsName $domainDN
    netbios_name                 = $netbios
    sid                          = $sid
    domain_mode                  = $domainMode
    forest                       = ConvertFrom-DNToDnsName $forestDN
    users_container              = $wellKnown['a9d1ca15768811d1aded00c04fd8d5cd']
    computers_container          = $wellKnown['aa312825768811d1aded00c04fd8d5cd']
    domain_controllers_container = $wellKnown['a361b2ffffd211d1aa4b00c04fd7d83a']
    pdc_emulator                 = $pdcEmulator
    infrastructure_master        = $infrastructureMaster
} | ConvertTo-Json -Compress
