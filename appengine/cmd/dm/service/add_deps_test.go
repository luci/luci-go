// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestAddDeps(t *testing.T) {
	t.Parallel()

	Convey("AddDeps", t, func() {
		c := memory.Use(context.Background())
		ds := datastore.Get(c)
		s := DungeonMaster{}

		a := &model.Attempt{AttemptID: *types.NewAttemptID("quest|fffffffe")}
		a.CurExecution = 1
		a.State = types.Executing
		ak := ds.KeyForObj(a)

		e := &model.Execution{ID: 1, Attempt: ak, ExecutionKey: []byte("key")}

		to := &model.Attempt{AttemptID: *types.NewAttemptID("to|fffffffe")}
		fwd := &model.FwdDep{Depender: ak, Dependee: to.AttemptID}

		req := &AddDepsReq{
			a.AttemptID,
			types.AttemptIDSlice{&to.AttemptID},
			[]byte("key"),
		}

		Convey("Bad", func() {
			Convey("No such originating attempt", func() {
				rsp, err := s.addDepsInternal(c, req)
				So(err, ShouldErrLike, "couldn't get attempt")
				So(rsp, ShouldBeNil)
			})

			Convey("No such destination quest", func() {
				So(ds.PutMulti([]interface{}{a, e}), ShouldBeNil)

				rsp, err := s.addDepsInternal(c, req)
				So(err, ShouldErrLike, `could not load quest "to"`)
				So(rsp, ShouldBeNil)
			})
		})

		Convey("Good", func() {
			So(ds.PutMulti([]interface{}{a, e}), ShouldBeNil)

			Convey("deps already exist", func() {
				So(ds.Put(fwd), ShouldBeNil)

				rsp, err := s.addDepsInternal(c, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleV, &AddDepsRsp{false})
			})

			Convey("deps already done", func() {
				to.State = types.Finished
				So(ds.Put(to), ShouldBeNil)

				rsp, err := s.addDepsInternal(c, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleV, &AddDepsRsp{false})

				So(ds.Get(fwd), ShouldBeNil)
			})

			Convey("adding new deps", func() {
				So(ds.Put(&model.Quest{ID: "to"}), ShouldBeNil)

				rsp, err := s.addDepsInternal(c, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleV, &AddDepsRsp{true})

				So(ds.Get(fwd), ShouldBeNil)
				So(ds.Get(a), ShouldBeNil)
				So(a.State, ShouldEqual, types.AddingDeps)
			})

		})
	})
}
