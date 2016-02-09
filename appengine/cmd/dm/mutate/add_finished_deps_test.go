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
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/errors"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestAddFinishedDeps(t *testing.T) {
	t.Parallel()

	Convey("AddFinishedDeps", t, func() {
		c := memory.Use(context.Background())
		f := &AddFinishedDeps{
			&model.AttemptFanout{
				Base: types.NewAttemptID("quest|fffffffe"),
				Edges: []*types.AttemptID{
					types.NewAttemptID("to|fffffffe"),
					types.NewAttemptID("to|fffffffd"),
					types.NewAttemptID("to|fffffffc"),
				},
			},
			[]byte("sup"),
		}

		fs := f.ToAdd.Fwds(c)

		ds := datastore.Get(c)
		fs[1].ForExecution = 1
		So(ds.Put(fs[1]), ShouldBeNil)

		a := &model.Attempt{AttemptID: *f.ToAdd.Base, State: attempt.Executing, CurExecution: 7}
		ak := ds.KeyForObj(a)
		e := &model.Execution{ID: 7, Attempt: ak, ExecutionKey: []byte("sup")}
		So(ds.PutMulti([]interface{}{a, e}), ShouldBeNil)

		Convey("Root", func() {
			So(f.Root(c).String(), ShouldEqual, `dev~app::/Attempt,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {
			err := ds.GetMulti(fs)
			So(err, ShouldResemble, errors.MultiError{
				datastore.ErrNoSuchEntity,
				nil,
				datastore.ErrNoSuchEntity,
			})

			muts, err := f.RollForward(c)
			So(err, ShouldBeNil)
			So(muts, ShouldResemble, []tumble.Mutation{
				&AddBackDep{Dep: fs[0].Edge()},
				&AddBackDep{Dep: fs[2].Edge()},
			})

			So(ds.GetMulti(fs), ShouldBeNil)
			So(fs[0].ForExecution, ShouldEqual, 7)
			So(fs[1].ForExecution, ShouldEqual, 1)
			So(fs[2].ForExecution, ShouldEqual, 7)

			muts, err = f.RollForward(c)
			So(err, ShouldBeNil)
			So(muts, ShouldBeEmpty)
		})

		Convey("RollForward (bad)", func() {
			So(ds.Delete(ak), ShouldBeNil)
			_, err := f.RollForward(c)
			So(err, ShouldErrLike, datastore.ErrNoSuchEntity)
		})
	})
}
