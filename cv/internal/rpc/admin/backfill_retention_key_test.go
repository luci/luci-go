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

package admin

import (
	"testing"
	"time"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBackfillRetentionKey(t *testing.T) {
	t.Parallel()

	Convey("BackfillRetentionKey", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		cl1 := changelist.MustGobID("example.com", 1).MustCreateIfNotExists(ctx)
		cl2 := changelist.MustGobID("example.com", 2).MustCreateIfNotExists(ctx)
		cl3 := changelist.MustGobID("example.com", 3).MustCreateIfNotExists(ctx)

		// cl1 and cl3 are missing retention key.
		cl1.RetentionKey = ""
		cl3.RetentionKey = ""
		So(datastore.Put(ctx, cl1, cl3), ShouldBeNil)

		// Run the migration.
		ct.Clock.Add(time.Minute)
		ctrl := &dsmapper.Controller{}
		ctrl.Install(ct.TQDispatcher)
		a := New(ct.TQDispatcher, ctrl, nil, nil, nil)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{allowGroup},
		})
		jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "backfill-retention-key"})
		So(err, ShouldBeNil)
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		jobInfo, err := a.DSMGetJob(ctx, jobID)
		So(err, ShouldBeNil)
		So(jobInfo.GetInfo().GetState(), ShouldEqual, dsmapperpb.State_SUCCESS)

		// verify
		So(datastore.Get(ctx, cl1, cl2, cl3), ShouldBeNil)
		So(cl1.RetentionKey, ShouldNotBeEmpty)
		So(cl1.EVersion, ShouldBeGreaterThan, 1)
		So(cl2.RetentionKey, ShouldNotBeEmpty)
		So(cl2.EVersion, ShouldEqual, 1) // not updated
		So(cl3.RetentionKey, ShouldNotBeEmpty)
		So(cl3.EVersion, ShouldBeGreaterThan, 1)
	})
}
