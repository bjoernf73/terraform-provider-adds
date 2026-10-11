$serverParams = Get-ServerParams
$name = [string]$payload.name

try {
    $gpo = Get-GPO -Name $name @serverParams -ErrorAction Stop
}
catch {
    if (Test-IsGPONotFound $_) {
        [pscustomobject]@{ exists = $false; guid = $null; backup_id = $null; files = @{} } | ConvertTo-Json -Compress
        return
    }
    throw
}

$backupRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("adlc-backup-" + [guid]::NewGuid().ToString('N'))
try {
    New-Item -Path $backupRoot -ItemType Directory -ErrorAction Stop | Out-Null
    $backup = Backup-GPO -Guid $gpo.Id -Path $backupRoot @serverParams -ErrorAction Stop
    $files = [ordered]@{}
    foreach ($file in @(Get-ChildItem -LiteralPath $backupRoot -File -Recurse -Force -ErrorAction Stop | Sort-Object FullName)) {
        $relativePath = $file.FullName.Substring($backupRoot.Length + 1).Replace('\', '/')
        $files[$relativePath] = [Convert]::ToBase64String([System.IO.File]::ReadAllBytes($file.FullName))
    }

    [pscustomobject]@{
        exists    = $true
        guid      = $gpo.Id.ToString()
        backup_id = $backup.Id.ToString()
        files     = $files
    } | ConvertTo-Json -Depth 4 -Compress
}
finally {
    Remove-Item -LiteralPath $backupRoot -Recurse -Force -ErrorAction SilentlyContinue
}