// Copyright 2015 The LUCI Authors.
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
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
)

func TestFlush(t *testing.T) {
	t.Parallel()

	defaultTarget := &target.Task{ServiceName: "test"}

	ftt.Run("With a testing State", t, func(t *ftt.Test) {
		c := WithState(context.Background(), NewState())

		t.Run("Sends a metric", func(t *ftt.Test) {
			c, s, m := WithFakes(c)
			s.Cells = []types.Cell{
				{
					types.MetricInfo{
						Name:      "foo",
						Fields:    []field.Field{},
						ValueType: types.StringType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []any{},
						ResetTime: time.Unix(1234, 1000),
						Value:     "bar",
					},
				},
			}
			s.DT = defaultTarget
			m.CS = 42

			assert.Loosely(t, Flush(c), should.BeNil)

			assert.Loosely(t, len(m.Cells), should.Equal(1))
			assert.Loosely(t, len(m.Cells[0]), should.Equal(1))
			assert.Loosely(t, m.Cells[0][0], should.Match(types.Cell{
				types.MetricInfo{
					Name:      "foo",
					Fields:    []field.Field{},
					ValueType: types.StringType,
				},
				types.MetricMetadata{},
				types.CellData{
					FieldVals: []any{},
					ResetTime: time.Unix(1234, 1000),
					Value:     "bar",
				},
			}))
		})

		t.Run("Splits up ChunkSize metrics", func(t *ftt.Test) {
			c, s, m := WithFakes(c)
			s.Cells = make([]types.Cell, 43)
			s.DT = defaultTarget
			m.CS = 42

			for i := 0; i < 43; i++ {
				s.Cells[i] = types.Cell{
					types.MetricInfo{
						Name:      "foo",
						Fields:    []field.Field{},
						ValueType: types.StringType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []any{},
						ResetTime: time.Unix(1234, 1000),
						Value:     "bar",
					},
				}
			}

			assert.Loosely(t, Flush(c), should.BeNil)

			assert.Loosely(t, len(m.Cells), should.Equal(2))
			assert.Loosely(t, len(m.Cells[0]), should.Equal(42))
			assert.Loosely(t, len(m.Cells[1]), should.Equal(1))
		})

		t.Run("Doesn't split metrics when ChunkSize is 0", func(t *ftt.Test) {
			c, s, m := WithFakes(c)
			s.Cells = make([]types.Cell, 43)
			s.DT = defaultTarget
			m.CS = 0

			for i := 0; i < 43; i++ {
				s.Cells[i] = types.Cell{
					types.MetricInfo{
						Name:      "foo",
						Fields:    []field.Field{},
						ValueType: types.StringType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []any{},
						ResetTime: time.Unix(1234, 1000),
						Value:     "bar",
					},
				}
			}

			assert.Loosely(t, Flush(c), should.BeNil)

			assert.Loosely(t, len(m.Cells), should.Equal(1))
			assert.Loosely(t, len(m.Cells[0]), should.Equal(43))
		})

		t.Run("No Monitor configured", func(t *ftt.Test) {
			c, _, _ := WithFakes(c)
			state := GetState(c)
			state.SetMonitor(nil)

			assert.Loosely(t, Flush(c), should.NotBeNil)
		})

		t.Run("Auto flush works", func(t *ftt.Test) {
			start := time.Unix(1454561232, 0)
			c, tc := testclock.UseTime(c, start)
			tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				tc.Add(d)
			})

			moments := make(chan int)
			flusher := autoFlusher{
				flush: func(ctx context.Context) error {
					select {
					case <-ctx.Done():
					case moments <- int(clock.Now(ctx).Sub(start).Seconds()):
					}
					return nil
				},
			}

			flusher.start(c, time.Second)

			// Each 'flush' gets blocked on sending into 'moments'. Once unblocked, it
			// advances timer by 'interval' sec (1 sec in the test).
			assert.Loosely(t, <-moments, should.Equal(1))
			assert.Loosely(t, <-moments, should.Equal(2))
			// and so on ...

			// Doesn't timeout => works.
			flusher.stop()
		})
	})
}
