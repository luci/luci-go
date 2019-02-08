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
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/type/dayofweek"

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTimeOfDay(t *testing.T) {
	t.Parallel()

	Convey("ToTime", t, func() {
		Convey("invalid", func() {
			Convey("location", func() {
				t := &TimeOfDay{
					Location: "location",
					Time:     "1:23",
				}
				_, err := t.ToTime()
				So(err, ShouldErrLike, "invalid location")
			})

			Convey("time", func() {
				Convey("mismatch", func() {
					t := &TimeOfDay{
						Time: "1/1/1970",
					}
					_, err := t.ToTime()
					So(err, ShouldErrLike, "time must match regex")
				})

				Convey("overflow", func() {
					Convey("hour", func() {
						t := &TimeOfDay{
							Time: "24:00",
						}
						_, err := t.ToTime()
						So(err, ShouldErrLike, "time must not exceed")
					})

					Convey("minute", func() {
						t := &TimeOfDay{
							Time: "00:60",
						}
						_, err := t.ToTime()
						So(err, ShouldErrLike, "time must not exceed")
					})
				})
			})
		})

		Convey("valid", func() {
			Convey("location", func() {
				Convey("implicit", func() {
					t := &TimeOfDay{
						Time: "1:23",
					}
					rel, err := t.ToTime()
					So(err, ShouldBeNil)
					So(rel.Location(), ShouldEqual, time.UTC)
				})

				Convey("explicit", func() {
					t := &TimeOfDay{
						Location: "America/Los_Angeles",
						Time:     "1:23",
					}
					rel, err := t.ToTime()
					So(err, ShouldBeNil)
					So(rel.Location().String(), ShouldEqual, "America/Los_Angeles")
				})
			})

			Convey("time", func() {
				Convey("UTC", func() {
					t := &TimeOfDay{
						Time: "1:23",
					}
					rel, err := t.ToTime()
					So(err, ShouldBeNil)
					So(rel.Hour(), ShouldEqual, 1)
					So(rel.Minute(), ShouldEqual, 23)
					So(rel.UTC().Hour(), ShouldEqual, 1)
				})

				Convey("MTV", func() {
					t := &TimeOfDay{
						Location: "America/Los_Angeles",
						Time:     "1:23",
					}
					rel, err := t.ToTime()
					So(err, ShouldBeNil)
					So(rel.Hour(), ShouldEqual, 1)
					So(rel.Minute(), ShouldEqual, 23)
					So(rel.UTC().Hour(), ShouldNotEqual, 1)
				})
			})
		})
	})

	Convey("Validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("invalid", func() {
			Convey("empty", func() {
				t := &TimeOfDay{}
				t.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "time must match regex")
				So(errs, ShouldContainErr, "at least one day is required")
			})

			Convey("day", func() {
				Convey("empty", func() {
					t := &TimeOfDay{
						Day: []dayofweek.DayOfWeek{},
					}
					t.Validate(c)
					errs := c.Finalize().(*validation.Error).Errors
					So(errs, ShouldContainErr, "at least one day is required")
				})

				Convey("unspecified", func() {
					t := &TimeOfDay{
						Day: []dayofweek.DayOfWeek{
							dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED,
						},
					}
					t.Validate(c)
					errs := c.Finalize().(*validation.Error).Errors
					So(errs, ShouldContainErr, "day must be specified")
				})
			})

			Convey("location", func() {
				t := &TimeOfDay{
					Location: "location",
				}
				t.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "invalid location")
			})
		})

		Convey("valid", func() {
			t := &TimeOfDay{
				Day: []dayofweek.DayOfWeek{
					dayofweek.DayOfWeek_MONDAY,
				},
				Time: "1:23",
			}
			t.Validate(c)
			So(c.Finalize(), ShouldBeNil)
		})
	})
}
