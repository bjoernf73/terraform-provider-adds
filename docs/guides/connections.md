---
page_title: "Connecting over WinRM and SSH"
subcategory: "Guides"
description: |-
  How the provider reaches the Windows host: WinRM Basic, NTLM and Kerberos over HTTP
  or HTTPS, SSH password or private-key authentication, host verification, and the
  limitations observed with OpenSSH on Windows.
---

# Connecting over WinRM and SSH

Every operation the provider performs is a PowerShell command executed on a remote
Windows host. There is no native LDAP client. The provider therefore needs a working
remote-execution transport, and the host needs the `ActiveDirectory` (and, for Group
Policy, `GroupPolicy`) modules installed.

Two transports are supported, selected with `transport`:

| Transport | Mechanisms | Default port |
| --- | --- | --- |
| `winrm` | `basic`, `ntlm`, `kerberos` | 5985 (HTTP), 5986 (HTTPS) |
| `ssh` | password, private key | 22 |

As a rule of thumb: inside the domain, WinRM with Kerberos or SSH are both good. From a
runner outside the domain, prefer WinRM over **HTTPS** or SSH — never WinRM over plain
HTTP across a trust boundary.

## WinRM

WinRM supports **Basic**, **NTLM** and **Kerberos** authentication. Each can be used
over HTTP or HTTPS; `winrm_use_tls` selects HTTPS and defaults the port to 5986 (HTTP
defaults to port 5985). The examples below show each combination. `insecure` controls
validation of the WinRM server's TLS certificate; it does not select an authentication
mechanism.

### Basic over HTTP

Basic sends the username and password as HTTP authentication credentials. Use HTTP only
on a trusted network; HTTPS is recommended otherwise.

```hcl
provider "adds" {
  transport     = "winrm"
  host          = "dc1.contoso.local"
  winrm_auth    = "basic"
  winrm_use_tls = false
  username      = "CONTOSO\\svc-adds"
  password      = var.password
}
```

### Basic over HTTPS

```hcl
provider "adds" {
  transport     = "winrm"
  host          = "dc1.contoso.local"
  winrm_auth    = "basic"
  winrm_use_tls = true
  username      = "CONTOSO\\svc-adds"
  password      = var.password
}
```

The target must present a certificate trusted by the machine running Terraform and valid
for the host name. `insecure = true` skips this server-certificate validation and is
appropriate only for isolated testing.

### NTLM over HTTP

```hcl
provider "adds" {
  transport     = "winrm"
  host          = "dc1.contoso.local"
  winrm_auth    = "ntlm"
  winrm_use_tls = false
  username      = "CONTOSO\\svc-adds"
  password      = var.password
}
```

NTLM authenticates per connection, and the server closes connections between operations.
A pooled connection can therefore be rejected with a transient `401`; the provider
retries these automatically (the script never ran, so a retry is safe).

### NTLM over HTTPS

```hcl
provider "adds" {
  transport     = "winrm"
  host          = "dc1.contoso.local"
  winrm_auth    = "ntlm"
  winrm_use_tls = true
  username      = "CONTOSO\\svc-adds"
  password      = var.password
}
```

### Kerberos over HTTP

Kerberos uses the supplied account credentials and realm. The Kerberos library does not
automatically use the operating system's default ticket cache, and Windows does not ship
with a `krb5.conf` file. If no config path is specified, the provider generates a basic
configuration using the target host as the KDC.

```hcl
provider "adds" {
  transport            = "winrm"
  host                 = "dc1.contoso.local"
  winrm_auth           = "kerberos"
  winrm_use_tls        = false
  winrm_kerberos_realm = "CONTOSO.LOCAL"
  username             = "svc-adds"
  password             = var.password
}
```

### Kerberos over HTTPS

```hcl
provider "adds" {
  transport            = "winrm"
  host                 = "dc1.contoso.local"
  winrm_auth           = "kerberos"
  winrm_use_tls        = true
  winrm_kerberos_realm = "CONTOSO.LOCAL"
  username             = "svc-adds"
  password             = var.password
}
```

Kerberos settings:

- **`winrm_kerberos_realm`** — the realm is upper-cased for you, so
  `contoso.local` and `CONTOSO.LOCAL` are equivalent.
