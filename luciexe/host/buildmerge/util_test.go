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

package buildmerge

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/logdog/common/types"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAbsolutize(t *testing.T) {
	t.Parallel()
	Convey(`absolutize`, t, func() {
		absolutize := func(logURL, viewURL string) (absLogURL, absViewURL string, err error) {
			return absolutizeURLs(logURL, viewURL, "ns/", func(ns, streamName types.StreamName) (url string, viewUrl string) {
				return fmt.Sprintf("url://%s%s", ns, streamName), fmt.Sprintf("viewURL://%s%s", ns, streamName)
			})

		}
		Convey(`no-op if both urls are absolute`, func() {
			absLogURL, absViewURL, err := absolutize("url://ns/log/foo", "viewURL://ns/log/foo")
			So(err, ShouldBeNil)
			So(absLogURL, ShouldEqual, "url://ns/log/foo")
			So(absViewURL, ShouldEqual, "viewURL://ns/log/foo")
		})

		Convey(`calc urls if log url is relative`, func() {
			absLogURL, absViewURL, err := absolutize("log/foo", "")
			So(err, ShouldBeNil)
			So(absLogURL, ShouldEqual, "url://ns/log/foo")
			So(absViewURL, ShouldEqual, "viewURL://ns/log/foo")
			Convey(`even when log url has non-alnum char but valid stream char`, func() {
				absLogURL, absViewURL, err := absolutize("log:hi.hello_hey-aloha/foo", "")
				So(err, ShouldBeNil)
				So(absLogURL, ShouldEqual, "url://ns/log:hi.hello_hey-aloha/foo")
				So(absViewURL, ShouldEqual, "viewURL://ns/log:hi.hello_hey-aloha/foo")
			})
			Convey(`omits provided view url`, func() {
				absLogURL, absViewURL, err := absolutize("log/foo", "Hi there!")
				So(err, ShouldBeNil)
				So(absLogURL, ShouldEqual, "url://ns/log/foo")
				So(absViewURL, ShouldEqual, "viewURL://ns/log/foo")
			})
		})

		Convey(`error`, func() {
			Convey(`if log url is absolute`, func() {
				Convey(`but view url is empty`, func() {
					absLogURL, absViewURL, err := absolutize("url://ns/log/foo", "")
					So(err, ShouldErrLike, "absolute log url is provided", "view url is empty")
					So(absLogURL, ShouldEqual, "url://ns/log/foo")
					So(absViewURL, ShouldEqual, "")
				})
				Convey(`but view url is not absolute`, func() {
					absLogURL, absViewURL, err := absolutize("url://ns/log/foo", "log/foo")
					So(err, ShouldErrLike, "expected absolute view url, got")
					So(absLogURL, ShouldEqual, "url://ns/log/foo")
					So(absViewURL, ShouldEqual, "log/foo")
				})
			})

			Convey(`if log url is relative but not a valid stream`, func() {
				absLogURL, absViewURL, err := absolutize("log/foo#key=value", "")
				So(err, ShouldErrLike, "bad log url", "illegal character")
				So(absLogURL, ShouldEqual, "log/foo#key=value")
				So(absViewURL, ShouldEqual, "")
			})
		})
	})
}
