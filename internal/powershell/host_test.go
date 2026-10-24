package powershell

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildHostCommandLoop(t *testing.T) {
	command, err := BuildHostCommand("pwsh")
	if err != nil {
		t.Fatalf("BuildHostCommand: %v", err)
	}

	bootstrap := decodeEncodedCommand(t, command)

	for _, want := range []string{"OpenStandardInput", "OpenStandardOutput", "WriteAllText", "& $p", "Remove-Item", "while($true)"} {
		if !strings.Contains(bootstrap, want) {
			t.Fatalf("host loop missing %q: %s", want, bootstrap)
		}
	}
	for _, forbidden := range []string{"Invoke-Expression", "FromBase64String", "GZipStream"} {
		if strings.Contains(bootstrap, forbidden) {
			t.Fatalf("host loop contains stager primitive %q: %s", forbidden, bootstrap)
		}
	}
}

func TestBuildHostCommandRejectsEmptyPath(t *testing.T) {
	if _, err := BuildHostCommand("  "); err == nil {
		t.Fatal("BuildHostCommand: expected an error for an empty powershell path")
	}
}

func TestDecodeHostResponse(t *testing.T) {
	out := "{\"exists\":true}\n"
	errText := "some error text"
	payload, err := json.Marshal(map[string]any{
		"exit": 1,
		"out":  base64.StdEncoding.EncodeToString([]byte(out)),
		"err":  base64.StdEncoding.EncodeToString([]byte(errText)),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	gotOut, gotErr, code, err := DecodeHostResponse(payload)
	if err != nil {
		t.Fatalf("DecodeHostResponse: %v", err)
	}
	if gotOut != out {
		t.Fatalf("stdout = %q, want %q", gotOut, out)
	}
	if gotErr != errText {
		t.Fatalf("stderr = %q, want %q", gotErr, errText)
	}
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestDecodeHostResponseRejectsGarbage(t *testing.T) {
	if _, _, _, err := DecodeHostResponse([]byte("not json")); err == nil {
		t.Fatal("expected an error decoding a non-JSON payload")
	}
}
