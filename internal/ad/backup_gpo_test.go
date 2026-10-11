package ad

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bjoernf73/terraform-provider-adlc/internal/client"
)

func TestBuildMigrationTableXMLEmpty(t *testing.T) {
	if xml := BuildMigrationTableXML(nil); xml != "" {
		t.Fatalf("expected empty string for no migrations, got %q", xml)
	}
}

func TestBuildMigrationTableXMLContents(t *testing.T) {
	xml := BuildMigrationTableXML([]BackupGPOMigration{
		{Source: `OLDDOMAIN\svc`, Destination: `NEWDOMAIN\svc`, Type: "User"},
		{Source: `\\old\share`, Destination: `\\new\share`, Type: "UNCPath"},
	})

	for _, want := range []string{
		`<MigrationTable xmlns="http://www.microsoft.com/GroupPolicy/GPOOperations/MigrationTable">`,
		"<Type>User</Type>",
		"<Source>OLDDOMAIN\\svc</Source>",
		"<Destination>NEWDOMAIN\\svc</Destination>",
		"<Type>UNCPath</Type>",
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("expected migration table XML to contain %q, got:\n%s", want, xml)
		}
	}
}

func TestBuildMigrationTableXMLSameAsSource(t *testing.T) {
	xml := BuildMigrationTableXML([]BackupGPOMigration{
		{Source: `AnotherGroup@utv.local`, SameAsSource: true, Type: "GlobalGroup"},
	})

	if strings.Contains(xml, "<Destination>") {
		t.Fatalf("expected no <Destination> element for a same_as_source entry, got:\n%s", xml)
	}
	if !strings.Contains(xml, "<DestinationSameAsSource>") {
		t.Fatalf("expected a <DestinationSameAsSource> element, got:\n%s", xml)
	}
}

func TestHashBackupGPOContentOrderIndependent(t *testing.T) {
	filesA := []BackupGPOFile{{Path: "a.xml", Content: "AA=="}, {Path: "b.xml", Content: "BB=="}}
	filesB := []BackupGPOFile{{Path: "b.xml", Content: "BB=="}, {Path: "a.xml", Content: "AA=="}}

	if HashBackupGPOContent(filesA, nil) != HashBackupGPOContent(filesB, nil) {
		t.Fatal("expected hash to be independent of file order")
	}

	migA := []BackupGPOMigration{{Source: "x", Destination: "y", Type: "Unknown"}}
	if HashBackupGPOContent(filesA, migA) == HashBackupGPOContent(filesA, nil) {
		t.Fatal("expected migrations to change the hash")
	}
}

