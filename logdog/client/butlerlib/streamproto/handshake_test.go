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

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestHandshakeProtocol(t *testing.T) {
	ftt.Run(`test WriteHandshake/FromHandshake`, t, func(t *ftt.Test) {
		buf := &bytes.Buffer{}
		writeUvarint := func(val uint64) {
			uvarBuf := make([]byte, binary.MaxVarintLen64)
			buf.Write(uvarBuf[:binary.PutUvarint(uvarBuf, val)])
		}

		f := &Flags{}

		t.Run(`Will fail if no handshake data is provided.`, func(t *ftt.Test) {
			assert.Loosely(t, f.FromHandshake(buf), should.ErrLike("reading magic number: EOF"))
		})

		t.Run(`Will fail with an invalid handshake protocol.`, func(t *ftt.Test) {
			buf.Write([]byte{0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA})
			assert.Loosely(t, f.FromHandshake(buf), should.ErrLike("magic number mismatch"))
		})

		t.Run(`Loading a handshake frame starting with an invalid size varint value must fail.`, func(t *ftt.Test) {
			buf.Write(ProtocolFrameHeaderMagic)
			buf.Write([]byte{
				// invalid uvarint
				0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x79,
			})
			assert.Loosely(t, f.FromHandshake(buf), should.ErrLike("overflows a 64-bit integer"))
		})

		t.Run(`Loading a handshake frame larger than the maximum header size must fail.`, func(t *ftt.Test) {
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(maxFrameSize + 1)
			assert.Loosely(t, f.FromHandshake(buf), should.ErrLike("frame size exceeds maximum"))
		})

		t.Run(`Loading an JSON object with just a name`, func(t *ftt.Test) {
			data := `{"name": "test"}`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data)))
			buf.Write([]byte(data))

			assert.Loosely(t, f.FromHandshake(buf), should.BeNil)
			assert.Loosely(t, f.Name, should.Equal(StreamNameFlag("test")))
		})

		t.Run(`Loading a fully-specified configuration`, func(t *ftt.Test) {
			date := "2015-05-07T01:29:51+00:00"
			timestamp, err := time.ParseInLocation(time.RFC3339, date, nil)
			assert.Loosely(t, err, should.BeNil)

			check := func(t testing.TB) {
				t.Helper()
				assert.Loosely(t, f.FromHandshake(buf), should.BeNil, truth.LineContext())

				assert.Loosely(t, f, should.Match(&Flags{
					Name:        "test",
					ContentType: "text/plain; charset=utf-8",
					Timestamp:   clockflag.Time(timestamp),
					Tags: map[string]string{
						"baz": "qux",
						"foo": "bar",
					},
				}), truth.LineContext())
			}

			t.Run(`manually written handshake`, func(t *ftt.Test) {
				data := fmt.Sprintf(`{
				"name": "test", "timestamp": %q,
				"contentType": "text/plain; charset=utf-8",
				"tags": {"foo": "bar", "baz": "qux"}
			}`, date)
				buf.Write(ProtocolFrameHeaderMagic)
				writeUvarint(uint64(len(data)))
				buf.Write([]byte(data))
				check(t)
			})
			t.Run(`WriteHandshake`, func(t *ftt.Test) {
				f.Name = "test"
				f.Timestamp = clockflag.Time(timestamp)
				f.ContentType = "text/plain; charset=utf-8"
				f.Tags = TagMap{
					"foo": "bar",
					"baz": "qux",
				}
				assert.Loosely(t, f.WriteHandshake(buf), should.BeNil)
				f = &Flags{}
				check(t)
			})
		})

		t.Run(`Loading a (valid) JSON array should fail to load.`, func(t *ftt.Test) {
			data := `["This is an array!"]`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data)))
			buf.Write([]byte(data))

			assert.Loosely(t, f.FromHandshake(buf), should.ErrLike("cannot unmarshal array"))
		})

		t.Run(`Loading an empty JSON object with a larger-than-necessary header size should fail.`, func(t *ftt.Test) {
			data := `{}`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data) + 10))
			buf.Write([]byte(data))

			assert.Loosely(t, f.FromHandshake(buf), should.ErrLike("handshake had 10 bytes of trailing data"))
		})

		t.Run(`Loading a JSON with bad field contents should fail.`, func(t *ftt.Test) {
			data := `{"timestamp": "text-for-some-reason"}`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data)))
			buf.Write([]byte(data))

			assert.Loosely(t, f.FromHandshake(buf), should.ErrLike("cannot parse"))
		})

		t.Run(`Loading an invalid JSON descriptor should fail.`, func(t *ftt.Test) {
			data := `invalid`
			buf.Write(ProtocolFrameHeaderMagic)
			writeUvarint(uint64(len(data)))
			buf.Write([]byte(data))

			assert.Loosely(t, f.FromHandshake(buf), should.ErrLike("invalid character 'i'"))
		})
	})
}
