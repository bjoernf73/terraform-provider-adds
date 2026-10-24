package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
	"github.com/bjoernf73/terraform-provider-adds/internal/transport"
)

func hostEnvelopePayload(t *testing.T, out, errText string, exit int) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"exit": exit,
		"out":  base64.StdEncoding.EncodeToString([]byte(out)),
		"err":  base64.StdEncoding.EncodeToString([]byte(errText)),
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return payload
}

type fakeHostRunner struct {
	runCalls      int
	exchangeCalls int
	runResult     transport.Result
	exchange      func(script string) ([]byte, error)
}

func (f *fakeHostRunner) Run(_ context.Context, _, _ string) (transport.Result, error) {
	f.runCalls++
	return f.runResult, nil
}

func (f *fakeHostRunner) StartHost(_ context.Context, _ string) (transport.HostSession, error) {
	return &fakeHostSession{owner: f}, nil
}

type fakeHostSession struct {
	owner *fakeHostRunner
}

func (s *fakeHostSession) Exchange(_ context.Context, request []byte) ([]byte, error) {
	s.owner.exchangeCalls++
	return s.owner.exchange(string(request))
}

func (s *fakeHostSession) Close() error { return nil }

// processOnlyRunner implements transport.Runner but not transport.HostRunner.
type processOnlyRunner struct {
	calls  int
	result transport.Result
}

func (r *processOnlyRunner) Run(_ context.Context, _, _ string) (transport.Result, error) {
	r.calls++
	return r.result, nil
}

func TestRunPowerShellUsesWarmHost(t *testing.T) {
	fake := &fakeHostRunner{
		exchange: func(_ string) ([]byte, error) {
			return hostEnvelopePayload(t, `{"ok":true}`, "", 0), nil
		},
	}
	remote := &Client{config: config.Config{PowerShellPath: "pwsh"}, runner: fake}

	result, err := remote.RunPowerShell(context.Background(), "Get-ADUser -Identity x")
	if err != nil {
		t.Fatalf("RunPowerShell: %v", err)
	}
	if result.Stdout != `{"ok":true}` {
		t.Fatalf("stdout = %q, want %q", result.Stdout, `{"ok":true}`)
	}
	if fake.exchangeCalls != 1 {
		t.Fatalf("expected one host exchange, got %d", fake.exchangeCalls)
	}
	if fake.runCalls != 0 {
		t.Fatalf("expected the per-operation path to be unused, got %d calls", fake.runCalls)
	}
}

func TestRunPowerShellDisablesHostOnGarbage(t *testing.T) {
	fake := &fakeHostRunner{
		exchange:  func(_ string) ([]byte, error) { return []byte("garbage"), nil },
		runResult: transport.Result{Stdout: `{"fallback":true}`},
	}
	remote := &Client{config: config.Config{PowerShellPath: "pwsh"}, runner: fake}

	result, err := remote.RunPowerShell(context.Background(), "Get-ADUser -Identity x")
	if err != nil {
		t.Fatalf("RunPowerShell: %v", err)
	}
	if result.Stdout != `{"fallback":true}` {
		t.Fatalf("stdout = %q, want fallback", result.Stdout)
	}
	if fake.exchangeCalls != 1 {
		t.Fatalf("expected one host exchange before disabling, got %d", fake.exchangeCalls)
	}
	if fake.runCalls != 1 {
		t.Fatalf("expected one fallback run, got %d", fake.runCalls)
	}
	if !remote.hostDisabled {
		t.Fatal("expected the host to be disabled after a malformed response")
	}

	// A second operation must skip the host entirely and use the per-operation path.
	if _, err := remote.RunPowerShell(context.Background(), "Get-ADGroup -Identity y"); err != nil {
		t.Fatalf("second RunPowerShell: %v", err)
	}
	if fake.exchangeCalls != 1 {
		t.Fatalf("expected no further host exchanges, got %d", fake.exchangeCalls)
	}
	if fake.runCalls != 2 {
		t.Fatalf("expected a second fallback run, got %d", fake.runCalls)
	}
}

func TestRunPowerShellFallsBackWithoutHostRunner(t *testing.T) {
	runner := &processOnlyRunner{result: transport.Result{Stdout: `{"process":true}`}}
	remote := &Client{config: config.Config{PowerShellPath: "pwsh"}, runner: runner}

	result, err := remote.RunPowerShell(context.Background(), "Get-ADUser -Identity x")
	if err != nil {
		t.Fatalf("RunPowerShell: %v", err)
	}
	if result.Stdout != `{"process":true}` {
		t.Fatalf("stdout = %q, want process path", result.Stdout)
	}
	if runner.calls != 1 {
		t.Fatalf("expected one per-operation run, got %d", runner.calls)
	}
}
