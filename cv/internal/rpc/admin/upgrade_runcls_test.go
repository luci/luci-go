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

	"go.chromium.org/luci/cv/internal/changelist"
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

		const gHost = "x-review.example.com"

		mkRunCL := func(gChange int64, missing bool) (*run.RunCL, *changelist.CL) {
			cl := changelist.MustGobID(gHost, gChange).MustCreateIfNotExists(ctx)
			rcl := &run.RunCL{
				ID:  cl.ID,
				Run: datastore.MakeKey(ctx, run.RunKind, fmt.Sprintf("proj/%d-beef", gChange)),
			}
			if !missing {
				rcl.ExternalID = cl.ExternalID
			}
			So(datastore.Put(ctx, rcl), ShouldBeNil)
			return rcl, cl
		}

		rcl1, cl1 := mkRunCL(1, true)
		rcl2, cl2 := mkRunCL(2, false)
		rcl3, cl3 := mkRunCL(3, true)
		rcl4, cl4 := mkRunCL(4, false)

		// Run the migration.
		ctrl := &dsmapper.Controller{}
		ctrl.Install(ct.TQDispatcher)
		a := New(ct.TQDispatcher, ctrl, nil, nil, nil)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{allowGroup},
		})
		jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "runcl-externalid"})
		So(err, ShouldBeNil)
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		jobInfo, err := a.DSMGetJob(ctx, jobID)
		So(err, ShouldBeNil)
		So(jobInfo.GetInfo().GetState(), ShouldEqual, dsmapperpb.State_SUCCESS)

		// Verify.
		So(datastore.Get(ctx, rcl1, rcl2, rcl3, rcl4), ShouldBeNil)
		So(rcl1.ExternalID, ShouldResemble, cl1.ExternalID)
		So(rcl2.ExternalID, ShouldResemble, cl2.ExternalID)
		So(rcl3.ExternalID, ShouldResemble, cl3.ExternalID)
		So(rcl4.ExternalID, ShouldResemble, cl4.ExternalID)
	})
}
