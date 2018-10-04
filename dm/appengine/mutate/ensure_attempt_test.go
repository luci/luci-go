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
	"context"
	"testing"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEnsureAttempt(t *testing.T) {
	t.Parallel()

	Convey("EnsureAttempt", t, func() {
		c := memory.Use(context.Background())
		ea := &EnsureAttempt{dm.NewAttemptID("quest", 1)}

		Convey("Root", func() {
			So(ea.Root(c).String(), ShouldEqual, `dev~app::/Attempt,"quest|fffffffe"`)
		})

		Convey("RollForward", func() {
			a := &model.Attempt{ID: *ea.ID}

			Convey("Good", func() {
				So(ds.Get(c, a), ShouldEqual, ds.ErrNoSuchEntity)

				muts, err := ea.RollForward(c)
				So(err, ShouldBeNil)
				So(muts, ShouldHaveLength, 1)

				So(ds.Get(c, a), ShouldEqual, nil)
				So(a.State, ShouldEqual, dm.Attempt_SCHEDULING)

				Convey("replaying the mutation after the state has evolved is a noop", func() {
					So(a.ModifyState(c, dm.Attempt_EXECUTING), ShouldBeNil)
					So(ds.Put(c, a), ShouldBeNil)

					muts, err = ea.RollForward(c)
					So(err, ShouldBeNil)
					So(muts, ShouldBeEmpty)

					So(ds.Get(c, a), ShouldEqual, nil)
					So(a.State, ShouldEqual, dm.Attempt_EXECUTING)
				})
			})

			Convey("Bad", func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(nil, "GetMulti")

				muts, err := ea.RollForward(c)
				So(err, ShouldErrLike, `feature "GetMulti" is broken`)
				So(muts, ShouldBeEmpty)

				fb.UnbreakFeatures("GetMulti")

				So(ds.Get(c, a), ShouldEqual, ds.ErrNoSuchEntity)
			})
		})
	})
}
