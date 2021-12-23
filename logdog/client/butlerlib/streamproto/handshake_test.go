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

package streamproto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock/clockflag"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestHandshakeProtocol(t *testing.T) {
	Convey(`test WriteHandshake/FromHandshake`, t, func() {
		buf := &bytes.Buffer{}
		writeUvarint := func(val uint64) {
			uvarBuf := make([]byte, binary.MaxVarintLen64)
			buf.Write(uvarBuf[:binary.PutUvarint(uvarBuf, val)])
		}

		f := &Flags{}

		Convey(`Will fail if no handshake data is provided.`, func() {
			So(f.FromHandshake(buf), ShouldErrLike, "reading magic number: EOF")
		})

		Convey(`Will fail with an invalid handshake protocol.`, func() {
			buf.Write([]byte{0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA})
			So(f.FromHandshake(buf), ShouldErrLike, "magic number mismatch")
		})

		Convey(`Loading a handshake frame starting with an invalid size varint value must fail.`, func() {
			buf.Write(ProtocolFrameHeaderMagic)
			buf.Write([]byte{
				// invalid uvarint
				0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x79,
			})
			So(f.FromHandshake(buf), ShouldErrLike, "overflows a 64-bit integer")
		})

		Convey(`Loading a handshake frame larger than the maximum header size must fail.`, func() {
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(maxFrameSize + 1)
			So(f.FromHandshake(buf), ShouldErrLike, "frame size exceeds maximum")
		})

		Convey(`Loading an JSON object with just a name`, func() {
			data := `{"name": "test"}`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data)))
			buf.Write([]byte(data))

			So(f.FromHandshake(buf), ShouldBeNil)
			So(f.Name, ShouldEqual, "test")
		})

		Convey(`Loading a fully-specified configuration`, func() {
			date := "2015-05-07T01:29:51+00:00"
			timestamp, err := time.ParseInLocation(time.RFC3339, date, nil)
			So(err, ShouldBeNil)

			Convey(`manually written handshake`, func() {
				data := fmt.Sprintf(`{
				"name": "test", "timestamp": %q,
				"contentType": "text/plain; charset=utf-8",
				"tags": {"foo": "bar", "baz": "qux"}
			}`, date)
				buf.Write(ProtocolFrameHeaderMagic)
				writeUvarint(uint64(len(data)))
				buf.Write([]byte(data))
			})
			Convey(`WriteHandshake`, func() {
				f.Name = "test"
				f.Timestamp = clockflag.Time(timestamp)
				f.ContentType = "text/plain; charset=utf-8"
				f.Tags = TagMap{
					"foo": "bar",
					"baz": "qux",
				}
				So(f.WriteHandshake(buf), ShouldBeNil)
				f = &Flags{}
			})

			So(f.FromHandshake(buf), ShouldBeNil)

			So(f, ShouldResemble, &Flags{
				Name:        "test",
				ContentType: "text/plain; charset=utf-8",
				Timestamp:   clockflag.Time(timestamp),
				Tags: map[string]string{
					"baz": "qux",
					"foo": "bar",
				},
			})
		})

		Convey(`Loading a (valid) JSON array should fail to load.`, func() {
			data := `["This is an array!"]`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data)))
			buf.Write([]byte(data))

			So(f.FromHandshake(buf), ShouldErrLike, "cannot unmarshal array")
		})

		Convey(`Loading an empty JSON object with a larger-than-necessary header size should fail.`, func() {
			data := `{}`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data) + 10))
			buf.Write([]byte(data))

			So(f.FromHandshake(buf), ShouldErrLike, "handshake had 10 bytes of trailing data")
		})

		Convey(`Loading a JSON with bad field contents should fail.`, func() {
			data := `{"timestamp": "text-for-some-reason"}`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data)))
			buf.Write([]byte(data))

			So(f.FromHandshake(buf), ShouldErrLike, "cannot parse")
		})

		Convey(`Loading an invalid JSON descriptor should fail.`, func() {
			data := `invalid`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data)))
			buf.Write([]byte(data))

			So(f.FromHandshake(buf), ShouldErrLike, "invalid character 'i'")
		})

	})
}
