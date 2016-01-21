// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package streamclient

import (
	"bytes"
	"io"
	"testing"

	"github.com/luci/luci-go/client/logdog/butlerlib/streamproto"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/recordio"
	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamImpl(t *testing.T) {
	Convey(`A stream writing to a buffer`, t, func() {
		buf := bytes.Buffer{}
		si := &streamImpl{
			Properties:  &streamproto.Properties{},
			WriteCloser: &nopWriteCloser{Writer: &buf},
		}
		Convey(`TEXT`, func() {
			si.Properties.StreamType = logpb.LogStreamDescriptor_TEXT

			Convey(`Will error if WriteDatagram is called.`, func() {
				So(si.WriteDatagram([]byte(nil)), ShouldNotBeNil)
			})

			Convey(`Can invoke Write.`, func() {
				amt, err := si.Write([]byte{0xd0, 0x65})
				So(err, ShouldBeNil)
				So(amt, ShouldEqual, 2)
				So(buf.Bytes(), ShouldResemble, []byte{0xd0, 0x65})
			})
		})

		Convey(`DATAGRAM`, func() {
			si.Properties.StreamType = logpb.LogStreamDescriptor_DATAGRAM

			Convey(`Will error if Write is called.`, func() {
				_, err := si.Write([]byte(nil))
				So(err, ShouldNotBeNil)
			})

			Convey(`Can invoke WriteDatagram.`, func() {
				fbuf := bytes.Buffer{}
				recordio.WriteFrame(&fbuf, []byte{0xd0, 0x65})

				So(si.WriteDatagram([]byte{0xd0, 0x65}), ShouldBeNil)
				So(buf.Bytes(), ShouldResemble, fbuf.Bytes())
			})
		})
	})
}

type nopWriteCloser struct {
	io.Writer
}

func (nwc *nopWriteCloser) Close() error { return nil }
