// Copyright 2021 The LUCI Authors.
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

package memory

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

func TestMemory(t *testing.T) {
	ftt.Run(`Test Memory Output`, t, func(t *ftt.Test) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		m := &Output{}
		defer m.Close() // noop

		butler, err := butler.New(ctx, butler.Config{Output: m})
		assert.Loosely(t, err, should.BeNil)

		didStop := false
		butlerStop := func() {
			if !didStop {
				didStop = true
				butler.Activate()
				assert.Loosely(t, butler.Wait(), should.BeNil)
			}
		}
		defer butlerStop()

		sc := streamclient.NewLoopback(butler, "ns")

		t.Run(`text`, func(t *ftt.Test) {
			stream, err := sc.NewStream(ctx, "hello", streamclient.WithTags("neat", "thingy"))
			assert.Loosely(t, err, should.BeNil)

			_, err = fmt.Fprintln(stream, "hello world!")
			assert.Loosely(t, err, should.BeNil)
			_, err = fmt.Fprintln(stream, "this is pretty cool.")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, stream.Close(), should.BeNil)
			butlerStop()

			outStream := m.GetStream("", "ns/hello")
			assert.Loosely(t, outStream, should.NotBeNil)

			assert.Loosely(t, outStream.Tags(), should.Resemble(map[string]string{"neat": "thingy"}))
			assert.Loosely(t, outStream.LastData(), should.Equal("hello world!\nthis is pretty cool.\n"))
			assert.Loosely(t, outStream.StreamType(), should.Equal(logpb.StreamType_TEXT))
		})

		t.Run(`binary`, func(t *ftt.Test) {
			stream, err := sc.NewStream(ctx, "hello", streamclient.WithTags("neat", "thingy"), streamclient.Binary())
			assert.Loosely(t, err, should.BeNil)

			_, err = fmt.Fprintln(stream, "hello world!")
			assert.Loosely(t, err, should.BeNil)
			_, err = fmt.Fprintln(stream, "this is pretty cool.")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, stream.Close(), should.BeNil)
			butlerStop()

			outStream := m.GetStream("", "ns/hello")
			assert.Loosely(t, outStream, should.NotBeNil)

			assert.Loosely(t, outStream.Tags(), should.Resemble(map[string]string{"neat": "thingy"}))
			assert.Loosely(t, outStream.LastData(), should.Equal("hello world!\nthis is pretty cool.\n"))
			assert.Loosely(t, outStream.StreamType(), should.Equal(logpb.StreamType_BINARY))
		})

		t.Run(`datagram`, func(t *ftt.Test) {
			stream, err := sc.NewDatagramStream(ctx, "hello", streamclient.WithTags("neat", "thingy"))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, stream.WriteDatagram([]byte("hello world")), should.BeNil)
			assert.Loosely(t, stream.WriteDatagram([]byte("this is pretty cool")), should.BeNil)

			assert.Loosely(t, stream.Close(), should.BeNil)
			butlerStop()

			outStream := m.GetStream("", "ns/hello")
			assert.Loosely(t, outStream, should.NotBeNil)

			assert.Loosely(t, outStream.Tags(), should.Resemble(map[string]string{"neat": "thingy"}))
			assert.Loosely(t, outStream.LastData(), should.Equal("this is pretty cool"))
			assert.Loosely(t, outStream.AllData(), should.Resemble([]string{
				"hello world",
				"this is pretty cool",
			}))
			assert.Loosely(t, outStream.StreamType(), should.Equal(logpb.StreamType_DATAGRAM))
		})

	})
}