func TestLoadBackupGPOFiles(t *testing.T) {
	root := t.TempDir()
	backupDir := filepath.Join(root, "MyGPO", "{9CB8219C-31FF-4A85-A7A3-9BCBB6A41D02}")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := os.WriteFile(filepath.Join(backupDir, "Backup.xml"), []byte("<Backup/>"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	files, err := LoadBackupGPOFiles(root, "MyGPO")
	if err != nil {
		t.Fatalf("LoadBackupGPOFiles: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	want := "{9CB8219C-31FF-4A85-A7A3-9BCBB6A41D02}/Backup.xml"
	if files[0].Path != want {
		t.Fatalf("expected path %q, got %q", want, files[0].Path)
	}

	decoded, err := base64.StdEncoding.DecodeString(files[0].Content)
	if err != nil {
		t.Fatalf("decoding content: %v", err)
	}
	if string(decoded) != "<Backup/>" {
		t.Fatalf("expected decoded content %q, got %q", "<Backup/>", string(decoded))
	}
}

func TestLoadBackupGPOFilesMissing(t *testing.T) {
	if _, err := LoadBackupGPOFiles(t.TempDir(), "DoesNotExist"); err == nil {
		t.Fatal("expected an error for a missing backup folder")
	}
}

func TestBackupGPOExportLive(t *testing.T) {
	name := os.Getenv("ADLC_BACKUP_GPO_EXPORT_NAME")
	if name == "" {
		t.Skip("ADLC_BACKUP_GPO_EXPORT_NAME not set")
	}
	base := liveADClient(t)
	for _, executable := range []string{"powershell.exe", "pwsh"} {
		t.Run(executable, func(t *testing.T) {
			cfg := base.Config()
			cfg.GPOPowerShellPath = executable
			remote, err := client.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			result, err := ExportBackupGPO(ctx, remote, name)
			if err != nil {
				t.Fatalf("exporting live GPO backup: %v", err)
			}
			if !result.Exists || result.GUID == "" || result.BackupID == "" || len(result.Files) == 0 {
				t.Fatal("incomplete backup export")
			}
			metadata, registry := false, false
			prefix := "{" + strings.ToLower(result.BackupID) + "}/"
			for name, content := range result.Files {
				if path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\:") ||
					(!strings.EqualFold(name, "manifest.xml") && !strings.HasPrefix(strings.ToLower(name), prefix)) {
					t.Fatalf("backup contains an unexpected relative file path %q (snapshot %s)", name, result.BackupID)
				}
				decoded, err := base64.StdEncoding.DecodeString(content)
				if err != nil {
					t.Fatalf("decoding backup file %s: %v", name, err)
				}
				if strings.EqualFold(name, prefix+"Backup.xml") {
					metadata = len(decoded) > 0
				}
				if strings.HasSuffix(strings.ToLower(name), "/machine/registry.pol") {
					registry = len(decoded) >= 8 && string(decoded[:4]) == "PReg"
				}
			}
			if !metadata || !registry {
				t.Fatal("backup must contain GPMC metadata and the Defender machine Registry.pol")
			}
			t.Logf("exported %d files under %s", len(result.Files), executable)
		})
	}
}

func TestBackupGPOExportScript(t *testing.T) {
	stub := `$ErrorActionPreference = 'Stop'
$script:fixtureRoot = $null
$payload = @{ name = 'Fixture'; domain_controller = 'fixture-dc' }
function Get-ServerParams { @{ Server = $payload.domain_controller } }
function Test-IsGPONotFound($ErrorRecord) { $ErrorRecord.Exception.Message -match 'could not be found' }
function Get-GPO {
    [CmdletBinding()]
    param([string]$Name, [string]$Server)
    if ($Name -ne 'Fixture' -or $Server -ne 'fixture-dc') { throw 'incorrect GPO lookup arguments' }
    if ($env:ADLC_BACKUP_FIXTURE_MODE -eq 'missing') { throw 'GPO could not be found' }
    [pscustomobject]@{ Id = [guid]'11111111-1111-1111-1111-111111111111' }
}
function Backup-GPO {
    [CmdletBinding()]
    param([guid]$Guid, [string]$Path, [string]$Server)
    $script:fixtureRoot = $Path
    if ($Guid -ne [guid]'11111111-1111-1111-1111-111111111111' -or $Server -ne 'fixture-dc') { throw 'incorrect backup arguments' }
    $snapshot = Join-Path $Path '{22222222-2222-2222-2222-222222222222}'
    New-Item -ItemType Directory -Path $snapshot | Out-Null
	[System.IO.File]::WriteAllText((Join-Path $Path 'manifest.xml'), '<Manifest/>')
    [System.IO.File]::WriteAllText((Join-Path $snapshot 'Backup.xml'), '<Backup/>')
    [System.IO.File]::WriteAllBytes((Join-Path $snapshot 'Registry.pol'), [byte[]](0, 128, 255))
    if ($env:ADLC_BACKUP_FIXTURE_MODE -eq 'failure') { throw 'fixture backup failed' }
    [pscustomobject]@{ Id = [guid]'22222222-2222-2222-2222-222222222222' }
}
try {
    $response = & {
`
	footer := `
    }
    $result = $response | ConvertFrom-Json
    $failure = $null
}
catch {
    $result = $null
    $failure = $_.Exception.Message
}
$cleanedUp = ($null -eq $script:fixtureRoot) -or (-not (Test-Path -LiteralPath $script:fixtureRoot))
[pscustomobject]@{ result = $result; failure = $failure; cleaned_up = $cleanedUp } | ConvertTo-Json -Depth 6 -Compress
`
	probe := filepath.Join(t.TempDir(), "backup-export.ps1")
	if err := os.WriteFile(probe, []byte(stub+script(backupGPOExportRead)+footer), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, executable := range []string{"powershell.exe", "pwsh"} {
		t.Run(executable, func(t *testing.T) {
			binary, err := exec.LookPath(executable)
			if err != nil {
				t.Skipf("%s not installed", executable)
			}
			for _, mode := range []string{"success", "missing", "failure"} {
				t.Run(mode, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					command := exec.CommandContext(ctx, binary, "-NoProfile", "-NonInteractive", "-File", probe)
					command.Env = append(os.Environ(), "ADLC_BACKUP_FIXTURE_MODE="+mode)
					output, err := command.Output()
					if err != nil {
						if failure, ok := err.(*exec.ExitError); ok {
							t.Fatalf("running backup fixture: %v; stderr: %s", err, failure.Stderr)
						}
						t.Fatalf("running backup fixture: %v", err)
					}
					var fixture struct {
						Result    BackupGPOExport `json:"result"`
						Failure   *string         `json:"failure"`
						CleanedUp bool            `json:"cleaned_up"`
					}
					if err := json.Unmarshal(output, &fixture); err != nil {
						t.Fatalf("decoding fixture: %v", err)
					}
					if !fixture.CleanedUp {
						t.Fatal("temporary backup directory was not removed")
					}
					if mode == "failure" {
						if fixture.Failure == nil || *fixture.Failure != "fixture backup failed" {
							t.Fatal("backup failure was not preserved")
						}
						return
					}
					if fixture.Failure != nil {
						t.Fatalf("unexpected fixture failure: %s", *fixture.Failure)
					}
					if mode == "missing" {
						if fixture.Result.Exists || len(fixture.Result.Files) != 0 {
							t.Fatal("missing GPO must return exists=false and no files")
						}
						return
					}
					if !fixture.Result.Exists || fixture.Result.GUID != "11111111-1111-1111-1111-111111111111" || fixture.Result.BackupID != "22222222-2222-2222-2222-222222222222" {
						t.Fatal("incorrect export identity")
					}
					prefix := "{22222222-2222-2222-2222-222222222222}/"
					for name, expected := range map[string]string{"Backup.xml": "<Backup/>", "Registry.pol": string([]byte{0, 128, 255})} {
						decoded, err := base64.StdEncoding.DecodeString(fixture.Result.Files[prefix+name])
						if err != nil || string(decoded) != expected {
							t.Fatalf("file %s did not round-trip through base64", name)
						}
					}
					manifest, err := base64.StdEncoding.DecodeString(fixture.Result.Files["manifest.xml"])
					if err != nil || string(manifest) != "<Manifest/>" {
						t.Fatal("root backup manifest was not preserved")
					}
					if len(fixture.Result.Files) != 3 {
						t.Fatal("incorrect number of backup files")
					}
				})
			}
		})
	}
}
