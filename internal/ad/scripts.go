package ad

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

//go:embed scripts/*.ps1
var scripts embed.FS

const commonScript = "common.ps1"

// moduleFreeOperations lists operation sources (the body script name without .ps1) whose scripts
// use only the LDAP helpers in common.ps1 and therefore do not need the ActiveDirectory module.
// buildScript omits the ~600 ms module import for these; migrating an operation to LDAP means
// adding it here.
var moduleFreeOperations = map[string]bool{
	"domain_read":              true,
	"organizational_unit_read": true,
	"group_read":               true,
	"user_read":                true,
	"computer_read":            true,
	"backup_gpo_read":          true,
	"gpo_link_read":            true,
	"gpo_link_ensure":          true,
	"gpo_link_delete":          true,
	"gpo_single_link_read":     true,
	"gpo_single_link_ensure":   true,
	"gpo_single_link_delete":   true,
}

func script(name string) string {
	content, err := scripts.ReadFile("scripts/" + name)
	if err != nil {
		panic(fmt.Sprintf("embedded script %q is missing: %v", name, err))
	}

	return string(content)
}

// buildScript concatenates the helper scripts, injects the inputs as a base64 JSON
// $payload and appends the operation body, which must be the last name given.
// Inputs are never interpolated into script text.
func buildScript(c *client.Client, payload map[string]any, names ...string) (string, error) {
	if len(names) < 2 {
		return "", fmt.Errorf("buildScript needs at least one helper script and a body")
	}

	payload["domain_controller"] = c.Config().DomainController

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encoding script payload: %w", err)
	}

	body := names[len(names)-1]
	source := strings.TrimSuffix(body, ".ps1")

	var builder strings.Builder
	// The ActiveDirectory module import costs ~600 ms per process, and the provider runs every
	// operation in a fresh remote PowerShell process (the shell/connection is reused, the process
	// is not), so it is paid again on every read and write. Operations migrated to the LDAP
	// helpers in common.ps1 skip it; every other operation still imports it.
	if !moduleFreeOperations[source] {
		// ADPS_LoadDefaultDrive=0 skips creating the AD: PSDrive at import time, which otherwise
		// makes an ADWS round trip on every import. The scripts only use -Server cmdlets, never
		// the drive, so this trims the per-operation module load with no behavioural change.
		builder.WriteString("$env:ADPS_LoadDefaultDrive = 0\nImport-Module ActiveDirectory -ErrorAction Stop\n")
	}
	for _, name := range names[:len(names)-1] {
		builder.WriteString(script(name))
		builder.WriteString("\n")
	}

	fmt.Fprintf(&builder, "\n$payloadJson = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('%s'))\n$payload = $payloadJson | ConvertFrom-Json -ErrorAction Stop\n", base64.StdEncoding.EncodeToString(jsonPayload))

	// Every operation runs inside a try/catch keyed to an event-log source named after the
	// operation script (without the .ps1 extension). Failures are logged to the
	// 'terraform-provider-adds' event log on the target host and re-thrown. Every operation
	// also logs on success: mutating operations as a change (event 1000), read-only
	// operations as a read (event 1002). Helpers live in common.ps1.
	fmt.Fprintf(&builder, "$scriptSource = '%s'\ntry {\n", source)
	builder.WriteString(script(body))
	if isMutatingScript(source) {
		fmt.Fprintf(&builder, "\nWrite-ADDSChange -Source $scriptSource -Message \"operation '%s' completed successfully\"\n", source)
	} else {
		fmt.Fprintf(&builder, "\nWrite-ADDSRead -Source $scriptSource -Message \"operation '%s' completed successfully\"\n", source)
	}
	builder.WriteString("}\ncatch {\n    Write-ADDSFailure -Source $scriptSource -ErrorRecord $_\n    throw\n}\n")

	return builder.String(), nil
}

// isMutatingScript reports whether an operation script changes AD, based on its name
// suffix. Mutating operations log a change entry on success; read-only operations log a
// read entry.
func isMutatingScript(source string) bool {
	for _, suffix := range []string{"_ensure", "_update", "_delete", "_set"} {
		if strings.HasSuffix(source, suffix) {
			return true
		}
	}

	return false
}
