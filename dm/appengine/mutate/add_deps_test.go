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
	"testing"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	dm "go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAddDeps(t *testing.T) {
	t.Parallel()

	Convey("AddDeps", t, func() {
		c := memory.Use(context.Background())

		aid := dm.NewAttemptID("quest", 1)
		a := model.MakeAttempt(c, aid)
		a.CurExecution = 1
		a.State = dm.Attempt_EXECUTING
		ex := &model.Execution{
			ID: 1, Attempt: ds.KeyForObj(c, a), Token: []byte("sup"),
			State: dm.Execution_RUNNING}

		ad := &AddDeps{
			Auth: &dm.Execution_Auth{
				Id:    dm.NewExecutionID("quest", 1, 1),
				Token: []byte("sup"),
			},
			Deps: dm.NewAttemptList(map[string][]uint32{
				"to":    {1, 2, 3},
				"top":   {1},
				"tp":    {1},
				"zebra": {17},
			}),
		}
		fds := model.FwdDepsFromList(c, aid, ad.Deps)

		Convey("Root", func() {
			So(ad.Root(c).String(), ShouldEqual, `dev~app::/Attempt,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {

			Convey("Bad", func() {
				Convey("Bad ExecutionKey", func() {
					So(ds.Put(c, a, ex), ShouldBeNil)

					ad.Auth.Token = []byte("nerp")
					muts, err := ad.RollForward(c)
					So(err, ShouldBeRPCPermissionDenied, "execution Auth")
					So(muts, ShouldBeEmpty)
				})
			})

			Convey("Good", func() {
				So(ds.Put(c, a, ex), ShouldBeNil)

				Convey("All added already", func() {
					So(ds.Put(c, fds), ShouldBeNil)

					muts, err := ad.RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeEmpty)
				})

				Convey("None added already", func() {
					muts, err := ad.RollForward(c)
					So(err, ShouldBeNil)
					So(len(muts), ShouldEqual, len(fds))

					So(muts[0], ShouldResemble, &AddBackDep{
						Dep: fds[0].Edge(), NeedsAck: true})

					So(ds.Get(c, a, fds), ShouldBeNil)
					So(a.DepMap.Size(), ShouldEqual, len(fds))
					So(a.State, ShouldEqual, dm.Attempt_EXECUTING)
					So(fds[0].ForExecution, ShouldEqual, 1)

					muts, err = (&FinishExecution{
						ad.Auth.Id, &dm.Result{Data: dm.NewJsonResult(`{"hi": true}`)},
					}).RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeNil)

					So(ds.Get(c, a), ShouldBeNil)
					So(a.State, ShouldEqual, dm.Attempt_WAITING)
				})

				Convey("adding new Attempts at the same time", func() {
					ad.Attempts = dm.NewAttemptList(map[string][]uint32{
						"to": {2, 3},
						"tp": {1},
					})

					muts, err := ad.RollForward(c)
					So(err, ShouldBeNil)
					So(len(muts), ShouldEqual, len(fds)+3)

					So(muts[0], ShouldResemble, &EnsureAttempt{dm.NewAttemptID("to", 3)})
					So(muts[1], ShouldResemble, &AddBackDep{
						Dep: fds[0].Edge(), NeedsAck: true})

					So(ds.Get(c, a, fds), ShouldBeNil)
					So(a.DepMap.Size(), ShouldEqual, len(fds))
					So(a.State, ShouldEqual, dm.Attempt_EXECUTING)
					So(fds[0].ForExecution, ShouldEqual, 1)

					muts, err = (&FinishExecution{
						ad.Auth.Id, &dm.Result{Data: dm.NewJsonResult(`{"hi":true}`)},
					}).RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeNil)

					So(ds.Get(c, a), ShouldBeNil)
					So(a.State, ShouldEqual, dm.Attempt_WAITING)
				})
			})
		})
	})
}
