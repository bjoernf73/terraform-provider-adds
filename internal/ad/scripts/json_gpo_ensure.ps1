# Imports a JSON GPO (see internal/ad/json_gpo.go) into $payload.target_name, creating
# it if needed. Re-running this against an existing target overwrites its SYSVOL content
# in place (same GUID, links and ACLs kept), so it also serves as the update path.
#
# Known limitation: each section (registry, security template, audit, scripts, GPP) is
# only overwritten when present in the JSON; a section removed from the JSON since the
# last apply leaves its previously written file behind rather than deleting it.
Import-Module GroupPolicy -ErrorAction Stop

$jsonRaw = Resolve-JsonGPOReplacements ([string]$payload.json_raw) $payload.replacements
$imported = $jsonRaw | ConvertFrom-Json -ErrorAction Stop

$serverParams = Get-ServerParams
$targetName = [string]$payload.target_name

$gpo = $null
try {
    $gpo = Get-GPO -Name $targetName @serverParams -ErrorAction Stop
}
catch {
    if (-not (Test-IsGPONotFound $_)) {
        throw
    }
}

if ($null -eq $gpo) {
    $gpo = New-GPO -Name $targetName @serverParams -ErrorAction Stop
}

$policyGuid = "{$($gpo.Id.ToString())}"
$sysvolRoot = Get-JsonGPOSysvolPath -PolicyGuid $policyGuid
$policyServerId = Get-JsonGPOPolicyServerId

# GPO comment
if ($imported.PolicySettings.GPOComments) {
    if (-not (Test-JsonGPOCommentInSpec $imported.PolicySettings.GPOComments)) {
        throw 'GPO comment exceeds the 2047 character limit (a line break counts as 2)'
    }
    $imported.PolicySettings.GPOComments | Out-File -FilePath "$sysvolRoot\GPO.cmt" -Encoding unicode -Force
}

# versionNumber packs two counters: the low 16 bits are the computer version, the high 16
# bits the user version. Track which side had any content written, so the matching half is
# bumped below - a user-only GPO must raise the user version, not leave it at 0.
$machineChanged = $false
$userChanged = $false

# Administrative template comments
if ($imported.PolicySettings.MachineComments) {
    New-Item -Path "$sysvolRoot\Machine" -ItemType Directory -Force -ErrorAction SilentlyContinue | Out-Null
    $imported.PolicySettings.MachineComments | Out-File -FilePath "$sysvolRoot\Machine\comment.cmtx" -Encoding utf8 -Force
    $machineChanged = $true
}
if ($imported.PolicySettings.UserComments) {
    New-Item -Path "$sysvolRoot\User" -ItemType Directory -Force -ErrorAction SilentlyContinue | Out-Null
    $imported.PolicySettings.UserComments | Out-File -FilePath "$sysvolRoot\User\comment.cmtx" -Encoding utf8 -Force
    $userChanged = $true
}

# Registry settings.
$allowedRegistryValueTypes = @('REG_SZ', 'REG_MULTI_SZ', 'REG_DWORD', 'REG_QWORD', 'REG_NONE')
if ($imported.PolicySettings.RegistrySettings) {
    $byTarget = @{ Machine = @(); User = @() }

    foreach ($setting in $imported.PolicySettings.RegistrySettings) {
        $valueType = [string]$setting.ValueType
        if ($allowedRegistryValueTypes -notcontains $valueType) {
            Write-Warning "skipping registry setting with unsupported value type '$valueType' (key '$($setting.KeyName)')"
            continue
        }

        $target = [string]$setting.Target
        if ($target -ne 'Machine' -and $target -ne 'User') {
            throw "registry setting target must be 'Machine' or 'User', got '$target'"
        }

        # A lone null character represents the (Default) value in some exports; treat it
        # the same as an empty value name.
        $valueName = [string]$setting.ValueName
        if ($valueName.Length -eq 1 -and [byte][char]$valueName -eq 0) {
            $valueName = ''
        }

        $valueData = $setting.ValueData
        switch ($valueType) {
            'REG_DWORD' { $valueData = [uint32]$valueData }
            'REG_QWORD' { $valueData = [uint64]$valueData }
            default {
                $valueData = Resolve-JsonGPOToken ([string]$valueData)
                $valueData = Resolve-JsonGPOPolicyServerToken $valueData $policyServerId
            }
        }

        $entry = New-GPRegistryPolicy -keyName ([string]$setting.KeyName) -valueName $valueName -valueType $valueType -valueData $valueData
        $byTarget[$target] += $entry
    }

    foreach ($target in @('Machine', 'User')) {
        if ($byTarget[$target].Count -eq 0) {
            continue
        }

        $dir = "$sysvolRoot\$target"
        New-Item -Path $dir -ItemType Directory -Force -ErrorAction SilentlyContinue | Out-Null
        $polPath = "$dir\Registry.pol"
        New-GPRegistryPolicyFile -Path $polPath
        Add-RegistryPolicies -Path $polPath -RegistryPolicies $byTarget[$target]

        if ($target -eq 'Machine') { $machineChanged = $true } else { $userChanged = $true }
    }
}

