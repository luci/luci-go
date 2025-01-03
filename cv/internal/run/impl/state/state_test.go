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

package state

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestCheckTree(t *testing.T) {
	t.Parallel()

	ftt.Run("CheckTree", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const lProject = "chromium"
		rs := &RunState{
			Run: run.Run{
				ID:         common.MakeRunID(lProject, ct.Clock.Now().Add(-2*time.Minute), 1, []byte("deadbeef")),
				Submission: &run.Submission{},
			},
		}
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						TreeStatus: &cfgpb.Verifiers_TreeStatus{
							TreeName: lProject,
						},
					},
				},
			},
		})
		meta, err := prjcfg.GetLatestMeta(ctx, lProject)
		assert.NoErr(t, err)
		assert.Loosely(t, meta.ConfigGroupIDs, should.HaveLength(1))
		rs.Run.ConfigGroupID = meta.ConfigGroupIDs[0]
		clientFactory := tree.NewClientFactory(ct.TreeFakeSrv.Host())
		t.Run("Open", func(t *ftt.Test) {
			ct.TreeFakeSrv.ModifyState(lProject, tspb.GeneralState_OPEN)
			open, err := rs.CheckTree(ctx, clientFactory)
			assert.NoErr(t, err)
			assert.That(t, open, should.BeTrue)
			assert.That(t, rs.Run.Submission.TreeOpen, should.BeTrue)
			assert.That(t, rs.Run.Submission.LastTreeCheckTime, should.Match(timestamppb.New(ct.Clock.Now().UTC())))
		})

		t.Run("Closed", func(t *ftt.Test) {
			ct.TreeFakeSrv.ModifyState(lProject, tspb.GeneralState_CLOSED)
			open, err := rs.CheckTree(ctx, clientFactory)
			assert.NoErr(t, err)
			assert.That(t, open, should.BeFalse)
			assert.That(t, rs.Run.Submission.TreeOpen, should.BeFalse)
			assert.That(t, rs.Run.Submission.LastTreeCheckTime, should.Match(timestamppb.New(ct.Clock.Now().UTC())))
		})

		t.Run("Closed but ignored", func(t *ftt.Test) {
			ct.TreeFakeSrv.ModifyState(lProject, tspb.GeneralState_CLOSED)
			rs.Run.Options = &run.Options{SkipTreeChecks: true}
			open, err := rs.CheckTree(ctx, clientFactory)
			assert.NoErr(t, err)
			assert.That(t, open, should.BeTrue)
			assert.That(t, rs.Run.Submission.TreeOpen, should.BeTrue)
			assert.That(t, rs.Run.Submission.LastTreeCheckTime, should.Match(timestamppb.New(ct.Clock.Now().UTC())))
		})

		t.Run("Tree not defined", func(t *ftt.Test) {
			ct.TreeFakeSrv.ModifyState(lProject, tspb.GeneralState_CLOSED)
			prjcfgtest.Update(ctx, lProject, &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{Name: "main"},
					// No Tree defined
				},
			})
			meta, err := prjcfg.GetLatestMeta(ctx, lProject)
			assert.NoErr(t, err)
			assert.Loosely(t, meta.ConfigGroupIDs, should.HaveLength(1))
			rs.Run.ConfigGroupID = meta.ConfigGroupIDs[0]
			open, err := rs.CheckTree(ctx, clientFactory)
			assert.NoErr(t, err)
			assert.That(t, open, should.BeTrue)
			assert.That(t, rs.Run.Submission.TreeOpen, should.BeTrue)
			assert.That(t, rs.Run.Submission.LastTreeCheckTime, should.Match(timestamppb.New(ct.Clock.Now().UTC())))
		})
	})
}
