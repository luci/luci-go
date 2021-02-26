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

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

func TestMemory(t *testing.T) {
	Convey(`Test Memory Output`, t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		m := &Output{}
		defer m.Close() // noop

		butler, err := butler.New(ctx, butler.Config{Output: m})
		So(err, ShouldBeNil)

		didStop := false
		butlerStop := func() {
			if !didStop {
				didStop = true
				butler.Activate()
				So(butler.Wait(), ShouldBeNil)
			}
		}
		defer butlerStop()

		sc := streamclient.NewLoopback(butler, "ns")

		Convey(`text`, func() {
			stream, err := sc.NewStream(ctx, "hello", streamclient.WithTags("neat", "thingy"))
			So(err, ShouldBeNil)

			_, err = fmt.Fprintln(stream, "hello world!")
			So(err, ShouldBeNil)
			_, err = fmt.Fprintln(stream, "this is pretty cool.")
			So(err, ShouldBeNil)

			So(stream.Close(), ShouldBeNil)
			butlerStop()

			outStream := m.GetStream("", "ns/hello")
			So(outStream, ShouldNotBeNil)

			So(outStream.Tags(), ShouldResemble, map[string]string{"neat": "thingy"})
			So(outStream.LastData(), ShouldEqual, "hello world!\nthis is pretty cool.\n")
			So(outStream.StreamType(), ShouldEqual, logpb.StreamType_TEXT)
		})

		Convey(`binary`, func() {
			stream, err := sc.NewStream(ctx, "hello", streamclient.WithTags("neat", "thingy"), streamclient.Binary())
			So(err, ShouldBeNil)

			_, err = fmt.Fprintln(stream, "hello world!")
			So(err, ShouldBeNil)
			_, err = fmt.Fprintln(stream, "this is pretty cool.")
			So(err, ShouldBeNil)

			So(stream.Close(), ShouldBeNil)
			butlerStop()

			outStream := m.GetStream("", "ns/hello")
			So(outStream, ShouldNotBeNil)

			So(outStream.Tags(), ShouldResemble, map[string]string{"neat": "thingy"})
			So(outStream.LastData(), ShouldEqual, "hello world!\nthis is pretty cool.\n")
			So(outStream.StreamType(), ShouldEqual, logpb.StreamType_BINARY)
		})

		Convey(`datagram`, func() {
			stream, err := sc.NewDatagramStream(ctx, "hello", streamclient.WithTags("neat", "thingy"))
			So(err, ShouldBeNil)

			So(stream.WriteDatagram([]byte("hello world")), ShouldBeNil)
			So(stream.WriteDatagram([]byte("this is pretty cool")), ShouldBeNil)

			So(stream.Close(), ShouldBeNil)
			butlerStop()

			outStream := m.GetStream("", "ns/hello")
			So(outStream, ShouldNotBeNil)

			So(outStream.Tags(), ShouldResemble, map[string]string{"neat": "thingy"})
			So(outStream.LastData(), ShouldEqual, "this is pretty cool")
			So(outStream.AllData(), ShouldResemble, []string{
				"hello world",
				"this is pretty cool",
			})
			So(outStream.StreamType(), ShouldEqual, logpb.StreamType_DATAGRAM)
		})

	})
}
