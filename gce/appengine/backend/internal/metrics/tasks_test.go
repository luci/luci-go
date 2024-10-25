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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestTasks(t *testing.T) {
	t.Parallel()

	ftt.Run("Tasks", t, func(t *ftt.Test) {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		s := tsmon.Store(c)

		t.Run("TaskCount", func(t *ftt.Test) {
			tc := &TaskCount{}
			assert.Loosely(t, tc.Executing, should.BeZero)
			assert.Loosely(t, tc.Total, should.BeZero)
			assert.Loosely(t, tc.Update(c, "queue", 1, 2), should.BeNil)

			tc = &TaskCount{
				ID: "queue",
			}
			assert.Loosely(t, datastore.Get(c, tc), should.BeNil)
			assert.Loosely(t, tc.Executing, should.Equal(1))
			assert.Loosely(t, tc.Total, should.Equal(2))
			assert.Loosely(t, tc.Queue, should.Equal(tc.ID))
		})

		t.Run("updateTasks", func(t *ftt.Test) {
			fields := []any{"queue"}

			tc := &TaskCount{
				ID:        "queue",
				Executing: 1,
				Queue:     "queue",
				Total:     3,
			}

			tc.Computed = time.Time{}
			assert.Loosely(t, datastore.Put(c, tc), should.BeNil)
			updateTasks(c)
			assert.Loosely(t, s.Get(c, tasksExecuting, fields), should.BeNil)
			assert.Loosely(t, s.Get(c, tasksPending, fields), should.BeNil)
			assert.Loosely(t, s.Get(c, tasksTotal, fields), should.BeNil)
			assert.Loosely(t, datastore.Get(c, &TaskCount{
				ID: tc.ID,
			}), should.Equal(datastore.ErrNoSuchEntity))

			tc.Computed = time.Now().UTC()
			assert.Loosely(t, datastore.Put(c, tc), should.BeNil)
			updateTasks(c)
			assert.Loosely(t, s.Get(c, tasksExecuting, fields).(int64), should.Equal(1))
			assert.Loosely(t, s.Get(c, tasksPending, fields).(int64), should.Equal(2))
			assert.Loosely(t, s.Get(c, tasksTotal, fields).(int64), should.Equal(3))
			assert.Loosely(t, datastore.Get(c, &TaskCount{
				ID: tc.ID,
			}), should.BeNil)
		})
	})
}
