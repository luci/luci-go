// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package renderer exports the capability to render a LogDog log stream to an
// io.Writer.
//
//   - Text streams are rendered by emitting the logs and their newlines in
//     order.
//   - Binary streams are rendered by emitting the sequential binary data
//     verbatim.
package renderer

import (
	"bytes"
	"io"

	"github.com/luci/luci-go/common/logdog/protocol"
)

// Source returns successive LogEntry records for the Renderer to render.
type Source interface {
	// NextLogEntry returns the next successive LogEntry record to render, or an
	// error if it could not be retrieved.
	NextLogEntry() (*protocol.LogEntry, error)
}

// Renderer is a stateful instance that provides an io.Reader interface to a
// log stream.
type Renderer struct {
	// Source is the log Source to use to retrieve the logs to render.
	Source Source
	// Reproduce, if true, attempts to reproduce the original stream data.
	//
	// For text streams, this means using the original streams' encoding and
	// newline delimiters. If this is false, UTF8 and "\n" will be used.
	Reproduce bool

	// Currently-buffered data.
	buf bytes.Buffer
	// bufPos is the current position in the buffer.
	bufPos int
}

var _ io.Reader = (*Renderer)(nil)

func (r *Renderer) Read(b []byte) (int, error) {
	buffered := r.buffered()
	for len(buffered) == 0 {
		r.buf.Reset()
		r.bufPos = 0
		if err := r.bufferNext(); err != nil {
			return 0, err
		}

		buffered = r.buffered()
	}

	count := copy(b, buffered)
	r.bufPos += count
	return count, nil
}

func (r *Renderer) buffered() []byte {
	return r.buf.Bytes()[r.bufPos:]
}

func (r *Renderer) bufferNext() error {
	// Fetch and buffer the next log entry.
	le, err := r.Source.NextLogEntry()
	if err != nil {
		return err
	}

	switch {
	case le.GetText() != nil:
		for _, line := range le.GetText().Lines {
			r.buf.WriteString(line.Value)
			if !r.Reproduce {
				r.buf.WriteRune('\n')
			} else {
				r.buf.WriteString(line.Delimiter)
			}
		}

	case le.GetBinary() != nil:
		r.buf.Write(le.GetBinary().Data)

	default:
		break
	}

	return nil
}
