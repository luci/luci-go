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

package streamclient

import (
	"bytes"
	"io"
	"testing"

	"github.com/luci/luci-go/common/data/recordio"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/butlerlib/streamproto"
	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamImpl(t *testing.T) {
	Convey(`A stream writing to a buffer`, t, func() {
		buf := bytes.Buffer{}
		si := BaseStream{
			WriteCloser: &nopWriteCloser{Writer: &buf},
			P:           (&streamproto.Flags{}).Properties(),
		}

		Convey(`TEXT`, func() {
			si.P.StreamType = logpb.StreamType_TEXT

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
			si.P.StreamType = logpb.StreamType_DATAGRAM

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
