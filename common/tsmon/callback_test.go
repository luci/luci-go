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

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCallbacks(t *testing.T) {
	t.Parallel()

	Convey("With a testing State", t, func() {
		c := WithState(context.Background(), NewState())

		Convey("Register global callback without metrics panics", func() {
			So(func() {
				RegisterGlobalCallbackIn(c, func(context.Context) {})
			}, ShouldPanic)
		})

		Convey("Callback is run on Flush", func() {
			c, s, m := WithFakes(c)

			RegisterCallbackIn(c, func(c context.Context) {
				s.Cells = append(s.Cells, types.Cell{
					types.MetricInfo{
						Name:      "foo",
						Fields:    []field.Field{},
						ValueType: types.StringType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []interface{}{},
						ResetTime: time.Unix(1234, 1000),
						Value:     "bar",
					},
				})
			})

			So(Flush(c), ShouldBeNil)

			So(len(m.Cells), ShouldEqual, 1)
			So(len(m.Cells[0]), ShouldEqual, 1)
			So(m.Cells[0][0], ShouldResemble, types.Cell{
				types.MetricInfo{
					Name:      "foo",
					Fields:    []field.Field{},
					ValueType: types.StringType,
				},
				types.MetricMetadata{},
				types.CellData{
					FieldVals: []interface{}{},
					ResetTime: time.Unix(1234, 1000),
					Value:     "bar",
				},
			})
		})
	})
}
