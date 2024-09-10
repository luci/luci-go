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
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"
)

func TestUpgradeCLs(t *testing.T) {
	t.Parallel()

	ftt.Run("Upgrade all RunCLs to not contain CL description", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		mkCL := func(id common.CLID, ci *gerritpb.ChangeInfo) *run.RunCL {
			cl := &run.RunCL{
				ID:  id,
				Run: datastore.MakeKey(ctx, common.RunKind, fmt.Sprintf("prj/%d", id)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Info: ci,
					}},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
			return cl
		}

		clDesc := func(cl *run.RunCL, patchset int32) string {
			ci := cl.Detail.GetGerrit().GetInfo()
			for _, revInfo := range ci.GetRevisions() {
				if revInfo.GetNumber() == patchset {
					return revInfo.GetCommit().GetMessage()
				}
			}
			return ""
		}

		cl1 := mkCL(1, gf.CI(1, gf.PS(1), gf.Desc("First")))
		cl2 := mkCL(2, gf.CI(2, gf.PS(1), gf.Desc("PS#1 blah"), gf.PS(2), gf.Desc("PS#2 foo")))

		// Check test setup.
		assert.Loosely(t, clDesc(cl1, 1), should.Match("First"))
		assert.Loosely(t, clDesc(cl2, 1), should.Match("PS#1 blah"))
		assert.Loosely(t, clDesc(cl2, 2), should.Match("PS#2 foo"))

		verify := func() {
			assert.Loosely(t, datastore.Get(ctx, cl1, cl2), should.BeNil)

			assert.Loosely(t, clDesc(cl1, 1), should.BeEmpty)

			assert.Loosely(t, cl2.Detail.GetGerrit().GetInfo().GetRevisions(), should.HaveLength(2))
			assert.Loosely(t, clDesc(cl2, 1), should.BeEmpty)
			assert.Loosely(t, clDesc(cl2, 2), should.BeEmpty)
		}

		// Run the migration.
		ct.Clock.Add(time.Minute)
		ctrl := &dsmapper.Controller{}
		ctrl.Install(ct.TQDispatcher)
		a := New(ct.TQDispatcher, ctrl, nil, nil, nil)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{allowGroup},
		})
		jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "runcl-description"})
		assert.Loosely(t, err, should.BeNil)
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		jobInfo, err := a.DSMGetJob(ctx, jobID)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, jobInfo.GetInfo().GetState(), should.Equal(dsmapperpb.State_SUCCESS))

		verify()
	})
}
