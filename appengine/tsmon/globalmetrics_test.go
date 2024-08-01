// Copyright 2016 The LUCI Authors.
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

package tsmon

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
)

func TestGlobalMetrics(t *testing.T) {
	t.Parallel()

	ftt.Run("Default version", t, func(t *ftt.Test) {
		c, _ := buildGAETestContext()
		tsmon.GetState(c).SetStore(store.NewInMemory(&target.Task{ServiceName: "default target"}))
		collectGlobalMetrics(c)
		tsmon.Flush(c)

		monitor := tsmon.GetState(c).Monitor().(*monitor.Fake)
		assert.Loosely(t, len(monitor.Cells), should.Equal(1))
		assert.Loosely(t, monitor.Cells[0][0].Name, should.Equal("appengine/default_version"))
		assert.Loosely(t, monitor.Cells[0][0].Value, should.Equal("testVersion1"))
	})
}