# Audit settings
if ($imported.PolicySettings.AuditSettings) {
    $auditDir = "$sysvolRoot\Machine\Microsoft\Windows NT\Audit"
    New-Item -Path $auditDir -ItemType Directory -Force -ErrorAction SilentlyContinue | Out-Null

    $csv = $imported.PolicySettings.AuditSettings |
        Select-Object -Property * -ExcludeProperty ObjectType |
        ConvertTo-Csv -NoTypeInformation -Delimiter ',' |
        ForEach-Object { $_.Replace('"', '') }
    $csv | Out-File -FilePath "$auditDir\audit.csv" -Encoding utf8 -Force
    $machineChanged = $true
}

# Security template (GptTmpl.inf); tokens can appear mid-line, embedded in SDDL strings.
if ($imported.PolicySettings.SecurityTemplate) {
    $secDir = "$sysvolRoot\Machine\Microsoft\Windows NT\SecEdit"
    New-Item -Path $secDir -ItemType Directory -Force -ErrorAction SilentlyContinue | Out-Null

    $lines = @($imported.PolicySettings.SecurityTemplate | ForEach-Object { Resolve-JsonGPOToken ([string]$_) })
    $lines | Out-File -FilePath "$secDir\GptTmpl.inf" -Encoding default -Force
    $machineChanged = $true
}

# Logon/logoff/startup/shutdown scripts
if ($imported.PolicySettings.Scripts) {
    foreach ($script in $imported.PolicySettings.Scripts) {
        $target = [string]$script.Target
        if ($target -eq 'Machine') { $machineChanged = $true } else { $userChanged = $true }
        $scriptsDir = "$sysvolRoot\$target\Scripts"
        New-Item -Path $scriptsDir -ItemType Directory -Force -ErrorAction SilentlyContinue | Out-Null

        Write-JsonGPOIniFile -Path "$scriptsDir\scripts.ini" -Sections (ConvertTo-JsonGPOHashtable $script.ScriptsIni)
        Write-JsonGPOIniFile -Path "$scriptsDir\psscripts.ini" -Sections (ConvertTo-JsonGPOHashtable $script.PSScriptsIni)

        foreach ($file in @($script.ScriptFiles)) {
            if ($null -eq $file) {
                continue
            }

            $typeDir = "$scriptsDir\$($file.Type)"
            New-Item -Path $typeDir -ItemType Directory -Force -ErrorAction SilentlyContinue | Out-Null
            $file.Content | Out-File -FilePath "$typeDir\$($file.Name)" -Encoding default -Force
        }
    }
}

# Group Policy Preferences
if ($imported.PolicySettings.GroupPolicyPreferences) {
    foreach ($gpp in $imported.PolicySettings.GroupPolicyPreferences) {
        if ([string]$gpp.Target -eq 'Machine') { $machineChanged = $true } else { $userChanged = $true }
        $gppDir = "$sysvolRoot\$($gpp.Target)\Preferences\$($gpp.Type)"
        New-Item -Path $gppDir -ItemType Directory -Force -ErrorAction SilentlyContinue | Out-Null

        $lines = @($gpp.XmlContent | ForEach-Object { Resolve-JsonGPOToken ([string]$_) })
        $lines | Out-File -FilePath "$gppDir\$($gpp.XmlFileName)" -Encoding utf8 -Force
    }
}

# Client-side extensions and the computer/user settings enabled flags
$gpcObject = Get-ADObject -LDAPFilter "(&(objectClass=groupPolicyContainer)(name=$policyGuid))" @serverParams -ErrorAction Stop

