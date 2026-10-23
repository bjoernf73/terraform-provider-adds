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

func TestBuildWriteCommandIsSmall(t *testing.T) {
	token, err := NewScriptToken()
	if err != nil {
		t.Fatalf("NewScriptToken: %v", err)
	}

	command, err := BuildWriteCommand("pwsh", RemoteScriptPath(token))
	if err != nil {
		t.Fatalf("BuildWriteCommand: %v", err)
	}

	// cmd.exe caps the command line at 8191 characters; the script body travels on stdin.
	if len(command) > 2000 {
		t.Fatalf("write command line grew to %d characters", len(command))
	}
}

func TestBuildFileCommandUsesFile(t *testing.T) {
	token, err := NewScriptToken()
	if err != nil {
		t.Fatalf("NewScriptToken: %v", err)
	}

	command, err := BuildFileCommand("pwsh", RemoteScriptPath(token))
	if err != nil {
		t.Fatalf("BuildFileCommand: %v", err)
	}

	if !strings.Contains(command, "-File") {
		t.Fatalf("file command does not use -File: %s", command)
	}
	if len(command) > 2000 {
		t.Fatalf("file command line grew to %d characters", len(command))
	}
}

func TestBuildCommandsRejectEmptyPath(t *testing.T) {
	path := RemoteScriptPath("deadbeef")

	if _, err := BuildWriteCommand("  ", path); err == nil {
		t.Fatal("BuildWriteCommand: expected an error for an empty powershell path")
	}
	if _, err := BuildFileCommand("  ", path); err == nil {
		t.Fatal("BuildFileCommand: expected an error for an empty powershell path")
	}
	if _, err := BuildDeleteCommand("  ", path); err == nil {
		t.Fatal("BuildDeleteCommand: expected an error for an empty powershell path")
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

// TestWriteBootstrapStagesScript confirms the write command reads stdin and writes it to the
// staged path without decoding, decompressing or executing it - none of the stager primitives
// that behavioural antivirus blocks.
func TestWriteBootstrapStagesScript(t *testing.T) {
	path := RemoteScriptPath("abc123")
	command, err := BuildWriteCommand("pwsh", path)
	if err != nil {
		t.Fatalf("BuildWriteCommand: %v", err)
	}

	bootstrap := decodeEncodedCommand(t, command)

	if !strings.Contains(bootstrap, "[Console]::In.ReadToEnd()") {
		t.Fatalf("write bootstrap does not read stdin: %s", bootstrap)
	}
	if !strings.Contains(bootstrap, "WriteAllText") || !strings.Contains(bootstrap, path) {
		t.Fatalf("write bootstrap does not write the staged file: %s", bootstrap)
	}
	for _, forbidden := range []string{"Invoke-Expression", "FromBase64String", "GZipStream"} {
		if strings.Contains(bootstrap, forbidden) {
			t.Fatalf("write bootstrap still contains stager primitive %q: %s", forbidden, bootstrap)
		}
	}
}

// TestWriteBootstrapEmptyStdinGuard checks the write bootstrap short-circuits with the exported
// sentinel when stdin is empty, so the transport can detect and retry a truncated payload.
func TestWriteBootstrapEmptyStdinGuard(t *testing.T) {
	command, err := BuildWriteCommand("pwsh", RemoteScriptPath("abc123"))
	if err != nil {
		t.Fatalf("BuildWriteCommand: %v", err)
	}
	bootstrap := decodeEncodedCommand(t, command)

	if !strings.Contains(bootstrap, EmptyStdinMarker) {
		t.Fatalf("write bootstrap does not emit EmptyStdinMarker %q", EmptyStdinMarker)
	}
	if !strings.Contains(bootstrap, "exit "+strconv.Itoa(EmptyStdinExitCode)) {
		t.Fatalf("write bootstrap does not exit with EmptyStdinExitCode %d", EmptyStdinExitCode)
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
