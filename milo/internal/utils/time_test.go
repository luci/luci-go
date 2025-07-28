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

package utils

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFuncs(t *testing.T) {
	ftt.Run("Time Tests", t, func(t *ftt.Test) {
		t.Run("Interval", func(t *ftt.Test) {
			from := time.Date(2019, time.February, 3, 4, 5, 0, 0, time.UTC)
			to := time.Date(2019, time.February, 3, 4, 6, 0, 0, time.UTC)
			now := time.Date(2019, time.February, 3, 4, 7, 0, 0, time.UTC)
			t.Run("Not started", func(t *ftt.Test) {
				i := Interval{time.Time{}, time.Time{}, now}
				assert.Loosely(t, i.Started(), should.BeFalse)
				assert.Loosely(t, i.Ended(), should.BeFalse)
				assert.Loosely(t, i.Duration(), should.BeZero)
			})
			t.Run("Started, not ended", func(t *ftt.Test) {
				i := Interval{from, time.Time{}, now}
				assert.Loosely(t, i.Started(), should.BeTrue)
				assert.Loosely(t, i.Ended(), should.BeFalse)
				assert.Loosely(t, i.Duration(), should.Equal(2*time.Minute))
			})
			t.Run("Started and ended", func(t *ftt.Test) {
				i := Interval{from, to, now}
				assert.Loosely(t, i.Started(), should.BeTrue)
				assert.Loosely(t, i.Ended(), should.BeTrue)
				assert.Loosely(t, i.Duration(), should.Equal(1*time.Minute))
			})
			t.Run("Ended before started", func(t *ftt.Test) {
				i := Interval{to, from, now}
				assert.Loosely(t, i.Started(), should.BeTrue)
				assert.Loosely(t, i.Ended(), should.BeTrue)
				assert.Loosely(t, i.Duration(), should.BeZero)
			})
			t.Run("Ended, not started", func(t *ftt.Test) {
				i := Interval{time.Time{}, to, now}
				assert.Loosely(t, i.Started(), should.BeFalse)
				assert.Loosely(t, i.Ended(), should.BeTrue)
				assert.Loosely(t, i.Duration(), should.BeZero)
			})
		})

		t.Run("humanDuration", func(t *ftt.Test) {
			t.Run("3 hrs", func(t *ftt.Test) {
				h := HumanDuration(3 * time.Hour)
				assert.Loosely(t, h, should.Equal("3 hrs"))
			})

			t.Run("2 hrs 59 mins", func(t *ftt.Test) {
				h := HumanDuration(2*time.Hour + 59*time.Minute)
				assert.Loosely(t, h, should.Equal("2 hrs 59 mins"))
			})
		})
	})
}
