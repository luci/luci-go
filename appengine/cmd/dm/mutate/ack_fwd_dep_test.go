// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/bit_field"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestAckFwdDep(t *testing.T) {
	t.Parallel()

	Convey("AckFwdDep", t, func() {
		c := memory.Use(context.Background())
		ds := datastore.Get(c)

		afd := &AckFwdDep{
			Dep: &model.FwdEdge{
				From: dm.NewAttemptID("quest", 1),
				To:   dm.NewAttemptID("to", 1),
			},
		}

		Convey("Root", func() {
			So(afd.Root(c), ShouldResemble, ds.MakeKey("Attempt", "quest|fffffffe"))
		})

		Convey("RollForward", func() {
			a, fwd := afd.Dep.Fwd(c)

			Convey("AddingDeps", func() {
				Convey("good", func() {
					a.State = dm.Attempt_AddingDeps
					a.AddingDepsBitmap = bf.Make(2)
					a.WaitingDepBitmap = bf.Make(2)
					So(ds.PutMulti([]interface{}{a, fwd}), ShouldBeNil)

					Convey("non-finished, not-last-adding", func() {
						muts, err := afd.RollForward(c)
						So(err, ShouldBeNil)
						So(muts, ShouldBeNil)

						So(ds.GetMulti([]interface{}{a, fwd}), ShouldBeNil)
						So(a.State, ShouldEqual, dm.Attempt_AddingDeps)
						So(a.AddingDepsBitmap.CountSet(), ShouldEqual, 1)
						So(a.WaitingDepBitmap.CountSet(), ShouldEqual, 0)
					})

					Convey("non-finished, last-adding", func() {
						a.AddingDepsBitmap.Set(1)
						So(ds.Put(a), ShouldBeNil)

						muts, err := afd.RollForward(c)
						So(err, ShouldBeNil)
						So(muts, ShouldBeNil)

						So(ds.GetMulti([]interface{}{a, fwd}), ShouldBeNil)
						So(a.State, ShouldEqual, dm.Attempt_Blocked)
						So(a.AddingDepsBitmap.CountSet(), ShouldEqual, 2)
						So(a.WaitingDepBitmap.CountSet(), ShouldEqual, 0)

						Convey("and then finished later", func() {
							// happens when we depend on an Attempt while it's not Finished,
							// but then it finishes later.

							afd.DepIsFinished = true

							muts, err := afd.RollForward(c)
							So(err, ShouldBeNil)
							So(muts, ShouldBeNil)

							So(ds.GetMulti([]interface{}{a, fwd}), ShouldBeNil)
							So(a.State, ShouldEqual, dm.Attempt_Blocked)
							So(a.AddingDepsBitmap.CountSet(), ShouldEqual, 2)
							So(a.WaitingDepBitmap.CountSet(), ShouldEqual, 1)
						})
					})

					Convey("finished, not-last-finished", func() {
						a.AddingDepsBitmap.Set(1)
						So(ds.Put(a), ShouldBeNil)

						afd.DepIsFinished = true

						muts, err := afd.RollForward(c)
						So(err, ShouldBeNil)
						So(muts, ShouldBeNil)

						So(ds.GetMulti([]interface{}{a, fwd}), ShouldBeNil)
						So(a.State, ShouldEqual, dm.Attempt_Blocked)
						So(a.AddingDepsBitmap.CountSet(), ShouldEqual, 2)
						So(a.WaitingDepBitmap.CountSet(), ShouldEqual, 1)
					})

					Convey("last-finished", func() {
						a.AddingDepsBitmap.Set(1)
						a.WaitingDepBitmap.Set(1)
						So(ds.Put(a), ShouldBeNil)

						afd.DepIsFinished = true

						muts, err := afd.RollForward(c)
						So(err, ShouldBeNil)
						So(muts, ShouldResemble, []tumble.Mutation{
							&ScheduleExecution{&a.ID}})

						So(ds.GetMulti([]interface{}{a, fwd}), ShouldBeNil)
						So(a.State, ShouldEqual, dm.Attempt_NeedsExecution)
						So(a.AddingDepsBitmap.CountSet(), ShouldEqual, 2)
						So(a.WaitingDepBitmap.CountSet(), ShouldEqual, 2)
					})
				})

				Convey("bad", func() {
					a.State = dm.Attempt_AddingDeps
					a.AddingDepsBitmap = bf.Make(2)
					a.WaitingDepBitmap = bf.Make(2)
					a.CurExecution = 1
					So(ds.PutMulti([]interface{}{a, fwd}), ShouldBeNil)

					Convey("CurExecution mismatch -> NOP", func() {
						muts, err := afd.RollForward(c)
						So(err, ShouldBeNil)
						So(muts, ShouldBeNil)

						So(ds.GetMulti([]interface{}{a, fwd}), ShouldBeNil)
						So(a.State, ShouldEqual, dm.Attempt_AddingDeps)
						So(a.AddingDepsBitmap.CountSet(), ShouldEqual, 0)
						So(a.WaitingDepBitmap.CountSet(), ShouldEqual, 0)
					})

					Convey("Missing data", func() {
						So(ds.Delete(ds.KeyForObj(a)), ShouldBeNil)

						_, err := afd.RollForward(c)
						So(err, ShouldErrLike, datastore.ErrNoSuchEntity)
					})
				})

			})
		})
	})
}
