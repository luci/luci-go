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
	"time"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/types"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type metricDef struct {
	types.MetricInfo
	types.MetricMetadata
	fixedResetTime time.Time
}

func (m *metricDef) Info() types.MetricInfo         { return m.MetricInfo }
func (m *metricDef) Metadata() types.MetricMetadata { return m.MetricMetadata }
func (m *metricDef) SetFixedResetTime(t time.Time)  { m.fixedResetTime = t }

func TestAdd(t *testing.T) {
	t.Parallel()

	Convey("Add", t, func() {
		newMetric := func(name string, fns ...string) *metricDef {
			ret := &metricDef{MetricInfo: types.MetricInfo{Name: name}}
			for _, n := range fns {
				f := field.Field{Name: n, Type: field.IntType}
				ret.MetricInfo.Fields = append(ret.MetricInfo.Fields, f)
			}
			return ret
		}

		Convey("works", func() {
			Global.Add(newMetric("my/metric_/1"))
			Global.Add(newMetric("my/metriC/2", "field_1"))
			Global.Add(newMetric("my/metric-/3", "field_1", "field_2"))
			Global.Add(newMetric("/my/metric_/1"))
		})

		Convey("panics", func() {
			m := func(n string, fns ...string) func() {
				return func() { Global.Add(newMetric(n, fns...)) }
			}

			Convey("if metric name is invalid", func() {
				So(m(""), ShouldPanicLike, "empty metric name")
				So(m("/"), ShouldPanicLike, "invalid metric name")
				So(m("//"), ShouldPanicLike, "invalid metric name")
				So(m("meric//1"), ShouldPanicLike, "invalid metric name")
				So(m("meric!"), ShouldPanicLike, "invalid metric name")
				So(m("meric#//"), ShouldPanicLike, "invalid metric name")
			})

			Convey("if metric name is duplicate", func() {
				m("metric")()
				So(m("metric"), ShouldPanicLike, "duplicate metric name")
			})

			Convey("if field name is invalid", func() {
				So(m("metric/a", ""), ShouldPanicLike, "invalid field name")
				So(m("metric/a", "-"), ShouldPanicLike, "invalid field name")
				So(m("metric/a", "f-1"), ShouldPanicLike, "invalid field name")
				So(m("metric/a", "f#1"), ShouldPanicLike, "invalid field name")
				So(m("metric/a", "1_/"), ShouldPanicLike, "invalid field name")
			})
		})
	})
}
