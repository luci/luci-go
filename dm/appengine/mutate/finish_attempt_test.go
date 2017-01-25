// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mutate

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logging/memlogger"
	"github.com/luci/luci-go/common/proto/google"
	dm "github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/dm/appengine/model"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFinishAttempt(t *testing.T) {
	t.Parallel()

	Convey("FinishAttempt", t, func() {
		c := memory.Use(context.Background())
		fa := &FinishAttempt{
			dm.FinishAttemptReq{
				Auth: &dm.Execution_Auth{
					Id:    dm.NewExecutionID("quest", 1, 1),
					Token: []byte("exekey"),
				},
				Data: dm.NewJsonResult(`{"result": true}`, testclock.TestTimeUTC),
			},
		}

		So(fa.Normalize(), ShouldBeNil)

		Convey("Root", func() {
			So(fa.Root(c).String(), ShouldEqual, `dev~app::/Attempt,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {
			a := &model.Attempt{
				ID:           *fa.Auth.Id.AttemptID(),
				State:        dm.Attempt_EXECUTING,
				CurExecution: 1,
			}
			ak := ds.KeyForObj(c, a)
			ar := &model.AttemptResult{Attempt: ak}
			e := &model.Execution{
				ID: 1, Attempt: ak, State: dm.Execution_RUNNING, Token: []byte("exekey")}

			So(ds.Put(c, a, e), ShouldBeNil)

			Convey("Good", func() {
				muts, err := fa.RollForward(c)
				memlogger.MustDumpStdout(c)
				So(err, ShouldBeNil)
				So(muts, ShouldBeEmpty)

				So(ds.Get(c, a, e, ar), ShouldBeNil)
				So(e.Token, ShouldBeEmpty)
				So(google.TimeFromProto(a.Result.Data.Expiration), ShouldResemble, testclock.TestTimeUTC)
				So(ar.Data.Object, ShouldResemble, `{"result":true}`)
			})

			Convey("Bad ExecutionKey", func() {
				fa.Auth.Token = []byte("wat")
				_, err := fa.RollForward(c)
				So(err, ShouldBeRPCPermissionDenied, "execution Auth")

				So(ds.Get(c, a, e), ShouldBeNil)
				So(e.Token, ShouldNotBeEmpty)
				So(a.State, ShouldEqual, dm.Attempt_EXECUTING)

				So(ds.Get(c, ar), ShouldEqual, ds.ErrNoSuchEntity)
			})

		})
	})
}
