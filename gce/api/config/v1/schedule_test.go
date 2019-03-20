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

func TestSchedule(t *testing.T) {
	t.Parallel()

	Convey("isSameDay", t, func() {
		So(isSameDay(time.Sunday, dayofweek.DayOfWeek_SUNDAY), ShouldBeTrue)
		So(isSameDay(time.Monday, dayofweek.DayOfWeek_MONDAY), ShouldBeTrue)
		So(isSameDay(time.Friday, dayofweek.DayOfWeek_FRIDAY), ShouldBeTrue)
		So(isSameDay(time.Sunday, dayofweek.DayOfWeek_MONDAY), ShouldBeFalse)
		So(isSameDay(time.Monday, dayofweek.DayOfWeek_SUNDAY), ShouldBeFalse)
		So(isSameDay(time.Sunday, dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED), ShouldBeFalse)
		So(isSameDay(time.Monday, dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED), ShouldBeFalse)
		So(isSameDay(time.Friday, dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED), ShouldBeFalse)
	})

	Convey("mostRecentStart", t, func() {
		now := time.Time{}.Add(time.Hour * 12)
		So(now.Weekday(), ShouldEqual, time.Monday)
		So(now.Hour(), ShouldEqual, 12)

		Convey("invalid", func() {
			Convey("day", func() {
				s := &Schedule{
					Start: &TimeOfDay{
						Time: "1:00",
					},
				}
				_, err := s.mostRecentStart(now)
				So(err, ShouldErrLike, "day must be specified")
			})

			Convey("location", func() {
				s := &Schedule{
					Start: &TimeOfDay{
						Day:      dayofweek.DayOfWeek_MONDAY,
						Location: "location",
						Time:     "1:00",
					},
				}
				_, err := s.mostRecentStart(now)
				So(err, ShouldErrLike, "invalid location")
			})

			Convey("time", func() {
				s := &Schedule{
					Start: &TimeOfDay{
						Day:  dayofweek.DayOfWeek_MONDAY,
						Time: "24:00",
					},
				}
				_, err := s.mostRecentStart(now)
				So(err, ShouldErrLike, "time must not exceed")
			})
		})

		Convey("valid", func() {
			Convey("past", func() {
				Convey("different day", func() {
					s := &Schedule{
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_SUNDAY,
							Time: "23:00",
						},
					}
					t, err := s.mostRecentStart(now)
					So(err, ShouldBeNil)
					So(t.Weekday(), ShouldEqual, time.Sunday)
					So(t.Before(now), ShouldBeTrue)
				})

				Convey("same day", func() {
					s := &Schedule{
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_MONDAY,
							Time: "11:00",
						},
					}
					t, err := s.mostRecentStart(now)
					So(err, ShouldBeNil)
					So(t.Weekday(), ShouldEqual, time.Monday)
					So(t.Before(now), ShouldBeTrue)
				})
			})

			Convey("present", func() {
				s := &Schedule{
					Start: &TimeOfDay{
						Day:  dayofweek.DayOfWeek_MONDAY,
						Time: "12:00",
					},
				}
				t, err := s.mostRecentStart(now)
				So(err, ShouldBeNil)
				So(t.Weekday(), ShouldEqual, time.Monday)
				So(t.Equal(now), ShouldBeTrue)
			})

			Convey("future", func() {
				Convey("same day", func() {
					s := &Schedule{
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_MONDAY,
							Time: "13:00",
						},
					}
					t, err := s.mostRecentStart(now)
					So(err, ShouldBeNil)
					So(t.Weekday(), ShouldEqual, time.Monday)
					So(t.Before(now), ShouldBeTrue)
				})

				Convey("different day", func() {
					s := &Schedule{
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_TUESDAY,
							Time: "1:00",
						},
					}
					t, err := s.mostRecentStart(now)
					So(err, ShouldBeNil)
					So(t.Weekday(), ShouldEqual, time.Tuesday)
					So(t.Before(now), ShouldBeTrue)
				})
			})
		})
	})

	Convey("Validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("invalid", func() {
			Convey("empty", func() {
				s := &Schedule{}
				s.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "duration or seconds is required")
				So(errs, ShouldContainErr, "time must match regex")
			})

			Convey("amount", func() {
				s := &Schedule{
					Amount: -1,
				}
				s.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "amount must be non-negative")
			})
		})

		Convey("valid", func() {
			s := &Schedule{
				Amount: 1,
				Length: &TimePeriod{
					Time: &TimePeriod_Duration{
						Duration: "1h",
					},
				},
				Start: &TimeOfDay{
					Day:  dayofweek.DayOfWeek_MONDAY,
					Time: "1:23",
				},
			}
			s.Validate(c)
			So(c.Finalize(), ShouldBeNil)
		})
	})
}
