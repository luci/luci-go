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
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUpdateBatch(t *testing.T) {
	t.Parallel()

	Convey("UpdateBatch works correctly", t, func() {
		epoch := testclock.TestRecentTimeLocal
		ctx := memory.Use(context.Background())
		ctx, tclock := testclock.UseTime(ctx, epoch)

		eid0, err := GobID("x-review.example.com", 10000)
		So(err, ShouldBeNil)
		cl0, err := eid0.GetOrInsert(ctx, func(cl *CL) {
			cl.Snapshot = makeSnapshot(epoch)
		})
		So(err, ShouldBeNil)

		eid1, err := GobID("x-review.example.com", 10001)
		So(err, ShouldBeNil)
		cl1, err := eid1.GetOrInsert(ctx, func(cl *CL) {
			cl.ApplicableConfig = makeApplicableConfig(epoch)
		})
		So(err, ShouldBeNil)

		tclock.Add(time.Hour)

		Convey("With existing CLs", func() {
			saved := false
			terr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				b, err := UpdateBatch(ctx, cl0.ID, cl1.ID)
				So(err, ShouldBeNil)
				So(b.CLs[0].ID, ShouldEqual, cl0.ID)
				So(b.CLs[0].Snapshot, ShouldResembleProto, cl0.Snapshot)
				So(b.CLs[1].ID, ShouldEqual, cl1.ID)
				So(b.CLs[1].ApplicableConfig, ShouldResembleProto, cl1.ApplicableConfig)

				b.CLs[1].Snapshot = b.CLs[0].Snapshot
				b.CLs[0].Snapshot = nil

				Convey("Must not modify EVersion", func() {
					b.CLs[0].EVersion += 100
					So(func() { b.Save(ctx) }, ShouldPanic)
				})
				Convey("First .Save works", func() {
					So(b.Save(ctx), ShouldBeNil)
					saved = true
					Convey("Second .Save panics", func() {
						So(func() { b.Save(ctx) }, ShouldPanic)
					})
				})
				return nil
			}, nil)
			So(terr, ShouldBeNil)

			// Re-load CLs.
			cl0, err = eid0.Get(ctx)
			So(err, ShouldBeNil)
			cl1, err = eid1.Get(ctx)
			So(err, ShouldBeNil)

			if !saved {
				So(cl0.EVersion, ShouldEqual, 1)
				So(cl1.EVersion, ShouldEqual, 1)
			} else {
				So(cl0.EVersion, ShouldEqual, 2)
				So(cl0.Snapshot, ShouldBeNil)
				So(cl0.UpdateTime, ShouldResemble, datastore.RoundTime(tclock.Now()).UTC())

				So(cl1.EVersion, ShouldEqual, 2)
				So(cl1.Snapshot, ShouldResembleProto, makeSnapshot(epoch))
				So(cl1.UpdateTime, ShouldResemble, datastore.RoundTime(tclock.Now()).UTC())
			}
		})

		Convey("Not exists is fatal", func() {
			const notExistingID = 404000404
			terr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				_, err = UpdateBatch(ctx, cl0.ID, notExistingID)
				So(err, ShouldErrLike, "1 out of 2 CLs don't exist")
				So(transient.Tag.In(err), ShouldBeFalse)
				return nil
			}, nil)
			So(terr, ShouldBeNil)
		})

		Convey("Invalid transactional usage", func() {
			Convey("Outside of transaction", func() {
				So(func() { UpdateBatch(ctx, cl0.ID, cl1.ID) }, ShouldPanic)
			})

			var b *BatchOp
			terr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				b, err = UpdateBatch(ctx, cl0.ID, cl1.ID)
				So(err, ShouldBeNil)
				return nil
			}, nil)
			So(terr, ShouldBeNil)
			So(b, ShouldNotBeNil)

			Convey("Save out of transactions", func() {
				So(func() { b.Save(ctx) }, ShouldPanic)
			})

			Convey("Crossing transactions", func() {
				So(func() {
					datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						b.Save(ctx)
						return nil
					}, nil)
				}, ShouldPanic)
			})
		})
	})
}
