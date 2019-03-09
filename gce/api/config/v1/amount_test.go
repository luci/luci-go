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
	"sort"
	"testing"

	"google.golang.org/genproto/googleapis/type/dayofweek"

	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAmount(t *testing.T) {
	t.Parallel()

	Convey("Sort", t, func() {
		Convey("empty", func() {
			s := make(indexedSchedules, 0)
			sort.Sort(s)
			So(s, ShouldBeEmpty)
		})

		Convey("day", func() {
			Convey("sorted", func() {
				s := indexedSchedules{
					{
						index: 0,
						Schedule: &Schedule{
							Start: &TimeOfDay{
								Day: dayofweek.DayOfWeek_MONDAY,
							},
						},
					},
					{
						index: 1,
						Schedule: &Schedule{
							Start: &TimeOfDay{
								Day: dayofweek.DayOfWeek_TUESDAY,
							},
						},
					},
				}
				sort.Sort(s)
				So(s[0].Schedule.Start.Day, ShouldEqual, dayofweek.DayOfWeek_MONDAY)
				So(s[1].Schedule.Start.Day, ShouldEqual, dayofweek.DayOfWeek_TUESDAY)
			})

			Convey("unsorted", func() {
				s := indexedSchedules{
					{
						index: 0,
						Schedule: &Schedule{
							Start: &TimeOfDay{
								Day: dayofweek.DayOfWeek_TUESDAY,
							},
						},
					},
					{
						index: 1,
						Schedule: &Schedule{
							Start: &TimeOfDay{
								Day: dayofweek.DayOfWeek_MONDAY,
							},
						},
					},
				}
				sort.Sort(s)
				So(s[0].Schedule.Start.Day, ShouldEqual, dayofweek.DayOfWeek_MONDAY)
				So(s[1].Schedule.Start.Day, ShouldEqual, dayofweek.DayOfWeek_TUESDAY)
			})
		})

		Convey("time", func() {
			Convey("sorted", func() {
				s := indexedSchedules{
					{
						index: 0,
						Schedule: &Schedule{
							Start: &TimeOfDay{
								Time: "1:00",
							},
						},
					},
					{
						index: 1,
						Schedule: &Schedule{
							Start: &TimeOfDay{
								Time: "2:00",
							},
						},
					},
				}
				sort.Sort(s)
				So(s[0].Schedule.Start.Time, ShouldEqual, "1:00")
				So(s[1].Schedule.Start.Time, ShouldEqual, "2:00")
			})

			Convey("unsorted", func() {
				s := indexedSchedules{
					{
						index: 0,
						Schedule: &Schedule{
							Start: &TimeOfDay{
								Time: "2:00",
							},
						},
					},
					{
						index: 1,
						Schedule: &Schedule{
							Start: &TimeOfDay{
								Time: "1:00",
							},
						},
					},
				}
				sort.Sort(s)
				So(s[0].Schedule.Start.Time, ShouldEqual, "1:00")
				So(s[1].Schedule.Start.Time, ShouldEqual, "2:00")
			})
		})
	})

	Convey("Validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("invalid", func() {
			Convey("default", func() {
				a := &Amount{
					Default: -1,
				}
				a.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "default amount must be non-negative")
			})

			Convey("schedule", func() {
				Convey("empty", func() {
					a := &Amount{
						Change: []*Schedule{
							{},
						},
					}
					a.Validate(c)
					errs := c.Finalize().(*validation.Error).Errors
					So(errs, ShouldContainErr, "duration or seconds is required")
					So(errs, ShouldContainErr, "time must match regex")
				})

				Convey("negative", func() {
					a := &Amount{
						Change: []*Schedule{
							{
								Amount: -1,
							},
						},
					}
					a.Validate(c)
					errs := c.Finalize().(*validation.Error).Errors
					So(errs, ShouldContainErr, "amount must be non-negative")
				})

				Convey("conflict", func() {
					Convey("same day", func() {
						a := &Amount{
							Change: []*Schedule{
								{
									Length: &TimePeriod{
										Time: &TimePeriod_Duration{
											Duration: "2h",
										},
									},
									Start: &TimeOfDay{
										Day:  dayofweek.DayOfWeek_WEDNESDAY,
										Time: "1:00",
									},
								},
								{
									Start: &TimeOfDay{
										Day:  dayofweek.DayOfWeek_WEDNESDAY,
										Time: "2:59",
									},
								},
							},
						}
						a.Validate(c)
						errs := c.Finalize().(*validation.Error).Errors
						So(errs, ShouldContainErr, "start time is before")
					})

					Convey("different day", func() {
						a := &Amount{
							Change: []*Schedule{
								{
									Length: &TimePeriod{
										Time: &TimePeriod_Duration{
											Duration: "48h",
										},
									},
									Start: &TimeOfDay{
										Day:  dayofweek.DayOfWeek_WEDNESDAY,
										Time: "1:00",
									},
								},
								{
									Start: &TimeOfDay{
										Day:  dayofweek.DayOfWeek_THURSDAY,
										Time: "1:00",
									},
								},
							},
						}
						a.Validate(c)
						errs := c.Finalize().(*validation.Error).Errors
						So(errs, ShouldContainErr, "start time is before")
					})

					Convey("different week", func() {
						a := &Amount{
							Change: []*Schedule{
								{
									Length: &TimePeriod{
										Time: &TimePeriod_Duration{
											Duration: "6d",
										},
									},
									Start: &TimeOfDay{
										Day:  dayofweek.DayOfWeek_TUESDAY,
										Time: "1:00",
									},
								},
								{
									Start: &TimeOfDay{
										Day:  dayofweek.DayOfWeek_MONDAY,
										Time: "0:59",
									},
								},
							},
						}
						a.Validate(c)
						errs := c.Finalize().(*validation.Error).Errors
						So(errs, ShouldContainErr, "start time is before")
					})
				})
			})
		})

		Convey("valid", func() {
			Convey("empty", func() {
				a := &Amount{}
				a.Validate(c)
				So(c.Finalize(), ShouldBeNil)
			})

			Convey("default", func() {
				a := &Amount{
					Default: 1,
				}
				a.Validate(c)
				So(c.Finalize(), ShouldBeNil)
			})

			Convey("schedule", func() {
				Convey("empty", func() {
					a := &Amount{
						Change: []*Schedule{},
					}
					a.Validate(c)
					So(c.Finalize(), ShouldBeNil)
				})

				Convey("same day", func() {
					a := &Amount{
						Change: []*Schedule{
							{
								Length: &TimePeriod{
									Time: &TimePeriod_Duration{
										Duration: "2h",
									},
								},
								Start: &TimeOfDay{
									Day:  dayofweek.DayOfWeek_WEDNESDAY,
									Time: "1:00",
								},
							},
							{
								Length: &TimePeriod{
									Time: &TimePeriod_Duration{
										Duration: "2h",
									},
								},
								Start: &TimeOfDay{
									Day:  dayofweek.DayOfWeek_WEDNESDAY,
									Time: "3:00",
								},
							},
						},
					}
					a.Validate(c)
					So(c.Finalize(), ShouldBeNil)
				})

				Convey("different day", func() {
					a := &Amount{
						Change: []*Schedule{
							{
								Length: &TimePeriod{
									Time: &TimePeriod_Duration{
										Duration: "48h",
									},
								},
								Start: &TimeOfDay{
									Day:  dayofweek.DayOfWeek_WEDNESDAY,
									Time: "1:00",
								},
							},
							{
								Length: &TimePeriod{
									Time: &TimePeriod_Duration{
										Duration: "48h",
									},
								},
								Start: &TimeOfDay{
									Day:  dayofweek.DayOfWeek_FRIDAY,
									Time: "1:00",
								},
							},
						},
					}
					a.Validate(c)
					So(c.Finalize(), ShouldBeNil)
				})

				Convey("different week", func() {
					a := &Amount{
						Change: []*Schedule{
							{
								Length: &TimePeriod{
									Time: &TimePeriod_Duration{
										Duration: "6d",
									},
								},
								Start: &TimeOfDay{
									Day:  dayofweek.DayOfWeek_TUESDAY,
									Time: "1:00",
								},
							},
							{
								Length: &TimePeriod{
									Time: &TimePeriod_Duration{
										Duration: "2h",
									},
								},
								Start: &TimeOfDay{
									Day:  dayofweek.DayOfWeek_MONDAY,
									Time: "1:00",
								},
							},
						},
					}
					a.Validate(c)
					So(c.Finalize(), ShouldBeNil)
				})
			})
		})
	})
}
