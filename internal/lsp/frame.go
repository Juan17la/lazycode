// Package lsp is a small language server client: Content-Length framed JSON-RPC over a child's
// stdio, with the few requests LazyCode needs (FR-4.5). Everything a server sends is untrusted,
// so sizes are capped and bad input ends the conversation rather than crashing the editor.
package lsp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	// maxMessage caps one message. A rename in a big project can be megabytes, but not this.
	maxMessage = 64 << 20
	// maxHeaderLine caps a header line, so that a server writing no newline can't fill memory.
	maxHeaderLine = 4096
)

// errBadFrame means the bytes aren't a valid frame. The server is treated as broken.
var errBadFrame = errors.New("bad message framing")

// readFrame reads one message body.
func readFrame(r *bufio.Reader) ([]byte, error) {
	length := -1
	for first := true; ; first = false {
		line, err := readLine(r)
		if err != nil {
			if first && errors.Is(err, io.EOF) {
				return nil, io.EOF // A clean end between messages.
			}
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("%w: header %q", errBadFrame, truncate(line))
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%w: length %q", errBadFrame, truncate(value))
			}
			length = n
		}
	}
	switch {
	case length < 0:
		return nil, fmt.Errorf("%w: no Content-Length", errBadFrame)
	case length > maxMessage:
		return nil, fmt.Errorf("%w: %d bytes is too large", errBadFrame, length)
	}
	// ReadAll grows as bytes arrive, so a short body can't make a big allocation up front.
	body, err := io.ReadAll(io.LimitReader(r, int64(length)))
	if err != nil {
		return nil, err
	}
	if len(body) != length {
		return nil, io.ErrUnexpectedEOF
	}
	return body, nil
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// readLine reads up to \r\n, or \n.
func readLine(r *bufio.Reader) (string, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > maxHeaderLine {
			return "", fmt.Errorf("%w: header line too long", errBadFrame)
		}
		switch {
		case err == nil:
			return string(bytes.TrimRight(line, "\r\n")), nil
		case errors.Is(err, bufio.ErrBufferFull):
		case len(line) > 0 && errors.Is(err, io.EOF):
			return "", io.ErrUnexpectedEOF
		default:
			return "", err
		}
	}
}

func writeFrame(w io.Writer, body []byte) error {
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}
