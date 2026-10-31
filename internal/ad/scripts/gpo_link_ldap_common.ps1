# LDAP helpers for GPO links. Requires common.ps1. Reads and writes the target's gPLink and
# gPOptions attributes directly, so no GroupPolicy module - and no PowerShell edition sensitivity
# (the module only loads native under Windows PowerShell; under pwsh it returns deserialized
# objects whose GpoId is null) - is involved.
#
# Precedence: in gPLink the highest-precedence link (GPMC "Link Order" 1) is the LAST entry; the
# string runs from lowest to highest precedence. Parsing therefore reverses the entries to produce
# order 1..N (1 = highest), and writing emits the desired links (first = highest) in reverse.
#
# Flags (the integer after the ';') is a bitmask: bit 0 (value 1) = link disabled, bit 1 (value 2)
# = enforced. gPOptions bit 0 (value 1) on the target = block inheritance.

# Extracts the lowercase GPO GUID from a "CN={GUID},CN=Policies,..." distinguished name.
function Get-ADDSGPOGuidFromDN([string]$DN) {
    if ($DN -match 'CN=\{(?<guid>[0-9A-Fa-f-]{36})\}') {
        return $Matches['guid'].ToLower()
    }
    return $null
}

# Reads the ordered links and block-inheritance flag for $TargetDN via LDAP. Returns $null when
# the target object does not exist.
function Get-ADDSGPOLinks([string]$TargetDN, [string]$DomainDN) {
    $entry = Get-ADDSEntry -DistinguishedName $TargetDN -Attributes @('gPLink', 'gPOptions')
    if ($null -eq $entry) {
        return $null
    }

    $gpLink = [string](Get-ADDSString $entry 'gPLink')
    $gpOptions = [int](Get-ADDSString $entry 'gPOptions')

    # gPLink is a concatenation of "[LDAP://<dn>;<flags>]" entries, lowest precedence first.
    $parsed = @()
    foreach ($match in [regex]::Matches($gpLink, '\[LDAP://(?<dn>[^;\]]+);(?<flags>\d+)\]')) {
        $parsed += [pscustomobject]@{
            DN    = $match.Groups['dn'].Value
            Flags = [int]$match.Groups['flags'].Value
        }
    }
    [array]::Reverse($parsed)

    $links = @()
    $order = 1
    foreach ($item in $parsed) {
        $guid = Get-ADDSGPOGuidFromDN $item.DN
        $name = $null
        if ($null -ne $guid) {
            $gpoEntry = Get-ADDSEntry -DistinguishedName "CN={$guid},CN=Policies,CN=System,$DomainDN" -Attributes @('displayName') -Filter '(objectClass=groupPolicyContainer)'
            $name = Get-ADDSString $gpoEntry 'displayName'
        }
        $links += [pscustomobject]@{
            gpo_guid = $guid
            gpo_name = $name
            enabled  = (($item.Flags -band 1) -eq 0)
            enforced = (($item.Flags -band 2) -ne 0)
            order    = $order
        }
        $order++
    }

    return [pscustomobject]@{
        exists            = $true
        target_dn         = $TargetDN
        block_inheritance = (($gpOptions -band 1) -eq 1)
        links             = @($links)
    }
}

# Resolves a GPO GUID or display name to its canonical distinguished name and lowercase GUID.
function Resolve-ADDSGPOLink([string]$Identity, [string]$DomainDN) {
    $policiesDN = "CN=Policies,CN=System,$DomainDN"
    $trimmed = ([string]$Identity).Trim()
    $guidValue = [guid]::Empty
    if ([guid]::TryParse($trimmed.Trim('{', '}'), [ref]$guidValue)) {
        $guid = $guidValue.ToString()
        $entry = Get-ADDSEntry -DistinguishedName "CN={$guid},CN=Policies,CN=System,$DomainDN" -Attributes @('cn') -Filter '(objectClass=groupPolicyContainer)'
        if ($null -eq $entry) {
            throw "GPO with GUID '$guid' was not found"
        }
        return [pscustomobject]@{ Guid = $guid; DN = [string]$entry.DistinguishedName }
    }

    $filter = "(&(objectClass=groupPolicyContainer)(displayName=$(ConvertTo-LDAPFilterValue $trimmed)))"
    $candidates = @(Search-ADDSEntries -BaseDN $policiesDN -Filter $filter -Scope ([System.DirectoryServices.Protocols.SearchScope]::OneLevel) -Attributes @('cn'))
    if ($candidates.Count -eq 0) {
        throw "GPO named '$trimmed' was not found"
    }
    if ($candidates.Count -gt 1) {
        throw "multiple GPOs named '$trimmed' were found; link by GUID instead"
    }
    $dn = [string]$candidates[0].DistinguishedName
    return [pscustomobject]@{ Guid = (Get-ADDSGPOGuidFromDN $dn); DN = $dn }
}

# Builds a gPLink attribute string from desired links in precedence order (first = highest).
# The highest-precedence link must be last, so the list is emitted in reverse.
function ConvertTo-ADDSGPLinkString($Entries) {
    $ordered = @($Entries)
    [array]::Reverse($ordered)
    $builder = New-Object System.Text.StringBuilder
    foreach ($entry in $ordered) {
        [void]$builder.Append("[LDAP://$($entry.DN);$($entry.Flags)]")
    }
    return $builder.ToString()
}

# Returns the target's GPO links as raw ordered entries (first = highest precedence), each a
# pscustomobject with DN and Flags, for non-authoritative read-modify-write of a single link.
# Returns $null when the target object does not exist, and an empty array when it has no links.
function Get-ADDSRawGPLinks([string]$TargetDN) {
    $entry = Get-ADDSEntry -DistinguishedName $TargetDN -Attributes @('gPLink')
    if ($null -eq $entry) {
        return $null
    }

    $gpLink = [string](Get-ADDSString $entry 'gPLink')
    $parsed = @()
    foreach ($match in [regex]::Matches($gpLink, '\[LDAP://(?<dn>[^;\]]+);(?<flags>\d+)\]')) {
        $parsed += [pscustomobject]@{
            DN    = $match.Groups['dn'].Value
            Flags = [int]$match.Groups['flags'].Value
        }
    }
    [array]::Reverse($parsed)

    return @($parsed)
}
