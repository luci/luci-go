// Copyright 2021 The LUCI Authors.
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

package admin

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUpgradeRunCLs(t *testing.T) {
	t.Parallel()

	Convey("Upgrade all RunCLs to contain IndexedID", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		mkRunCL := func(id, idx common.CLID) (*run.RunCL, *datastore.Key) {
			rcl := &run.RunCL{
				ID:        id,
				IndexedID: idx,
				Run:       datastore.MakeKey(ctx, run.RunKind, fmt.Sprintf("proj/%d-beef", id)),
			}
			So(datastore.Put(ctx, rcl), ShouldBeNil)
			k, err := datastore.KeyForObjErr(ctx, rcl)
			So(err, ShouldBeNil)
			return rcl, k
		}

		rcl1, k1 := mkRunCL(1, 1)
		rcl2, k2 := mkRunCL(2, 0)
		rcl3, k3 := mkRunCL(3, 0)
		rcl4, k4 := mkRunCL(4, 4)

		assertMigrated := func(rcls ...*run.RunCL) {
			So(datastore.Get(ctx, rcls), ShouldBeNil)
			for _, rcl := range rcls {
				So(rcl.ID, ShouldEqual, rcl.IndexedID)
			}
		}

		Convey("All", func() {
			So(upgradeRunCLs(ctx, []*datastore.Key{k1, k2, k3, k4}), ShouldBeNil)
			assertMigrated(rcl1, rcl2, rcl3, rcl4)
		})
		Convey("Quick exit", func() {
			So(upgradeRunCLs(ctx, []*datastore.Key{k1, k4}), ShouldBeNil)
			assertMigrated(rcl1, rcl4)
		})

		Convey("End to end", func() {
			ctrl := &dsmapper.Controller{}
			ctrl.Install(ct.TQDispatcher)
			a := New(ct.TQDispatcher, ctrl, nil, nil, nil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})

			jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "runcl-indexedid"})
			So(err, ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
			jobInfo, err := a.DSMGetJob(ctx, jobID)
			So(err, ShouldBeNil)
			So(jobInfo.GetInfo().GetState(), ShouldEqual, dsmapperpb.State_SUCCESS)

			assertMigrated(rcl1, rcl2, rcl3, rcl4)
		})
	})
}
