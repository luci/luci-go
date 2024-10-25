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

func TestSchedule(t *testing.T) {
	t.Parallel()

	ftt.Run("isSameDay", t, func(t *ftt.Test) {
		assert.Loosely(t, isSameDay(time.Sunday, dayofweek.DayOfWeek_SUNDAY), should.BeTrue)
		assert.Loosely(t, isSameDay(time.Monday, dayofweek.DayOfWeek_MONDAY), should.BeTrue)
		assert.Loosely(t, isSameDay(time.Friday, dayofweek.DayOfWeek_FRIDAY), should.BeTrue)
		assert.Loosely(t, isSameDay(time.Sunday, dayofweek.DayOfWeek_MONDAY), should.BeFalse)
		assert.Loosely(t, isSameDay(time.Monday, dayofweek.DayOfWeek_SUNDAY), should.BeFalse)
		assert.Loosely(t, isSameDay(time.Sunday, dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED), should.BeFalse)
		assert.Loosely(t, isSameDay(time.Monday, dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED), should.BeFalse)
		assert.Loosely(t, isSameDay(time.Friday, dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED), should.BeFalse)
	})

	ftt.Run("mostRecentStart", t, func(t *ftt.Test) {
		now := time.Time{}.Add(time.Hour * 12)
		assert.Loosely(t, now.Weekday(), should.Equal(time.Monday))
		assert.Loosely(t, now.Hour(), should.Equal(12))

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("day", func(t *ftt.Test) {
				s := &Schedule{
					Start: &TimeOfDay{
						Time: "1:00",
					},
				}
				_, err := s.mostRecentStart(now)
				assert.Loosely(t, err, should.ErrLike("day must be specified"))
			})

			t.Run("location", func(t *ftt.Test) {
				s := &Schedule{
					Start: &TimeOfDay{
						Day:      dayofweek.DayOfWeek_MONDAY,
						Location: "location",
						Time:     "1:00",
					},
				}
				_, err := s.mostRecentStart(now)
				assert.Loosely(t, err, should.ErrLike("invalid location"))
			})

			t.Run("time", func(t *ftt.Test) {
				s := &Schedule{
					Start: &TimeOfDay{
						Day:  dayofweek.DayOfWeek_MONDAY,
						Time: "24:00",
					},
				}
				_, err := s.mostRecentStart(now)
				assert.Loosely(t, err, should.ErrLike("time must not exceed"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("past", func(t *ftt.Test) {
				t.Run("different day", func(t *ftt.Test) {
					s := &Schedule{
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_SUNDAY,
							Time: "23:00",
						},
					}
					ts, err := s.mostRecentStart(now)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ts.Weekday(), should.Equal(time.Sunday))
					assert.Loosely(t, ts.Before(now), should.BeTrue)
				})

				t.Run("same day", func(t *ftt.Test) {
					s := &Schedule{
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_MONDAY,
							Time: "11:00",
						},
					}
					ts, err := s.mostRecentStart(now)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ts.Weekday(), should.Equal(time.Monday))
					assert.Loosely(t, ts.Before(now), should.BeTrue)
				})
			})

			t.Run("present", func(t *ftt.Test) {
				s := &Schedule{
					Start: &TimeOfDay{
						Day:  dayofweek.DayOfWeek_MONDAY,
						Time: "12:00",
					},
				}
				ts, err := s.mostRecentStart(now)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ts.Weekday(), should.Equal(time.Monday))
				assert.Loosely(t, ts.Equal(now), should.BeTrue)
			})

			t.Run("future", func(t *ftt.Test) {
				t.Run("same day", func(t *ftt.Test) {
					s := &Schedule{
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_MONDAY,
							Time: "13:00",
						},
					}
					ts, err := s.mostRecentStart(now)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ts.Weekday(), should.Equal(time.Monday))
					assert.Loosely(t, ts.Before(now), should.BeTrue)
				})

				t.Run("different day", func(t *ftt.Test) {
					s := &Schedule{
						Start: &TimeOfDay{
							Day:  dayofweek.DayOfWeek_TUESDAY,
							Time: "1:00",
						},
					}
					ts, err := s.mostRecentStart(now)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ts.Weekday(), should.Equal(time.Tuesday))
					assert.Loosely(t, ts.Before(now), should.BeTrue)
				})
			})
		})
	})

	ftt.Run("Validate", t, func(t *ftt.Test) {
		c := &validation.Context{Context: context.Background()}

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				s := &Schedule{}
				s.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("duration or seconds is required"))
				assert.Loosely(t, errs, should.UnwrapToErrStringLike("time must match regex"))
			})

			t.Run("min", func(t *ftt.Test) {
				s := &Schedule{
					Min: -1,
				}
				s.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.ErrLike("minimum amount must be non-negative"))
			})

			t.Run("max", func(t *ftt.Test) {
				s := &Schedule{
					Max: -1,
				}
				s.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.ErrLike("maximum amount must be non-negative"))
			})

			t.Run("min > max", func(t *ftt.Test) {
				s := &Schedule{
					Min: 2,
					Max: 1,
				}
				s.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.ErrLike("minimum amount must not exceed maximum amount"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			s := &Schedule{
				Min: 1,
				Max: 2,
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
			assert.Loosely(t, c.Finalize(), should.BeNil)
		})
	})
}
