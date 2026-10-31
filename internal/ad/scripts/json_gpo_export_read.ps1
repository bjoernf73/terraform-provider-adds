# Exports a live GPO's SYSVOL content + relevant AD attributes as JSON, in the same
# shape json_gpo_ensure.ps1 consumes (and the shape ref/dry.module.ad's
# Export-GroupPolicyFromAD produces, for round-trip compatibility). The mirror image of
# json_gpo_ensure.ps1: SIDs become ####Replace[DOMAIN\Name] tokens here instead of being
# resolved from them. Links, permissions and WMI filters are out of scope, same as import.
Import-Module GroupPolicy -ErrorAction Stop

$serverParams = Get-ServerParams
$name = [string]$payload.name

$gpo = $null
try {
    $gpo = Get-GPO -Name $name @serverParams -ErrorAction Stop
}
catch {
    if (Test-IsGPONotFound $_) {
        [pscustomobject]@{ exists = $false; guid = $null; json = $null } | ConvertTo-Json -Compress
        return
    }
    throw
}

$policyGuid = "{$($gpo.Id.ToString())}"
$sysvolRoot = Get-JsonGPOSysvolPath -PolicyGuid $policyGuid
$policyServerId = Get-JsonGPOPolicyServerId

$policySettings = [ordered]@{
    RegistrySettings       = @()
    AuditSettings          = @()
    SecurityTemplate       = @()
    GPOComments            = @()
    MachineComments        = @()
    UserComments           = @()
    Scripts                = @()
    GroupPolicyPreferences = @()
}

# GPO comment
$gpoCommentPath = "$sysvolRoot\GPO.cmt"
if (Test-Path -Path $gpoCommentPath) {
    $policySettings.GPOComments = @(Get-Content -Path $gpoCommentPath -Encoding Unicode | ForEach-Object { [string]$_ })
}

# Administrative template comments
$machineCommentPath = "$sysvolRoot\Machine\comment.cmtx"
if (Test-Path -Path $machineCommentPath) {
    $policySettings.MachineComments = @(Get-Content -Path $machineCommentPath -Encoding UTF8 | ForEach-Object { [string]$_ })
}
$userCommentPath = "$sysvolRoot\User\comment.cmtx"
if (Test-Path -Path $userCommentPath) {
    $policySettings.UserComments = @(Get-Content -Path $userCommentPath -Encoding UTF8 | ForEach-Object { [string]$_ })
}

# Registry settings. Exported as-is regardless of type (a live GPO can contain types
# json_gpo_ensure.ps1 won't re-import, e.g. REG_BINARY - it warns and skips those).
foreach ($target in @('Machine', 'User')) {
    $polPath = "$sysvolRoot\$target\Registry.pol"
    if (-not (Test-Path -Path $polPath)) {
        continue
    }

    foreach ($item in @(Read-PolFile -Path $polPath)) {
        $valueType = [string]$item.ValueType
        $valueData = [string]$item.ValueData
        if ($valueType -ne 'REG_DWORD' -and $valueType -ne 'REG_QWORD') {
            $valueData = Resolve-JsonGPOSid $valueData
            $valueData = ConvertTo-JsonGPOPolicyServerToken $valueData $policyServerId
        }

        $policySettings.RegistrySettings += [ordered]@{
            Target    = $target
            KeyName   = $item.KeyName
            ValueType = $valueType
            ValueName = $item.ValueName
            ValueData = $valueData
        }
    }
}

# Audit settings
$auditPath = "$sysvolRoot\Machine\Microsoft\Windows NT\Audit\audit.csv"
if (Test-Path -Path $auditPath) {
    foreach ($item in @(Import-Csv -Path $auditPath -Delimiter ',' -Encoding UTF8)) {
        $policySettings.AuditSettings += [ordered]@{
            'Machine Name'      = $item.'Machine Name'
            'Policy Target'     = $item.'Policy Target'
            SubCategory         = $item.Subcategory
            'SubCategory GUID'  = $item.'Subcategory GUID'
            'Inclusion Setting' = $item.'Inclusion Setting'
            'Exclusion Setting' = $item.'Exclusion Setting'
            'Setting Value'     = $item.'Setting Value'
        }
    }
}

