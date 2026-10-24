package powershell

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// hostLoopScript is the long-running warm host. It imports no module itself; instead it runs each
// operation's script in the same process, so the first operation pays the ActiveDirectory module
// import (~600 ms) and every later operation reuses the already-loaded module. Each request is a
// plain-text script body read from stdin and staged to a real .ps1 run with the call operator -
// the same AV-safe disk-staging the one-shot bootstrap uses, never an in-memory Invoke-Expression.
//
// Framing matches internal/transport: a request is a decimal length line plus that many body
// bytes; a response is a decimal length line plus that many JSON bytes. The JSON envelope carries
// the operation's exit code and base64-encoded stdout and stderr, so the frame stays clean ASCII.
const hostLoopScript = `$ErrorActionPreference='Stop'
if($null -ne $PSStyle){$PSStyle.OutputRendering='PlainText'}
$stdin=[Console]::OpenStandardInput()
$stdout=[Console]::OpenStandardOutput()
$enc=New-Object System.Text.UTF8Encoding($false)
$bom=New-Object System.Text.UTF8Encoding($true)
$nl=[char]10
function Read-HostFrame {
    $h=New-Object System.Collections.Generic.List[byte]
    while($true){
        $b=$stdin.ReadByte()
        if($b -lt 0){return $null}
        if($b -eq 10){break}
        [void]$h.Add([byte]$b)
    }
    $len=[int][System.Text.Encoding]::ASCII.GetString($h.ToArray())
    if($len -lt 0){return $null}
    $buf=New-Object byte[] $len
    $read=0
    while($read -lt $len){
        $n=$stdin.Read($buf,$read,$len-$read)
        if($n -le 0){return $null}
        $read+=$n
    }
    return ,$buf
}
function Write-HostFrame([byte[]]$payload){
    $header=[System.Text.Encoding]::ASCII.GetBytes([string]$payload.Length + $nl)
    $stdout.Write($header,0,$header.Length)
    $stdout.Write($payload,0,$payload.Length)
    $stdout.Flush()
}
while($true){
    $req=Read-HostFrame
    if($null -eq $req){break}
    $script=$enc.GetString($req)
    $tok=[guid]::NewGuid().ToString('N')
    $p='C:\Windows\Temp\adds-'+$tok+'.ps1'
    $outText=''
    $errText=''
    $code=0
    try{
        [System.IO.File]::WriteAllText($p,$script,$bom)
        $outText=(& $p | Out-String -Width 2147483647)
    }catch{
        $code=1
        $errText=($_ | Out-String)
    }finally{
        Remove-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue
    }
    $resp=[ordered]@{exit=$code;out=[Convert]::ToBase64String($enc.GetBytes([string]$outText));err=[Convert]::ToBase64String($enc.GetBytes([string]$errText))}
    $json=$resp|ConvertTo-Json -Compress
    Write-HostFrame($enc.GetBytes($json))
}`

// BuildHostCommand returns the command line that starts the warm host. It is encoded like the
// one-shot run command so the line carries only the two quotes around the executable.
func BuildHostCommand(powerShellPath string) (string, error) {
	if strings.TrimSpace(powerShellPath) == "" {
		return "", fmt.Errorf("powershell_path must not be empty")
	}

	return fmt.Sprintf(`"%s" -NoLogo -NoProfile -NonInteractive -EncodedCommand %s`, powerShellPath, encodeUTF16LEBase64(hostLoopScript)), nil
}

type hostEnvelope struct {
	Exit int    `json:"exit"`
	Out  string `json:"out"`
	Err  string `json:"err"`
}

// DecodeHostResponse parses a host response payload into the operation's stdout, stderr and exit
// code. The host base64-encodes stdout and stderr so the envelope stays clean ASCII JSON.
func DecodeHostResponse(payload []byte) (stdout string, stderr string, exitCode int, err error) {
	var env hostEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return "", "", 0, fmt.Errorf("decoding host response: %w; payload: %s", err, string(payload))
	}

	out, err := base64.StdEncoding.DecodeString(env.Out)
	if err != nil {
		return "", "", 0, fmt.Errorf("decoding host stdout: %w", err)
	}

	errText, err := base64.StdEncoding.DecodeString(env.Err)
	if err != nil {
		return "", "", 0, fmt.Errorf("decoding host stderr: %w", err)
	}

	return string(out), string(errText), env.Exit, nil
}
