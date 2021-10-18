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
	"testing"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/migration"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeleteFinishedCQDRun(t *testing.T) {
	t.Parallel()

	Convey("Delete FinishedCQDRun entities", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		So(datastore.Put(
			ctx,
			&migration.FinishedCQDRun{AttemptKey: "beef", RunID: ""},
			&migration.FinishedCQDRun{AttemptKey: "cafe", RunID: "project/123-1-cafe"},
		), ShouldBeNil)

		fetchAll := func() (entities []*migration.FinishedCQDRun) {
			err := datastore.GetAll(ctx, datastore.NewQuery("migration.FinishedCQDRun"), &entities)
			So(err, ShouldBeNil)
			return entities
		}
		So(fetchAll(), ShouldHaveLength, 2)

		// Run the migration.
		ctrl := &dsmapper.Controller{}
		ctrl.Install(ct.TQDispatcher)
		a := New(ct.TQDispatcher, ctrl, nil, nil, nil)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{allowGroup},
		})
		jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "delete-FinishedCQDRun"})
		So(err, ShouldBeNil)
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		jobInfo, err := a.DSMGetJob(ctx, jobID)
		So(err, ShouldBeNil)
		So(jobInfo.GetInfo().GetState(), ShouldEqual, dsmapperpb.State_SUCCESS)

		// Verify.
		So(fetchAll(), ShouldBeEmpty)
	})
}
