// Copyright 2020 The LUCI Authors.
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

package invocations

import (
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/spanutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSpannerConversion(t *testing.T) {
	t.Parallel()
	Convey(`ID`, t, func() {
		var b spanutil.Buffer

		Convey(`ID`, func() {
			id := ID("a")
			So(id.ToSpanner(), ShouldEqual, "ca978112:a")

			row, err := spanner.NewRow([]string{"a"}, []interface{}{id.ToSpanner()})
			So(err, ShouldBeNil)
			var actual ID
			err = b.FromSpanner(row, &actual)
			So(err, ShouldBeNil)
			So(actual, ShouldEqual, id)
		})

		Convey(`IDSet`, func() {
			ids := NewIDSet("a", "b")
			So(ids.ToSpanner(), ShouldResemble, []string{"3e23e816:b", "ca978112:a"})

			row, err := spanner.NewRow([]string{"a"}, []interface{}{ids.ToSpanner()})
			So(err, ShouldBeNil)
			var actual IDSet
			err = b.FromSpanner(row, &actual)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, ids)
		})
	})
}
