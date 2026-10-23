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

// EmptyStdinExitCode is the exit code the run bootstrap uses when no script arrives on stdin
// (a truncated or undelivered transport Send). It lets the transport distinguish an empty
// payload from a genuine failure and retry it. EmptyStdinMarker is written to stderr alongside
// it. Both must stay in sync with the literals in runBootstrap.
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

// BuildRunCommand returns the single command line that stages, runs and removes a script in one
// remote process. The bootstrap reads the script as plain text from stdin, writes it verbatim to
// remotePath, runs it from disk with the call operator, and deletes it in a finally block.
// Collapsing the former write/run/delete trio into one process cuts the per-operation connection
// and server-process count (sshd.exe, winrshost.exe, conhost.exe) and latency roughly threefold.
// It stays clear of the stager primitives behavioural antivirus blocks: plain-text stdin, a real
// .ps1 run from disk by path, and no decoding, decompression or Invoke-Expression. Keeping the
// script body on stdin still avoids the 8191 character cmd.exe command-line limit.
func BuildRunCommand(powerShellPath, remotePath string) (string, error) {
	if strings.TrimSpace(powerShellPath) == "" {
		return "", fmt.Errorf("powershell_path must not be empty")
	}

	return fmt.Sprintf(`"%s" -NoLogo -NoProfile -NonInteractive -EncodedCommand %s`, powerShellPath, encodeUTF16LEBase64(runBootstrap(remotePath))), nil
}

// runBootstrap is the fixed script the run command executes. It reads the operation script from
// stdin, writes it to remotePath as UTF-8 with a BOM (read back correctly by both Windows
// PowerShell 5.1 and PowerShell 7), runs it from disk with the call operator, and always removes
// it afterwards in a finally block. An empty stdin (a truncated or undelivered transport Send) is
// reported with the sentinel exit code and marker so the transport retries it. A terminating
// error from the script propagates out of the try/finally so the process exits non-zero with the
// error on stderr, exactly as running it with -File did.
func runBootstrap(remotePath string) string {
	return `$ErrorActionPreference='Stop';` +
		`if($null -ne $PSStyle){$PSStyle.OutputRendering='PlainText'};` +
		`$script=[Console]::In.ReadToEnd();` +
		`if([string]::IsNullOrWhiteSpace($script)){[Console]::Error.Write('` + EmptyStdinMarker + `');exit ` + strconv.Itoa(EmptyStdinExitCode) + `};` +
		`$p='` + remotePath + `';` +
		`[System.IO.File]::WriteAllText($p,$script,(New-Object System.Text.UTF8Encoding($true)));` +
		`try{& $p}finally{Remove-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue}`
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
