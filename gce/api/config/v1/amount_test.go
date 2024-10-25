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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestAmount(t *testing.T) {
	t.Parallel()

	ftt.Run("getAmount", t, func(t *ftt.Test) {
		now := time.Time{}
		assert.Loosely(t, now.Weekday(), should.Equal(time.Monday))
		assert.Loosely(t, now.Hour(), should.BeZero)

		t.Run("min", func(t *ftt.Test) {
			a := &Amount{
				Min: 1,
				Max: 3,
			}
			n, err := a.getAmount(0, now)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(1))
		})

		t.Run("max", func(t *ftt.Test) {
			a := &Amount{
				Min: 1,
				Max: 3,
			}
			n, err := a.getAmount(4, now)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(3))
		})

		t.Run("proposed", func(t *ftt.Test) {
			a := &Amount{
				Min: 1,
				Max: 3,
			}
			n, err := a.getAmount(2, now)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))
		})

		t.Run("equal", func(t *ftt.Test) {
			a := &Amount{
				Min: 2,
				Max: 2,
			}
			n, err := a.getAmount(2, now)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))
		})

		t.Run("schedule", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))

			n, err = a.getAmount(5, now.Add(time.Minute*59))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))

			n, err = a.getAmount(5, now.Add(time.Hour))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(1))

			n, err = a.getAmount(5, now.Add(time.Hour*-1))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))

			n, err = a.getAmount(5, now.Add(time.Minute*-61))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(1))
		})

		t.Run("schedules", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))

			n, err = a.getAmount(5, now.Add(time.Minute*59))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))

			n, err = a.getAmount(5, now.Add(time.Hour))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(3))

			n, err = a.getAmount(5, now.Add(time.Minute*61))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(1))
		})
	})

	ftt.Run("Validate", t, func(t *ftt.Test) {
		c := &validation.Context{Context: context.Background()}

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("min", func(t *ftt.Test) {
				a := &Amount{
					Min: -1,
				}
				a.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("minimum amount must be non-negative"))
			})

			t.Run("max", func(t *ftt.Test) {
				a := &Amount{
					Max: -1,
				}
				a.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("maximum amount must be non-negative"))
			})

			t.Run("min > max", func(t *ftt.Test) {
				a := &Amount{
					Min: 2,
					Max: 1,
				}
				a.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("minimum amount must not exceed maximum amount"))
			})

			t.Run("schedule", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					a := &Amount{
						Change: []*Schedule{
							{},
						},
					}
					a.Validate(c)
					errs := c.Finalize().(*validation.Error).Errors
					assert.Loosely(t, errs, should.UnwrapToErrStringLike("duration or seconds is required"))
					assert.Loosely(t, errs, should.UnwrapToErrStringLike("time must match regex"))
				})

				t.Run("conflict", func(t *ftt.Test) {
					t.Run("same day", func(t *ftt.Test) {
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
						assert.Loosely(t, errs, should.UnwrapToErrStringLike("start time is before"))
					})

					t.Run("different day", func(t *ftt.Test) {
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
						assert.Loosely(t, errs, should.UnwrapToErrStringLike("start time is before"))
					})

					t.Run("different week", func(t *ftt.Test) {
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
						assert.Loosely(t, errs, should.UnwrapToErrStringLike("start time is before"))
					})
				})
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				a := &Amount{}
				a.Validate(c)
				assert.Loosely(t, c.Finalize(), should.BeNil)
			})

			t.Run("non-empty", func(t *ftt.Test) {
				a := &Amount{
					Min: 1,
					Max: 1,
				}
				a.Validate(c)
				assert.Loosely(t, c.Finalize(), should.BeNil)
			})

			t.Run("schedule", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					a := &Amount{
						Change: []*Schedule{},
					}
					a.Validate(c)
					assert.Loosely(t, c.Finalize(), should.BeNil)
				})

				t.Run("same day", func(t *ftt.Test) {
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
					assert.Loosely(t, c.Finalize(), should.BeNil)
				})

				t.Run("different day", func(t *ftt.Test) {
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
					assert.Loosely(t, c.Finalize(), should.BeNil)
				})

				t.Run("different week", func(t *ftt.Test) {
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
					assert.Loosely(t, c.Finalize(), should.BeNil)
				})

				t.Run("different location", func(t *ftt.Test) {
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
					assert.Loosely(t, c.Finalize(), should.BeNil)
				})
			})
		})
	})
}
