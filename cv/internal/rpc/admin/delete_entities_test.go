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

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/cvtesting"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeleteEntities(t *testing.T) {
	t.Parallel()

	Convey("Delete entities", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		ent := &MockEntity{
			ID: "foo",
		}
		So(datastore.Put(ctx, ent), ShouldBeNil)

		runJobAndEnsureSuccess := func() {
			ctrl := &dsmapper.Controller{}
			ctrl.Install(ct.TQDispatcher)
			a := New(ct.TQDispatcher, ctrl, nil, nil, nil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})

			jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "delete-entities"})
			So(err, ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
			jobInfo, err := a.DSMGetJob(ctx, jobID)
			So(err, ShouldBeNil)
			So(jobInfo.GetInfo().GetState(), ShouldEqual, dsmapperpb.State_SUCCESS)
		}

		Convey("can delete target entities", func() {
			entities := []*MockEntity{
				{ID: "foo"},
				{ID: "bar"},
				{ID: "baz"},
			}
			So(datastore.Put(ctx, entities), ShouldBeNil)
			runJobAndEnsureSuccess()
			res, err := datastore.Exists(ctx, entities)
			So(err, ShouldBeNil)
			So(res.List(0).Any(), ShouldBeFalse)
		})

		Convey("can handle missing entity", func() {
			entities := []*MockEntity{
				{ID: "foo"},
				{ID: "bar"},
				{ID: "baz"},
			}
			So(datastore.Put(ctx, entities[0], entities[2]), ShouldBeNil)
			runJobAndEnsureSuccess()
			res, err := datastore.Exists(ctx, entities)
			So(err, ShouldBeNil)
			So(res.List(0).Any(), ShouldBeFalse)
		})
	})
}

type MockEntity struct {
	// Kind must match the kind of `deleteEntityKind`.
	_kind string `gae:"$kind,migration.ReportedTryjobs"`
	ID    string `gae:"$id"`
}
