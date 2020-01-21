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

package internal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestIsolatedUtils(t *testing.T) {
	t.Parallel()

	Convey(`URL`, t, func() {
		Convey(`Valid`, func() {
			u := IsolateURL("isolate.example.com", "default-gzip", "deadbeef")
			So(u, ShouldEqual, "isolate://isolate.example.com/default-gzip/deadbeef")

			host, ns, digest, err := ParseIsolateURL(u)
			So(err, ShouldBeNil)
			So(host, ShouldEqual, "isolate.example.com")
			So(ns, ShouldEqual, "default-gzip")
			So(digest, ShouldEqual, "deadbeef")
		})

		Convey(`Invalid`, func() {
			_, _, _, err := ParseIsolateURL("https://isolate.example.com")
			So(err, ShouldErrLike, "does not match")
		})
	})
}
