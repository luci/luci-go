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

// testFetcher implements the fetcher interface using stubbed data.
type testFetcher struct {
	logs []*protocol.LogEntry
	err  error
}

func (tf *testFetcher) NextLogEntry() (*protocol.LogEntry, error) {
	if tf.err != nil {
		return nil, tf.err
	}
	if len(tf.logs) == 0 {
		return nil, io.EOF
	}

	var le *protocol.LogEntry
	le, tf.logs = tf.logs[0], tf.logs[1:]
	return le, nil
}

func (tf *testFetcher) loadLogEntry(le *protocol.LogEntry) {
	tf.logs = append(tf.logs, le)
}

func (tf *testFetcher) loadText(line, delim string) {
	tf.loadLogEntry(&protocol.LogEntry{
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

func (tf *testFetcher) loadBinary(data []byte) {
	tf.loadLogEntry(&protocol.LogEntry{
		Content: &protocol.LogEntry_Binary{
			Binary: &protocol.Binary{
				Data: data,
			},
		},
	})
}

func TestRenderer(t *testing.T) {
	t.Parallel()

	Convey(`A Renderer connected to a test fetcher`, t, func() {
		tf := testFetcher{}
		b := bytes.Buffer{}
		r := &Renderer{
			fetcher: &tf,
		}

		Convey(`With no log data, will render nothing and return.`, func() {
			c, err := b.ReadFrom(r)
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 0)
		})

		Convey(`With TEXT log entries ["1", "2"] using "DELIM" as the delimiter`, func() {
			tf.loadText("1", "DELIM")
			tf.loadText("2", "DELIM")

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
			tf.loadBinary([]byte{0x00})
			tf.loadBinary([]byte{0x01, 0x02})
			tf.loadBinary([]byte{})
			tf.loadBinary([]byte{0x03})

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
			tf.loadLogEntry(&protocol.LogEntry{})
			c, err := b.ReadFrom(r)
			So(err, ShouldBeNil)
			So(c, ShouldEqual, 0)
		})
	})
}
