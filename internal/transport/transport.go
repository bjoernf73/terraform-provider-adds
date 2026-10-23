package transport

import (
	"context"
	"fmt"
	"strings"

	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Runner interface {
	Run(ctx context.Context, command string, stdin string) (Result, error)
}

func NewRunner(cfg config.Config) (Runner, error) {
	switch strings.ToLower(cfg.Transport) {
	case "winrm":
		return NewWinRMRunner(cfg)
	case "ssh":
		return NewSSHRunner(cfg)
	default:
		return nil, fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
}
