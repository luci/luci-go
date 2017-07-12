// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"encoding/hex"
	"fmt"
	"io"

	"github.com/luci/luci-go/logdog/api/logpb"
)

// DatagramWriter is a callback function that, given datagram bytes, writes them
// to the specified io.Writer.
//
// Returns true if the datagram was successfully rendered, false if it could not
// be.
type DatagramWriter func(io.Writer, []byte) bool

// Renderer is a stateful instance that provides an io.Reader interface to a
// log stream.
type Renderer struct {
	// Source is the log Source to use to retrieve the logs to render.
	Source Source

	// Raw, if true, attempts to reproduce the original stream data rather
	// than pretty-rendering it.
	//
	// - For text streams, this means using the original streams' encoding and
	//   newline delimiters. If this is false, UTF8 and "\n" will be used.
	// - For binary and datagram streams, this skips any associated
	//   DatagramWriters and hex translation and dumps data directly to output.
	Raw bool

	// TextPrefix, if not nil, is called prior to rendering a text line. The
	// resulting string is prepended to that text line on render.
	TextPrefix func(le *logpb.LogEntry, line *logpb.Text_Line) string

	// DatagramWriter is a function to call to render a complete datagram stream.
	// If it returns false, or if nil, a hex dump renderer will be used to
	// render the datagram.
	DatagramWriter DatagramWriter

	// Currently-buffered data.
	buf bytes.Buffer
	// bufPos is the current position in the buffer.
	bufPos int
	// err is the error returned by bufferNext. Once this is set, no more logs
	// will be read and any buffered logs will be drained.
	err error

	// dgBuf is a buffer used for partial datagrams.
	dgBuf bytes.Buffer
}

var _ io.Reader = (*Renderer)(nil)

func (r *Renderer) Read(b []byte) (int, error) {
	buffered := r.buffered()
	for r.err == nil && len(buffered) == 0 {
		r.err = r.bufferNext()
		r.bufPos = 0

		buffered = r.buffered()
	}

	count := copy(b, buffered)
	r.bufPos += count
	return count, r.err
}

func (r *Renderer) buffered() []byte {
	return r.buf.Bytes()[r.bufPos:]
}

func (r *Renderer) bufferNext() error {
	r.buf.Reset()

	// Fetch and buffer the next log entry.
	le, err := r.Source.NextLogEntry()
	if le != nil {
		switch {
		case le.GetText() != nil:
			for _, line := range le.GetText().Lines {
				if r.TextPrefix != nil {
					r.buf.WriteString(r.TextPrefix(le, line))
				}

				r.buf.WriteString(line.Value)
				if !r.Raw {
					r.buf.WriteRune('\n')
				} else {
					r.buf.WriteString(line.Delimiter)
				}
			}

		case le.GetBinary() != nil:
			data := le.GetBinary().Data
			if !r.Raw {
				r.buf.WriteString(hex.EncodeToString(data))
			} else {
				r.buf.Write(data)
			}

		case le.GetDatagram() != nil:
			dg := le.GetDatagram()
			if r.Raw {
				r.buf.Write(dg.Data)
				break
			}

			// Buffer the datagram until it's complete.
			if _, err := r.dgBuf.Write(dg.Data); err != nil {
				return err
			}

			if p := dg.GetPartial(); p == nil || p.Last {
				// Datagram is complete. render it.
				bytesStr := "bytes"
				if r.dgBuf.Len() == 1 {
					bytesStr = "byte"
				}

				fmt.Fprintf(&r.buf, "Datagram #%d (%d %s)\n", le.Sequence, r.dgBuf.Len(), bytesStr)
				if f := r.DatagramWriter; f == nil || !f(&r.buf, r.dgBuf.Bytes()) {
					// Writer failed, or no writer configured. Use a hex dump.
					if err := dumpHex(&r.buf, r.dgBuf.Bytes()); err != nil {
						return err
					}
				}
				r.buf.WriteRune('\n')

				r.dgBuf.Reset()
			}
		}
	}

	return err
}

func dumpHex(w io.Writer, data []byte) (err error) {
	// Hex dump.
	d := hex.Dumper(w)
	defer func() {
		if cerr := d.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	_, err = d.Write(data)
	return
}
