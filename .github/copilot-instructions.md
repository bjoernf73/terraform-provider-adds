# terraform-provider-adds — repository instructions

## What this repository is

A Terraform provider (`adds`) that manages **Active Directory** objects by executing
**PowerShell 7** on a remote Windows host over **WinRM** or **SSH**. There is no native
LDAP client — every operation is a PowerShell script that emits JSON on stdout, which Go
decodes into a typed struct.

Module path: `github.com/bjoernf73/terraform-provider-adds`
Provider address: `registry.terraform.io/bjoernf73/adds`

## Non-negotiable rules

1. **Use `terraform-plugin-framework` only.** Never use `helper/schema`, `terraform-plugin-sdk`,
   `*schema.Resource`, `d.Get`/`d.Set`, or `schema.CreateContext`. `ref/terraform-provider-ad` is a
   rich source of *behaviour* (what a resource does, which attributes it exposes, how it talks to AD),
   but its *mechanics* are SDKv2 — always restate them in framework terms:
   `d.Get("x")` → typed model + `req.Plan.Get`, `d.Set` → `resp.State.Set`,
   `ForceNew` → `RequiresReplace()`, `ValidateFunc` → validators,
   `d.SetId("")` → `resp.State.RemoveResource(ctx)`, `diag.Diagnostics` return → `resp.Diagnostics.AddError`.
2. **Never edit anything under `ref/`.** It is read-only reference material (see below).
3. **Every resource implements full CRUD**, not just create. The PowerShell reference module
   is create-oriented; you must add read, update (or `RequiresReplace`), delete and import.
4. **Remote scripts must be idempotent and JSON-only.** stdout is parsed as JSON; anything else
   breaks the client.

## The `ref/` folder (read-only reference)

| Folder | What it is | How to use it |
| --- | --- | --- |
| `ref/dry.module.ad` | The author's PowerShell module for AD objects (classes, functions, `scriptblocks/DryAD_SB_*.ps1`). Create-only. | Source of truth for **AD semantics**: attribute names, OU path handling, group/user/GPO/WMI-filter logic. Adapt scriptblocks into Go string constants. |
| `ref/terraform-plugin-framework` | Upstream HashiCorp framework source. | Source of truth for **provider structure and APIs**: schema types, plan modifiers, validators, `resource.ResourceWith*` interfaces, acceptance test helpers. |
| `ref/terraform-provider-ad` | The existing community AD provider (SDKv2). | A full functional reference, not just transport: resource/attribute coverage and naming, AD object semantics and PowerShell for users/groups/computers/OUs/GPOs/GPLinks, import and ID formats, CLIXML handling, WinRM/Kerberos/NTLM wiring, docs and build/release tooling. **Translate, never transplant — its SDKv2 architecture must not be copied.** |

`ref/` subfolders are separate Go modules / PowerShell modules, so they are not part of this
module's build. `go build ./...` at the repo root must never need them.

## Architecture

```
main.go                          providerserver.Serve -> internal/provider
internal/config                  Config struct: transport, creds, timeouts, powershell_path, domain_controller
internal/transport               Runner interface { Run(ctx, command, stdin) (Result, error) } + optional
                                 HostRunner { StartHost -> HostSession }; winrm.go, ssh.go reuse one connection
internal/powershell              BuildRunCommand (one-shot stage+run+delete), BuildHostCommand (warm host loop), DecodeCLIXML
internal/client                  Client: RunPowerShell, RunPowerShellJSON (decode stdout into target); warm-host + fallback
internal/ad                      One file per AD object type + scripts/*.ps1 embedded via go:embed
internal/provider                Provider definition + one resource_*.go per resource
examples/<resource>/             Runnable HCL example per resource
```

Data flow for any operation:
`resource_x.go` → `internal/ad` builds a script → `client.RunPowerShellJSON` →
`transport.Runner` (WinRM/SSH) stages the script on the host and runs it → JSON on stdout → struct.

Default `pwsh` operations run on a **warm host**: one long-lived remote `pwsh` process per
connection (`internal/powershell` `BuildHostCommand`) that loops reading a plain-text script from
stdin, staging it to `C:\Windows\Temp\adds-<token>.ps1`, running it with `& $p`, deleting it, and
returning a length-framed JSON envelope (`{exit, out, err}`, stdout/stderr base64). Because the
process stays alive, the first operation pays the ~600 ms ActiveDirectory module import and later
ones reuse the loaded module. `internal/client` holds the host, restarts it on a transport fault
(`hostMaxAttempts`), and latches it off (`hostDisabled`) on a malformed response, falling back to
the one-shot path. The transport also keeps **one connection/shell alive** across operations
(SSH session-per-op on a persistent dial; a persistent WinRM shell), so `sshd.exe`/`winrshost.exe`
stay ~constant instead of growing per operation.

