package powershell

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

// remoteTempDir is the directory on the target host where a script is staged before it runs.
// It exists and is writable for an administrative account on every Windows host.
const remoteTempDir = `C:\Windows\Temp`

// EmptyStdinExitCode is the exit code the write bootstrap uses when no script arrives on stdin
// (a truncated or undelivered transport Send). It lets the transport distinguish an empty
// payload from a genuine failure and retry it. EmptyStdinMarker is written to stderr alongside
// it. Both must stay in sync with the literals in writeBootstrap.
const (
	EmptyStdinExitCode = 97
	EmptyStdinMarker   = "ADDS_EMPTY_STDIN"
)

// NewScriptToken returns a random hex token used to name a staged script file so that repeated
// or concurrent operations never collide on one path.
func NewScriptToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating script token: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

// RemoteScriptPath returns the absolute path of the staged script file for a token.
func RemoteScriptPath(token string) string {
	return remoteTempDir + `\adds-` + token + `.ps1`
}

// BuildWriteCommand returns the command line that stages a script on the target host. The
// bootstrap reads the script as plain text from stdin and writes it verbatim to remotePath; it
// performs no decoding, decompression or execution, so it does not resemble the in-memory
// decompress-and-Invoke-Expression pattern that behavioural antivirus blocks as a stager.
// Keeping the script body on stdin still avoids the 8191 character cmd.exe command-line limit.
func BuildWriteCommand(powerShellPath, remotePath string) (string, error) {
	if strings.TrimSpace(powerShellPath) == "" {
		return "", fmt.Errorf("powershell_path must not be empty")
	}

	return fmt.Sprintf(`"%s" -NoLogo -NoProfile -NonInteractive -EncodedCommand %s`, powerShellPath, encodeUTF16LEBase64(writeBootstrap(remotePath))), nil
}

// BuildFileCommand returns the command line that runs a staged script with -File. Executing a
// named script file is indistinguishable from an administrator running a script, which keeps it
// clear of the stager heuristics the old in-memory stdin bootstrap tripped. The path is left
// unquoted so the line carries only the two quotes around the executable: cmd.exe (the WinRM/SSH
// shell) strips the outer quote pair when a line has four or more quotes, which would mangle the
// executable. RemoteScriptPath never contains spaces, so an unquoted path is a single argument.
func BuildFileCommand(powerShellPath, remotePath string) (string, error) {
	if strings.TrimSpace(powerShellPath) == "" {
		return "", fmt.Errorf("powershell_path must not be empty")
	}

	return fmt.Sprintf(`"%s" -NoLogo -NoProfile -NonInteractive -File %s`, powerShellPath, remotePath), nil
}

// BuildDeleteCommand returns the command line that removes a staged script. It is encoded like
// the write command so the line carries only the two quotes around the executable (see
// BuildFileCommand). Cleanup is best effort: a leftover file in the machine temp directory is
// harmless.
func BuildDeleteCommand(powerShellPath, remotePath string) (string, error) {
	if strings.TrimSpace(powerShellPath) == "" {
		return "", fmt.Errorf("powershell_path must not be empty")
	}

	return fmt.Sprintf(`"%s" -NoLogo -NoProfile -NonInteractive -EncodedCommand %s`, powerShellPath, encodeUTF16LEBase64(deleteBootstrap(remotePath))), nil
}

// deleteBootstrap removes the staged script. It performs a single Remove-Item and nothing else.
func deleteBootstrap(remotePath string) string {
	return `Remove-Item -LiteralPath '` + remotePath + `' -Force -ErrorAction SilentlyContinue`
}

// writeBootstrap is the small, fixed script the write command runs. It reads the script text
// from stdin and writes it to remotePath as UTF-8 with a BOM, which both Windows PowerShell 5.1
// and PowerShell 7 read back correctly under -File. An empty stdin (a truncated or undelivered
// transport Send) is reported with the sentinel exit code and marker so the transport retries it.
func writeBootstrap(remotePath string) string {
	return `$ErrorActionPreference='Stop';` +
		`if($null -ne $PSStyle){$PSStyle.OutputRendering='PlainText'};` +
		`$script=[Console]::In.ReadToEnd();` +
		`if([string]::IsNullOrWhiteSpace($script)){[Console]::Error.Write('` + EmptyStdinMarker + `');exit ` + strconv.Itoa(EmptyStdinExitCode) + `};` +
		`[System.IO.File]::WriteAllText('` + remotePath + `',$script,(New-Object System.Text.UTF8Encoding($true)))`
}

func encodeUTF16LEBase64(input string) string {
	utf16Data := utf16.Encode([]rune(input))
	bytes := make([]byte, 0, len(utf16Data)*2)

	for _, codeUnit := range utf16Data {
		bytes = append(bytes, byte(codeUnit), byte(codeUnit>>8))
	}

	return base64.StdEncoding.EncodeToString(bytes)
}

type clixmlMessage struct {
	Strings []clixmlString `xml:"S"`
}

type clixmlString string

func (s *clixmlString) UnmarshalText(text []byte) error {
	value := strings.TrimSpace(string(text))
	if strings.HasPrefix(value, "+ ") {
		value = "\n" + strings.TrimPrefix(value, "+ ")
	}
	*s = clixmlString(value)
	return nil
}

func DecodeCLIXML(input string) string {
	if !strings.Contains(input, "#< CLIXML") {
		return input
	}

	xmlInput := strings.ReplaceAll(input, "#< CLIXML", "")
	var message clixmlMessage

	if err := xml.Unmarshal([]byte(xmlInput), &message); err != nil {
		return input
	}

	parts := make([]string, 0, len(message.Strings))
	for _, item := range message.Strings {
		parts = append(parts, string(item))
	}

	joined := strings.Join(parts, "")
	return cleanConsoleText(joined)
}

// clixmlEscape matches CLIXML character-reference escapes such as _x001B_ (ESC) and _x000A_
// (LF), which PowerShell emits for control characters when serializing the error stream.
var clixmlEscape = regexp.MustCompile(`_x([0-9A-Fa-f]{4})_`)

// ansiEscape matches ANSI/VT control sequences (colour codes, cursor moves) that PowerShell 7
// bakes into error records for terminal rendering.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// cleanConsoleText turns CLIXML/ANSI-laden remote output into plain, readable text: it decodes
// the _xNNNN_ character escapes, then strips the ANSI colour sequences that decoding reveals.
func cleanConsoleText(input string) string {
	decoded := clixmlEscape.ReplaceAllStringFunc(input, func(match string) string {
		code, err := strconv.ParseInt(match[2:6], 16, 32)
		if err != nil {
			return match
		}
		if code == '\r' {
			return ""
		}
		return string(rune(code))
	})

	stripped := ansiEscape.ReplaceAllString(decoded, "")
	return strings.TrimSpace(stripped)
}
