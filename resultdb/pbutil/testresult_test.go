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

package pbutil

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTestResultName(t *testing.T) {
	t.Parallel()
	Convey("ParseTestResultName", t, func() {
		Convey("Parse", func() {
			invID, testPath, resultID, err := ParseTestResultName(
				"invocations/a/tests/gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5")
			So(err, ShouldBeNil)
			So(invID, ShouldEqual, "a")
			So(testPath, ShouldEqual, "gn://chrome/test:foo_tests/BarTest.DoBaz")
			So(resultID, ShouldEqual, "result5")
		})

		Convey("Invalid", func() {
			Convey(`has slashes`, func() {
				_, _, _, err := ParseTestResultName(
					"invocations/inv/tests/gn://test/results/result1")
				So(err, ShouldErrLike, "does not match")
			})

			Convey(`bad unescape`, func() {
				_, _, _, err := ParseTestResultName(
					"invocations/a/tests/bad_hex_%gg/results/result1")
				So(err, ShouldErrLike, "test path")
			})

			Convey(`unescaped unprintable`, func() {
				_, _, _, err := ParseTestResultName(
					"invocations/a/tests/unprintable_%07/results/result1")
				So(err, ShouldErrLike, "does not match")
			})
		})

		Convey("Format", func() {
			So(TestResultName("a", "gn://chrome/test:foo_tests/BarTest.DoBaz", "result5"),
				ShouldEqual,
				"invocations/a/tests/gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5")
		})
	})
}
