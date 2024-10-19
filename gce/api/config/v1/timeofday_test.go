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

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTimeOfDay(t *testing.T) {
	t.Parallel()

	ftt.Run("toTime", t, func(t *ftt.Test) {
		t.Run("invalid", func(t *ftt.Test) {
			t.Run("location", func(t *ftt.Test) {
				tod := &TimeOfDay{
					Location: "location",
					Time:     "1:23",
				}
				_, err := tod.toTime()
				assert.Loosely(t, err, should.ErrLike("invalid location"))
			})

			t.Run("time", func(t *ftt.Test) {
				t.Run("mismatch", func(t *ftt.Test) {
					tod := &TimeOfDay{
						Time: "1/1/1970",
					}
					_, err := tod.toTime()
					assert.Loosely(t, err, should.ErrLike("time must match regex"))
				})

				t.Run("overflow", func(t *ftt.Test) {
					t.Run("hour", func(t *ftt.Test) {
						tod := &TimeOfDay{
							Time: "24:00",
						}
						_, err := tod.toTime()
						assert.Loosely(t, err, should.ErrLike("time must not exceed"))
					})

					t.Run("minute", func(t *ftt.Test) {
						tod := &TimeOfDay{
							Time: "00:60",
						}
						_, err := tod.toTime()
						assert.Loosely(t, err, should.ErrLike("time must not exceed"))
					})
				})
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("location", func(t *ftt.Test) {
				t.Run("implicit", func(t *ftt.Test) {
					tod := &TimeOfDay{
						Time: "1:23",
					}
					rel, err := tod.toTime()
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rel.Location(), should.Equal(time.UTC))
				})

				t.Run("explicit", func(t *ftt.Test) {
					tod := &TimeOfDay{
						Location: "America/Los_Angeles",
						Time:     "1:23",
					}
					rel, err := tod.toTime()
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rel.Location().String(), should.Equal("America/Los_Angeles"))
				})
			})

			t.Run("time", func(t *ftt.Test) {
				t.Run("UTC", func(t *ftt.Test) {
					tod := &TimeOfDay{
						Time: "1:23",
					}
					rel, err := tod.toTime()
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rel.Hour(), should.Equal(1))
					assert.Loosely(t, rel.Minute(), should.Equal(23))
					assert.Loosely(t, rel.UTC().Hour(), should.Equal(1))
				})

				t.Run("MTV", func(t *ftt.Test) {
					tod := &TimeOfDay{
						Location: "America/Los_Angeles",
						Time:     "1:23",
					}
					rel, err := tod.toTime()
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rel.Hour(), should.Equal(1))
					assert.Loosely(t, rel.Minute(), should.Equal(23))
					assert.Loosely(t, rel.UTC().Hour(), should.NotEqual(1))
				})
			})
		})
	})

	ftt.Run("Validate", t, func(t *ftt.Test) {
		c := &validation.Context{Context: context.Background()}

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				tod := &TimeOfDay{}
				tod.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, convey.Adapt(ShouldContainErr)("time must match regex"))
				assert.Loosely(t, errs, convey.Adapt(ShouldContainErr)("day must be specified"))
			})

			t.Run("day", func(t *ftt.Test) {
				tod := &TimeOfDay{
					Day: dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED,
				}
				tod.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, convey.Adapt(ShouldContainErr)("day must be specified"))
			})

			t.Run("location", func(t *ftt.Test) {
				tod := &TimeOfDay{
					Location: "location",
				}
				tod.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, convey.Adapt(ShouldContainErr)("invalid location"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			tod := &TimeOfDay{
				Day:  dayofweek.DayOfWeek_MONDAY,
				Time: "1:23",
			}
			tod.Validate(c)
			assert.Loosely(t, c.Finalize(), should.BeNil)
		})
	})
}
