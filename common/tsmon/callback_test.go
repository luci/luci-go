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
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/types"
)

func TestCallbacks(t *testing.T) {
	t.Parallel()

	ftt.Run("With a testing State", t, func(t *ftt.Test) {
		c := WithState(context.Background(), NewState())

		t.Run("Register global callback without metrics panics", func(t *ftt.Test) {
			assert.Loosely(t, func() {
				RegisterGlobalCallbackIn(c, func(context.Context) {})
			}, should.Panic)
		})

		t.Run("Callback is run on Flush", func(t *ftt.Test) {
			c, s, m := WithFakes(c)

			RegisterCallbackIn(c, func(ctx context.Context) {
				s.Cells = append(s.Cells, types.Cell{
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
				})
			})

			assert.Loosely(t, Flush(c), should.BeNil)

			assert.Loosely(t, len(m.Cells), should.Equal(1))
			assert.Loosely(t, len(m.Cells[0]), should.Equal(1))
			assert.Loosely(t, m.Cells[0][0], should.Resemble(types.Cell{
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
	})
}
