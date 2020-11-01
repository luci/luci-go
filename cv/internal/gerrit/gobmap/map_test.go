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

package gobmap

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/internal"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestUpdate(t *testing.T) {
	t.Parallel()

	Convey("Update", t, func() {
		ctx := memory.Use(context.Background())

		Convey("not implemented", func() {
			So(datastore.Put(ctx, &gobWatchMap{
				ID:      "tbd",
				Project: "test",
				RefSpecGroupMap: &internal.RefSpecGroupMap{
					Groups: []*internal.RefSpecGroupMap_Group{
						{Name: "abc"},
					},
				},
			}), ShouldBeNil)
			So(
				Update(ctx, "project-foo"),
				ShouldErrLike, "not implemented")

			i := gobWatchMap{ID: "tbd"}
			So(datastore.Get(ctx, &i), ShouldBeNil)
			So(len(i.RefSpecGroupMap.GetGroups()), ShouldEqual, 1)
		})
	})
}

func TestLookup(t *testing.T) {
	t.Parallel()

	Convey("Lookup", t, func() {
		ctx := memory.Use(context.Background())

		Convey("not implemented", func() {
			ids, err := Lookup(
				ctx, "foo-review.googlesource.com", "re/po", "main")
			So(ids, ShouldBeNil)
			So(err, ShouldErrLike, "not implemented")
		})
	})
}
