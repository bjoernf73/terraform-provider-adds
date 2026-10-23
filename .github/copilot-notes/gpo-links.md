# GPO & gpo_links — working notes

Context for Group Policy work in this provider, kept in-repo so it travels across machines.
(There is a parallel copy in this workspace's local Copilot memory on the author's Mac; this
committed file is the portable one.)

## Resources involved
- `adds_backup_gpo` — imports a GPMC `Backup-GPO` folder via `Import-GPO`.
- `adds_json_gpo` — rewrites SYSVOL files directly from a JSON description. Embeds Microsoft's
  `GPRegistryPolicyParser` (`internal/ad/scripts/gpregistrypolicyparser.ps1`, MIT-licensed; see
  `THIRD_PARTY_NOTICES.md`).
- `adds_gpo_links` — authoritative set of GPO links on one OU/domain/site.
- `adds_gpo_permission`, `adds_gpo_security_filter`, `adds_gpo_wmi_filter`.
- `adds_gpo` data source — read a GPO by display name or GUID.
- Every GPO script does `Import-Module GroupPolicy`.

## FIXED: gpo_links create path (Set-GPLink vs New-GPLink)
- Symptom: linking a GPO onto an OU with no existing link failed with
  "There is no GPO with ID {..} ... Make sure that a GPLink exists ... Parameter name: Guid".
- Cause: `internal/ad/scripts/gpo_link_ensure.ps1` used `Set-GPLink` for creation. `Set-GPLink`
  only UPDATES an existing link; `New-GPLink` CREATES one.
- Fix: capture the target's existing link GUIDs (`Get-GPInheritance`) up front, then `New-GPLink`
  for links that don't exist yet and `Set-GPLink` to update existing ones. Order / LinkEnabled /
  Enforced applied the same way; authoritative removal of undeclared links unchanged.
- Coverage: `adds_gpo_links` now exercised in `test/e2e` (ssh job) and `test/showcase`.

## FIXED: json_gpo version not bumping (user/computer version stuck at 0)
- `versionNumber` (AD) and SYSVOL `GPT.INI` `Version` pack two counters: LOW 16 bits = computer
  version, HIGH 16 bits = user version. `+1` bumps computer, `+65536` bumps user.
- Bug: `json_gpo_ensure.ps1` only bumped the version inside the REGISTRY-settings branch, so a
  GPO whose user content was GPP/scripts (not admin templates) left the user version at 0.
- Fix: track `$machineChanged` / `$userChanged` across ALL setting types (comments, registry,
  audit, security template, scripts, GPP), then bump the matching half once at the end.

## SHELVED: native GroupPolicy under Windows PowerShell (powershell.exe)
- GroupPolicy is NOT native to pwsh 7. Under pwsh it loads via the Windows PowerShell
  Compatibility layer -> warnings + DESERIALIZED objects (nested props like `Get-GPO`
  `.Computer.DSVersion` / `.User.DSVersion` can be lossy).
- v0.0.24 added `gpo_powershell_path` (default `powershell.exe`) to route GPO scripts to 5.1.
  It BROKE every GPO op over WinRM: "remote PowerShell returned no JSON output" (exit 0, empty
  stdout, NO event-log entry => the script body never ran). The bootstrap's
  `[Console]::In.ReadToEnd()` returns "" under `powershell.exe` over WinRM (wsmprovhost);
  `pwsh` reads the same piped stdin fine.
- v0.0.25 REMOVED `gpo_powershell_path` entirely; all ops use `powershell_path` (pwsh), compat
  layer accepted for now.
- Retry plan (needs a live Windows host; cannot repro from macOS): change `stdinBootstrap` in
  `internal/powershell/command.go` to read stdin via the `$input` pipeline enumerator, OR hand
  the gzipped script off via a temp file the command reads then deletes. Verify over WinRM AND
  SSH before re-adding any opt-in setting.

