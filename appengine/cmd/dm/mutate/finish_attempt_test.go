// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"testing"
	"time"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/enums/attempt"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/appengine/tumble"
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
			*types.NewAttemptID("quest|fffffffe"),
			[]byte("exekey"),
			[]byte(`{"result": true}`),
			testclock.TestTimeUTC,
		}

		ds := datastore.Get(c)

		Convey("Root", func() {
			So(fa.Root(c).String(), ShouldEqual, `dev~app::/Attempt,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {
			a := &model.Attempt{
				AttemptID:    fa.ID,
				State:        attempt.Executing,
				CurExecution: 1,
			}
			ak := ds.KeyForObj(a)
			ar := &model.AttemptResult{Attempt: ak}
			e := &model.Execution{
				ID: 1, Attempt: ak, ExecutionKey: []byte("exekey")}

			So(ds.PutMulti([]interface{}{a, e}), ShouldBeNil)

			Convey("Good", func() {
				muts, err := fa.RollForward(c)
				So(err, ShouldBeNil)
				So(muts, ShouldResembleV, []tumble.Mutation{
					&RecordCompletion{For: a.AttemptID}})

				So(ds.GetMulti([]interface{}{a, e, ar}), ShouldBeNil)
				So(e.Done(), ShouldBeTrue)
				So(a.State, ShouldEqual, attempt.Finished)
				So(a.ResultExpiration, ShouldResembleV,
					testclock.TestTimeUTC.Round(time.Microsecond))
				So(ar.Data, ShouldResembleV, []byte(`{"result": true}`))
			})

			Convey("Bad ExecutionKey", func() {
				fa.ExecutionKey = []byte("wat")
				_, err := fa.RollForward(c)
				So(err, ShouldErrLike, "Incorrect ExecutionKey")

				So(ds.GetMulti([]interface{}{a, e}), ShouldBeNil)
				So(e.Done(), ShouldBeFalse)
				So(a.State, ShouldEqual, attempt.Executing)

				So(ds.Get(ar), ShouldEqual, datastore.ErrNoSuchEntity)
			})

		})
	})
}
