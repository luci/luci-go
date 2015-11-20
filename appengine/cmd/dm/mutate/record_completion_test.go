// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/appengine/tumble"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestRecordCompletion(t *testing.T) {
	t.Parallel()

	Convey("RecordCompletion", t, func() {
		c := memory.Use(context.Background())
		rc := &RecordCompletion{*types.NewAttemptID("quest|fffffffe")}

		bdg := &model.BackDepGroup{Dependee: rc.For}

		ds := datastore.Get(c)

		Convey("Root", func() {
			So(rc.Root(c).String(), ShouldEqual, `dev~app::/BackDepGroup,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {

			Convey("Good", func() {
				Convey("No BDG", func() {
					muts, err := rc.RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeEmpty)

					So(ds.Get(bdg), ShouldBeNil)
					So(bdg.AttemptFinished, ShouldBeTrue)
				})

				Convey("BDG exists, no deps", func() {
					So(ds.Put(bdg), ShouldBeNil)

					muts, err := rc.RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeEmpty)

					So(ds.Get(bdg), ShouldBeNil)
					So(bdg.AttemptFinished, ShouldBeTrue)
				})

				Convey("BDG exists, with unfinished deps", func() {
					bd := &model.BackDep{
						Depender:      *types.NewAttemptID("from|fffffffe"),
						DependeeGroup: rc.Root(c),
					}
					So(ds.PutMulti([]interface{}{bdg, bd}), ShouldBeNil)

					muts, err := rc.RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldResembleV, []tumble.Mutation{
						&AckFwdDep{Dep: bd.Edge(), DepIsFinished: true},
					})

					So(ds.GetMulti([]interface{}{bdg, bd}), ShouldBeNil)
					So(bdg.AttemptFinished, ShouldBeTrue)
					So(bd.Propagated, ShouldBeTrue)
				})

				Convey("BDG exists, with finished deps", func() {
					bd := &model.BackDep{
						Depender:      *types.NewAttemptID("from|fffffffe"),
						DependeeGroup: rc.Root(c),
						Propagated:    true,
					}
					So(ds.PutMulti([]interface{}{bdg, bd}), ShouldBeNil)

					muts, err := rc.RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeEmpty)

					So(ds.GetMulti([]interface{}{bdg, bd}), ShouldBeNil)
					So(bdg.AttemptFinished, ShouldBeTrue)
					So(bd.Propagated, ShouldBeTrue)
				})

				Convey("BDG exists, but too much to do in one TXN!", func() {
					// amtWork is 1.5*completionLimit
					amtWork := completionLimit + (completionLimit >> 1)

					for i := 0; i < amtWork; i++ {
						bd := &model.BackDep{
							Depender:      *types.NewAttemptID("from|ffffffff"),
							DependeeGroup: rc.Root(c),
						}
						bd.Depender.AttemptNum = uint32(i + 1)
						So(ds.Put(bd), ShouldBeNil)
					}
					So(ds.Put(bdg), ShouldBeNil)

					muts, err := rc.RollForward(c)
					So(err, ShouldBeNil)
					So(len(muts), ShouldEqual, completionLimit+1)

					So(muts[completionLimit], ShouldResembleV, rc)

					muts, err = rc.RollForward(c)
					So(err, ShouldBeNil)
					So(len(muts), ShouldEqual, amtWork-completionLimit)

					muts, err = rc.RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeEmpty)
				})
			})
		})
	})
}
