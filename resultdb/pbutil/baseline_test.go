// Copyright 2023 The LUCI Authors.
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

func TestBaselineID(t *testing.T) {
	t.Parallel()
	Convey(`ValidateBaselineID`, t, func() {
		Convey(`Valid`, func() {
			So(ValidateBaselineID("try:linux-rel"), ShouldBeNil)
			So(ValidateBaselineID("try:linux asan"), ShouldBeNil)
		})

		Convey(`Invalid`, func() {
			Convey(`Empty`, func() {
				So(ValidateBaselineID(""), ShouldErrLike, `unspecified`)
			})
			Convey(`Unsupported Symbol`, func() {
				So(ValidateBaselineID("try/linux-rel"), ShouldErrLike, `does not match`)
				So(ValidateBaselineID("try :rel"), ShouldErrLike, `does not match`)
			})
		})
	})
}

func TestBaselineName(t *testing.T) {
	t.Parallel()
	Convey(`ParseBaselineName`, t, func() {
		Convey(`Parse`, func() {
			proj, baseline, err := ParseBaselineName("projects/foo/baselines/bar:baz")
			So(err, ShouldBeNil)
			So(proj, ShouldEqual, "foo")
			So(baseline, ShouldEqual, "bar:baz")
		})

		Convey("Invalid", func() {
			_, _, err := ParseBaselineName("projects/-/baselines/b!z")
			So(err, ShouldErrLike, `does not match`)
		})
	})
}
