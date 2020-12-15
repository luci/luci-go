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
	"os"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestNullProtocol(t *testing.T) {
	Convey(`"null" protocol Client`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)

		Convey(`good`, func() {
			client, err := New("null", "namespace")
			So(err, ShouldBeNil)

			Convey(`can use a text stream`, func() {
				stream, err := client.NewStream(ctx, "test")
				So(err, ShouldBeNil)

				n, err := stream.Write([]byte("hi"))
				So(n, ShouldEqual, 2)
				So(err, ShouldBeNil)
				So(stream.Close(), ShouldBeNil)
			})

			Convey(`can use a text stream for a subprocess`, func() {
				stream, err := client.NewStream(ctx, "test", ForProcess())
				So(err, ShouldBeNil)
				defer stream.Close()
				So(stream, ShouldHaveSameTypeAs, (*os.File)(nil))

				n, err := stream.Write([]byte("hi"))
				So(n, ShouldEqual, 2)
				So(err, ShouldBeNil)
				So(stream.Close(), ShouldBeNil)
				So(stream.Close(), ShouldErrLike, os.ErrClosed)
			})

			Convey(`can use a binary stream`, func() {
				stream, err := client.NewStream(ctx, "test", Binary())
				So(err, ShouldBeNil)

				n, err := stream.Write([]byte{0, 1, 2, 3})
				So(n, ShouldEqual, 4)
				So(err, ShouldBeNil)
				So(stream.Close(), ShouldBeNil)
			})

			Convey(`can use a datagram stream`, func() {
				stream, err := client.NewDatagramStream(ctx, "test")
				So(err, ShouldBeNil)

				So(stream.WriteDatagram([]byte("hi")), ShouldBeNil)
				So(stream.WriteDatagram([]byte("there")), ShouldBeNil)
				So(stream.Close(), ShouldBeNil)
			})
		})
	})
}
