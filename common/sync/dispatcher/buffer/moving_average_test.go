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

package buffer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMovingAverage(t *testing.T) {
	Convey(`movingAverage`, t, func() {

		Convey(`construction`, func() {
			Convey(`panics on bad window`, func() {
				So(func() {
					newMovingAverage(0, 100)
				}, ShouldPanicLike, "window must be")
			})
		})

		Convey(`usage`, func() {
			ma := newMovingAverage(10, 17)

			Convey(`avg matches seed`, func() {
				So(ma.get(), ShouldEqual, 17)
			})

			Convey(`adding new data changes the average`, func() {
				ma.record(100)
				So(ma.get(), ShouldEqual, 26)
			})

			Convey(`adding a lot of data is fine`, func() {
				for i := 0; i < 100; i++ {
					ma.record(100)
				}
				So(ma.get(), ShouldEqual, 100)
			})
		})

	})
}
