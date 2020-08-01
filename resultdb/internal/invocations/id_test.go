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

func TestIDSet(t *testing.T) {
	Convey(`IDSet`, t, func() {
		Convey(`Batches`, func() {
			ids := NewIDSet("0", "1", "2", "3", "4", "5")
			So(ids.batches(2), ShouldResemble, []IDSet{
				// Hashes:
				// 3: 4e07408562bedb8b60ce05c1decfe3ad16b72230967de01f640b7e4729b49fce
				// 4: 4b227777d4dd1fc61c6f884f48641d02b4d121d3fd328cb08b5531fcacdabf8a
				NewIDSet("3", "4"),
				// Hashes:
				// 0: 5feceb66ffc86f38d952786c6d696c79c2dbc239dd4e91b46729d73a27fb57e9
				// 1: 6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b
				NewIDSet("0", "1"),
				// Hashes:
				// 2: d4735e3a265e16eee03f59718b9b5d03019c07d8b6c51f90da3a666eec13ab35
				// 5: ef2d127de37b942baad06145e54b0c619a1f22327b2ebbcfbec78f5564afe39d
				NewIDSet("2", "5"),
			})
		})
	})
}

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
