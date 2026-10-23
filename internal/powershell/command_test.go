package powershell

import (
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
)

// decodeEncodedCommand extracts and UTF-16LE-decodes the -EncodedCommand argument from a built
// command line, returning the bootstrap script it carries.
func decodeEncodedCommand(t *testing.T, command string) string {
	t.Helper()

	fields := strings.Fields(command)
	raw, err := base64.StdEncoding.DecodeString(fields[len(fields)-1])
	if err != nil {
		t.Fatalf("decoding command: %v", err)
	}

	codeUnits := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		codeUnits = append(codeUnits, uint16(raw[i])|uint16(raw[i+1])<<8)
	}

	return string(utf16.Decode(codeUnits))
}

func TestBuildRunCommandIsSmall(t *testing.T) {
	token, err := NewScriptToken()
	if err != nil {
		t.Fatalf("NewScriptToken: %v", err)
	}

	command, err := BuildRunCommand("pwsh", RemoteScriptPath(token))
	if err != nil {
		t.Fatalf("BuildRunCommand: %v", err)
	}

	// cmd.exe caps the command line at 8191 characters; the script body travels on stdin.
	if len(command) > 2000 {
		t.Fatalf("run command line grew to %d characters", len(command))
	}
}

func TestBuildRunCommandRunsStagedFile(t *testing.T) {
	token, err := NewScriptToken()
	if err != nil {
		t.Fatalf("NewScriptToken: %v", err)
	}

	path := RemoteScriptPath(token)
	command, err := BuildRunCommand("pwsh", path)
	if err != nil {
		t.Fatalf("BuildRunCommand: %v", err)
	}

	bootstrap := decodeEncodedCommand(t, command)
	if !strings.Contains(bootstrap, "& $p") {
		t.Fatalf("run bootstrap does not invoke the staged file: %s", bootstrap)
	}
	if !strings.Contains(bootstrap, path) {
		t.Fatalf("run bootstrap does not reference the staged path: %s", bootstrap)
	}
	if len(command) > 2000 {
		t.Fatalf("run command line grew to %d characters", len(command))
	}
}

func TestBuildRunCommandRejectsEmptyPath(t *testing.T) {
	path := RemoteScriptPath("deadbeef")

	if _, err := BuildRunCommand("  ", path); err == nil {
		t.Fatal("BuildRunCommand: expected an error for an empty powershell path")
	}
}

func TestRemoteScriptPathIsUnderTemp(t *testing.T) {
	path := RemoteScriptPath("abc123")
	want := `C:\Windows\Temp\adds-abc123.ps1`
	if path != want {
		t.Fatalf("RemoteScriptPath = %q, want %q", path, want)
	}
}

func TestNewScriptTokenIsUnique(t *testing.T) {
	first, err := NewScriptToken()
	if err != nil {
		t.Fatalf("NewScriptToken: %v", err)
	}
	second, err := NewScriptToken()
	if err != nil {
		t.Fatalf("NewScriptToken: %v", err)
	}
	if first == "" || first == second {
		t.Fatalf("expected two distinct non-empty tokens, got %q and %q", first, second)
	}
}

// TestRunBootstrapStagesRunsAndRemovesScript confirms the single run command reads stdin, writes
// it to the staged path, runs it from disk and removes it - without any of the stager primitives
// that behavioural antivirus blocks.
func TestRunBootstrapStagesRunsAndRemovesScript(t *testing.T) {
	path := RemoteScriptPath("abc123")
	command, err := BuildRunCommand("pwsh", path)
	if err != nil {
		t.Fatalf("BuildRunCommand: %v", err)
	}

	bootstrap := decodeEncodedCommand(t, command)

	if !strings.Contains(bootstrap, "[Console]::In.ReadToEnd()") {
		t.Fatalf("run bootstrap does not read stdin: %s", bootstrap)
	}
	if !strings.Contains(bootstrap, "WriteAllText") || !strings.Contains(bootstrap, path) {
		t.Fatalf("run bootstrap does not write the staged file: %s", bootstrap)
	}
	if !strings.Contains(bootstrap, "& $p") {
		t.Fatalf("run bootstrap does not run the staged file: %s", bootstrap)
	}
	if !strings.Contains(bootstrap, "Remove-Item") || !strings.Contains(bootstrap, "finally") {
		t.Fatalf("run bootstrap does not remove the staged file in a finally block: %s", bootstrap)
	}
	for _, forbidden := range []string{"Invoke-Expression", "FromBase64String", "GZipStream"} {
		if strings.Contains(bootstrap, forbidden) {
			t.Fatalf("run bootstrap still contains stager primitive %q: %s", forbidden, bootstrap)
		}
	}
}

// TestRunBootstrapEmptyStdinGuard checks the run bootstrap short-circuits with the exported
// sentinel when stdin is empty, so the transport can detect and retry a truncated payload.
func TestRunBootstrapEmptyStdinGuard(t *testing.T) {
	command, err := BuildRunCommand("pwsh", RemoteScriptPath("abc123"))
	if err != nil {
		t.Fatalf("BuildRunCommand: %v", err)
	}
	bootstrap := decodeEncodedCommand(t, command)

	if !strings.Contains(bootstrap, EmptyStdinMarker) {
		t.Fatalf("run bootstrap does not emit EmptyStdinMarker %q", EmptyStdinMarker)
	}
	if !strings.Contains(bootstrap, "exit "+strconv.Itoa(EmptyStdinExitCode)) {
		t.Fatalf("run bootstrap does not exit with EmptyStdinExitCode %d", EmptyStdinExitCode)
	}
}

func TestDecodeCLIXMLStripsEscapesAndAnsi(t *testing.T) {
	input := `#< CLIXML
<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><S S="Error">_x001B_[31;1mGet-OptionalString: _x001B_[0mThe term 'Get-OptionalString' is not recognized._x000D__x000A_</S></Objs>`

	got := DecodeCLIXML(input)
	want := "Get-OptionalString: The term 'Get-OptionalString' is not recognized."

	if got != want {
		t.Fatalf("DecodeCLIXML returned %q, want %q", got, want)
	}
}

func TestDecodeCLIXMLPassesThroughPlainText(t *testing.T) {
	input := "just a plain error message"
	if got := DecodeCLIXML(input); got != input {
		t.Fatalf("DecodeCLIXML altered plain text: %q", got)
	}
}