if ($imported.gPCMachineExtensionNames) {
    Set-ADObject -Identity $gpcObject.DistinguishedName -Replace @{ gPCMachineExtensionNames = [string]$imported.gPCMachineExtensionNames } @serverParams -ErrorAction Stop
}
if ($imported.gPCUserExtensionNames) {
    Set-ADObject -Identity $gpcObject.DistinguishedName -Replace @{ gPCUserExtensionNames = [string]$imported.gPCUserExtensionNames } @serverParams -ErrorAction Stop
}

# flags: 0 = both enabled, 1 = user disabled, 2 = computer disabled, 3 = both disabled
$computerEnabled = [bool]$imported.ComputerSettingsEnabled
$userEnabled = [bool]$imported.UserSettingsEnabled
$flags = 0
if ($computerEnabled -and -not $userEnabled) { $flags = 1 }
if ((-not $computerEnabled) -and $userEnabled) { $flags = 2 }
if ((-not $computerEnabled) -and (-not $userEnabled)) { $flags = 3 }
Set-ADObject -Identity $gpcObject.DistinguishedName -Replace @{ flags = $flags } @serverParams -ErrorAction Stop

# Bump the GPO version so GPMC and gpupdate see the change. versionNumber packs two 16-bit
# counters as Version = userVersion * 65536 + computerVersion: read the current pair from AD,
# raise the side(s) whose settings changed by 1, then repack.
$current = Get-ADObject -Identity $gpcObject.DistinguishedName -Properties versionNumber @serverParams -ErrorAction Stop
[int64]$currentVersion = [int64]$current.versionNumber
[int64]$computerVersion = $currentVersion % 65536
[int64]$userVersion = [math]::Floor($currentVersion / 65536)

if ($machineChanged) { $computerVersion += 1 }
if ($userChanged) { $userVersion += 1 }

$newVersion = ($userVersion * 65536) + $computerVersion
if ($machineChanged -or $userChanged) {
    Set-ADObject -Identity $gpcObject.DistinguishedName -Replace @{ versionNumber = $newVersion } @serverParams -ErrorAction Stop
}

# SYSVOL replication can lag behind the AD object New-GPO just created.
$gptIniPath = "$sysvolRoot\GPT.INI"
$attempts = 0
while (-not (Test-Path -Path $gptIniPath) -and $attempts -lt 30) {
    Start-Sleep -Seconds 1
    $attempts++
}

if (Test-Path -Path $gptIniPath) {
    # A standard GPT.ini carries only [General] Version=N, the same packed number as the AD
    # versionNumber. New-GPO leaves a cosmetic displayName line that GPMC ignores; write the
    # file clean and BOM-less, as GPMC does, so SYSVOL stays in lockstep with AD.
    Write-JsonGPOIniFile -Path $gptIniPath -Sections ([ordered]@{ General = [ordered]@{ Version = $newVersion } }) -NoBom
}

# Report the version we just wrote to both AD and GPT.ini rather than reading SYSVOL back:
# AD == SYSVOL by construction here, and a read-back of GPT.ini can lag at create time. Like
# backup_gpo, the resource tracks only version_number; computer/user are its decoded words.
$computerAdVersion = $newVersion -band 0xFFFF
$userAdVersion = ($newVersion -shr 16) -band 0xFFFF

switch ($flags) {
    1 { $status = 'UserSettingsDisabled' }
    2 { $status = 'ComputerSettingsDisabled' }
    3 { $status = 'AllSettingsDisabled' }
    default { $status = 'AllSettingsEnabled' }
}

[pscustomobject]@{
    exists              = $true
    guid                = $gpo.Id.ToString()
    name                = $targetName
    # Build the DN from $gpo.Id (lowercase GUID) to match the LDAP read; $gpcObject's own
    # DistinguishedName renders the GUID uppercase and would show as permanent drift.
    distinguished_name  = "CN={$($gpo.Id.ToString())},CN=Policies,CN=System,$(Get-DomainDN)"
    domain              = ConvertFrom-DNToDnsName (Get-DomainDN)
    status              = $status
    version_number      = $newVersion
    computer_ad_version = $computerAdVersion
    user_ad_version     = $userAdVersion
} | ConvertTo-Json -Compress
