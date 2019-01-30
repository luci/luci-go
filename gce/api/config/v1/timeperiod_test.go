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

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTimePeriod(t *testing.T) {
	t.Parallel()

	Convey("normalize", t, func() {
		Convey("invalid", func() {
			tp := &TimePeriod{
				Time: &TimePeriod_Duration{
					Duration: "-1h",
				},
			}
			So(tp.Normalize(), ShouldErrLike, "invalid duration")
		})

		Convey("valid", func() {
			Convey("empty", func() {
				tp := &TimePeriod{}
				So(tp.Normalize(), ShouldBeNil)
				So(tp.Time, ShouldBeNil)
			})

			Convey("normalizes", func() {
				Convey("zero", func() {
					tp := &TimePeriod{
						Time: &TimePeriod_Duration{
							Duration: "0h",
						},
					}
					So(tp.Normalize(), ShouldBeNil)
					So(tp.Time, ShouldHaveSameTypeAs, &TimePeriod_Seconds{})
					So(tp.Time.(*TimePeriod_Seconds).Seconds, ShouldEqual, 0)
				})

				Convey("nonzero", func() {
					tp := &TimePeriod{
						Time: &TimePeriod_Duration{
							Duration: "1h",
						},
					}
					So(tp.Normalize(), ShouldBeNil)
					So(tp.Time, ShouldHaveSameTypeAs, &TimePeriod_Seconds{})
					So(tp.Time.(*TimePeriod_Seconds).Seconds, ShouldEqual, 3600)
				})
			})

			Convey("normalized", func() {
				tp := &TimePeriod{
					Time: &TimePeriod_Seconds{
						Seconds: 3600,
					},
				}
				So(tp.Normalize(), ShouldBeNil)
				So(tp.Time, ShouldHaveSameTypeAs, &TimePeriod_Seconds{})
				So(tp.Time.(*TimePeriod_Seconds).Seconds, ShouldEqual, 3600)
			})
		})
	})

	Convey("validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("invalid", func() {
			Convey("empty", func() {
				tp := &TimePeriod{}
				tp.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "duration or seconds is required")
			})

			Convey("duration", func() {
				tp := &TimePeriod{
					Time: &TimePeriod_Duration{
						Duration: "1yr",
					},
				}
				tp.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldErrLike, "duration must match regex")
			})

			Convey("seconds", func() {
				tp := &TimePeriod{
					Time: &TimePeriod_Seconds{
						Seconds: -1,
					},
				}
				tp.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "seconds must be positive")
			})
		})

		Convey("valid", func() {
			Convey("duration", func() {
				tp := &TimePeriod{
					Time: &TimePeriod_Duration{
						Duration: "1h",
					},
				}
				tp.Validate(c)
				So(c.Finalize(), ShouldBeNil)
			})

			Convey("seconds", func() {
				tp := &TimePeriod{
					Time: &TimePeriod_Seconds{
						Seconds: 3600,
					},
				}
				tp.Validate(c)
				So(c.Finalize(), ShouldBeNil)
			})
		})
	})
}
