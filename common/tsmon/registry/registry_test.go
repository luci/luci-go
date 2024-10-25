// Copyright 2023 The LUCI Authors.
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

package registry

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/types"
)

type metricDef struct {
	types.MetricInfo
	types.MetricMetadata
}

func (m *metricDef) Info() types.MetricInfo         { return m.MetricInfo }
func (m *metricDef) Metadata() types.MetricMetadata { return m.MetricMetadata }

func TestAdd(t *testing.T) {
	t.Parallel()

	ftt.Run("Add", t, func(t *ftt.Test) {
		newMetric := func(name string, fns ...string) *metricDef {
			ret := &metricDef{MetricInfo: types.MetricInfo{Name: name}}
			for _, n := range fns {
				f := field.Field{Name: n, Type: field.IntType}
				ret.MetricInfo.Fields = append(ret.MetricInfo.Fields, f)
			}
			return ret
		}

		t.Run("works", func(t *ftt.Test) {
			Global.Add(newMetric("my/metric_/1"))
			Global.Add(newMetric("my/metriC/2", "field_1"))
			Global.Add(newMetric("my/metric-/3", "field_1", "field_2"))
			Global.Add(newMetric("/my/metric_/1"))
		})

		t.Run("panics", func(t *ftt.Test) {
			m := func(n string, fns ...string) func() {
				return func() { Global.Add(newMetric(n, fns...)) }
			}

			t.Run("if metric name is invalid", func(t *ftt.Test) {
				assert.Loosely(t, m(""), should.PanicLike("empty metric name"))
				assert.Loosely(t, m("/"), should.PanicLike("invalid metric name"))
				assert.Loosely(t, m("//"), should.PanicLike("invalid metric name"))
				assert.Loosely(t, m("meric//1"), should.PanicLike("invalid metric name"))
				assert.Loosely(t, m("meric!"), should.PanicLike("invalid metric name"))
				assert.Loosely(t, m("meric#//"), should.PanicLike("invalid metric name"))
			})

			t.Run("if metric name is duplicate", func(t *ftt.Test) {
				m("metric")()
				assert.Loosely(t, m("metric"), should.PanicLike("duplicate metric name"))
			})

			t.Run("if field name is invalid", func(t *ftt.Test) {
				assert.Loosely(t, m("metric/a", ""), should.PanicLike("invalid field name"))
				assert.Loosely(t, m("metric/a", "-"), should.PanicLike("invalid field name"))
				assert.Loosely(t, m("metric/a", "f-1"), should.PanicLike("invalid field name"))
				assert.Loosely(t, m("metric/a", "f#1"), should.PanicLike("invalid field name"))
				assert.Loosely(t, m("metric/a", "1_/"), should.PanicLike("invalid field name"))
			})
		})
	})
}
