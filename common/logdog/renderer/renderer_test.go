// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package renderer

import (
	"bytes"
	"io"
	"testing"

	"github.com/luci/luci-go/common/logdog/protocol"
	. "github.com/smartystreets/goconvey/convey"
)

// testSource implements the Source interface using stubbed data.
type testSource struct {
	logs []*protocol.LogEntry
	err  error
}

func (ts *testSource) NextLogEntry() (*protocol.LogEntry, error) {
	if ts.err != nil {
		return nil, ts.err
	}
	if len(ts.logs) == 0 {
		return nil, io.EOF
	}

	var le *protocol.LogEntry
	le, ts.logs = ts.logs[0], ts.logs[1:]
	return le, nil
}

func (ts *testSource) loadLogEntry(le *protocol.LogEntry) {
	ts.logs = append(ts.logs, le)
}

func (ts *testSource) loadText(line, delim string) {
	ts.loadLogEntry(&protocol.LogEntry{
		Content: &protocol.LogEntry_Text{
			Text: &protocol.Text{
				Lines: []*protocol.Text_Line{
					{
						Value:     line,
						Delimiter: delim,
					},
				},
			},
		},
	})
}

func (ts *testSource) loadBinary(data []byte) {
	ts.loadLogEntry(&protocol.LogEntry{
		Content: &protocol.LogEntry_Binary{
			Binary: &protocol.Binary{
				Data: data,
			},
		},
	})
}

func TestRenderer(t *testing.T) {
	t.Parallel()

	Convey(`A Renderer connected to a test Source`, t, func() {
		ts := testSource{}
		b := bytes.Buffer{}
		r := &Renderer{
			Source: &ts,
		}

		Convey(`With no log data, will render nothing and return.`, func() {
			c, err := b.ReadFrom(r)
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 0)
		})

		Convey(`With TEXT log entries ["1", "2"] using "DELIM" as the delimiter`, func() {
			ts.loadText("1", "DELIM")
			ts.loadText("2", "DELIM")

			Convey(`When not configured to reproduce, renders "1\n2\n".`, func() {
				_, err := b.ReadFrom(r)
				So(err, ShouldBeNil)
				So(b.String(), ShouldEqual, "1\n2\n")
			})

			Convey(`When configured to reproduce, renders "1DELIM2DELIM".`, func() {
				r.Reproduce = true

				_, err := b.ReadFrom(r)
				So(err, ShouldBeNil)
				So(b.String(), ShouldEqual, "1DELIM2DELIM")
			})
		})

		Convey(`With BINARY log entries {{0x00}, {0x01, 0x02}, {}, {0x03}}`, func() {
			ts.loadBinary([]byte{0x00})
			ts.loadBinary([]byte{0x01, 0x02})
			ts.loadBinary([]byte{})
			ts.loadBinary([]byte{0x03})

			Convey(`Renders {0x00, 0x01, 0x02, 0x03}.`, func() {
				_, err := b.ReadFrom(r)
				So(err, ShouldBeNil)
				So(b.Bytes(), ShouldResemble, []byte{0x00, 0x01, 0x02, 0x03})
			})

			Convey(`Can read the stream byte-by-byte.`, func() {
				b := [1]byte{}

				c, err := r.Read(b[:])
				So(err, ShouldBeNil)
				So(c, ShouldEqual, 1)
				So(b[0], ShouldEqual, 0x00)

				c, err = r.Read(b[:])
				So(err, ShouldBeNil)
				So(c, ShouldEqual, 1)
				So(b[0], ShouldEqual, 0x01)

				c, err = r.Read(b[:])
				So(err, ShouldBeNil)
				So(c, ShouldEqual, 1)
				So(b[0], ShouldEqual, 0x02)

				c, err = r.Read(b[:])
				So(err, ShouldBeNil)
				So(c, ShouldEqual, 1)
				So(b[0], ShouldEqual, 0x03)

				c, err = r.Read(b[:])
				So(err, ShouldEqual, io.EOF)
				So(c, ShouldEqual, 0)
			})
		})

		Convey(`With empty log entries, renders nothing.`, func() {
			ts.loadLogEntry(&protocol.LogEntry{})
			c, err := b.ReadFrom(r)
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 0)
		})
	})
}
