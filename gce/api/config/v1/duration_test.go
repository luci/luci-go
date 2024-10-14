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

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDuration(t *testing.T) {
	t.Parallel()

	ftt.Run("ToSeconds", t, func(t *ftt.Test) {
		t.Run("invalid", func(t *ftt.Test) {
			t.Run("mismatch", func(t *ftt.Test) {
				d := &TimePeriod_Duration{
					Duration: "1yr",
				}
				_, err := d.ToSeconds()
				assert.Loosely(t, err, should.ErrLike("duration must match regex"))
			})

			t.Run("overflow", func(t *ftt.Test) {
				d := &TimePeriod_Duration{
					Duration: "9223372036854775808s",
				}
				_, err := d.ToSeconds()
				assert.Loosely(t, err, should.ErrLike("duration must not exceed"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("converts", func(t *ftt.Test) {
				d := &TimePeriod_Duration{
					Duration: "1h",
				}
				n, err := d.ToSeconds()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, n, should.Equal(3600))
			})

			t.Run("clamps", func(t *ftt.Test) {
				d := &TimePeriod_Duration{
					Duration: "9223372036854775807mo",
				}
				n, err := d.ToSeconds()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, n, should.Equal(int64(math.MaxInt64)))
			})
		})
	})

	ftt.Run("Validate", t, func(t *ftt.Test) {
		c := &validation.Context{Context: context.Background()}

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				d := &TimePeriod_Duration{}
				d.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, convey.Adapt(ShouldContainErr)("duration must match regex"))
			})

			t.Run("invalid", func(t *ftt.Test) {
				d := &TimePeriod_Duration{
					Duration: "1yr",
				}
				d.Validate(c)
				errs := c.Finalize().(*validation.Error).Errors
				assert.Loosely(t, errs, should.ErrLike("duration must match regex"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			d := &TimePeriod_Duration{
				Duration: "1h",
			}
			d.Validate(c)
			assert.Loosely(t, c.Finalize(), should.BeNil)
		})
	})
}
