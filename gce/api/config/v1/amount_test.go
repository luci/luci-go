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

func TestAmount(t *testing.T) {
	t.Parallel()

	Convey("getAmount", t, func() {
		now := time.Time{}
		So(now.Weekday(), ShouldEqual, time.Monday)
		So(now.Hour(), ShouldEqual, 0)

		Convey("min", func() {
			a := &Amount{
				Min: 1,
				Max: 3,
			}
			n, err := a.getAmount(0, now)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 1)
		})

		Convey("max", func() {
			a := &Amount{
				Min: 1,
				Max: 3,
			}
			n, err := a.getAmount(4, now)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 3)
		})

		Convey("proposed", func() {
			a := &Amount{
				Min: 1,
				Max: 3,
			}
			n, err := a.getAmount(2, now)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 2)
		})

		Convey("equal", func() {
			a := &Amount{
				Min: 2,
				Max: 2,
			}
			n, err := a.getAmount(2, now)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 2)
		})

		Convey("schedule", func() {
			a := &Amount{
				Min: 1,
				Max: 1,
				Change: []*Schedule{
					{
						Min: 2,
						Max: 2,
						Length: &TimePeriod{
							Time: &TimePeriod_Duration{
								Duration: "2h",
							},
						},
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_SUNDAY,
							Time: "23:00",
						},
					},
				},
			}
			n, err := a.getAmount(5, now)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 2)

			n, err = a.getAmount(5, now.Add(time.Minute*59))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 2)

			n, err = a.getAmount(5, now.Add(time.Hour))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 1)

			n, err = a.getAmount(5, now.Add(time.Hour*-1))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 2)

			n, err = a.getAmount(5, now.Add(time.Minute*-61))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 1)
		})

		Convey("schedules", func() {
			a := &Amount{
				Min: 1,
				Max: 1,
				Change: []*Schedule{
					{
						Min: 2,
						Max: 2,
						Length: &TimePeriod{
							Time: &TimePeriod_Duration{
								Duration: "2h",
							},
						},
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_SUNDAY,
							Time: "23:00",
						},
					},
					{
						Min: 3,
						Max: 3,
						Length: &TimePeriod{
							Time: &TimePeriod_Duration{
								Duration: "1m",
							},
						},
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_MONDAY,
							Time: "1:00",
						},
					},
				},
			}
			n, err := a.getAmount(5, now)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 2)

			n, err = a.getAmount(5, now.Add(time.Minute*59))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 2)

			n, err = a.getAmount(5, now.Add(time.Hour))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 3)

			n, err = a.getAmount(5, now.Add(time.Minute*61))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 1)
		})
	})

	Convey("Validate", t, func() {
		c := &validation.Context{Context: context.Background()}

		Convey("invalid", func() {
			Convey("min", func() {
				a := &Amount{
					Min: -1,
				}
				a.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "minimum amount must be non-negative")
			})

			Convey("max", func() {
				a := &Amount{
					Max: -1,
				}
				a.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "maximum amount must be non-negative")
			})

			Convey("min > max", func() {
				a := &Amount{
					Min: 2,
					Max: 1,
				}
				a.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				So(errs, ShouldContainErr, "minimum amount must not exceed maximum amount")
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

			Convey("non-empty", func() {
				a := &Amount{
					Min: 1,
					Max: 1,
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
									Day:      dayofweek.DayOfWeek_TUESDAY,
									Location: "America/Los_Angeles",
									Time:     "1:00",
								},
							},
							{
								Length: &TimePeriod{
									Time: &TimePeriod_Duration{
										Duration: "2h",
									},
								},
								Start: &TimeOfDay{
									Day:      dayofweek.DayOfWeek_MONDAY,
									Location: "America/Los_Angeles",
									Time:     "1:00",
								},
							},
						},
					}
					a.Validate(c)
					So(c.Finalize(), ShouldBeNil)
				})

				Convey("different location", func() {
					a := &Amount{
						Change: []*Schedule{
							{
								Length: &TimePeriod{
									Time: &TimePeriod_Duration{
										Duration: "1h",
									},
								},
								Start: &TimeOfDay{
									Day:      dayofweek.DayOfWeek_MONDAY,
									Location: "America/Los_Angeles",
									Time:     "23:00",
								},
							},
							{
								Length: &TimePeriod{
									Time: &TimePeriod_Duration{
										Duration: "6h",
									},
								},
								Start: &TimeOfDay{
									Day:  dayofweek.DayOfWeek_TUESDAY,
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
