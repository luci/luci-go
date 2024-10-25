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
	"math"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
)

func TestTimes(t *testing.T) {
	t.Parallel()

	ftt.Run("ReportCreationTime", t, func(t *ftt.Test) {
		c, _ := tsmon.WithDummyInMemory(context.Background())
		s := tsmon.Store(c)

		fields := []any{"prefix", "project", "zone"}

		ReportCreationTime(c, 60.0, "prefix", "project", "zone")
		d := s.Get(c, creationTime, fields).(*distribution.Distribution)
		assert.Loosely(t, d.Count(), should.Equal(1))
		assert.Loosely(t, d.Sum(), should.Equal(60.0))

		ReportCreationTime(c, 120.0, "prefix", "project", "zone")
		d = s.Get(c, creationTime, fields).(*distribution.Distribution)
		assert.Loosely(t, d.Count(), should.Equal(2))
		assert.Loosely(t, d.Sum(), should.Equal(180.0))

		ReportCreationTime(c, math.Inf(1), "prefix", "project", "zone")
		d = s.Get(c, creationTime, fields).(*distribution.Distribution)
		assert.Loosely(t, d.Count(), should.Equal(3))
		assert.Loosely(t, d.Sum(), should.Equal(math.Inf(1)))
	})

	ftt.Run("ReportConnectionTime", t, func(t *ftt.Test) {
		c, _ := tsmon.WithDummyInMemory(context.Background())
		s := tsmon.Store(c)

		fields := []any{"prefix", "project", "server", "zone"}

		ReportConnectionTime(c, 120.0, "prefix", "project", "server", "zone")
		d := s.Get(c, connectionTime, fields).(*distribution.Distribution)
		assert.Loosely(t, d.Count(), should.Equal(1))
		assert.Loosely(t, d.Sum(), should.Equal(120.0))

		ReportConnectionTime(c, 180.0, "prefix", "project", "server", "zone")
		d = s.Get(c, connectionTime, fields).(*distribution.Distribution)
		assert.Loosely(t, d.Count(), should.Equal(2))
		assert.Loosely(t, d.Sum(), should.Equal(300.0))

		ReportConnectionTime(c, math.Inf(1), "prefix", "project", "server", "zone")
		d = s.Get(c, connectionTime, fields).(*distribution.Distribution)
		assert.Loosely(t, d.Count(), should.Equal(3))
		assert.Loosely(t, d.Sum(), should.Equal(math.Inf(1)))
	})

	ftt.Run("ReportOnlineTime", t, func(t *ftt.Test) {
		c, _ := tsmon.WithDummyInMemory(context.Background())
		s := tsmon.Store(c)

		fields := []any{"prefix", "project", "resource_group", "server", "zone"}

		ReportBotConnectionTime(c, 120.0, "prefix", "project", "resource_group", "server", "zone")
		d := s.Get(c, botConnectionTime, fields).(*distribution.Distribution)
		assert.Loosely(t, d.Count(), should.Equal(1))
		assert.Loosely(t, d.Sum(), should.Equal(120.0))

		ReportBotConnectionTime(c, 180.0, "prefix", "project", "resource_group", "server", "zone")
		d = s.Get(c, botConnectionTime, fields).(*distribution.Distribution)
		assert.Loosely(t, d.Count(), should.Equal(2))
		assert.Loosely(t, d.Sum(), should.Equal(300.0))

		ReportBotConnectionTime(c, math.Inf(1), "prefix", "project", "resource_group", "server", "zone")
		d = s.Get(c, botConnectionTime, fields).(*distribution.Distribution)
		assert.Loosely(t, d.Count(), should.Equal(3))
		assert.Loosely(t, d.Sum(), should.Equal(math.Inf(1)))
	})
}
