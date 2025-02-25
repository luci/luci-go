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

package renderer

import (
	"bytes"
	"io"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
)

// testSource implements the Source interface using stubbed data.
type testSource struct {
	logs []*logpb.LogEntry
	err  error
}

func (ts *testSource) NextLogEntry() (le *logpb.LogEntry, err error) {
	if ts.err != nil {
		return nil, ts.err
	}

	if len(ts.logs) > 0 {
		le, ts.logs = ts.logs[0], ts.logs[1:]
	}
	if len(ts.logs) == 0 {
		err = io.EOF
	}
	return
}

func (ts *testSource) loadLogEntry(le *logpb.LogEntry) {
	ts.logs = append(ts.logs, le)
}

func (ts *testSource) loadText(line, delim string) {
	ts.loadLogEntry(&logpb.LogEntry{
		Content: &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: []*logpb.Text_Line{
					{
						Value:     []byte(line),
						Delimiter: delim,
					},
				},
			},
		},
	})
}

func (ts *testSource) loadBinary(data []byte) {
	ts.loadLogEntry(&logpb.LogEntry{
		Content: &logpb.LogEntry_Binary{
			Binary: &logpb.Binary{
				Data: data,
			},
		},
	})
}

func (ts *testSource) loadDatagram(data []byte, term bool) {
	ts.loadLogEntry(&logpb.LogEntry{
		Content: &logpb.LogEntry_Datagram{
			Datagram: &logpb.Datagram{
				Partial: &logpb.Datagram_Partial{
					Last: term,
				},
				Data: data,
			},
		},
	})
}

func TestRenderer(t *testing.T) {
	t.Parallel()

	ftt.Run(`A Renderer connected to a test Source`, t, func(t *ftt.Test) {
		ts := testSource{}
		b := bytes.Buffer{}
		r := &Renderer{
			Source: &ts,
		}

		t.Run(`With no log data, will render nothing and return.`, func(t *ftt.Test) {
			c, err := b.ReadFrom(r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, c, should.BeZero)
		})

		t.Run(`With TEXT log entries ["1", "2"] using "DELIM" as the delimiter`, func(t *ftt.Test) {
			ts.loadText("1", "DELIM")
			ts.loadText("2", "DELIM")

			t.Run(`When not configured to render raw, renders "1\n2\n".`, func(t *ftt.Test) {
				_, err := b.ReadFrom(r)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b.String(), should.Equal("1\n2\n"))
			})

			t.Run(`When configured to render raw, renders "1DELIM2DELIM".`, func(t *ftt.Test) {
				r.Raw = true

				_, err := b.ReadFrom(r)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b.String(), should.Equal("1DELIM2DELIM"))
			})
		})

		t.Run(`With BINARY log entries {{0x00}, {0x01, 0x02}, {}, {0x03}}`, func(t *ftt.Test) {
			ts.loadBinary([]byte{0x00})
			ts.loadBinary([]byte{0x01, 0x02})
			ts.loadBinary([]byte{})
			ts.loadBinary([]byte{0x03})

			t.Run(`Renders {0x00, 0x01, 0x02, 0x03}.`, func(t *ftt.Test) {
				_, err := b.ReadFrom(r)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b.Bytes(), should.Match([]byte("00010203")))
			})

			t.Run(`Renders raw {0x00, 0x01, 0x02, 0x03}.`, func(t *ftt.Test) {
				r.Raw = true

				_, err := b.ReadFrom(r)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b.Bytes(), should.Match([]byte{0x00, 0x01, 0x02, 0x03}))
			})

			t.Run(`Can read the raw stream byte-by-byte.`, func(t *ftt.Test) {
				r.Raw = true
				b := [1]byte{}

				c, err := r.Read(b[:])
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, c, should.Equal(1))
				assert.Loosely(t, b[0], should.Equal(0x00))

				c, err = r.Read(b[:])
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, c, should.Equal(1))
				assert.Loosely(t, b[0], should.Equal(0x01))

				c, err = r.Read(b[:])
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, c, should.Equal(1))
				assert.Loosely(t, b[0], should.Equal(0x02))

				c, err = r.Read(b[:])
				assert.Loosely(t, err, should.Equal(io.EOF))
				assert.Loosely(t, c, should.Equal(1))
				assert.Loosely(t, b[0], should.Equal(0x03))
			})
		})

		t.Run(`With partial DATAGRAM log entries {{0x00}, {0x01, 0x02}, {}, {0x03}}.`, func(t *ftt.Test) {
			ts.loadDatagram([]byte{0x00}, false)
			ts.loadDatagram([]byte{0x01, 0x02}, false)
			ts.loadDatagram([]byte{}, false)
			ts.loadDatagram([]byte{0x03}, true)

			hexDump := "Datagram #0 (4 bytes)\n" +
				"00000000  00 01 02 03                                       |....|\n\n"

			t.Run(`Renders a full hex dump.`, func(t *ftt.Test) {
				_, err := b.ReadFrom(r)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b.String(), should.Equal(hexDump))
			})

			t.Run(`When deferring to a datagram writer`, func(t *ftt.Test) {

				t.Run(`Uses the writer instead of a hex dump.`, func(t *ftt.Test) {
					var bytes []byte
					r.DatagramWriter = func(w io.Writer, dg []byte) bool {
						bytes = make([]byte, len(dg))
						copy(bytes, dg)

						w.Write([]byte("rendered"))
						return true
					}

					_, err := b.ReadFrom(r)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, b.String(), should.Equal("Datagram #0 (4 bytes)\nrendered\n"))
					assert.Loosely(t, bytes, should.Match([]byte{0x00, 0x01, 0x02, 0x03}))
				})

				t.Run(`Renders a full hex dump when the writer returns false.`, func(t *ftt.Test) {
					r.DatagramWriter = func(w io.Writer, dg []byte) bool { return false }

					_, err := b.ReadFrom(r)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, b.String(), should.Equal(hexDump))
				})
			})
		})

		t.Run(`With empty log entries, renders nothing.`, func(t *ftt.Test) {
			ts.loadLogEntry(&logpb.LogEntry{})
			c, err := b.ReadFrom(r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, c, should.BeZero)
		})
	})
}
