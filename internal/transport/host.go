package transport

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// HostSession is a persistent remote PowerShell host process that runs framed script requests one
// at a time and returns framed responses, keeping the process (and its loaded modules) warm
// between operations. Exchange is never called concurrently: the Client serializes all operations.
type HostSession interface {
	// Exchange writes one request payload to the host and returns the single response payload.
	Exchange(ctx context.Context, request []byte) ([]byte, error)
	// Close terminates the host process and frees its transport resources.
	Close() error
}

// HostRunner is implemented by transports that can start a warm host. The Client type-asserts the
// runner to this interface; transports that do not implement it fall back to per-operation runs.
type HostRunner interface {
	StartHost(ctx context.Context, command string) (HostSession, error)
}

// writeFrame writes a length-prefixed frame: an ASCII decimal byte count, a newline, then the
// payload bytes. The host loop reads the same framing. Length framing (rather than a delimiter)
// lets the plain-text script body carry arbitrary bytes, including newlines, without escaping.
func writeFrame(w io.Writer, payload []byte) error {
	if _, err := io.WriteString(w, strconv.Itoa(len(payload))+"\n"); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return nil
}

// readFrame reads a frame written by writeFrame (or the host loop): a decimal length line followed
// by exactly that many payload bytes.
func readFrame(r *bufio.Reader) ([]byte, error) {
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}

	n, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil {
		return nil, fmt.Errorf("invalid host frame header %q: %w", strings.TrimSpace(header), err)
	}
	if n < 0 {
		return nil, fmt.Errorf("negative host frame length %d", n)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
