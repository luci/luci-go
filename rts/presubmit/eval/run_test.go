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

package eval

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScoreString(t *testing.T) {
	t.Parallel()

	Convey(`ScoreString`, t, func() {
		Convey("NaN", func() {
			So(scoreString(math.NaN()), ShouldEqual, "?")
		})
		Convey("0%", func() {
			So(scoreString(0), ShouldEqual, "0.00%")
		})
		Convey("0.0001%", func() {
			So(scoreString(0.000001), ShouldEqual, "<0.01%")
		})
		Convey("50%", func() {
			So(scoreString(0.5), ShouldEqual, "50.00%")
		})
		Convey("99.999%", func() {
			So(scoreString(0.99999), ShouldEqual, ">99.99%")
		})
		Convey("100%", func() {
			So(scoreString(1), ShouldEqual, "100.00%")
		})
	})
}
