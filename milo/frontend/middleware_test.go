// Copyright 2016 The LUCI Authors.
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

package frontend

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFuncs(t *testing.T) {
	//t.Parallel()

	Convey("Middleware Tests", t, func() {
		Convey("humanDuration", func() {
			Convey("3 hrs", func() {
				h := humanDuration(3 * time.Hour)
				So(h, ShouldEqual, "3 hrs")
			})

			Convey("2 hrs 59 mins", func() {
				h := humanDuration(2*time.Hour + 59*time.Minute)
				So(h, ShouldEqual, "2 hrs 59 mins")
			})
		})

		Convey("Format Commit Description", func() {
			Convey("linkify https://", func() {
				So(formatCommitDesc("https://foo.com"),
					ShouldEqual,
					"<a href=\"https://foo.com\">https://foo.com</a>")
				Convey("but not http://", func() {
					So(formatCommitDesc("http://foo.com"), ShouldEqual, "http://foo.com")
				})
			})
			Convey("linkify b/ and crbug/", func() {
				So(formatCommitDesc("blah blah b/123456 blah"), ShouldEqual, "blah blah <a href=\"http://b/123456\">b/123456</a> blah")
				So(formatCommitDesc("crbug:foo/123456"), ShouldEqual, "<a href=\"https://crbug.com/foo/123456\">crbug:foo/123456</a>")
			})
			Convey("linkify Bug: lines", func() {
				So(formatCommitDesc("\nBug: 12345\n"), ShouldEqual, "\nBug: <a href=\"https://crbug.com/12345\">12345</a>\n")
				So(
					formatCommitDesc(" > > BugS=  12345, butter:12345"),
					ShouldEqual,
					" &gt; &gt; BugS=  <a href=\"https://crbug.com/12345\">12345</a>, "+
						"<a href=\"https://crbug.com/butter/12345\">butter:12345</a>")
			})
		})
	})
}
