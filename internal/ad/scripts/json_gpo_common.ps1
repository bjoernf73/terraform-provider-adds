# JSON GPO helpers shared by export/import. Requires common.ps1 and
# gpregistrypolicyparser.ps1. Token grammar matches ref/dry.module.ad's
# dry.ad.gpohelper exactly (####Replace[DOMAIN\Name], ####PolicyServerId####,
# ####defaultNamingContext####), so JSON exported by either tool imports with the
# other.

# Converts every SID in $InputString into a portable ####Replace[DOMAIN\Name] token.
# Well-known SIDs (12 characters or less, e.g. S-1-5-18) are left alone: they resolve
# the same way in every domain, so replacing them would only add noise.
function Resolve-JsonGPOSid([string]$InputString) {
    if ([string]::IsNullOrEmpty($InputString)) {
        return $InputString
    }

    $serverParams = Get-ServerParams
    $domain = Get-ADDomain @serverParams -ErrorAction Stop

    $matches = [regex]::new('[Ss]-\d-(?:\d+-){1,14}\d+').Matches($InputString)
    foreach ($value in ($matches.Value | Select-Object -Unique)) {
        if ($value.Length -le 12) {
            continue
        }

        try {
            $sid = New-Object System.Security.Principal.SecurityIdentifier($value)
            $actualDomain, $actualName = ($sid.Translate([System.Security.Principal.NTAccount])).Value.Split('\')

            if (($actualDomain -eq $domain.DNSRoot) -or ($actualDomain -eq $domain.NetBIOSName)) {
                $actualDomain = 'DOMAIN'
            }

            $InputString = $InputString.Replace($value, "####Replace[$actualDomain\$actualName]")
        }
        catch {
            # Untranslatable SID (most likely a well-known one this length check missed); leave as-is.
        }
    }

    return $InputString
}

# Reverses Resolve-JsonGPOSid: resolves each ####Replace[DOMAIN\Name] token to a live
# SID in the current domain. 'DOMAIN' resolves to this domain's NetBIOS name, so tokens
# stay portable across target domains. -LowerCase is for fdeploy1.ini, which requires
# lower-cased SIDs.
function Resolve-JsonGPOToken([string]$InputString, [switch]$LowerCase) {
    if ([string]::IsNullOrEmpty($InputString) -or $InputString -notmatch '####Replace') {
        return $InputString
    }

    $serverParams = Get-ServerParams
    $domainNetBIOSName = (Get-ADDomain @serverParams -ErrorAction Stop).NetBIOSName

    $matches = [regex]::new('####Replace\[(.*?)\]').Matches($InputString)
    foreach ($value in ($matches.Value | Select-Object -Unique)) {
        $domain, $name = ($value.Replace('####Replace[', '').Replace(']', '')).Split('\')
        if ($domain -eq 'DOMAIN') {
            $domain = $domainNetBIOSName
        }

        $account = New-Object System.Security.Principal.NTAccount($domain, $name)
        try {
            $sid = $account.Translate([System.Security.Principal.SecurityIdentifier]).Value
        }
        catch {
            throw "unable to resolve '$domain\$name' (from token '$value') to a SID in the target domain; the referenced user, group or computer must exist there"
        }

        if ($LowerCase) {
            $sid = $sid.ToLowerInvariant()
        }

        $InputString = $InputString.Replace($value, $sid)
    }

    return $InputString.Replace('####Replace', '')
}

# The certificate enrollment policy server's Id is a GUID tied to the source domain's AD CS
# deployment, so it must be tokenized like SIDs to keep a certificate-autoenrollment GPO
# portable. Reads the policy server configured on the host these scripts run on (same
# machine-local semantics as ref/dry.ad.gpohelper.psm1). Returns $null when none is
# configured or the PKI cmdlet is unavailable, which callers treat as "nothing to replace".
function Get-JsonGPOPolicyServerId {
    try {
        $server = Get-CertificateEnrollmentPolicyServer -Scope All -Context Machine -ErrorAction Stop | Select-Object -First 1
    }
    catch {
        return $null
    }

    if ($null -eq $server -or [string]::IsNullOrEmpty($server.Id)) {
        return $null
    }

    return [string]$server.Id
}

# Export direction: replaces the live policy server Id with the ####PolicyServerId#### token.
# No-op when no policy server is configured ($PolicyServerId empty), avoiding the empty-pattern
# -ireplace that would otherwise splice the token between every character.
function ConvertTo-JsonGPOPolicyServerToken([string]$InputString, [string]$PolicyServerId) {
    if ([string]::IsNullOrEmpty($InputString) -or [string]::IsNullOrEmpty($PolicyServerId)) {
        return $InputString
    }

    return $InputString -ireplace [regex]::Escape($PolicyServerId), '####PolicyServerId####'
}

# Import direction: resolves the ####PolicyServerId#### token back to the target host's live
# policy server Id. Throws when the token is present but no policy server is configured, rather
# than writing a broken registry value.
function Resolve-JsonGPOPolicyServerToken([string]$InputString, [string]$PolicyServerId) {
    if ([string]::IsNullOrEmpty($InputString) -or $InputString -notmatch '####PolicyServerId####') {
        return $InputString
    }

    if ([string]::IsNullOrEmpty($PolicyServerId)) {
        throw 'the GPO references ####PolicyServerId#### but no certificate enrollment policy server is configured on the target; configure AD CS certificate autoenrollment policy there before importing this GPO'
    }

    return $InputString.Replace('####PolicyServerId####', $PolicyServerId)
}

# Applies the user-supplied free-text replacements to the raw JSON text before it is
# parsed. Each map key is a bare name (e.g. "DomainFQDN"); the #### delimiters are
# implied, the same way Ansible variables imply {{ }}, so this wraps the key before
# matching it case-insensitively against the text.
function Resolve-JsonGPOReplacements([string]$JsonRaw, $Replacements) {
    if ($null -eq $Replacements) {
        return $JsonRaw
    }

    foreach ($property in $Replacements.PSObject.Properties) {
        $token = "####$($property.Name)####"
        $JsonRaw = $JsonRaw -ireplace [regex]::Escape($token), $property.Value
    }

    return $JsonRaw
}

# INI files here (scripts.ini, psscripts.ini) are read into a hashtable of sections,
# each section a hashtable of key/value pairs, plus numbered Comment<n> keys for
# comment lines - matching how they are written back out by Write-JsonGPOIniFile.
function Get-JsonGPOIniFile([string]$Path) {
    $ini = [ordered]@{}
    $section = $null
    $commentCount = 0

    switch -regex -file $Path {
        '^\[(.+)\]' {
            $section = $Matches[1]
            $ini[$section] = [ordered]@{}
            $commentCount = 0
        }
        '^(;.*)$' {
            $commentCount++
            $ini[$section]["Comment$commentCount"] = $Matches[1]
        }
        '(.+?)\s*=(.*)' {
            $name, $value = $Matches[1, 2]
            $ini[$section][$name] = $value.Trim()
        }
    }

    return $ini
}

function Write-JsonGPOIniFile([string]$Path, $Sections, [switch]$NoBom) {
    $lines = [System.Collections.Generic.List[string]]::new()

    foreach ($sectionName in $Sections.Keys) {
        $lines.Add("[$sectionName]")

        foreach ($key in ($Sections[$sectionName].Keys | Sort-Object)) {
            if ($key -match '^Comment\d+$') {
                $lines.Add([string]$Sections[$sectionName][$key])
            }
            else {
                $lines.Add("$key=$($Sections[$sectionName][$key])")
            }
        }

        $lines.Add('')
    }

    # CRLF line endings. GPT.ini must be UTF-8 without a BOM (GPMC writes it that way); -NoBom
    # selects that, other callers keep the BOM.
    [System.IO.File]::WriteAllText($Path, (($lines -join "`r`n") + "`r`n"), [System.Text.UTF8Encoding]::new(-not $NoBom))
}

# Deserialized JSON gives PSCustomObject; ini writing needs plain hashtables.
function ConvertTo-JsonGPOHashtable($Value) {
    if ($null -eq $Value) {
        return [ordered]@{}
    }

    $hash = [ordered]@{}
    foreach ($property in $Value.PSObject.Properties) {
        $hash[$property.Name] = $property.Value
    }

    return $hash
}

function Format-JsonGPOXml([xml]$Xml, [int]$Indent) {
    $stringWriter = New-Object System.IO.StringWriter
    $xmlWriter = New-Object System.Xml.XmlTextWriter $stringWriter
    $xmlWriter.Formatting = 'Indented'
    $xmlWriter.Indentation = $Indent

    $Xml.WriteContentTo($xmlWriter)
    $xmlWriter.Flush()
    $stringWriter.Flush()

    # One array element per line, matching GroupPolicyPreference.XmlContent's shape.
    return $stringWriter.ToString().Split("`r`n", [System.StringSplitOptions]::RemoveEmptyEntries)
}

# The GPO.cmt comment file is limited to 2047 characters, with each line break
# counting as 2 (\r\n).
function Test-JsonGPOCommentInSpec([string[]]$Lines) {
    $count = 0
    foreach ($line in $Lines) {
        $count += $line.Length + 2
    }
    $count -= 2

    return $count -le 2047
}

# Root SYSVOL path for a GPO's own folder. Uses the configured domain_controller when
# set, so writes land on the same DC as the AD object update; falls back to the domain's
# DFS namespace (\\<domain>\SYSVOL\...), which resolves to any available DC.
function Get-JsonGPOSysvolPath([string]$PolicyGuid) {
    $serverParams = Get-ServerParams
    $domainDnsRoot = (Get-ADDomain @serverParams -ErrorAction Stop).DNSRoot
    $sysvolHost = if ($payload.domain_controller) { [string]$payload.domain_controller } else { $domainDnsRoot }

    return "\\$sysvolHost\SYSVOL\$domainDnsRoot\Policies\$PolicyGuid"
}

