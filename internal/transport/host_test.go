package transport

import (
	"bufio"
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	cases := [][]byte{
		[]byte(""),
		[]byte("hello"),
		[]byte("line1\nline2\r\nwith\ttabs and \"quotes\""),
		bytes.Repeat([]byte("x"), 5000),
	}

	for _, payload := range cases {
		var buf bytes.Buffer
		if err := writeFrame(&buf, payload); err != nil {
			t.Fatalf("writeFrame: %v", err)
		}

		got, err := readFrame(bufio.NewReader(&buf))
		if err != nil {
			t.Fatalf("readFrame: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("round trip mismatch: got %q want %q", got, payload)
		}
	}
}

func TestReadFrameRejectsBadHeader(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte("notanumber\n")))
	if _, err := readFrame(r); err == nil {
		t.Fatal("expected an error for a non-numeric frame header")
	}
}

func TestReadFrameRejectsTruncatedPayload(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte("10\nshort")))
	if _, err := readFrame(r); err == nil {
		t.Fatal("expected an error when the payload is shorter than the header length")
	}
}
