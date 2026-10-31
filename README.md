# terraform-provider-adds

A Terraform **Active Directory Domain Services** provider that manages Active Directory
objects and policy through **PowerShell 7** on a remote Windows host over **WinRM**
or **SSH**. Execution is remote-only today; local execution may be supported in the
future.

> [!WARNING]
> This is an **experimental** provider.  Use it with care and expect breaking changes.

## Resources

| Resource | Manages |
| --- | --- |
| `adds_organizational_unit` | Organizational units, creating missing parents on demand |
| `adds_group` | Groups: `Security` or `Distribution` category, `DomainLocal`, `Global` or `Universal` scope |
| `adds_group_member` | A single group membership |
| `adds_user` | User accounts. Passwords are not managed here. |
| `adds_user_password` | Sets a user's initial password, generated or supplied |
| `adds_gmsa` | Group managed service accounts |
| `adds_computer` | Computer accounts, for pre-staging or reconciling machine objects |
| `adds_kds_root_key` | Ensures a forest KDS root key exists (the gMSA prerequisite) |
| `adds_ace` | A single access control entry (ACE) on any directory object (formerly `adds_access_rule`) |
| `adds_acl` | The complete DACL of a directory object, managed authoritatively |
| `adds_backup_gpo` | Imports a `Backup-GPO` folder into a GPO |
| `adds_json_gpo` | Imports a JSON-described GPO |
| `adds_gpo_links` | The full, ordered set of GPO links on an OU, domain or site |
| `adds_gpo_link` | A single, non-authoritative GPO link on an OU, domain or site |
| `adds_wmi_filter` | A WMI filter (`msWMI-Som` object) |
| `adds_gpo_wmi_filter` | The WMI filter assigned to a single GPO |
| `adds_gpo_permission` | One trustee's named Group Policy permission |
| `adds_gpo_security_filter` | The complete set of principals allowed to apply one GPO |
| `adds_netlogon_files` | A Terraform-owned file tree below NETLOGON |
| `adds_administrative_templates` | A Terraform-owned file tree at the Central Store root |
| `adds_site` | An Active Directory replication site |
| `adds_subnet` | A CIDR network assigned to an Active Directory site |

## Data sources

| Data source | Reads |
| --- | --- |
| `adds_domain` | The connected domain: DN, DNS root, NetBIOS name, well-known containers |
| `adds_computer` | An existing computer account, by DN, GUID, SID, `DOMAIN\name` or `sAMAccountName` |
| `adds_group` | An existing group, by DN, GUID, SID, `DOMAIN\name` or `sAMAccountName` |
| `adds_user` | An existing user, by DN, GUID, SID, `DOMAIN\name`, `sAMAccountName` or UPN |
| `adds_organizational_unit` | An existing OU, by slash path or distinguished name |
| `adds_gpo` | An existing Group Policy Object, by display name or GUID |
| `adds_json_gpo_export` | A live GPO's SYSVOL content, as JSON |
| `adds_backup_gpo_export` | A live GPO as native GPMC backup files, base64-encoded |

## Documentation

Full documentation lives in [docs/](docs/):

- [Provider configuration and authentication](docs/index.md)
- [Paths and distinguished names](docs/guides/paths.md) — how object locations are resolved
- [Access rules and delegation](docs/guides/access-rules.md) — all six ACE constructors
- [Group scope conversions](docs/guides/group-scope-conversions.md) — Global, DomainLocal, Universal and the Universal hop
- [Managing Group Policy Objects](docs/guides/gpos.md) — backup vs. JSON GPOs, migration, links, drift
- [Managing SYSVOL files](docs/guides/sysvol-files.md) — NETLOGON scripts and Administrative Templates
- [Managing Active Directory Sites](docs/guides/sites-and-subnets.md) — replication sites and subnet assignment
- [Dependencies and ordering](docs/guides/dependencies.md) — references, `depends_on` and cycles
- [User passwords and secret storage](docs/guides/passwords.md) — generation, rotation, Vault composition
- [Repeating object patterns across systems](docs/guides/repeating-patterns.md) — `for_each` and modules
- Resource reference under [docs/resources/](docs/resources/)

