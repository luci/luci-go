// Copyright 2024 The LUCI Authors.
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

package retention

import (
	"testing"
	"time"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestWipeoutRun(t *testing.T) {
	t.Parallel()

	Convey("Wipeout", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const lProject = "infra"
		makeRun := func(createTime time.Time) *run.Run {
			r := &run.Run{
				ID:         common.MakeRunID(lProject, createTime, 1, []byte("deadbeef")),
				CreateTime: createTime,
			}
			So(datastore.Put(ctx, r), ShouldBeNil)
			return r
		}

		Convey("wipeout Run and children", func() {
			r := makeRun(ct.Clock.Now().Add(-2 * retentionPeriod).UTC())
			cl1 := &run.RunCL{
				ID:  1,
				Run: datastore.KeyForObj(ctx, r),
			}
			cl2 := &run.RunCL{
				ID:  2,
				Run: datastore.KeyForObj(ctx, r),
			}
			log := &run.RunLog{
				ID:  555,
				Run: datastore.KeyForObj(ctx, r),
			}
			So(datastore.Put(ctx, cl1, cl2, log), ShouldBeNil)
			err := wipeoutRun(ctx, r.ID)
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, r), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, cl1), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, cl2), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, log), ShouldErrLike, datastore.ErrNoSuchEntity)
		})

		Convey("handle run doesn't exist", func() {
			createTime := ct.Clock.Now().Add(-2 * retentionPeriod).UTC()
			rid := common.MakeRunID(lProject, createTime, 1, []byte("deadbeef"))
			err := wipeoutRun(ctx, rid)
			So(err, ShouldBeNil)
		})

		Convey("handle run should still be retained", func() {
			r := makeRun(ct.Clock.Now().Add(-retentionPeriod / 2).UTC())
			err := wipeoutRun(ctx, r.ID)
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, r), ShouldBeNil)
		})
	})
}
