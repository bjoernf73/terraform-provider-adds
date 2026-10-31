package ad

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

func TestJsonGPOExportLive(t *testing.T) {
	name := os.Getenv("ADDS_GPO_EXPORT_NAME")
	if name == "" {
		t.Skip("ADDS_GPO_EXPORT_NAME not set")
	}
	base := liveADClient(t)
	type exportedMetadata struct {
		Name                    string
		ComputerSettingsEnabled *bool
		UserSettingsEnabled     *bool
	}
	var compatibility *exportedMetadata
	for _, executable := range []string{"pwsh", "powershell.exe"} {
		t.Run(executable, func(t *testing.T) {
			cfg := base.Config()
			cfg.GPOPowerShellPath = executable
			remote, err := client.New(cfg)
			if err != nil {
				t.Fatalf("creating client: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			start := time.Now()
			body := script(jsonGPOExportRead)
			for _, phase := range []struct{ anchor, label string }{
				{"$gpo = $null", "Get-GPO"},
				{"$policyGuid =", "SYSVOL path"},
				{"$policySettings =", "comments"},
				{"# Registry settings.", "registry policies"},
				{"# Audit settings", "audit settings"},
				{"# Security template", "security template"},
				{"# Logon/logoff/startup/shutdown scripts", "scripts"},
				{"# Group Policy Preferences", "preferences"},
				{"# Client-side extensions", "AD extensions"},
				{"$exported =", "export metadata"},
				{"$exportedJson =", "JSON serialization"},
			} {
				body = strings.Replace(body, phase.anchor, "[Console]::Error.WriteLine('ADDS_EXPORT_PHASE: "+phase.label+"')\n"+phase.anchor, 1)
			}
			composed, err := buildScript(remote, map[string]any{"name": name}, commonScript, backupGPOCommon, gpRegistryPolicyParser, jsonGPOCommon, jsonGPOExportRead)
			if err != nil {
				t.Fatalf("building export script: %v", err)
			}
			composed = strings.Replace(composed, script(jsonGPOExportRead), body, 1)
			for _, module := range []string{"ActiveDirectory", "GroupPolicy"} {
				statement := "Import-Module " + module + " -ErrorAction Stop"
				composed = strings.ReplaceAll(composed, statement, "[Console]::Error.WriteLine('ADDS_EXPORT_PHASE: importing "+module+"')\n"+statement+"\n[Console]::Error.WriteLine('ADDS_EXPORT_PHASE: imported "+module+"')")
			}
			var result JsonGPOExport
			err = remote.RunPowerShellJSON(ctx, composed, &result)
			if err != nil {
				t.Fatalf("export failed after %s: %v", time.Since(start), err)
			}
			if !result.Exists {
				t.Fatal("GPO not found")
			}
			var metadata exportedMetadata
			if err := json.Unmarshal([]byte(result.JSON), &metadata); err != nil {
				t.Fatalf("decoding exported policy: %v", err)
			}
			if metadata.Name != name || metadata.ComputerSettingsEnabled == nil || metadata.UserSettingsEnabled == nil {
				t.Fatal("export has missing enabled flags or an unexpected name")
			}
			t.Logf("export completed in %s; computer enabled=%t, user enabled=%t", time.Since(start), *metadata.ComputerSettingsEnabled, *metadata.UserSettingsEnabled)
			if executable == "pwsh" {
				compatibility = &metadata
			} else if compatibility != nil {
				if *metadata.ComputerSettingsEnabled != *compatibility.ComputerSettingsEnabled || *metadata.UserSettingsEnabled != *compatibility.UserSettingsEnabled {
					t.Error("native and compatibility exports disagree on enabled flags")
				}
			}
		})
	}
}

func TestJsonGPOExportTextSerialization(t *testing.T) {
	root := t.TempDir()
	startup := filepath.Join(root, "Machine", "Scripts", "Startup")
	if err := os.MkdirAll(startup, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Join(root, "Machine", "comment.cmtx"), filepath.Join(startup, "fixture.ps1")} {
		if err := os.WriteFile(name, []byte("fixture text\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stub := `$ErrorActionPreference = 'Stop'
function Get-ServerParams { @{} }
function Get-GPO {
    [pscustomobject]@{
        Id = [guid]'11111111-1111-1111-1111-111111111111'
        DisplayName = 'Fixture'
    }
}
function Get-JsonGPOSysvolPath { $env:ADDS_EXPORT_FIXTURE }
function Get-JsonGPOPolicyServerId { $null }
function Get-ADObject {
	[pscustomobject]@{ flags = [int]$env:ADDS_EXPORT_FLAGS; gPCMachineExtensionNames = $null; gPCUserExtensionNames = $null }
}
$payload = @{ name = 'Fixture' }
`
	probe := filepath.Join(t.TempDir(), "export.ps1")
	body := strings.ReplaceAll(script(jsonGPOExportRead), "Import-Module GroupPolicy -ErrorAction Stop", "")
	if err := os.WriteFile(probe, []byte(stub+body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, executable := range []string{"powershell.exe", "pwsh"} {
		t.Run(executable, func(t *testing.T) {
			binary, err := exec.LookPath(executable)
			if err != nil {
				t.Skipf("%s not installed", executable)
			}
			for flags := 0; flags < 4; flags++ {
				t.Run("flags="+strconv.Itoa(flags), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					command := exec.CommandContext(ctx, binary, "-NoProfile", "-NonInteractive", "-File", probe)
					command.Env = append(os.Environ(), "ADDS_EXPORT_FIXTURE="+root, "ADDS_EXPORT_FLAGS="+strconv.Itoa(flags))
					output, err := command.Output()
					if err != nil {
						t.Fatalf("local export fixture failed: %v (context: %v)", err, ctx.Err())
					}
					var result JsonGPOExport
					if err := json.Unmarshal(output, &result); err != nil {
						t.Fatalf("decoding export response: %v", err)
					}
					var policy struct {
						ComputerSettingsEnabled bool
						UserSettingsEnabled     bool
						PolicySettings          struct {
							MachineComments []string
							Scripts         []struct {
								ScriptFiles []struct{ Content []string }
							}
						}
					}
					if err := json.Unmarshal([]byte(result.JSON), &policy); err != nil {
						t.Fatalf("exported text must be JSON strings, not PowerShell metadata objects: %v", err)
					}
					if policy.ComputerSettingsEnabled != (flags&2 == 0) || policy.UserSettingsEnabled != (flags&1 == 0) {
						t.Fatalf("incorrect enabled flags for AD flags=%d: computer=%t, user=%t", flags, policy.ComputerSettingsEnabled, policy.UserSettingsEnabled)
					}
					settings := policy.PolicySettings
					if len(settings.MachineComments) != 1 || settings.MachineComments[0] != "fixture text" {
						t.Fatal("comment text was not preserved")
					}
					if len(settings.Scripts) != 1 || len(settings.Scripts[0].ScriptFiles) != 1 {
						t.Fatal("script file was not exported")
					}
					content := settings.Scripts[0].ScriptFiles[0].Content
					if len(content) != 1 || content[0] != "fixture text" {
						t.Fatal("script text was not preserved")
					}
				})
			}
		})
	}
}
