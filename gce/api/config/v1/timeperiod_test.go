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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
)

func TestTimePeriod(t *testing.T) {
	t.Parallel()

	ftt.Run("normalize", t, func(t *ftt.Test) {
		t.Run("invalid", func(t *ftt.Test) {
			tp := &TimePeriod{
				Time: &TimePeriod_Duration{
					Duration: "-1h",
				},
			}
			assert.Loosely(t, tp.Normalize(), should.ErrLike("invalid duration"))
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				tp := &TimePeriod{}
				assert.Loosely(t, tp.Normalize(), should.BeNil)
				assert.Loosely(t, tp.Time, should.BeNil)
			})

			t.Run("normalizes", func(t *ftt.Test) {
				t.Run("zero", func(t *ftt.Test) {
					tp := &TimePeriod{
						Time: &TimePeriod_Duration{
							Duration: "0h",
						},
					}
					assert.Loosely(t, tp.Normalize(), should.BeNil)
					assert.Loosely(t, tp.Time, should.HaveType[*TimePeriod_Seconds])
					assert.Loosely(t, tp.Time.(*TimePeriod_Seconds).Seconds, should.BeZero)
				})

				t.Run("nonzero", func(t *ftt.Test) {
					tp := &TimePeriod{
						Time: &TimePeriod_Duration{
							Duration: "1h",
						},
					}
					assert.Loosely(t, tp.Normalize(), should.BeNil)
					assert.Loosely(t, tp.Time, should.HaveType[*TimePeriod_Seconds])
					assert.Loosely(t, tp.Time.(*TimePeriod_Seconds).Seconds, should.Equal(3600))
				})
			})

			t.Run("normalized", func(t *ftt.Test) {
				tp := &TimePeriod{
					Time: &TimePeriod_Seconds{
						Seconds: 3600,
					},
				}
				assert.Loosely(t, tp.Normalize(), should.BeNil)
				assert.Loosely(t, tp.Time, should.HaveType[*TimePeriod_Seconds])
				assert.Loosely(t, tp.Time.(*TimePeriod_Seconds).Seconds, should.Equal(3600))
			})
		})
	})

	ftt.Run("validate", t, func(t *ftt.Test) {
		c := &validation.Context{Context: context.Background()}

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("duration", func(t *ftt.Test) {
				tp := &TimePeriod{
					Time: &TimePeriod_Duration{
						Duration: "1yr",
					},
				}
				tp.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.ErrLike("duration must match regex"))
			})

			t.Run("seconds", func(t *ftt.Test) {
				tp := &TimePeriod{
					Time: &TimePeriod_Seconds{
						Seconds: -1,
					},
				}
				tp.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.ErrLike("seconds must be non-negative"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				tp := &TimePeriod{}
				tp.Validate(c)
				assert.Loosely(t, c.Finalize(), should.BeNil)
			})

			t.Run("duration", func(t *ftt.Test) {
				tp := &TimePeriod{
					Time: &TimePeriod_Duration{
						Duration: "1h",
					},
				}
				tp.Validate(c)
				assert.Loosely(t, c.Finalize(), should.BeNil)
			})

			t.Run("seconds", func(t *ftt.Test) {
				tp := &TimePeriod{
					Time: &TimePeriod_Seconds{
						Seconds: 3600,
					},
				}
				tp.Validate(c)
				assert.Loosely(t, c.Finalize(), should.BeNil)
			})
		})
	})
}
