// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestAddDeps(t *testing.T) {
	t.Parallel()

	Convey("AddDeps", t, func() {
		c := memory.Use(context.Background())
		ds := datastore.Get(c)
		s := &deps{}

		a := &model.Attempt{ID: *dm.NewAttemptID("quest", 1)}
		a.CurExecution = 1
		a.State = dm.Attempt_EXECUTING
		ak := ds.KeyForObj(a)

		e := &model.Execution{
			ID: 1, Attempt: ak, Token: []byte("key"),
			State: dm.Execution_RUNNING}

		to := &model.Attempt{ID: *dm.NewAttemptID("to", 1)}
		fwd := &model.FwdDep{Depender: ak, Dependee: to.ID}

		req := &dm.AddDepsReq{
			Auth: &dm.Execution_Auth{
				Id:    dm.NewExecutionID(a.ID.Quest, a.ID.Id, 1),
				Token: []byte("key"),
			},
			Deps: dm.NewAttemptList(map[string][]uint32{
				to.ID.Quest: {to.ID.Id},
			}),
		}

		Convey("Bad", func() {
			Convey("No such originating attempt", func() {
				rsp, err := s.AddDeps(c, req)
				So(err, ShouldBeRPCUnauthenticated, "execution Auth")
				So(rsp, ShouldBeNil)
			})

			Convey("No such destination quest", func() {
				So(ds.PutMulti([]interface{}{a, e}), ShouldBeNil)

				rsp, err := s.AddDeps(c, req)
				So(err, ShouldBeRPCInvalidArgument, "one or more quests")
				So(rsp, ShouldBeNil)
			})
		})

		Convey("Good", func() {
			So(ds.PutMulti([]interface{}{a, e}), ShouldBeNil)

			Convey("deps already exist", func() {
				So(ds.Put(fwd), ShouldBeNil)

				rsp, err := s.AddDeps(c, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &dm.AddDepsRsp{})
			})

			Convey("deps already done", func() {
				to.State = dm.Attempt_FINISHED
				So(ds.Put(to), ShouldBeNil)

				rsp, err := s.AddDeps(c, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &dm.AddDepsRsp{})

				So(ds.Get(fwd), ShouldBeNil)
			})

			Convey("adding new deps", func() {
				So(ds.Put(&model.Quest{ID: "to"}), ShouldBeNil)

				rsp, err := s.AddDeps(c, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResemble, &dm.AddDepsRsp{ShouldHalt: true})

				So(ds.Get(fwd), ShouldBeNil)
				So(ds.Get(a), ShouldBeNil)
				So(a.State, ShouldEqual, dm.Attempt_ADDING_DEPS)
			})

		})
	})
}
