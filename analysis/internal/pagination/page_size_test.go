// Copyright 2022 The LUCI Authors.
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

package pagination

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPageSizeLimiter(t *testing.T) {
	t.Parallel()

	Convey(`PageSizeLimiter`, t, func() {
		psl := PageSizeLimiter{
			Max:     1000,
			Default: 10,
		}

		Convey(`Adjust works`, func() {
			So(psl.Adjust(0), ShouldEqual, 10)
			So(psl.Adjust(10000), ShouldEqual, 1000)
			So(psl.Adjust(500), ShouldEqual, 500)
			So(psl.Adjust(5), ShouldEqual, 5)
		})
	})
}

func TestValidatePageSize(t *testing.T) {
	t.Parallel()

	Convey(`ValidatePageSize`, t, func() {
		Convey(`Positive`, func() {
			So(ValidatePageSize(10), ShouldBeNil)
		})
		Convey(`Zero`, func() {
			So(ValidatePageSize(0), ShouldBeNil)
		})
		Convey(`Negative`, func() {
			So(ValidatePageSize(-10), ShouldErrLike, "negative")
		})
	})
}
