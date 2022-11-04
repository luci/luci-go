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

package handler

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStart(t *testing.T) {
	t.Parallel()

	Convey("StartRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject           = "chromium"
			configGroupName    = "combinable"
			gerritHost         = "chromium-review.googlesource.com"
			committers         = "committer-group"
			dryRunners         = "dry-runner-group"
			stabilizationDelay = time.Minute
			startLatency       = 2 * time.Minute
		)

		builder := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "cool_tester",
		}
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{
			Name: configGroupName,
			CombineCls: &cfgpb.CombineCLs{
				StabilizationDelay: durationpb.New(stabilizationDelay),
			},
			Verifiers: &cfgpb.Verifiers{
				GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
					CommitterList:    []string{committers},
					DryRunAccessList: []string{dryRunners},
				},
				Tryjob: &cfgpb.Verifiers_Tryjob{
					Builders: []*cfgpb.Verifiers_Tryjob_Builder{
						{
							Name: bbutil.FormatBuilderID(builder),
						},
					},
				},
			},
		}}})

		So(srvcfg.SetTestMigrationConfig(ctx, &migrationpb.Settings{
			ApiHosts: []*migrationpb.Settings_ApiHost{
				{
					Host:          ct.Env.LogicalHostname,
					Prod:          true,
					ProjectRegexp: []string{".*"},
				},
			},
			UseCvTryjobExecutor: &migrationpb.Settings_UseCVTryjobExecutor{
				ProjectRegexp: []string{lProject},
			},
		}), ShouldBeNil)

		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-deadbeef",
				Status:        run.Status_PENDING,
				CreateTime:    clock.Now(ctx).UTC().Add(-startLatency),
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				Mode:          run.DryRun,
			},
		}
		h, _ := makeTestHandler(&ct)

		var clid common.CLID
		addCL := func(triggerer, owner string) *changelist.CL {
			clid++
			rs.CLs = append(rs.CLs, clid)
			ci := gf.CI(100+int(clid),
				gf.Owner(owner),
				gf.CQ(+1, rs.CreateTime, gf.U(triggerer)))
			cl := &changelist.CL{
				ID:         clid,
				ExternalID: changelist.MustGobID(gerritHost, ci.GetNumber()),
				Snapshot: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gerritHost,
							Info: ci,
						},
					},
				},
			}
			rCL := &run.RunCL{
				ID:  clid,
				Run: datastore.MakeKey(ctx, common.RunKind, string(rs.ID)),
				Trigger: &run.Trigger{
					Email: gf.U(triggerer).Email,
					Time:  timestamppb.New(rs.CreateTime),
					Mode:  string(rs.Mode),
				},
			}
			So(datastore.Put(ctx, cl, rCL), ShouldBeNil)
			return cl
		}

		const (
			owner     = "user-1"
			triggerer = owner
		)
		cl := addCL(triggerer, owner)
		ct.AddMember(owner, dryRunners)
		ct.AddMember(owner, committers)

		Convey("Starts when Run is PENDING", func() {
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)

			So(res.State.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.StartTime, ShouldEqual, ct.Clock.Now().UTC())
			So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
				Requirement: &tryjob.Requirement{
					Definitions: []*tryjob.Definition{
						{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host:    chromeinfra.BuildbucketHost,
									Builder: builder,
								},
							},
							Critical: true,
						},
					},
				},
				RequirementVersion:    1,
				RequirementComputedAt: timestamppb.New(ct.Clock.Now().UTC()),
			})
			So(res.State.UseCVTryjobExecutor, ShouldBeTrue)
			So(res.State.LogEntries, ShouldHaveLength, 2)
			So(res.State.LogEntries[0].GetInfo().GetMessage(), ShouldEqual, "LUCI CV is managing the Tryjobs for this Run")
			So(res.State.LogEntries[1].GetStarted(), ShouldNotBeNil)

			So(res.State.NewLongOpIDs, ShouldHaveLength, 2)
			So(res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]].GetExecuteTryjobs(), ShouldNotBeNil)
			So(res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[1]].GetPostStartMessage(), ShouldBeTrue)

			So(res.SideEffectFn, ShouldNotBeNil)
			So(datastore.RunInTransaction(ctx, res.SideEffectFn, nil), ShouldBeNil)
			So(ct.TSMonSentValue(ctx, metrics.Public.RunStarted, lProject, configGroupName, string(run.DryRun)), ShouldEqual, 1)
			So(ct.TSMonSentDistr(ctx, metricPickupLatencyS, lProject).Sum(),
				ShouldAlmostEqual, startLatency.Seconds())
			So(ct.TSMonSentDistr(ctx, metricPickupLatencyAdjustedS, lProject).Sum(),
				ShouldAlmostEqual, (startLatency - stabilizationDelay).Seconds())
		})

		Convey("Don't use CV tryjob executor", func() {
			So(srvcfg.SetTestMigrationConfig(ctx, &migrationpb.Settings{
				ApiHosts: []*migrationpb.Settings_ApiHost{
					{
						Host:          ct.Env.LogicalHostname,
						Prod:          true,
						ProjectRegexp: []string{".*"},
					},
				},
				UseCvTryjobExecutor: &migrationpb.Settings_UseCVTryjobExecutor{
					ProjectRegexpExclude: []string{lProject},
				},
			}), ShouldBeNil)
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.SideEffectFn, ShouldNotBeNil)
			So(res.PreserveEvents, ShouldBeFalse)

			So(res.State.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.Tryjobs, ShouldBeNil)
			So(res.State.UseCVTryjobExecutor, ShouldBeFalse)

			So(res.State.NewLongOpIDs, ShouldHaveLength, 1)
			So(res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]].GetPostStartMessage(), ShouldBeTrue)
		})
		Convey("Fail the Run if Run Option is invalid", func() {
			rs.Options = &run.Options{
				CustomTryjobTags: []string{"BAD TAG", "ANOTHER_ONE", "good:tag_foo"},
			}
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)

			So(res.State.Status, ShouldEqual, run.Status_PENDING)
			So(res.State.Tryjobs, ShouldBeNil)
			So(res.State.NewLongOpIDs, ShouldHaveLength, 1)
			op := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
			So(op.GetCancelTriggers(), ShouldNotBeNil)
			So(op.GetCancelTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_FAILED)
			cancelledCLs := common.CLIDs{}
			for _, req := range op.GetCancelTriggers().GetRequests() {
				cancelledCLs = append(cancelledCLs, common.CLID(req.Clid))
			}
			So(cancelledCLs, ShouldResemble, res.State.CLs)
			So(op.GetCancelTriggers().GetRequests()[0].GetMessage(), ShouldEqual, strings.TrimSpace(`
Failed to start the Run. Reason:

* malformed tag: "BAD TAG"; expecting format "^[a-z0-9_\\-]+:.+$"
* malformed tag: "ANOTHER_ONE"; expecting format "^[a-z0-9_\\-]+:.+$"
`))
		})

		Convey("Fail the Run if tryjob computation fails", func() {
			if rs.Options == nil {
				rs.Options = &run.Options{}
			}
			// included a builder that doesn't exist
			rs.Options.IncludedTryjobs = append(rs.Options.IncludedTryjobs, "fooproj/ci:bar_builder")
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)

			So(res.State.Status, ShouldEqual, run.Status_PENDING)
			So(res.State.Tryjobs, ShouldBeNil)
			So(res.State.UseCVTryjobExecutor, ShouldBeTrue)
			So(res.State.NewLongOpIDs, ShouldHaveLength, 1)
			op := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
			So(op.GetCancelTriggers(), ShouldNotBeNil)
			So(op.GetCancelTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_FAILED)
			cancelledCLs := common.CLIDs{}
			for _, req := range op.GetCancelTriggers().GetRequests() {
				cancelledCLs = append(cancelledCLs, common.CLID(req.Clid))
			}
			So(cancelledCLs, ShouldResemble, res.State.CLs)
			So(res.State.LogEntries, ShouldHaveLength, 1)
			So(res.State.LogEntries[0].GetInfo(), ShouldResembleProto, &run.LogEntry_Info{
				Label:   "Tryjob Requirement Computation",
				Message: "Failed to compute tryjob requirement. Reason: builder \"fooproj/ci/bar_builder\" is included but not defined in the LUCI project",
			})
		})

		Convey("Fail the Run if acls.CheckRunCreate fails", func() {
			ct.ResetMockedAuthDB(ctx)
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)

			So(res.State.Status, ShouldEqual, run.Status_PENDING)
			So(res.State.LogEntries, ShouldHaveLength, 1)
			So(res.State.LogEntries[0].GetInfo(), ShouldResembleProto, &run.LogEntry_Info{
				Label: "Run failed",
				Message: "" +
					"the Run does not pass eligibility checks. See reasons at:" +
					"\n  * " + cl.ExternalID.MustURL(),
			})

			So(res.State.NewLongOpIDs, ShouldHaveLength, 1)
			longOp := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
			cancelOp := longOp.GetCancelTriggers()
			So(cancelOp.Requests, ShouldHaveLength, 1)
			So(cancelOp.Requests[0], ShouldResembleProto,
				&run.OngoingLongOps_Op_TriggersCancellation_Request{
					Clid: int64(cl.ID),
					Message: fmt.Sprintf(
						"CV cannot start a Run for `%s` because the user is not a dry-runner.", gf.U(triggerer).Email,
					),
					Notify: []run.OngoingLongOps_Op_TriggersCancellation_Whom{
						run.OngoingLongOps_Op_TriggersCancellation_OWNER,
						run.OngoingLongOps_Op_TriggersCancellation_CQ_VOTERS,
					},
					AddToAttention: []run.OngoingLongOps_Op_TriggersCancellation_Whom{
						run.OngoingLongOps_Op_TriggersCancellation_OWNER,
						run.OngoingLongOps_Op_TriggersCancellation_CQ_VOTERS,
					},
					AddToAttentionReason: "CQ/CV Run failed",
				},
			)
			So(cancelOp.RunStatusIfSucceeded, ShouldEqual, run.Status_FAILED)
		})

		statuses := []run.Status{
			run.Status_RUNNING,
			run.Status_WAITING_FOR_SUBMISSION,
			run.Status_SUBMITTING,
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Status = status
				res, err := h.Start(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			})
		}
	})
}

