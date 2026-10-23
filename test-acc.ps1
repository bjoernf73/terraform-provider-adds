<#
.SYNOPSIS
    Loads local env vars from .env.ps1 and runs the adds acceptance tests.

.DESCRIPTION
    Convenience wrapper so you don't have to dot-source .env.ps1 by hand every
    session. It sources .env.ps1 (gitignored) into the current process, then
    runs the Go acceptance tests. Any extra arguments are passed straight through
    to `go test`.

.EXAMPLE
    .\test-acc.ps1
    Runs every acceptance test in internal/provider.

.EXAMPLE
    .\test-acc.ps1 -run TestAccUser -v
    Runs a single test verbosely.
#>
[CmdletBinding()]
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$GoTestArgs
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$envScript = Join-Path $root '.env.ps1'

if (-not (Test-Path $envScript)) {
    throw ".env.ps1 not found at $envScript. Copy the template and fill in your test DC host."
}

. $envScript

if (-not $env:ADDS_HOST) {
    throw 'ADDS_HOST is empty. Set $Host_ in .env.ps1 to a reachable test domain controller.'
}

$args = @('test', './internal/provider/...')
if ($GoTestArgs) { $args += $GoTestArgs } else { $args += '-v' }

Write-Host "go $($args -join ' ')" -ForegroundColor Cyan
& go @args
exit $LASTEXITCODE
