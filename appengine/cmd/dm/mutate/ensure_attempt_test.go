// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestEnsureAttempt(t *testing.T) {
	t.Parallel()

	Convey("EnsureAttempt", t, func() {
		c := memory.Use(context.Background())
		ea := &EnsureAttempt{*types.NewAttemptID("quest|fffffffe")}

		Convey("Root", func() {
			So(ea.Root(c).String(), ShouldEqual, `dev~app::/Attempt,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {
			a := &model.Attempt{AttemptID: ea.ID}

			Convey("Good", func() {
				So(datastore.Get(c).Get(a), ShouldEqual, datastore.ErrNoSuchEntity)

				muts, err := ea.RollForward(c)
				So(err, ShouldBeNil)
				So(muts, ShouldBeEmpty)

				ds := datastore.Get(c)
				So(ds.Get(a), ShouldEqual, nil)
				So(a.State, ShouldEqual, types.NeedsExecution)

				Convey("replaying the mutation after the state has evolved is a noop", func() {
					So(a.ChangeState(types.Executing), ShouldBeNil)
					So(ds.Put(a), ShouldBeNil)

					muts, err = ea.RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeEmpty)

					So(ds.Get(a), ShouldEqual, nil)
					So(a.State, ShouldEqual, types.Executing)
				})
			})

			Convey("Bad", func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(nil, "GetMulti")

				muts, err := ea.RollForward(c)
				So(err, ShouldErrLike, `feature "GetMulti" is broken`)
				So(muts, ShouldBeEmpty)

				fb.UnbreakFeatures("GetMulti")

				So(datastore.Get(c).Get(a), ShouldEqual, datastore.ErrNoSuchEntity)
			})
		})
	})
}
