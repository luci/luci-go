// Copyright 2015 The LUCI Authors.
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

package mutate

import (
	"context"
	"testing"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/data/bit_field"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"
	"go.chromium.org/luci/tumble"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAckFwdDep(t *testing.T) {
	t.Parallel()

	Convey("AckFwdDep", t, func() {
		c := memory.Use(context.Background())

		afd := &AckFwdDep{
			Dep: &model.FwdEdge{
				From: dm.NewAttemptID("quest", 1),
				To:   dm.NewAttemptID("to", 1),
			},
		}

		Convey("Root", func() {
			So(afd.Root(c), ShouldResemble, ds.MakeKey(c, "Attempt", "quest|fffffffe"))
		})

		Convey("RollForward", func() {
			a, fwd := afd.Dep.Fwd(c)

			Convey("AddingDeps", func() {
				Convey("good", func() {
					a.State = dm.Attempt_WAITING
					a.DepMap = bit_field.Make(2)
					So(ds.Put(c, a, fwd), ShouldBeNil)

					Convey("not-last", func() {
						muts, err := afd.RollForward(c)
						So(err, ShouldBeNil)
						So(muts, ShouldBeNil)

						So(ds.Get(c, a, fwd), ShouldBeNil)
						So(a.State, ShouldEqual, dm.Attempt_WAITING)
						So(a.DepMap.CountSet(), ShouldEqual, 1)
					})

					Convey("last-finished", func() {
						a.DepMap.Set(1)
						So(ds.Put(c, a), ShouldBeNil)

						muts, err := afd.RollForward(c)
						So(err, ShouldBeNil)
						So(muts, ShouldResemble, []tumble.Mutation{
							&ScheduleExecution{&a.ID}})

						So(ds.Get(c, a, fwd), ShouldBeNil)
						So(a.State, ShouldEqual, dm.Attempt_SCHEDULING)
						So(a.DepMap.CountSet(), ShouldEqual, 0) // was reset
					})
				})

				Convey("bad", func() {
					a.State = dm.Attempt_WAITING
					a.DepMap = bit_field.Make(2)
					a.CurExecution = 1
					So(ds.Put(c, a, fwd), ShouldBeNil)

					Convey("CurExecution mismatch -> NOP", func() {
						muts, err := afd.RollForward(c)
						So(err, ShouldBeNil)
						So(muts, ShouldBeNil)

						So(ds.Get(c, a, fwd), ShouldBeNil)
						So(a.State, ShouldEqual, dm.Attempt_WAITING)
						So(a.DepMap.CountSet(), ShouldEqual, 0)
					})

					Convey("Missing data", func() {
						So(ds.Delete(c, ds.KeyForObj(c, a)), ShouldBeNil)

						_, err := afd.RollForward(c)
						So(err, ShouldErrLike, ds.ErrNoSuchEntity)
					})
				})

			})
		})
	})
}