- **`password`** — used with username for Kerberos authentication. To use an explicit
  credential cache instead, set `winrm_kerberos_ccache_path` to its path on the machine
  running Terraform. The provider does not discover the OS ticket cache automatically.
- **`username`** — give it bare (`svc-adds`). A NetBIOS or UPN form
  (`CONTOSO\svc-adds`, `svc-adds@contoso.local`) also works; the provider strips the
  prefix/suffix, since the realm is supplied separately.

What the provider does for you:

- **Generates a `krb5.conf`** when `winrm_kerberos_config_path` is not set, using the
  realm and the target host as the KDC (`dns_lookup_kdc = true`), written to a per-realm
  temp file. On a domain-joined machine this works out of the box.
- **Defaults the SPN** to `HTTP/<host>`.

Optional overrides:

| Setting | Purpose |
| --- | --- |
| `winrm_kerberos_config_path` | Use your own `krb5.conf` instead of the generated one. |
| `winrm_kerberos_spn` | Override the service principal name (default `HTTP/<host>`). |
| `winrm_kerberos_ccache_path` | Use an existing Kerberos credential cache on the Terraform machine. |

If the host named in `host` is not itself a KDC, supply a `krb5.conf` via
`winrm_kerberos_config_path` whose `[realms]` block lists a reachable KDC.

### WinRM client certificates

WinRM HTTPS uses a **server** certificate to protect and authenticate the endpoint. The
provider does not currently support authenticating to WinRM with a client certificate;
the supported WinRM mechanisms are Basic, NTLM and Kerberos. `insecure` only disables
server-certificate validation and does not enable client-certificate authentication.

## SSH

SSH uses the Windows OpenSSH server. Password and private-key authentication are both
supported. The provider accepts private-key **PEM contents** in `ssh_private_key_pem`;
it has no separate private-key-path setting. The private key must be readable by the
machine running Terraform and its matching public key must be authorized for the remote
SSH account.

### SSH password authentication

```hcl
provider "adds" {
  transport = "ssh"
  host      = "dc1.contoso.local"
  username  = "svc-adds"
  password  = var.password
}
```

### SSH private key loaded from a file

Use Terraform's `file()` function to read the PEM file and `pathexpand()` to expand a
home-directory path:

```hcl
provider "adds" {
  transport           = "ssh"
  host                = "dc1.contoso.local"
  username            = "svc-adds"
  ssh_private_key_pem = file(pathexpand("~/.ssh/id_ed25519"))
}
```

### SSH private key supplied as a PEM value

The value can also come directly from a sensitive Terraform variable, such as one
populated by a secrets manager:

```hcl
variable "ssh_private_key_pem" {
  type      = string
  sensitive = true
}

provider "adds" {
  transport           = "ssh"
  host                = "dc1.contoso.local"
  username            = "svc-adds"
  ssh_private_key_pem = var.ssh_private_key_pem
}
```

The variable must contain the complete PEM-encoded private key, not a file path. Avoid
committing private-key contents to configuration. The SSH client currently parses keys
without a passphrase; encrypted private keys are not supported.

Both `password` and `ssh_private_key_pem` may be set. The SSH client offers both
authentication methods, and the server can accept whichever its policy allows.

### SSH host-key verification

Host-key verification is required unless `insecure = true`. This is independent of
whether SSH authentication uses a password or a private key. Choose one of these:

| Setting | Purpose |
| --- | --- |
| `ssh_known_hosts_path` | Path to a `known_hosts` file on the Terraform machine. |
| `ssh_host_key` | The host public key in `authorized_keys` format. |
| *(neither)* | Falls back to `~/.ssh/known_hosts` if present. |
| `insecure = true` | Skips verification entirely (lab only). |

### Observed limitations with OpenSSH on Windows

Win32-OpenSSH is noticeably less robust than WinRM for this workload, and it is worth
knowing why:

- **The command always runs under `cmd.exe`.** The Windows OpenSSH server launches the
  remote command through its configured default shell, which is `cmd.exe` unless the
  `DefaultShell` registry value is changed. The provider accounts for this: the real
  script is streamed on **stdin** (gzipped and base64-encoded) rather than placed on the
  command line, so `cmd.exe`'s ~8191-character command-line limit never applies. You do
  not need to change `DefaultShell`.
