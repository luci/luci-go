// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/enums/attempt"
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

		a := &model.Attempt{
			AttemptID:    *types.NewAttemptID("quest|fffffffe"),
			State:        attempt.Executing,
			CurExecution: 1,
		}
		ak := ds.KeyForObj(a)
		ex := &model.Execution{
			ID: 1, Attempt: ds.KeyForObj(a), ExecutionKey: []byte("sup")}
		fds := []*model.FwdDep{
			{Depender: ak, Dependee: *types.NewAttemptID("to|fffffffe")},
			{Depender: ak, Dependee: *types.NewAttemptID("to|fffffffd")},
			{Depender: ak, Dependee: *types.NewAttemptID("to|fffffffc")},
			{Depender: ak, Dependee: *types.NewAttemptID("top|fffffffe")},
			{Depender: ak, Dependee: *types.NewAttemptID("tp|fffffffe")},
			{Depender: ak, Dependee: *types.NewAttemptID("zebra|ffffffee")},
		}
		ad := &AddDeps{
			ToAdd: &model.AttemptFanout{
				Base:  &a.AttemptID,
				Edges: make(types.AttemptIDSlice, len(fds)),
			},
			ExecutionKey: []byte("sup"),
		}
		for i, fd := range fds {
			fd.BitIndex = types.UInt32(i)
			fd.ForExecution = 1
			ad.ToAdd.Edges[i] = &fd.Dependee
		}

		Convey("Root", func() {
			So(ad.Root(c).String(), ShouldEqual, `dev~app::/Attempt,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {

			Convey("Bad", func() {
				Convey("Bad ExecutionKey", func() {
					So(ds.PutMulti([]interface{}{a, ex}), ShouldBeNil)

					ad.ExecutionKey = []byte("nerp")
					muts, err := ad.RollForward(c)
					So(err, ShouldErrLike, "Incorrect ExecutionKey")
					So(muts, ShouldBeEmpty)
				})
			})

			Convey("Good", func() {
				So(ds.PutMulti([]interface{}{a, ex}), ShouldBeNil)

				Convey("All added already", func() {
					So(ds.PutMulti(fds), ShouldBeNil)

					muts, err := ad.RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeEmpty)
				})

				Convey("None added already", func() {
					muts, err := ad.RollForward(c)
					So(err, ShouldBeNil)
					So(len(muts), ShouldEqual, 2*len(fds))

					So(muts[0], ShouldResemble, &EnsureAttempt{fds[0].Dependee})
					So(muts[1], ShouldResemble, &AddBackDep{
						Dep: fds[0].Edge(), NeedsAck: true})

					So(ds.Get(a), ShouldBeNil)
					So(ds.GetMulti(fds), ShouldBeNil)
					So(a.AddingDepsBitmap.Size(), ShouldEqual, len(fds))
					So(a.WaitingDepBitmap.Size(), ShouldEqual, len(fds))
					So(a.State, ShouldEqual, attempt.AddingDeps)
					So(fds[0].ForExecution, ShouldEqual, 1)
				})
			})
		})
	})
}
