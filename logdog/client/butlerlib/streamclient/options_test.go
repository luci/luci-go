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
	"context"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOptions(t *testing.T) {
	t.Parallel()

	Convey(`options`, t, func() {
		scFake, client := NewUnregisteredFake("")

		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)

		Convey(`defaults`, func() {
			_, err := client.NewStream(ctx, "test")
			So(err, ShouldBeNil)
			defaultFlags := scFake.Data()["test"].GetFlags()
			So(defaultFlags.ContentType, ShouldEqual, "text/plain; charset=utf-8")
			So(defaultFlags.Timestamp.Time(), ShouldEqual, testclock.TestTimeUTC)
			So(defaultFlags.Tags, ShouldBeEmpty)
		})

		Convey(`can change content type`, func() {
			_, err := client.NewStream(ctx, "test", WithContentType("narple"))
			So(err, ShouldBeNil)
			testFlags := scFake.Data()["test"].GetFlags()
			So(testFlags.ContentType, ShouldEqual, "narple")
		})

		Convey(`can set initial timestamp`, func() {
			_, err := client.NewStream(ctx, "test", WithTimestamp(testclock.TestRecentTimeUTC))
			So(err, ShouldBeNil)
			testFlags := scFake.Data()["test"].GetFlags()
			So(testFlags.Timestamp.Time(), ShouldEqual, testclock.TestRecentTimeUTC)
		})

		Convey(`can set tags nicely`, func() {
			_, err := client.NewStream(ctx, "test", WithTags(
				"key1", "value",
				"key2", "value",
			))
			So(err, ShouldBeNil)
			testFlags := scFake.Data()["test"].GetFlags()
			So(testFlags.Tags, ShouldResemble, streamproto.TagMap{
				"key1": "value",
				"key2": "value",
			})
		})

		Convey(`WithTags expects an even number of args`, func() {
			So(func() {
				WithTags("hi")
			}, ShouldPanicLike, "even number of arguments")
		})

		Convey(`can set tags practically`, func() {
			_, err := client.NewStream(ctx, "test", WithTagMap(map[string]string{
				"key1": "value",
				"key2": "value",
			}))
			So(err, ShouldBeNil)
			testFlags := scFake.Data()["test"].GetFlags()
			So(testFlags.Tags, ShouldResemble, streamproto.TagMap{
				"key1": "value",
				"key2": "value",
			})
		})

	})
}