`docs/` is generated — edit the schema `MarkdownDescription` strings, the snippets in
[examples/](examples/), or the page templates in [templates/](templates/), then run:

```sh
make docs
```

## Quick start

A minimal configuration:

```hcl
terraform {
  required_providers {
    adds = {
      source  = "bjoernf73/adds"
      version = "0.0.32"
    }
  }
}

provider "adds" {
  transport       = "winrm"
  host            = "dc1.contoso.local"
  username        = "CONTOSO\\terraform"
  password        = var.password
  winrm_auth      = "ntlm"
  powershell_path = "pwsh"
}

resource "adds_organizational_unit" "servers" {
  path        = "Contoso/Servers/Windows"
  description = "Windows server OU"
}

resource "adds_group" "server_admins" {
  name  = "Server Admins"
  path  = "Contoso/Groups"
  scope = "DomainLocal"
}

resource "adds_access_rule" "delegate_computers" {
  target                = adds_organizational_unit.servers.distinguished_name
  trustee               = adds_group.server_admins.sid
  rights                = ["CreateChild", "DeleteChild"]
  object_type           = "computer"
  inherited_object_type = "organizationalUnit"
  inheritance           = "Descendents"
}
```

## Requirements

- PowerShell 7 (`pwsh`) on the target host
- The `ActiveDirectory` and `GroupPolicy` PowerShell modules on the target host
- WinRM or OpenSSH reachable from wherever Terraform runs

Most operations use PowerShell 7, selected with `powershell_path` (default `pwsh`).
GPO operations that load the `GroupPolicy` module use Windows PowerShell 5.1 by
default (`gpo_powershell_path`, default `powershell.exe`). This avoids issues with
the PowerShell 7 Windows PowerShell Compatibility layer, which returns deserialized
objects and can lose nested properties. Set either path to choose a different
executable; setting `gpo_powershell_path` to `pwsh` opts GPO operations into that
compatibility layer.

## Transports

This provider is tested on gitlab runners running in a kubernetes cluster and virtual windows core and linux boxes. Mac and freebsd probably works, but that is untested.

| Transport | Authentication |
| --- | --- |
| `winrm` | `basic`, `ntlm`, `kerberos` |
| `ssh` | password, private key |

Notes that save time:

- Don't spend time trying to get WinRM over http work when terraform runner is outside of domain, it is faster to configure https on 5986 using a self-signed certificate - it is done in minutes.

- `kerberos` of course needs the target FQDN, not an IP address, because the SPN is derived from
  the host name. There are other requirements as well.

## Development

```sh
make build       # go build ./...
make vet
make test
make docs        # regenerate docs/ (needs the terraform CLI)
```

End-to-end tests live in [test/e2e/](test/e2e/) and run against a real domain controller
from CI over both transports. See [.gitlab-ci.yml](.gitlab-ci.yml).

A manual transport check, for when CI is too slow a feedback loop:

```sh
ADDS_HOST=10.0.13.6 ADDS_USERNAME='CONTOSO\Administrator' ADDS_PASSWORD=... \
  go test ./internal/transport -run TestWinRMSmoke -v
```

## Releasing

### Building releases

Releases are **automatically built** via GitHub Actions when you push a tag to the `github` remote:

```sh
# Create and push a new version
git tag -s v0.0.32
git push github v0.0.30       # Triggers GitHub Actions build and release 
```

The GitHub Actions workflow ([.github/workflows/release.yml](.github/workflows/release.yml)):

- Builds binaries for **linux_amd64**, **darwin_amd64**, **darwin_arm64**, **windows_amd64** ++
- Creates ZIP archives with proper Terraform naming (`terraform-provider-adds_0.0.15_linux_amd64.zip`)
- Generates SHA256 checksums
- Creates a GitHub release with all artifacts

## License

Copyright (c) bjoernf73.

This provider is licensed under the **Mozilla Public License 2.0** — see [LICENSE](LICENSE).

It embeds third-party source code under its own license; see
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) (currently Microsoft's MIT-licensed
`GPRegistryPolicyParser`).

