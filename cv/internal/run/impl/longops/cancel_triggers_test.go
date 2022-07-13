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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCancelTriggers(t *testing.T) {
	t.Parallel()

	Convey("CancelTriggers works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject = "infra"
			gHost    = "g-review.example.com"
		)
		runCreateTime := clock.Now(ctx)
		runID := common.MakeRunID(lProject, runCreateTime, 1, []byte("deadbeef"))

		cfg := cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "test"},
			},
		}
		prjcfgtest.Create(ctx, lProject, &cfg)

		initRunAndCLs := func(cis []*gerritpb.ChangeInfo) (*run.Run, common.CLIDs) {
			clids := make(common.CLIDs, len(cis))
			cls := make([]*changelist.CL, len(cis))
			runCLs := make([]*run.RunCL, len(cis))
			for i, ci := range cis {
				So(ci.GetNumber(), ShouldBeGreaterThan, 0)
				So(ci.GetNumber(), ShouldBeLessThan, 1000)
				triggers := trigger.Find(ci, cfg.GetConfigGroups()[0])
				So(triggers.GetCqVoteTrigger(), ShouldNotBeNil)
				So(ct.GFake.Has(gHost, int(ci.GetNumber())), ShouldBeFalse)
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))
				cl := changelist.MustGobID(gHost, ci.GetNumber()).MustCreateIfNotExists(ctx)
				cl.Snapshot = &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: ci,
					}},
					LuciProject:        lProject,
					ExternalUpdateTime: timestamppb.New(runCreateTime),
				}
				cl.EVersion++
				clids[i] = cl.ID
				runCLs[i] = &run.RunCL{
					ID:         cl.ID,
					ExternalID: cl.ExternalID,
					IndexedID:  cl.ID,
					Trigger:    triggers.GetCqVoteTrigger(),
					Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
					Detail:     cl.Snapshot,
				}
				cls[i] = cl
			}
			r := &run.Run{
				ID:            runID,
				Status:        run.Status_RUNNING,
				CLs:           clids,
				Mode:          run.DryRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
			}
			So(datastore.Put(ctx, r, cls, runCLs), ShouldBeNil)
			return r, clids
		}

		makeOp := func(r *run.Run) *CancelTriggersOp {
			reqs := make([]*run.OngoingLongOps_Op_TriggersCancellation_Request, len(r.CLs))
			for i, clid := range r.CLs {
				reqs[i] = &run.OngoingLongOps_Op_TriggersCancellation_Request{
					Clid:    int64(clid),
					Message: fmt.Sprintf("cancel message for CL %d", clid),
					Notify: []run.OngoingLongOps_Op_TriggersCancellation_Whom{
						run.OngoingLongOps_Op_TriggersCancellation_OWNER,
						run.OngoingLongOps_Op_TriggersCancellation_REVIEWERS,
					},
					AddToAttention: []run.OngoingLongOps_Op_TriggersCancellation_Whom{
						run.OngoingLongOps_Op_TriggersCancellation_OWNER,
						run.OngoingLongOps_Op_TriggersCancellation_CQ_VOTERS,
					},
					AddToAttentionReason: fmt.Sprintf("attention reason for CL %d", clid),
				}
			}

			return &CancelTriggersOp{
				Base: &Base{
					Op: &run.OngoingLongOps_Op{
						Deadline:        timestamppb.New(clock.Now(ctx).Add(10000 * time.Hour)), // infinite
						CancelRequested: false,
						Work: &run.OngoingLongOps_Op_CancelTriggers{
							CancelTriggers: &run.OngoingLongOps_Op_TriggersCancellation{
								Requests: reqs,
							},
						},
					},
					IsCancelRequested: func() bool { return false },
					Run:               r,
				},
				GFactory: ct.GFactory(),
			}
		}

		assertTriggerRemoved := func(eid changelist.ExternalID) {
			host, changeID, err := changelist.ExternalID(eid).ParseGobID()
			So(err, ShouldBeNil)
			So(host, ShouldEqual, gHost)
			changeInfo := ct.GFake.GetChange(gHost, int(changeID)).Info
			So(trigger.Find(changeInfo, cfg.GetConfigGroups()[0]), ShouldBeNil)
		}

		testHappyPath := func(prefix string, clCount, concurrency int) {
			Convey(fmt.Sprintf("%s [%d CLs with concurrency %d]", prefix, clCount, concurrency), func() {
				cis := make([]*gerritpb.ChangeInfo, clCount)
				for i := range cis {
					cis[i] = gf.CI(i+1, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute)))
				}
				r, _ := initRunAndCLs(cis)
				startTime := clock.Now(ctx)
				op := makeOp(r)
				op.CancelConcurrency = concurrency
				res, err := op.Do(ctx)
				So(err, ShouldBeNil)
				So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_SUCCEEDED)
				results := res.GetCancelTriggers().GetResults()
				So(results, ShouldHaveLength, clCount)
				processedCLIDs := make(common.CLIDsSet, clCount)
				for _, result := range results {
					So(processedCLIDs.HasI64(result.Id), ShouldBeFalse) // duplicate processing
					processedCLIDs.AddI64(result.Id)
					assertTriggerRemoved(changelist.ExternalID(result.ExternalId))
					So(result.GetSuccessInfo().GetCancelledAt().AsTime(), ShouldHappenOnOrAfter, startTime)
				}
			})
		}

		testHappyPath("single", 1, 1)
		testHappyPath("serial", 4, 1)
		testHappyPath("concurrent", 80, 8)

		// TODO(crbug/1297723): re-enable this test after fixing the flake.
		SkipConvey("Retry on alreadyInLease failure", func() {
			// Creating changes from 1 to `clCount`, lease the CL with duration ==
			// change number * time.Minute.
			clCount := 6
			cis := make([]*gerritpb.ChangeInfo, clCount)
			for i := 1; i <= clCount; i++ {
				cis[i-1] = gf.CI(i, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute)))
			}
			r, clids := initRunAndCLs(cis)
			for i, clid := range clids {
				_, _, err := lease.ApplyOnCL(ctx, clid, time.Duration(cis[i].GetNumber())*time.Minute, "FooBar")
				So(err, ShouldBeNil)
			}
			startTime := clock.Now(ctx)
			op := makeOp(r)
			op.CancelConcurrency = clCount
			op.testAfterTryCancelFn = func() {
				// Advance the clock by 1 minute + 1 second so that the lease will
				// be guaranteed to expire in the next attempt.
				ct.Clock.Add(1*time.Minute + 1*time.Second)
			}
			res, err := op.Do(ctx)
			So(err, ShouldBeNil)
			So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_SUCCEEDED)
			results := res.GetCancelTriggers().GetResults()
			So(results, ShouldHaveLength, len(cis))
			for i, result := range results {
				So(result.Id, ShouldEqual, clids[i])
				So(result.GetSuccessInfo().GetCancelledAt().AsTime(), ShouldHappenAfter, startTime.Add(time.Duration(cis[i].GetNumber())*time.Minute))
				assertTriggerRemoved(changelist.ExternalID(result.ExternalId))
			}
		})

		// TODO(crbug/1199880): test can retry transient failure once Gerrit fake
		// gain the flakiness mode.

		Convey("Failed permanently for non-transient error", func() {
			cis := []*gerritpb.ChangeInfo{
				gf.CI(1, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute))),
				gf.CI(2, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute))),
			}
			r, clids := initRunAndCLs(cis)
			ct.GFake.MutateChange(gHost, 2, func(c *gf.Change) {
				c.ACLs = gf.ACLReadOnly(lProject) // can't mutate
			})
			op := makeOp(r)
			startTime := clock.Now(ctx)
			res, err := op.Do(ctx)
			So(err, ShouldNotBeNil)
			So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_FAILED)
			results := res.GetCancelTriggers().GetResults()
			So(results, ShouldHaveLength, len(cis))
			for _, result := range results {
				switch common.CLID(result.Id) {
				case clids[0]: // Change 1
					So(result.GetSuccessInfo().GetCancelledAt().AsTime(), ShouldHappenAfter, startTime)
				case clids[1]: // Change 2
					So(result.GetFailureInfo().GetFailureMessage(), ShouldNotBeEmpty)
				}
				So(result.ExternalId, ShouldNotBeEmpty)
			}
		})

		Convey("Doesn't obey long op cancellation", func() {
			ci := gf.CI(1, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute)))
			cis := []*gerritpb.ChangeInfo{ci}
			r, clids := initRunAndCLs(cis)
			op := makeOp(r)
			op.IsCancelRequested = func() bool { return true }
			res, err := op.Do(ctx)
			So(err, ShouldBeNil)
			So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_SUCCEEDED)
			results := res.GetCancelTriggers().GetResults()
			So(results, ShouldHaveLength, len(cis))
			for i, result := range results {
				So(result.Id, ShouldEqual, clids[i])
				assertTriggerRemoved(changelist.ExternalID(result.ExternalId))
				So(result.GetSuccessInfo().GetCancelledAt(), ShouldNotBeNil)
			}
		})
	})
}
