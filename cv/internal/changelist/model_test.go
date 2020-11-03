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

package changelist

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGobMap(t *testing.T) {
	t.Parallel()

	Convey("ExternalID", t, func() {
		ctx := memory.Use(context.Background())

		eid, err := GobID("x-review.example.com", 12)
		Convey("GobID", func() {
			So(err, ShouldBeNil)
			_, err := GobID("https://example.com", 12)
			So(err, ShouldErrLike, "invalid host")
		})

		Convey("get not exists", func() {
			_, err := eid.get(ctx)
			So(err, ShouldResemble, datastore.ErrNoSuchEntity)
		})

		Convey("create", func() {
			cl, err := eid.getOrInsert(ctx, func(cl *changeList) {
				cl.Patchset = 10
				cl.MinEquivalentPatchset = 5
			})

			Convey("getOrInsert succeed", func() {
				So(err, ShouldBeNil)
				So(cl.ExternalID, ShouldResemble, eid)
				// ID must be autoset to non-0 value.
				So(cl.ID, ShouldNotEqual, 0)
				So(cl.Patchset, ShouldEqual, 10)
			})

			Convey("get exists", func() {
				cl2, err := eid.get(ctx)
				So(err, ShouldBeNil)
				// Can't use ShouldResemble on cl, cl2 because they will contain proto
				// data soon.
				// TODO(tandrii): s/will contain/contain.
				So(cl2.ID, ShouldEqual, cl.ID)
				So(cl2.ExternalID, ShouldEqual, eid)
				So(cl2.UpdateTime, ShouldEqual, cl.UpdateTime)
				So(cl2.Patchset, ShouldEqual, cl.Patchset)
			})

			Convey("getOrInsert already exists", func() {
				cl3, err := eid.getOrInsert(ctx, func(cl *changeList) {
					cl.Patchset = 999
				})
				So(err, ShouldBeNil)
				So(cl3.ID, ShouldEqual, cl.ID)
				So(cl3.ExternalID, ShouldResemble, eid)
				So(cl3.UpdateTime, ShouldEqual, cl.UpdateTime)
				So(cl3.Patchset, ShouldNotEqual, 999)
			})

			Convey("delete works", func() {
				err := Delete(ctx, cl.ID)
				So(err, ShouldBeNil)
				_, err = eid.get(ctx)
				So(err, ShouldResemble, datastore.ErrNoSuchEntity)
				So(datastore.Get(ctx, cl), ShouldResemble, datastore.ErrNoSuchEntity)

				Convey("delete is now noop", func() {
					err := Delete(ctx, cl.ID)
					So(err, ShouldBeNil)
				})
			})
		})
	})
}