func TestOnCompletedPostStartMessage(t *testing.T) {
	t.Parallel()

	Convey("onCompletedPostStartMessage works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject = "chromium"
			opID     = "1-1"
		)

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{Name: "single"}}})

		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-1-deadbeef",
				Status:        run.Status_RUNNING,
				Mode:          run.DryRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						opID: {
							Work: &run.OngoingLongOps_Op_PostStartMessage{
								PostStartMessage: true,
							},
						},
					},
				},
			},
		}
		result := &eventpb.LongOpCompleted{
			OperationId: opID,
		}
		h, _ := makeTestHandler(&ct)

		Convey("if Run isn't RUNNING, just cleans up the operation", func() {
			// NOTE: This should be rare. And since posting the starting message isn't
			// a critical operation, it's OK to ignore its failures if the Run is
			// already submitting the CL.
			rs.Run.Status = run.Status_SUBMITTING
			result.Status = eventpb.LongOpCompleted_FAILED
			// The result is set in practice but serves debugging purposes only,
			// and is ignored by the onCompletedPostStartMessage.
			result.Result = &eventpb.LongOpCompleted_PostStartMessage_{
				PostStartMessage: &eventpb.LongOpCompleted_PostStartMessage{
					PermanentErrors: map[int64]string{1: "Gerrit refused to post the start message"},
				},
			}

			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_SUBMITTING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
		})

		Convey("on cancellation, cleans up Run's state", func() {
			// NOTE: as of this writing (Oct 2021), the only time posting start
			// message is cancelled is if the Run was already finalized. Therefore,
			// Run can't be in RUNNING state any more.
			// However, this test aims to cover possible future logic change in CV.
			result.Status = eventpb.LongOpCompleted_CANCELLED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
		})

		Convey("on success, cleans Run's state", func() {
			result.Status = eventpb.LongOpCompleted_SUCCEEDED
			// The result is set in practice but serves debugging purposes only,
			// and is ignored by the onCompletedPostStartMessage.
			postedAt := ct.Clock.Now().Add(-time.Second)
			result.Result = &eventpb.LongOpCompleted_PostStartMessage_{
				PostStartMessage: &eventpb.LongOpCompleted_PostStartMessage{
					Posted: []int64{1},
					Time:   timestamppb.New(postedAt),
				},
			}
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.State.LogEntries[0].GetTime().AsTime(), ShouldResemble, postedAt.UTC())
		})

		Convey("on failure, cleans Run's state and record reasons", func() {
			result.Status = eventpb.LongOpCompleted_FAILED
			// The result is set in practice but serves debugging purposes only,
			// and is ignored by the onCompletedPostStartMessage.
			result.Result = &eventpb.LongOpCompleted_PostStartMessage_{
				PostStartMessage: &eventpb.LongOpCompleted_PostStartMessage{
					PermanentErrors: map[int64]string{1: "Gerrit refused to post the start message"},
					Posted:          []int64{2},
				},
			}
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.State.LogEntries[0].GetInfo().GetMessage(), ShouldContainSubstring, "Failed to post the starting message")
		})

		Convey("on expiration,cleans Run's state and record reasons", func() {
			result.Status = eventpb.LongOpCompleted_EXPIRED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.State.LogEntries[0].GetInfo().GetMessage(), ShouldContainSubstring, "Failed to post the starting message")
		})
	})
}
