// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"testing"
	"time"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestFinishAttempt(t *testing.T) {
	t.Parallel()

	Convey("FinishAttempt", t, func() {
		c := memory.Use(context.Background())
		fa := &FinishAttempt{
			&dm.Execution_Auth{
				Id:    dm.NewExecutionID("quest", 1, 1),
				Token: []byte("exekey"),
			},
			`{"result": true}`,
			testclock.TestTimeUTC,
		}

		ds := datastore.Get(c)

		Convey("Root", func() {
			So(fa.Root(c).String(), ShouldEqual, `dev~app::/Attempt,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {
			a := &model.Attempt{
				ID:           *fa.Auth.Id.AttemptID(),
				State:        dm.Attempt_EXECUTING,
				CurExecution: 1,
			}
			ak := ds.KeyForObj(a)
			ar := &model.AttemptResult{Attempt: ak}
			e := &model.Execution{
				ID: 1, Attempt: ak, State: dm.Execution_RUNNING, Token: []byte("exekey")}

			So(ds.PutMulti([]interface{}{a, e}), ShouldBeNil)

			Convey("Good", func() {
				muts, err := fa.RollForward(c)
				So(err, ShouldBeNil)
				So(muts, ShouldResemble, []tumble.Mutation{
					&RecordCompletion{For: &a.ID}})

				So(ds.GetMulti([]interface{}{a, e, ar}), ShouldBeNil)
				So(e.Token, ShouldBeEmpty)
				So(a.State, ShouldEqual, dm.Attempt_FINISHED)
				So(a.ResultExpiration, ShouldResemble,
					testclock.TestTimeUTC.Round(time.Microsecond))
				So(ar.Data, ShouldResemble, `{"result": true}`)
			})

			Convey("Bad ExecutionKey", func() {
				fa.Auth.Token = []byte("wat")
				_, err := fa.RollForward(c)
				So(err, ShouldBeRPCUnauthenticated, "execution Auth")

				So(ds.GetMulti([]interface{}{a, e}), ShouldBeNil)
				So(e.Token, ShouldNotBeEmpty)
				So(a.State, ShouldEqual, dm.Attempt_EXECUTING)

				So(ds.Get(ar), ShouldEqual, datastore.ErrNoSuchEntity)
			})

		})
	})
}
