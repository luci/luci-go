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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/cvtesting"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
)

func TestDeleteEntities(t *testing.T) {
	t.Parallel()

	ftt.Run("Delete entities", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		ent := &MockEntity{
			ID: "foo",
		}
		assert.NoErr(t, datastore.Put(ctx, ent))

		runJobAndEnsureSuccess := func() {
			ctrl := &dsmapper.Controller{}
			ctrl.Install(ct.TQDispatcher)
			a := New(ct.TQDispatcher, ctrl, nil, nil, nil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})

			jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "delete-entities"})
			assert.NoErr(t, err)
			ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
			jobInfo, err := a.DSMGetJob(ctx, jobID)
			assert.NoErr(t, err)
			assert.Loosely(t, jobInfo.GetInfo().GetState(), should.Equal(dsmapperpb.State_SUCCESS))
		}

		t.Run("can delete target entities", func(t *ftt.Test) {
			entities := []*MockEntity{
				{ID: "foo"},
				{ID: "bar"},
				{ID: "baz"},
			}
			assert.NoErr(t, datastore.Put(ctx, entities))
			runJobAndEnsureSuccess()
			res, err := datastore.Exists(ctx, entities)
			assert.NoErr(t, err)
			assert.Loosely(t, res.List(0).Any(), should.BeFalse)
		})

		t.Run("can handle missing entity", func(t *ftt.Test) {
			entities := []*MockEntity{
				{ID: "foo"},
				{ID: "bar"},
				{ID: "baz"},
			}
			assert.NoErr(t, datastore.Put(ctx, entities[0], entities[2]))
			runJobAndEnsureSuccess()
			res, err := datastore.Exists(ctx, entities)
			assert.NoErr(t, err)
			assert.Loosely(t, res.List(0).Any(), should.BeFalse)
		})
	})
}

type MockEntity struct {
	// Kind must match the kind of `deleteEntityKind`.
	_kind string `gae:"$kind,migration.VerifiedCQDRun"`
	ID    string `gae:"$id"`
}
