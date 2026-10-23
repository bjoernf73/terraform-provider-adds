---
page_title: "Anatomy of a Group Policy Object"
subcategory: "Guides"
description: |-
  How a GPO is laid out across Active Directory and SYSVOL, the files that make up its
  SYSVOL template, the text encoding each one uses, and why GPT.INI must be UTF-8 without
  a BOM.
---

# Anatomy of a Group Policy Object

A GPO is **two linked halves**: a small object in Active Directory and a folder of files in
SYSVOL. Dissecting - or hand-building, which is what `adds_json_gpo` does - a GPO means getting
both halves, and the exact encoding of every file, right.

## The two halves

**1. The AD object** — a `groupPolicyContainer` at `CN={GUID},CN=Policies,CN=System,DC=...`.
It holds the GPO's metadata, not its settings:

| Attribute | Holds |
| --- | --- |
| `versionNumber` | The packed version counter (see below) |
| `gPCFileSysPath` | UNC path to the SYSVOL folder (`\\<domain>\SysVol\<domain>\Policies\{GUID}`) |
| `flags` | Which sides are enabled: `0` both, `1` user disabled, `2` computer disabled, `3` both disabled |
| `gPCMachineExtensionNames` / `gPCUserExtensionNames` | The client-side extensions (CSEs) that must process this GPO |
| `displayName` | The friendly name shown in GPMC |
| `nTSecurityDescriptor` | Security-filtering / delegation ACL |

**2. The SYSVOL folder** (the *Group Policy Template*) at
`\\<domain>\SysVol\<domain>\Policies\{GUID}\`. This is where the actual settings live, as files
under `Machine\` and `User\` subtrees.

The two are tied together by the shared `{GUID}` and by `gPCFileSysPath`. When they disagree -
including on the version - clients can skip or mis-apply the policy.

## SYSVOL folder layout

```text
{GUID}\
  GPT.INI                                      # version watermark
  GPO.cmt                                      # GPO-level comment (optional)
  Machine\
    Registry.pol                               # administrative templates (registry)
    comment.cmtx                               # admin-template comments
    Microsoft\Windows NT\SecEdit\GptTmpl.inf   # security settings
    Microsoft\Windows NT\Audit\audit.csv       # advanced audit policy
    Scripts\scripts.ini                        # startup/shutdown script registration
    Scripts\psscripts.ini                      # PowerShell startup/shutdown registration
    Preferences\<Type>\<Type>.xml              # Group Policy Preferences
  User\
    Registry.pol                               # same set, user side
    comment.cmtx
    Scripts\scripts.ini                        # logon/logoff
    Scripts\psscripts.ini
    Preferences\<Type>\<Type>.xml
```

A given GPO only contains the files for the settings it actually has.

## File encodings

Encoding matters more than people expect. Each client-side extension parses its file with fixed
expectations, and the failures are **silent** - a mis-encoded file is quietly ignored or
mis-read, not rejected with an error. This is how `adds_json_gpo` writes each file:

| File | Content | Encoding |
| --- | --- | --- |
| `GPT.INI` | Version watermark | **UTF-8, no BOM** |
| `Registry.pol` | Registry policy (admin templates) | Binary (`PReg` format) |
| `GptTmpl.inf` | Security template | ANSI (system code page) |
| `audit.csv` | Advanced audit policy | UTF-8 |
| `comment.cmtx` | Admin-template comments | UTF-8 |
| `GPO.cmt` | GPO comment | UTF-16 LE (Unicode) |
| `scripts.ini` / `psscripts.ini` | Script registration | UTF-8 |
| `Preferences\*.xml` | Group Policy Preferences | UTF-8 |

The one that bites hardest is `GPT.INI`, covered below.

## The version watermark

`GPT.INI` is a tiny INI file with one section and one key:

```ini
[General]
Version=65537
```

That is the complete, correct content - **no settings live in GPT.INI**, only the version.

`Version` is **one 32-bit integer** that packs two 16-bit counters: the **low 16 bits** are the
**computer** version and the **high 16 bits** are the **user** version.

```text
Version = userVersion * 65536 + computerVersion
```

```powershell
$computerVersion = $Version % 65536                  # low word
$userVersion     = [math]::Floor($Version / 65536)   # high word
```

| computer | user | packed `Version` |
| --- | --- | --- |
| 1 | 0 | 1 |
| 9 | 0 | 9 |
| 0 | 1 | 65536 |
| 1 | 1 | 65537 |
| 19 | 19 | 1245203 |

A small number like `9` is correct for a machine-only GPO (computer 9, user 0); the number only
grows past `65535` once the user side has been edited.

This value is **the same number** as the AD `versionNumber` attribute - not a derived or separate
number. `adds_json_gpo` keeps them in lockstep: on each apply it reads `versionNumber`, raises the
side whose settings changed (a computer change is `+1`, a user change is `+65536`), writes the new
value back to `versionNumber`, and writes the identical number into `GPT.INI`. That bump is what
makes clients re-apply the GPO on the next `gpupdate`. (`adds_backup_gpo` leaves this to
`Import-GPO`, which writes both itself.)

## Why a BOM breaks the SYSVOL version

`GPT.INI` must be **UTF-8 without a byte order mark**. If it is written with a BOM (the three
bytes `EF BB BF` at the start), the Group Policy engine cannot parse the leading `[General]` line,
so it reads the **SYSVOL version as `0`**.

The AD `versionNumber` is unaffected - it is an attribute, not the file - so you get a permanent
split that GPMC reports as, for example:

```text
Computer version: 17 (AD), 0 (SYSVOL)
```

Because AD and SYSVOL no longer match, the GPO looks perpetually out of date, and in Terraform it
surfaces as a never-settling diff on `computer_sysvol_version` / `user_sysvol_version` (they keep
reading `0`). The fix is simply to write `GPT.INI` without a BOM, which is what this provider does.
