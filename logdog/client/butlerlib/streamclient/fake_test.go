// Copyright 2019 The LUCI Authors.
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
	"context"
	"io"
	"testing"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFakeProtocol(t *testing.T) {
	t.Parallel()

	Convey(`"fake" protocol Client`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)

		Convey(`good`, func() {
			scFake, client := NewUnregisteredFake("namespace")

			Convey(`can use a text stream`, func() {
				stream, err := client.NewStream(ctx, "test")
				So(err, ShouldBeNil)

				n, err := stream.Write([]byte("hi"))
				So(n, ShouldEqual, 2)
				So(err, ShouldBeNil)
				So(stream.Close(), ShouldBeNil)

				streamData := scFake.Data()["namespace/test"]
				So(streamData, ShouldNotBeNil)
				So(streamData.GetStreamData(), ShouldEqual, "hi")
				So(streamData.GetDatagrams(), ShouldResemble, []string{})
				So(streamData.GetFlags(), ShouldResemble, streamproto.Flags{
					Name:        "namespace/test",
					ContentType: "text/plain; charset=utf-8",
					Type:        streamproto.StreamType(logpb.StreamType_TEXT),
					Timestamp:   clockflag.Time(testclock.TestTimeUTC),
					Tags:        nil,
				})
			})

			Convey(`can use a binary stream`, func() {
				stream, err := client.NewStream(ctx, "test", Binary())
				So(err, ShouldBeNil)

				n, err := stream.Write([]byte{0, 1, 2, 3})
				So(n, ShouldEqual, 4)
				So(err, ShouldBeNil)
				So(stream.Close(), ShouldBeNil)

				streamData := scFake.Data()["namespace/test"]
				So(streamData, ShouldNotBeNil)
				So(streamData.GetStreamData(), ShouldEqual, "\x00\x01\x02\x03")
				So(streamData.GetDatagrams(), ShouldResemble, []string{})
				So(streamData.GetFlags(), ShouldResemble, streamproto.Flags{
					Name:        "namespace/test",
					ContentType: "application/octet-stream",
					Type:        streamproto.StreamType(logpb.StreamType_BINARY),
					Timestamp:   clockflag.Time(testclock.TestTimeUTC),
					Tags:        nil,
				})
			})

			Convey(`can use a datagram stream`, func() {
				stream, err := client.NewDatagramStream(ctx, "test")
				So(err, ShouldBeNil)

				So(stream.WriteDatagram([]byte("hi")), ShouldBeNil)
				So(stream.WriteDatagram([]byte("there")), ShouldBeNil)
				So(stream.Close(), ShouldBeNil)

				streamData := scFake.Data()["namespace/test"]
				So(streamData, ShouldNotBeNil)
				So(streamData.GetStreamData(), ShouldEqual, "")
				So(streamData.GetDatagrams(), ShouldResemble, []string{"hi", "there"})
				So(streamData.GetFlags(), ShouldResemble, streamproto.Flags{
					Name:        "namespace/test",
					ContentType: "application/x-logdog-datagram",
					Type:        streamproto.StreamType(logpb.StreamType_DATAGRAM),
					Timestamp:   clockflag.Time(testclock.TestTimeUTC),
					Tags:        nil,
				})
			})
		})

		Convey(`bad`, func() {
			Convey(`duplicate stream`, func() {
				_, client := NewUnregisteredFake("")

				stream, err := client.NewStream(ctx, "test")
				So(err, ShouldBeNil)
				So(stream.Close(), ShouldBeNil)

				_, err = client.NewStream(ctx, "test")
				So(err, ShouldErrLike, `stream "test": stream "test" already dialed`)

				_, err = client.NewStream(ctx, "test", Binary())
				So(err, ShouldErrLike, `stream "test": stream "test" already dialed`)

				_, err = client.NewDatagramStream(ctx, "test")
				So(err, ShouldErrLike, `datagram stream "test": stream "test" already dialed`)
			})

			Convey(`simulated stream errors`, func() {
				Convey(`connection error`, func() {
					scFake, client := NewUnregisteredFake("")
					scFake.SetError(errors.New("bad juju"))

					_, err := client.NewStream(ctx, "test")
					So(err, ShouldErrLike, `stream "test": bad juju`)
				})

				Convey(`use of a stream after close`, func() {
					_, client := NewUnregisteredFake("")

					stream, err := client.NewStream(ctx, "test")
					So(err, ShouldBeNil)
					So(stream.Close(), ShouldBeNil)

					So(stream.Close(), ShouldErrLike, io.ErrClosedPipe)
					_, err = stream.Write([]byte("hi"))
					So(err, ShouldErrLike, io.ErrClosedPipe)

					stream2, err := client.NewDatagramStream(ctx, "test2")
					So(err, ShouldBeNil)
					So(stream2.Close(), ShouldBeNil)

					So(stream2.Close(), ShouldErrLike, io.ErrClosedPipe)
					So(stream2.WriteDatagram([]byte("hi")), ShouldErrLike, io.ErrClosedPipe)
				})
			})
		})
	})
}
