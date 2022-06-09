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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

type testEntity struct {
	ID  int `gae:"$id"`
	Val int
}

func TestGetIgnoreMissing(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)

		for i := 1; i < 5; i++ {
			So(datastore.Put(ctx, &testEntity{ID: i, Val: i}), ShouldBeNil)
		}

		Convey("No missing", func() {
			exists := testEntity{ID: 1}
			So(GetIgnoreMissing(ctx, &exists), ShouldBeNil)
			So(exists.Val, ShouldEqual, 1)
		})

		Convey("Scalar args", func() {
			exists1 := testEntity{ID: 1}
			exists2 := testEntity{ID: 2}
			missing := testEntity{ID: 1000}

			Convey("One", func() {
				So(GetIgnoreMissing(ctx, &exists1), ShouldBeNil)
				So(exists1.Val, ShouldEqual, 1)

				So(GetIgnoreMissing(ctx, &missing), ShouldBeNil)
				So(missing.Val, ShouldEqual, 0)
			})

			Convey("Many", func() {
				So(GetIgnoreMissing(ctx, &exists1, &missing, &exists2), ShouldBeNil)
				So(exists1.Val, ShouldEqual, 1)
				So(missing.Val, ShouldEqual, 0)
				So(exists2.Val, ShouldEqual, 2)
			})
		})

		Convey("Vector args", func() {
			Convey("One", func() {
				ents := []testEntity{{ID: 1}, {ID: 1000}, {ID: 2}}

				So(GetIgnoreMissing(ctx, ents), ShouldBeNil)
				So(ents[0].Val, ShouldEqual, 1)
				So(ents[1].Val, ShouldEqual, 0)
				So(ents[2].Val, ShouldEqual, 2)
			})

			Convey("Many", func() {
				ents1 := []testEntity{{ID: 1}, {ID: 1000}, {ID: 2}}
				ents2 := []testEntity{{ID: 3}, {ID: 1001}, {ID: 4}}

				So(GetIgnoreMissing(ctx, ents1, ents2), ShouldBeNil)
				So(ents1[0].Val, ShouldEqual, 1)
				So(ents1[1].Val, ShouldEqual, 0)
				So(ents1[2].Val, ShouldEqual, 2)
				So(ents2[0].Val, ShouldEqual, 3)
				So(ents2[1].Val, ShouldEqual, 0)
				So(ents2[2].Val, ShouldEqual, 4)
			})
		})

		Convey("Mixed args", func() {
			exists := testEntity{ID: 1}
			missing := testEntity{ID: 1000}
			ents := []testEntity{{ID: 3}, {ID: 1001}, {ID: 4}}

			So(GetIgnoreMissing(ctx, &exists, &missing, ents), ShouldBeNil)
			So(exists.Val, ShouldEqual, 1)
			So(missing.Val, ShouldEqual, 0)
			So(ents[0].Val, ShouldEqual, 3)
			So(ents[1].Val, ShouldEqual, 0)
			So(ents[2].Val, ShouldEqual, 4)
		})
	})
}
