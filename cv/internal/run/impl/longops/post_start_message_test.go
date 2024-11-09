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

package longops

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/configs/validation"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/util"
)

func TestPostStartMessage(t *testing.T) {
	t.Parallel()

	ftt.Run("PostStartMessageOp works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject = "chromeos"
			runID    = lProject + "/777-1-deadbeef"
			gHost    = "g-review.example.com"
			gChange1 = 111
			gChange2 = 222
		)

		cfg := cfgpb.Config{
			CqStatusHost: validation.CQStatusHostPublic,
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "test"},
			},
		}
		prjcfgtest.Create(ctx, lProject, &cfg)

		ensureCL := func(ci *gerritpb.ChangeInfo) (*changelist.CL, *run.RunCL) {
			triggers := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: cfg.GetConfigGroups()[0]})

			if ct.GFake.Has(gHost, int(ci.GetNumber())) {
				ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
					c.Info = ci
				})
			} else {
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))
			}

			cl := changelist.MustGobID(gHost, ci.GetNumber()).MustCreateIfNotExists(ctx)
			rcl := &run.RunCL{
				ID:         cl.ID,
				ExternalID: cl.ExternalID,
				IndexedID:  cl.ID,
				Trigger:    triggers.GetCqVoteTrigger(),
				Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: ci,
					}},
					ExternalUpdateTime: timestamppb.New(ct.Clock.Now()),
				},
			}
			cl.Snapshot = rcl.Detail
			cl.EVersion++
			assert.Loosely(t, datastore.Put(ctx, cl, rcl), should.BeNil)
			return cl, rcl
		}

		makeRunWithCLs := func(r *run.Run, cis ...*gerritpb.ChangeInfo) *run.Run {
			if len(cis) == 0 {
				panic(fmt.Errorf("at least one CL required"))
			}
			if r == nil {
				r = &run.Run{}
			}
			r.ID = runID
			r.Status = run.Status_RUNNING
			for _, ci := range cis {
				_, rcl := ensureCL(ci)
				r.CLs = append(r.CLs, rcl.ID)
			}
			if r.Mode == "" {
				r.Mode = run.FullRun
			}
			if r.ConfigGroupID == "" {
				r.ConfigGroupID = prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0]
			}
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			return r
		}

		makeOp := func(r *run.Run) *PostStartMessageOp {
			return &PostStartMessageOp{
				Base: &Base{
					Op: &run.OngoingLongOps_Op{
						Deadline:        timestamppb.New(ct.Clock.Now().Add(10000 * time.Hour)),
						CancelRequested: false,
						Work: &run.OngoingLongOps_Op_PostStartMessage{
							PostStartMessage: true,
						},
					},
					IsCancelRequested: func() bool { return false },
					Run:               r,
				},
				Env:      ct.Env,
				GFactory: ct.GFactory(),
			}
		}

		t.Run("Happy path without status URL", func(t *ftt.Test) {
			cfg.CqStatusHost = ""
			prjcfgtest.Update(ctx, lProject, &cfg)

			op := makeOp(makeRunWithCLs(nil, gf.CI(gChange1, gf.CQ(+2))))
			res, err := op.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))
			assert.Loosely(t, ct.GFake.GetChange(gHost, gChange1).Info, convey.Adapt(gf.ShouldLastMessageContain)("CV is trying the patch.\n\nBot data: "))
		})

		t.Run("Happy path", func(t *ftt.Test) {
			op := makeOp(makeRunWithCLs(nil, gf.CI(gChange1, gf.CQ(+2))))
			res, err := op.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))

			ci := ct.GFake.GetChange(gHost, gChange1).Info
			assert.Loosely(t, ci, convey.Adapt(gf.ShouldLastMessageContain)("CV is trying the patch.\n\nFollow status at:"))
			assert.Loosely(t, ci, convey.Adapt(gf.ShouldLastMessageContain)("https://luci-change-verifier.appspot.com/ui/run/chromeos/777-1-deadbeef"))
			assert.Loosely(t, ci, convey.Adapt(gf.ShouldLastMessageContain)("Bot data:"))
			// Should post exactly one message.
			assert.Loosely(t, ci.GetMessages(), should.HaveLength(1))

			// Recorded timestamp must be approximately correct.
			assert.Loosely(t, res.GetPostStartMessage().GetTime().AsTime(), should.HappenWithin(time.Second, ci.GetMessages()[0].GetDate().AsTime()))
		})

		t.Run("Happy path with multiple CLs for runs with root CL", func(t *ftt.Test) {
			r := makeRunWithCLs(
				&run.Run{Mode: run.DryRun},
				gf.CI(gChange1),
				gf.CI(gChange2, gf.CQ(+1)),
			)
			r.RootCL = r.CLs[1]
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			op := makeOp(r)

			res, err := op.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))

			ci1 := ct.GFake.GetChange(gHost, gChange1).Info
			assert.Loosely(t, ci1.GetMessages(), should.BeEmpty)
			ci2 := ct.GFake.GetChange(gHost, gChange2).Info
			assert.Loosely(t, ci2.GetMessages(), should.HaveLength(1))
			assert.Loosely(t, ci2, convey.Adapt(gf.ShouldLastMessageContain)("Dry run: CV is trying the patch.\n\nFollow status at:"))
			bd, ok := botdata.Parse(ci2.GetMessages()[0])
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, bd.Action, should.Equal(botdata.Start))
			assert.Loosely(t, res.GetPostStartMessage().GetTime().AsTime(), should.HappenWithin(time.Second, ci2.GetMessages()[0].GetDate().AsTime()))
		})

		t.Run("Happy path with multiple CLs for run without root CL", func(t *ftt.Test) {
			op := makeOp(makeRunWithCLs(
				&run.Run{Mode: run.DryRun},
				gf.CI(gChange1, gf.CQ(+1)),
				gf.CI(gChange2, gf.CQ(+1)),
			))
			res, err := op.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))

			for _, gChange := range []int{gChange1, gChange2} {
				ci := ct.GFake.GetChange(gHost, gChange).Info
				assert.Loosely(t, ci, convey.Adapt(gf.ShouldLastMessageContain)("Dry run: CV is trying the patch.\n\nFollow status at:"))
				// Should post exactly one message.
				assert.Loosely(t, ci.GetMessages(), should.HaveLength(1))
				bd, ok := botdata.Parse(ci.GetMessages()[0])
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, bd.Action, should.Equal(botdata.Start))

				// Recorded timestamp must be approximately correct since both CLs are
				// posted at around the same time.
				assert.Loosely(t, res.GetPostStartMessage().GetTime().AsTime(), should.HappenWithin(time.Second, ci.GetMessages()[0].GetDate().AsTime()))
			}
		})

		t.Run("Best effort avoidance of duplicated messages", func(t *ftt.Test) {
			// Make two same PostStartMessageOp objects, since they are single-use
			// only.
			opFirst := makeOp(makeRunWithCLs(nil, gf.CI(gChange1, gf.CQ(+2))))
			opRetry := makeOp(makeRunWithCLs(nil, gf.CI(gChange1, gf.CQ(+2))))

			// Simulate first try updating Gerrit, but somehow crashing before getting
			// response from Gerrit.
			_, err := opFirst.Do(ctx)
			assert.NoErr(t, err)
			ci := ct.GFake.GetChange(gHost, gChange1).Info
			assert.Loosely(t, ci, convey.Adapt(gf.ShouldLastMessageContain)("CV is trying the patch"))
			assert.Loosely(t, ci.GetMessages(), should.HaveLength(1))

			t.Run("very quick retry leads to dups", func(t *ftt.Test) {
				ct.Clock.Add(time.Second)
				res, err := opRetry.Do(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))
				assert.Loosely(t, ct.GFake.GetChange(gHost, gChange1).Info.GetMessages(), should.HaveLength(2))
				// And the timestamp isn't entirely right, but that's fine.
				assert.Loosely(t, res.GetPostStartMessage().GetTime().AsTime(), should.Resemble(ct.Clock.Now().UTC().Truncate(time.Second)))
			})

			t.Run("later retry", func(t *ftt.Test) {
				ct.Clock.Add(util.StaleCLAgeThreshold)
				res, err := opRetry.Do(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))
				// There should still be exactly 1 message.
				ci := ct.GFake.GetChange(gHost, gChange1).Info
				assert.Loosely(t, ci.GetMessages(), should.HaveLength(1))
				// and the timestamp must be exactly correct.
				assert.Loosely(t, res.GetPostStartMessage().GetTime().AsTime(), should.Resemble(ci.GetMessages()[0].GetDate().AsTime()))
			})
		})

		t.Run("Failures", func(t *ftt.Test) {
			op := makeOp(makeRunWithCLs(
				&run.Run{Mode: run.DryRun},
				gf.CI(gChange1, gf.CQ(+1)),
			))
			ctx, cancel := clock.WithDeadline(ctx, op.Op.Deadline.AsTime())
			defer cancel()
			ct.Clock.Set(op.Op.Deadline.AsTime().Add(-8 * time.Minute))

			check := func(t testing.TB) {
				t.Helper()

				res, err := op.Do(ctx)
				// Given any failure, the status should be set to FAILED,
				// but the returned error is nil to prevent the TQ retry.
				assert.NoErr(t, err)
				assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_FAILED))
				assert.Loosely(t, res.GetPostStartMessage().GetTime(), should.BeNil)
				assert.Loosely(t, ct.GFake.GetChange(gHost, gChange1).Info.GetMessages(), should.HaveLength(0))
			}

			t.Run("With a non transient failure", func(t *ftt.Test) {
				ct.GFake.MutateChange(gHost, gChange1, func(c *gf.Change) {
					c.ACLs = func(_ gf.Operation, _ string) *status.Status {
						return status.New(codes.PermissionDenied, "admin-is-angry-today")
					}
				})
				check(t)
			})
			t.Run("With a transient failure", func(t *ftt.Test) {
				ct.GFake.MutateChange(gHost, gChange1, func(c *gf.Change) {
					c.ACLs = func(_ gf.Operation, _ string) *status.Status {
						return status.New(codes.Internal, "oops, temp error")
					}
				})
				check(t)
			})
		})
	})
}
