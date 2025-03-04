// Copyright 2022 The LUCI Authors.
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

package e2e

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestNewPatchsetUploadRun(t *testing.T) {
	t.Parallel()
	ftt.Run("Non-combinable", t, func(t *ftt.Test) {
		ct := Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"
		const gChangeFirst = 1001

		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef, &cfgpb.Verifiers_Tryjob_Builder{
			Host:          buildbucketHost,
			Name:          fmt.Sprintf("%s/test.bucket/static-analyzer", lProject),
			ModeAllowlist: []string{string(run.NewPatchsetRun)},
		})
		ct.BuildbucketFake.EnsureBuilders(cfg)
		prjcfgtest.Create(ctx, lProject, cfg)

		t.Run("A single patchset is uploaded", func(t *ftt.Test) {
			assert.NoErr(t, ct.PMNotifier.UpdateConfig(ctx, lProject))
			updated := ct.Clock.Now().Add(time.Minute)
			ct.AddCommitter("uploader-99")
			ct.AddNewPatchsetRunner("uploader-99")
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				gChangeFirst,
				gf.Updated(updated),
				gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("uploader-99"), gf.PSWithUploader(1, "uploader-99", updated),
			)))

			var rs []*run.Run
			ct.RunUntil(ctx, func() bool {
				rs = ct.LoadRunsOf(ctx, lProject)
				return len(rs) > 0
			})
			assert.Loosely(t, rs, should.HaveLength(1))
			assert.Loosely(t, rs[0].Mode, should.Equal(run.NewPatchsetRun))
			ct.Clock.Add(5 * time.Minute)
			t.Run("And then commit-queued", func(t *ftt.Test) {
				ct.GFake.MutateChange(gHost, gChangeFirst, func(c *gf.Change) {
					gf.CQ(2, ct.Clock.Now(), "uploader-99")(c.Info)

					gf.Updated(ct.Clock.Now())(c.Info)
				})
				assert.Loosely(t, ct.GFake.GetChange(gHost, gChangeFirst).Info.Labels["Commit-Queue"].All[0].Value, should.Equal(2))

				// The new run should be created.
				ct.RunUntil(ctx, func() bool {
					rs = ct.LoadRunsOf(ctx, lProject)
					for _, r := range rs {
						return r.Mode == run.FullRun
					}
					return false
				})

				// We should now have one run in each mode.
				assert.Loosely(t, rs, should.HaveLength(2))
				modes := make(map[run.Mode]bool)
				for _, r := range rs {
					modes[r.Mode] = true
				}
				assert.Loosely(t, modes[run.NewPatchsetRun], should.BeTrue)
				assert.Loosely(t, modes[run.FullRun], should.BeTrue)
			})
			t.Run("And then a new patchset is uploaded", func(t *ftt.Test) {
				originalRun := rs[0]
				originalID := rs[0].ID
				now := ct.Clock.Now()
				ct.GFake.MutateChange(gHost, gChangeFirst, func(c *gf.Change) {
					gf.PSWithUploader(2, "uploader-100", now)(c.Info)
					gf.Updated(ct.Clock.Now())(c.Info)
				})
				// A new run should be created.
				var newRun *run.Run
				ct.RunUntil(ctx, func() bool {
					runs := ct.LoadRunsOf(ctx, lProject)
					for _, r := range runs {
						if r.ID == originalID {
							originalRun = r
						} else {
							newRun = r
						}
					}
					return newRun != nil
				})

				assert.Loosely(t, newRun, should.NotBeNil)
				assert.Loosely(t, newRun.Mode, should.Equal(run.NewPatchsetRun))
				assert.Loosely(t, newRun.Status, should.Equal(run.Status_PENDING))
				assert.Loosely(t, originalRun.Status, should.Equal(run.Status_CANCELLED))
			})
		})

		t.Run("A single patchset is uploaded, and commit-queued at the same time", func(t *ftt.Test) {
			assert.NoErr(t, ct.PMNotifier.UpdateConfig(ctx, lProject))
			updated := ct.Clock.Now().Add(time.Minute)
			ct.AddCommitter("uploader-99")
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				gChangeFirst,
				gf.Updated(updated),
				gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("uploader-99"), gf.PSWithUploader(1, "uploader-99", updated),
				gf.CQ(2),
			)))

			// Wait for both Runs to be created.
			var rs []*run.Run
			ct.RunUntil(ctx, func() bool {
				rs = ct.LoadRunsOf(ctx, lProject)
				return len(rs) > 1
			})

			assert.Loosely(t, rs, should.HaveLength(2))
			modes := make(map[run.Mode]bool, 2)
			for _, r := range rs {
				modes[r.Mode] = true
			}
			assert.Loosely(t, modes[run.NewPatchsetRun], should.BeTrue)
			assert.Loosely(t, modes[run.FullRun], should.BeTrue)
		})

		t.Run("A patchset is uploaded after a previous patchset upload run is complete", func(t *ftt.Test) {
			assert.NoErr(t, ct.PMNotifier.UpdateConfig(ctx, lProject))
			updated := ct.Clock.Now().Add(time.Minute)
			ct.AddCommitter("uploader-99")
			ct.AddNewPatchsetRunner("uploader-99")
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				gChangeFirst,
				gf.Updated(updated),
				gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("uploader-99"), gf.PSWithUploader(1, "uploader-99", updated),
				gf.CQ(0, time.Time{}, "uploader-99"),
			)))

			// Wait until the first Run starts.
			var newRun, originalRun *run.Run
			ct.RunUntil(ctx, func() bool {
				rs := ct.LoadRunsOf(ctx, lProject)
				if len(rs) > 0 {
					assert.Loosely(t, rs, should.HaveLength(1))
					originalRun = rs[0]
					return true
				}
				return false
			})
			assert.Loosely(t, originalRun.Mode, should.Equal(run.NewPatchsetRun))

			// Make the first Run succeed.
			ct.LogPhase(ctx, "Tryjob for the first Run has passed")
			var buildID int64
			ct.RunUntil(ctx, func() bool {
				// Check whether the build has been successfully triggered and get the
				// build ID.
				r := ct.LoadRun(ctx, originalRun.ID)
				if executions := r.Tryjobs.GetState().GetExecutions(); len(executions) > 0 {
					if eid := tryjob.LatestAttempt(executions[0]).GetExternalId(); eid != "" {
						_, buildID = tryjob.ExternalID(eid).MustParseBuildbucketID()
						return true
					}
				}
				return false
			})
			ct.Clock.Add(time.Minute)
			ct.BuildbucketFake.MutateBuild(ctx, buildbucketHost, buildID, func(b *buildbucketpb.Build) {
				b.Status = buildbucketpb.Status_SUCCESS
				b.StartTime = timestamppb.New(ct.Clock.Now())
				b.EndTime = timestamppb.New(ct.Clock.Now())
			})
			ct.RunUntil(ctx, func() bool {
				originalRun = ct.LoadRun(ctx, originalRun.ID)
				return run.IsEnded(originalRun.Status)
			})
			assert.Loosely(t, originalRun.Status, should.Equal(run.Status_SUCCEEDED))

			// Upload a new patch, wait for it to create a new run.
			now := ct.Clock.Now()
			ct.GFake.MutateChange(gHost, gChangeFirst, func(c *gf.Change) {
				gf.PS(2)(c.Info)
				gf.PSWithUploader(2, "uploader-100", now)(c.Info)
				c.Info.Updated = timestamppb.New(now)
			})
			ct.RunUntil(ctx, func() bool {
				for _, r := range ct.LoadRunsOf(ctx, lProject) {
					if r.ID != originalRun.ID {
						newRun = r
						return true
					}
				}
				return false
			})

			assert.Loosely(t, newRun.Mode, should.Equal(run.NewPatchsetRun))
			assert.Loosely(t, ct.LoadCL(ctx, newRun.CLs[0]).Snapshot.Patchset, should.Equal(2))
		})

		t.Run("An untriggered dependent CL is uploaded, this should create an NP Run and not a CQ Run", func(t *ftt.Test) {
			// CL A depends on CL B, CL A is CQ+2 but CL B is not.
			// This should create two NPR Runs and no CQ Run.
			gChangeA, gChangeB := 1002, 1003
			assert.NoErr(t, ct.PMNotifier.UpdateConfig(ctx, lProject))
			updated := ct.Clock.Now().Add(time.Minute)
			ct.AddDryRunner("user-1")
			ct.AddNewPatchsetRunner("user-1")
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				gChangeA,
				gf.Updated(updated),
				gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("user-1"), gf.PSWithUploader(1, "user-1", updated),
				gf.CQ(1, updated, "user-1"),
			), gf.CI(
				gChangeB,
				gf.Updated(updated),
				gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("user-1"), gf.PSWithUploader(1, "user-1", updated),
			)))
			ct.GFake.SetDependsOn(gHost, fmt.Sprintf("%d_1", gChangeA), fmt.Sprintf("%d_1", gChangeB))

			// Wait for all three runs to be created.
			var rs []*run.Run
			ct.RunUntil(ctx, func() bool {
				rs = ct.LoadRunsOf(ctx, lProject)
				return len(rs) >= 3
			})
			assert.Loosely(t, len(rs), should.Equal(3))
			// Sort them by mode, then external ID.
			sort.Slice(rs, func(i, j int) bool {
				if rs[i].Mode != rs[j].Mode {
					return rs[i].Mode < rs[j].Mode
				}
				clI := &changelist.CL{ID: rs[i].CLs[0]}
				clJ := &changelist.CL{ID: rs[j].CLs[0]}
				assert.NoErr(t, datastore.Get(ctx, clI, clJ))
				return clI.ExternalID < clJ.ExternalID
			})

			cla, err := changelist.MustGobID(gHost, int64(gChangeA)).Load(ctx)
			assert.NoErr(t, err)
			clb, err := changelist.MustGobID(gHost, int64(gChangeB)).Load(ctx)
			assert.NoErr(t, err)

			assert.Loosely(t, rs[0].Mode, should.Equal(run.DryRun))
			assert.Loosely(t, rs[0].CLs, should.HaveLength(1))
			assert.Loosely(t, rs[0].CLs[0], should.Equal(cla.ID))

			assert.Loosely(t, rs[1].Mode, should.Equal(run.NewPatchsetRun))
			assert.Loosely(t, rs[1].CLs, should.HaveLength(1))
			assert.Loosely(t, rs[1].CLs[0], should.Equal(cla.ID))

			assert.Loosely(t, rs[2].Mode, should.Equal(run.NewPatchsetRun))
			assert.Loosely(t, rs[2].CLs, should.HaveLength(1))
			assert.Loosely(t, rs[2].CLs[0], should.Equal(clb.ID))
		})

		t.Run("If user is not authorized then the new patchset run is immediately cancelled", func(t *ftt.Test) {
			assert.NoErr(t, ct.PMNotifier.UpdateConfig(ctx, lProject))
			updated := ct.Clock.Now().Add(time.Minute)
			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				{Email: "uploader-99@example.com"},
			})
			ct.AddCommitter("uploader-99")
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				gChangeFirst,
				gf.Updated(updated),
				gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("uploader-99"), gf.PSWithUploader(1, "uploader-99", updated),
				gf.CQ(0, time.Time{}, "uploader-99"),
			)))

			// The run should be created and immediately failed.
			var rs []*run.Run
			ct.RunUntil(ctx, func() bool {
				rs = ct.LoadRunsOf(ctx, lProject)
				for _, r := range rs {
					return r.Status == run.Status_FAILED
				}
				return false
			})
			assert.Loosely(t, ct.GFake.GetChange(gHost, gChangeFirst).Info.Messages, should.HaveLength(0))
		})
	})

	ftt.Run("Combinable", t, func(t *ftt.Test) {
		ct := Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"

		cfg := MakeCfgCombinable("cg0", gHost, gRepo, gRef, &cfgpb.Verifiers_Tryjob_Builder{
			Host:          buildbucketHost,
			Name:          fmt.Sprintf("%s/test.bucket/static-analyzer", lProject),
			ModeAllowlist: []string{string(run.NewPatchsetRun)},
		})
		ct.BuildbucketFake.EnsureBuilders(cfg)
		prjcfgtest.Create(ctx, lProject, cfg)
		// A depends on B, both are ready. We should get three runs. Two NPR and one CQ
		gChangeA, gChangeB := 1002, 1003
		assert.NoErr(t, ct.PMNotifier.UpdateConfig(ctx, lProject))
		updated := ct.Clock.Now().Add(time.Minute)
		ct.AddDryRunner("user-1")
		ct.AddNewPatchsetRunner("user-1")
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
			gChangeA,
			gf.Updated(updated),
			gf.Project(gRepo), gf.Ref(gRef),
			gf.Owner("user-1"), gf.PSWithUploader(1, "user-1", updated),
			gf.CQ(2, updated, "user-1"),
		), gf.CI(
			gChangeB,
			gf.Updated(updated),
			gf.Project(gRepo), gf.Ref(gRef),
			gf.Owner("user-1"), gf.PSWithUploader(1, "user-1", updated),
			gf.CQ(2, updated, "user-1"),
		)))
		ct.GFake.SetDependsOn(gHost, fmt.Sprintf("%d_1", gChangeA), fmt.Sprintf("%d_1", gChangeB))

		// Wait for all three runs to be created.
		var rs []*run.Run
		ct.RunUntil(ctx, func() bool {
			rs = ct.LoadRunsOf(ctx, lProject)
			return len(rs) >= 3
		})
		assert.Loosely(t, len(rs), should.Equal(3))

		// Sort by mode then by external ID of the first CL in the Run.
		sort.Slice(rs, func(i, j int) bool {
			if rs[i].Mode != rs[j].Mode {
				return rs[i].Mode < rs[j].Mode
			}
			clI := &changelist.CL{ID: rs[i].CLs[0]}
			clJ := &changelist.CL{ID: rs[j].CLs[0]}
			assert.NoErr(t, datastore.Get(ctx, clI, clJ))
			return clI.ExternalID < clJ.ExternalID
		})

		cla, err := changelist.MustGobID(gHost, int64(gChangeA)).Load(ctx)
		assert.NoErr(t, err)
		clb, err := changelist.MustGobID(gHost, int64(gChangeB)).Load(ctx)
		assert.NoErr(t, err)
		// The full run includes both CLs.
		assert.Loosely(t, rs[0].Mode, should.Equal(run.FullRun))
		assert.Loosely(t, rs[0].CLs, should.HaveLength(2))
		actual := rs[0].CLs
		actual.Dedupe() // Sort.
		expected := common.CLIDs{cla.ID, clb.ID}
		expected.Dedupe() // Sort.
		assert.That(t, actual, should.Match(expected))

		// The new patchset runs each include one CL.
		assert.Loosely(t, rs[1].Mode, should.Equal(run.NewPatchsetRun))
		assert.Loosely(t, rs[1].CLs, should.HaveLength(1))
		assert.Loosely(t, rs[1].CLs[0], should.Equal(cla.ID))

		assert.Loosely(t, rs[2].Mode, should.Equal(run.NewPatchsetRun))
		assert.Loosely(t, rs[2].CLs, should.HaveLength(1))
		assert.Loosely(t, rs[2].CLs[0], should.Equal(clb.ID))
	})
}