GPO operations (Windows PowerShell via `GPOPowerShellPath`) and any host-unavailable case use the
one-shot path: a single `-EncodedCommand` run bootstrap reads the script as **plain text** from
**stdin**, writes it to `C:\Windows\Temp\adds-<token>.ps1`, runs it from disk with the call
operator (`& $p`), and deletes it in a `finally` block — all in one remote process. The body travels
on stdin, not the command line, because WinRM shells run under `cmd.exe` whose 8191 character limit
the scripts would otherwise exceed — **never put the script on the command line**. The script is
staged and run as a file (not decompressed and `Invoke-Expression`-ed in memory) because the old
in-memory bootstrap is behaviourally identical to a PowerShell stager and is blocked by behavioural
antivirus (Defender `Behavior:Win32/PShellCobStager`). Keep both the run bootstrap and the host
loop free of `FromBase64String`/`GZipStream`/`Invoke-Expression`; embedded scripts must stay ASCII so the
plaintext stdin transfer is encoding-safe.
   equivalent resource in `ref/terraform-provider-ad/ad/` + its page under
   `ref/terraform-provider-ad/docs/` — use them to decide the attribute set, defaults, read/update
   behaviour and import ID before writing any Goruct.

## Adding a new AD resource

1. Find the matching logic in `ref/dry.module.ad` (`functions/` and `scriptblocks/`).
2. Create `internal/ad/scripts/<object>_common.ps1` plus one `.ps1` per operation
   (`_ensure`, `_read`, `_update`, `_delete`). They are embedded by
   [internal/ad/scripts.go](internal/ad/scripts.go) — never inline PowerShell in Go strings.
3. Create `internal/ad/<object>.go` with:
   - a result struct with `json:"snake_case"` tags,
   - `Ensure…`, `Read…`, `Update…`, `Delete…` functions taking `(ctx, *client.Client, …)`,
   - script filename constants passed to `buildScript(c, common, body, payload)`, which
     marshals inputs to JSON, base64-encodes them and injects them as `$payload` —
     **never** string-interpolate user input directly into PowerShell.
4. Create `internal/provider/resource_<object>.go` implementing `resource.Resource`,
   `resource.ResourceWithConfigure`, `resource.ResourceWithImportState`.
5. Register the constructor in `Resources()` in [internal/provider/provider.go](internal/provider/provider.go).
6. Add `examples/<resource>/main.tf` + `variables.tf` + `README.md`.

### Resource conventions

- `Metadata`: `resp.TypeName = req.ProviderTypeName + "_<object>"`.
- `id` is `Computed` and equals the object's **distinguished name**; `distinguished_name` is
  also exposed as a computed attribute.
- Immutable inputs get `stringplanmodifier.RequiresReplace()`; anything reconcilable gets a real
  `Update` implementation.
- Computed attributes derived from immutable inputs get `stringplanmodifier.UseStateForUnknown()`
  so in-place updates do not mark them unknown.
- Every attribute has a `MarkdownDescription`. Secrets are `Sensitive: true`.
- `Read` must remove the resource from state (`resp.State.RemoveResource(ctx)`) when the remote
  object no longer exists — the script should return `exists: false` rather than throwing.
- `Configure` type-asserts `req.ProviderData.(*client.Client)` and returns early when it is nil.
- Errors surface via `resp.Diagnostics.AddError(summary, err.Error())`; never `panic` or `log.Fatal`.
- Import via `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

### PowerShell script conventions

- Start with `$ErrorActionPreference = 'Stop'`. Operations that need the ActiveDirectory module get
  its import injected by `buildScript` as `$env:ADPS_LoadDefaultDrive = 0` + `Import-Module
  ActiveDirectory -ErrorAction Stop` (the env var skips the unused `AD:` drive to speed the load);
  do not re-import it in the body.
- Honour `$payload.domain_controller` by splatting a `-Server` parameter (`Get-ServerParams`).
- Output exactly one `ConvertTo-Json` document on stdout; write nothing else to stdout.
- Any property that maps to a Go slice MUST be wrapped so a single item still serializes as a
  JSON array: build it with `@(...)` (or return it from a helper via `, @(...)`). An un-wrapped
  assignment from cmdlet/pipeline output (`prop = $result`) emits a scalar for one item and
  `null`/absent for none, which breaks the Go `[]T` decode. Verified: an `@()`-built property
  serializes as `[...]` for one element and `[]` for none under both `pwsh` and `powershell.exe`.
- Treat "identity not found" as a normal state (`exists: false`), not an error.
- Paths are slash-delimited relative to the domain root (`Contoso/Servers/Windows`) and converted
  to DNs in PowerShell; missing parent OUs are created on demand.

## Go conventions

- Go 1.25, standard `gofmt`. Wrap errors with `%w` and lowercase messages
  (`fmt.Errorf("reading organizational unit: %w", err)`).
- Keep `internal/ad` free of any `terraform-plugin-framework` types; keep
  `internal/provider` free of raw PowerShell.
- Helpers converting between `types.String` and `*string` live in the `provider` package
  (`optionalString`, `stringPointerToTerraform`) — reuse them.

## Commands

```sh
go build ./...
go vet ./...
gofmt -l .
go test ./...
TF_ACC=1 go test ./internal/provider/... -v   # acceptance tests (needs a live DC)
```

Acceptance tests require a reachable domain controller and are opt-in via `TF_ACC`.

## Security

- Credentials and keys must be `Sensitive: true` and must never be logged or interpolated
  into script text in cleartext beyond what the transport requires.
- `insecure` disables TLS/host-key verification — keep it opt-in and documented as unsafe.
- Prefer host key verification (`ssh_known_hosts_path` / `ssh_host_key`) and TLS for WinRM.
- All remote input crosses a trust boundary: pass it as base64 JSON `$payload`, never as
  concatenated script text.