# Security template
$secPath = "$sysvolRoot\Machine\Microsoft\Windows NT\SecEdit\GptTmpl.inf"
if (Test-Path -Path $secPath) {
    $policySettings.SecurityTemplate = @(Get-Content -Path $secPath | ForEach-Object { Resolve-JsonGPOSid $_ })
}

# Logon/logoff/startup/shutdown scripts
foreach ($target in @('Machine', 'User')) {
    $scriptsPath = "$sysvolRoot\$target\Scripts"
    if (-not (Test-Path -Path $scriptsPath)) {
        continue
    }

    $script = [ordered]@{
        Target       = $target
        ScriptsIni   = [ordered]@{}
        PSScriptsIni = [ordered]@{}
        ScriptFiles  = @()
    }

    if (Test-Path -Path "$scriptsPath\scripts.ini") {
        $script.ScriptsIni = Get-JsonGPOIniFile -Path "$scriptsPath\scripts.ini"
    }
    if (Test-Path -Path "$scriptsPath\psscripts.ini") {
        $script.PSScriptsIni = Get-JsonGPOIniFile -Path "$scriptsPath\psscripts.ini"
    }

    foreach ($folder in @(Get-ChildItem -Path $scriptsPath -Directory -Force -ErrorAction SilentlyContinue)) {
        foreach ($file in @(Get-ChildItem -Path $folder.FullName -File -Force)) {
            $script.ScriptFiles += [ordered]@{
                Type    = $folder.Name
                Name    = $file.Name
                Content = @(Get-Content -Path $file.FullName | ForEach-Object { [string]$_ })
            }
        }
    }

    $policySettings.Scripts += $script
}

# Group Policy Preferences
foreach ($target in @('Machine', 'User')) {
    $preferencesPath = "$sysvolRoot\$target\Preferences"
    if (-not (Test-Path -Path $preferencesPath)) {
        continue
    }

    foreach ($typeFolder in @(Get-ChildItem -Path $preferencesPath -Directory -Force -ErrorAction SilentlyContinue)) {
        foreach ($file in @(Get-ChildItem -Path $typeFolder.FullName -File -Force)) {
            $xml = [xml](Get-Content -Path $file.FullName -Raw)
            $lines = @(Format-JsonGPOXml -Xml $xml -Indent 4 | ForEach-Object { Resolve-JsonGPOSid $_ })

            $policySettings.GroupPolicyPreferences += [ordered]@{
                Target      = $target
                Type        = $typeFolder.Name
                XmlFileName = $file.Name
                XmlContent  = $lines
            }
        }
    }
}

# Client-side extensions
$gpcObject = Get-ADObject -LDAPFilter "(&(objectClass=groupPolicyContainer)(name=$policyGuid))" -Properties flags, gPCMachineExtensionNames, gPCUserExtensionNames @serverParams -ErrorAction Stop
$gpoFlags = [int]$gpcObject.flags

$exported = [ordered]@{
    Name                     = $gpo.DisplayName
    ComputerSettingsEnabled  = ($gpoFlags -band 2) -eq 0
    UserSettingsEnabled      = ($gpoFlags -band 1) -eq 0
    PolicySettings           = $policySettings
    gPCMachineExtensionNames = if ($null -ne $gpcObject.gPCMachineExtensionNames) { [string]$gpcObject.gPCMachineExtensionNames } else { $null }
    gPCUserExtensionNames    = if ($null -ne $gpcObject.gPCUserExtensionNames) { [string]$gpcObject.gPCUserExtensionNames } else { $null }
}

$exportedJson = $exported | ConvertTo-Json -Depth 20

[pscustomobject]@{
    exists = $true
    guid   = $gpo.Id.ToString()
    json   = $exportedJson
} | ConvertTo-Json -Depth 5 -Compress