- **The stdin pipe is occasionally torn down mid-command.** Under load — or sometimes
  for no obvious reason — the server-side stdin pipe to the child process breaks. On the
  server this shows up in the *PowerShellCore/Operational* event log as
  *"An error has occurred in PowerShell IPC listening thread … The pipe has been
  ended."* This is a transport-level fault, not a provider bug, and it can fail an apply
  even when the host is otherwise idle.
- **The provider retries transport-level failures.** Dial failures, abnormal
  terminations and timeouts are retried up to three times with exponential backoff, which
  absorbs the intermittent pipe break. A *clean non-zero exit* (a genuine remote/AD
  error) is **never** retried, so non-idempotent operations are not re-run.

### Known upstream issue: intermittent connection delays

Win32-OpenSSH has a documented, still-open bug where establishing a session stalls for
**15–120 seconds** at random
([PowerShell/Win32-OpenSSH#2425](https://github.com/PowerShell/Win32-OpenSSH/issues/2425)).
The stall happens while the server spawns the `sshd-session.exe` subprocess — before the
SSH protocol or your command even starts — and it reproduces across OpenSSH 9.1, 9.2 and
10.0. Notably:

- It is **not** resource exhaustion: it occurs with CPU under 10% and RAM to spare, so
  adding CPU/RAM may not help.
- It correlates with **idle periods**: the first connection after a quiet spell is the
  slow one, then connections are fast again for a while. This matches the slow runs that
  cluster right after a bulk delete or an idle gap.
- A reported workaround is to create a local user named `sshd` on the server (see the
  linked issue and #1817); the underlying cause appears to involve an `lsass` timeout
  during subprocess creation, so a domain controller — where `lsass` is already busy —
  can feel it more.

The provider copes with this by enforcing `timeout_seconds` and retrying transport-level
failures (above). Setting `timeout_seconds` to **120** or more clears the worst of the
delay band so a stalled connect is retried rather than failing the apply.

## Timeouts

`timeout_seconds` (default 30) bounds both the connection and each remote command. A
command that never returns — a stuck session, for example — fails after the timeout
instead of blocking the apply indefinitely. Group Policy imports over SSH are the slowest
operations; raise `timeout_seconds` if you import large GPOs against a loaded host.

## Scaling to large configurations

Every operation is a separate remote PowerShell process: the provider opens a shell,
streams the script on stdin, reads the result, and closes the shell. Terraform runs these
in parallel — ten at a time by default — so a large configuration, where hundreds of
objects are refreshed on every `plan`, opens hundreds of short-lived WinRM (or SSH) shells
against a single domain controller.

The usual symptom of overload is a sporadic `dial tcp ...:5986: i/o timeout` (or a
truncated-stdin error) on one resource while the rest succeed, after which a re-run or a
serial apply works. That pattern is **concurrency contention, not bandwidth**: the payloads
are small (a few KB each, gzipped), but the number of concurrent shells and the domain
controller's WinRM limits are the real ceiling.

If you hit this on a large directory, in order of effort:

- **Lower Terraform's parallelism.** `terraform apply -parallelism=3` (or even `1`) is the
  single biggest lever and needs no configuration change. It trades wall-clock time for a
  far lower concurrent-shell count.
- **Raise `timeout_seconds`.** A busy controller answers more slowly under load;
  `timeout_seconds = 120` gives each request room before it is retried.
- **Raise the host's WinRM limits.** Inspect them with `winrm get winrm/config`; the ones
  that bite first are `MaxShellsPerUser`, `MaxConcurrentOperationsPerUser` and
  `MaxMemoryPerShellMB`. The defaults are easily exhausted by parallel applies.

The provider also retries transient transport failures automatically: network timeouts,
connection resets, dropped connections, and an undelivered script payload (empty stdin) are
retried up to three times with backoff, the same way a stale-connection `401` is. A clean
non-zero exit — a genuine Active Directory error — is never retried. Retries absorb the
occasional blip but are not a substitute for lowering parallelism on a genuinely overloaded
host.

## Troubleshooting

The provider writes diagnostic logs through Terraform's logger, not to the Windows event
log. To capture them — including SSH retry warnings — set the standard Terraform log
variables on the machine running Terraform:

```sh
export TF_LOG=DEBUG
export TF_LOG_PATH=./adds.log
terraform apply
```

A retried SSH command logs a warning with the attempt number, the backoff and the
underlying error, so you can see whether the retry mechanism is engaging and how often.
