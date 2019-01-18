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

package common

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFuncs(t *testing.T) {
	Convey("Time Tests", t, func() {
		Convey("humanDuration", func() {
			Convey("3 hrs", func() {
				h := HumanDuration(3 * time.Hour)
				So(h, ShouldEqual, "3 hrs")
			})

			Convey("2 hrs 59 mins", func() {
				h := HumanDuration(2*time.Hour + 59*time.Minute)
				So(h, ShouldEqual, "2 hrs 59 mins")
			})
		})
	})
}
