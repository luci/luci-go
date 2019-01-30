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

package config

import (
	"context"
	"math"
	"testing"

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDuration(t *testing.T) {
	t.Parallel()

	Convey("ToSeconds", t, func() {
		Convey("invalid", func() {
			Convey("mismatch", func() {
				d := &TimePeriod_Duration{
					Duration: "1yr",
				}
				_, err := d.ToSeconds()
				So(err, ShouldErrLike, "duration must match regex")
			})

			Convey("overflow", func() {
				d := &TimePeriod_Duration{
					Duration: "9223372036854775808s",
				}
				_, err := d.ToSeconds()
				So(err, ShouldErrLike, "duration must not exceed")
			})
		})

		Convey("valid", func() {
			Convey("converts", func() {
				d := &TimePeriod_Duration{
					Duration: "1h",
				}
				n, err := d.ToSeconds()
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 3600)
			})

			Convey("clamps", func() {
				d := &TimePeriod_Duration{
					Duration: "9223372036854775807mo",
				}
				n, err := d.ToSeconds()
				So(err, ShouldBeNil)
				So(n, ShouldEqual, math.MaxInt64)
			})
		})
	})

	Convey("Validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("invalid", func() {
			Convey("empty", func() {
				d := &TimePeriod_Duration{}
				d.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "duration must match regex")
			})

			Convey("invalid", func() {
				d := &TimePeriod_Duration{
					Duration: "1yr",
				}
				d.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldErrLike, "duration must match regex")
			})
		})

		Convey("valid", func() {
			d := &TimePeriod_Duration{
				Duration: "1h",
			}
			d.Validate(c)
			So(c.Finalize(), ShouldBeNil)
		})
	})
}
