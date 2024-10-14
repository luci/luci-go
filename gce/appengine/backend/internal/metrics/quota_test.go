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

package metrics

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
)

func TestQuota(t *testing.T) {
	t.Parallel()

	ftt.Run("UpdateQuota", t, func(t *ftt.Test) {
		c, _ := tsmon.WithDummyInMemory(context.Background())
		s := tsmon.Store(c)

		fields := []any{"metric", "region", "project"}

		UpdateQuota(c, 100.0, 25.0, "metric", "region", "project")
		assert.Loosely(t, s.Get(c, quotaLimit, time.Time{}, fields).(float64), should.Equal(100.0))
		assert.Loosely(t, s.Get(c, quotaRemaining, time.Time{}, fields).(float64), should.Equal(75.0))
		assert.Loosely(t, s.Get(c, quotaUsage, time.Time{}, fields).(float64), should.Equal(25.0))

		UpdateQuota(c, 120.0, 40.0, "metric", "region", "project")
		assert.Loosely(t, s.Get(c, quotaLimit, time.Time{}, fields).(float64), should.Equal(120.0))
		assert.Loosely(t, s.Get(c, quotaRemaining, time.Time{}, fields).(float64), should.Equal(80.0))
		assert.Loosely(t, s.Get(c, quotaUsage, time.Time{}, fields).(float64), should.Equal(40.0))
	})
}