## CORRECTION (2026-10-03, verified in-domain over WinRM/Kerberos to the DC)
The "stdin returns empty under powershell.exe over WinRM" hypothesis above is WRONG for a real
in-domain WinRM connection. Reproduced with `internal/powershell/stdin_winrm_repro_test.go`
against dc1-s13-utv.utv.local (WinRM 5985, Kerberos):
- `powershell.exe` + the CURRENT `[Console]::In.ReadToEnd()` bootstrap WORKS — returns the probe
  JSON, exit 0, non-empty stdout. Edition reported `Desktop`.
- No size issue either: a ~69 KB script round-trips fine under `powershell.exe`.
- The REAL defect is the compat layer, now measured directly with `Get-GPO`:
  - pwsh (compat layer): `.Computer.DSVersion` / `.User.DSVersion` come back NULL (lossy
    deserialization) + the "WinPSCompatSession ... deserialized objects" warning.
  - powershell.exe (native): the same props return correct values (e.g. computer=26, user=4).
- So routing GPO ops to `powershell.exe` over WinRM is viable AND preferable. The v0.0.24
  failure was NOT seen on a dev machine at all - it surfaced on a GitLab Linux CI runner that
  spins up a container and drives the suite from there. The Linux->Windows transport path (and/or
  runner<->server network) differs from an in-domain Windows client; that is the suspect, not
  WinRM in general.
- STILL TO VERIFY before re-adding `gpo_powershell_path`: `powershell.exe` stdin over the SSH
  transport (OpenSSH subsystem), where the original emptiness may genuinely occur. The candidate
  `[Console]::OpenStandardInput()` bootstrap also works for both editions over WinRM and is the
  likely cross-transport fix — test it over SSH next.
## RESOLVED (2026-10-03): gpo_powershell_path re-added, default powershell.exe (v0.0.26)
- SSH verified too: `powershell.exe` + the CURRENT `[Console]::In.ReadToEnd()` bootstrap reads
  stdin fine over OpenSSH on this host (same probe JSON, exit 0). So the stdin bug reproduces on
  NEITHER WinRM NOR SSH here; no bootstrap change was made (current bootstrap left as-is).
- Re-added `gpo_powershell_path` (config + provider schema + `usesGroupPolicyModule` routing in
  `internal/client/client.go`), default `powershell.exe`. Kept the v0.0.25 read-event (1002)
  logging - only the GPO-routing pieces were restored, not reverted wholesale.
- Full-stack proof in `internal/client/grouppolicy_routing_live_test.go`: via `RunPowerShellJSON`,
  powershell.exe => edition Desktop with non-null nested versions; pwsh => edition Core with
  NULL `.Computer.DSVersion` / `.User.DSVersion`. No "remote PowerShell returned no JSON output".
- The v0.0.24 failure was observed on a GitLab Linux CI runner (container-based), not on an
  in-domain Windows client - which cannot reproduce it on either WinRM or SSH. Suspected cause
  is the Linux-container->Windows transport path or runner<->server connectivity. A plain Linux
  runner (no container) re-tests the suite on push; v0.0.26 is the re-validation.
- Lesson: validate transport-level claims on the ACTUAL failing environment (the CI runner)
  before shipping a revert; an in-domain host can hide an environment-specific transport bug.


## Event logging (diagnostic aid)
- Every op is wrapped in a try/catch (`internal/ad/scripts.go` `buildScript`). Failures ->
  `Write-ADDSFailure` (event 1001). Mutating success -> `Write-ADDSChange` (1000). As of
  v0.0.25, read/query success -> `Write-ADDSRead` (1002). Event log name on the target:
  `terraform-provider-adds`.
- Key signal: the ABSENCE of an event for a failed op means the script never executed (that is
  how the powershell.exe stdin bug was identified).

## Test environment (in progress)
- Plan: log onto a MEMBER SERVER as domain admin, point the provider at the DC over WinRM/5986.
  That reproduces the real transport path where the `powershell.exe` stdin bug lives (testing a
  LOCAL powershell.exe on the DC would not — it must go through WinRM).
- Credentials as env vars (`ADDS_HOST` / `ADDS_USERNAME` / `ADDS_PASSWORD`), never in chat.
- Smoke: `go test ./internal/transport -run TestWinRMSmoke -v`, or the `TF_ACC` acceptance tests.
