// Copyright 2020 The LUCI Authors.
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

package botdata

import (
	"testing"
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParse(t *testing.T) {
	t.Parallel()

	Convey("Parse", t, func() {

		Convey("Nil ChangeMessageInfo", func() {
			_, ok := Parse(nil)
			So(ok, ShouldBeFalse)
		})

		Convey("Success", func() {
			cmi := &gerritpb.ChangeMessageInfo{
				Message: `human message

				Bot data: {"action":"start","triggered_at":"2013-03-23T21:36:52.332Z","revision":"abcd","cls":["chromium-review.googlesource.com:1111","chromium-review.googlesource.com:2222"]}`,
			}
			ret, ok := Parse(cmi)
			So(ok, ShouldBeTrue)
			So(ret, ShouldResemble, BotData{
				Action:      Start,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			})
		})

		Convey("BotDataPrefix Missing", func() {
			cmi := &gerritpb.ChangeMessageInfo{
				Message: `{"action": "start", "triggered_at": "2013-03-23T21:36:52.332Z", "revision": "abcdef"}`,
			}
			_, ok := Parse(cmi)
			So(ok, ShouldBeFalse)
		})

		Convey("Invalid BotData JSON string", func() {
			cmi := &gerritpb.ChangeMessageInfo{
				Message: `Bot data: I'm a plain string`,
			}
			_, ok := Parse(cmi)
			So(ok, ShouldBeFalse)
		})
	})
}

func TestAppend(t *testing.T) {
	t.Parallel()

	Convey("Append", t, func() {
		Convey("Bot Message too long", func() {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			_, err := append("", bd, 10)
			So(err, ShouldErrLike, "bot data too long; max length: 10")
		})

		Convey("Empty human message", func() {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			ret, err := append("", bd, 1000)
			So(err, ShouldBeNil)
			So(ret, ShouldEqual, `Bot data: {"action":"cancel","triggered_at":"2013-03-23T21:36:52.332Z","revision":"abcd"}`)
		})

		Convey("Full human message", func() {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			ret, err := append("Message for human", bd, 1000)
			So(err, ShouldBeNil)
			So(ret, ShouldEqual, `Message for human

Bot data: {"action":"cancel","triggered_at":"2013-03-23T21:36:52.332Z","revision":"abcd"}`)
		})

		Convey("Truncated human message", func() {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			ret, err := append("Message for human. But it's way tooooooooooooooooooooooo long", bd, 150)
			So(err, ShouldBeNil)
			So(ret, ShouldEqual, `Message for human. But it's
...[truncated too long message]

Bot data: {"action":"cancel","triggered_at":"2013-03-23T21:36:52.332Z","revision":"abcd"}`)
		})

		Convey("Bot Data too long to fit human message", func() {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			// Bot data itself is 89 characters long already.
			// Placeholder is 31 characters long.
			_, err := append("Message for human", bd, 100)
			So(err, ShouldErrLike, "bot data too long to display human message; max length: 100")
		})
	})
}
