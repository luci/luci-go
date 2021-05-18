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
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOnReadyForSubmission(t *testing.T) {
	t.Parallel()

	Convey("OnReadyForSubmission", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "l_project"
		const gHost = "x-review.example.com"
		rid := common.MakeRunID(lProject, ct.Clock.Now().Add(-2*time.Minute), 1, []byte("deadbeef"))
		runCLs := common.CLIDs{1, 2}
		r := run.Run{
			ID:         rid,
			Status:     run.Status_RUNNING,
			CreateTime: ct.Clock.Now().UTC().Add(-2 * time.Minute),
			StartTime:  ct.Clock.Now().UTC().Add(-1 * time.Minute),
			CLs:        runCLs,
		}
		cg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "main"},
			},
		}
		ct.Cfg.Create(ctx, rid.LUCIProject(), cg)
		meta, err := config.GetLatestMeta(ctx, rid.LUCIProject())
		So(err, ShouldBeNil)
		So(meta.ConfigGroupIDs, ShouldHaveLength, 1)
		r.ConfigGroupID = meta.ConfigGroupIDs[0]

		ci1 := gf.CI(
			1111, gf.PS(2),
			gf.CQ(2, ct.Clock.Now().Add(-2*time.Minute), gf.U("user-100")),
			gf.Updated(clock.Now(ctx).Add(-1*time.Minute)))
		trigger1 := trigger.Find(ci1, cg.ConfigGroups[0])
		So(trigger1.GetGerritAccountId(), ShouldEqual, 100)

		ci2 := gf.CI(
			2222, gf.PS(3),
			gf.CQ(2, ct.Clock.Now().Add(-2*time.Minute), gf.U("user-100")),
			gf.Updated(clock.Now(ctx).Add(-1*time.Minute)))
		trigger2 := trigger.Find(ci1, cg.ConfigGroups[0])
		So(trigger2.GetGerritAccountId(), ShouldEqual, 100)
		So(datastore.Put(ctx, &r,
			&run.RunCL{
				ID:         1,
				Run:        datastore.MakeKey(ctx, run.RunKind, string(rid)),
				ExternalID: changelist.MustGobID(gHost, ci1.GetNumber()),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: proto.Clone(ci1).(*gerritpb.ChangeInfo),
						},
					},
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
					},
				},
				Trigger: trigger1,
			},
			&changelist.CL{
				ID:         1,
				ExternalID: changelist.MustGobID(gHost, ci1.GetNumber()),
				EVersion:   10,
				Snapshot: &changelist.Snapshot{
					ExternalUpdateTime:    timestamppb.New(clock.Now(ctx).Add(-1 * time.Minute)),
					LuciProject:           lProject,
					Patchset:              2,
					MinEquivalentPatchset: 1,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: proto.Clone(ci1).(*gerritpb.ChangeInfo),
						},
					},
				},
			},
			&run.RunCL{
				ID:         2,
				Run:        datastore.MakeKey(ctx, run.RunKind, string(rid)),
				ExternalID: changelist.MustGobID(gHost, ci2.GetNumber()),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: proto.Clone(ci2).(*gerritpb.ChangeInfo),
						},
					},
				},
				Trigger: trigger2,
			},
			&changelist.CL{
				ID:         2,
				ExternalID: changelist.MustGobID(gHost, ci2.GetNumber()),
				EVersion:   10,
				Snapshot: &changelist.Snapshot{
					ExternalUpdateTime:    timestamppb.New(clock.Now(ctx).Add(-1 * time.Minute)),
					LuciProject:           lProject,
					Patchset:              2,
					MinEquivalentPatchset: 1,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: proto.Clone(ci2).(*gerritpb.ChangeInfo),
						},
					},
				},
			},
		), ShouldBeNil)

		ct.GFake.CreateChange(&gf.Change{
			Host: gHost,
			Info: proto.Clone(ci1).(*gerritpb.ChangeInfo),
			ACLs: gf.ACLRestricted(lProject),
		})
		ct.GFake.CreateChange(&gf.Change{
			Host: gHost,
			Info: proto.Clone(ci2).(*gerritpb.ChangeInfo),
			ACLs: gf.ACLRestricted(lProject),
		})

		rs := &state.RunState{Run: r}

		h := &Impl{
			RM: run.NewNotifier(ct.TQDispatcher),
		}

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Release submit queue when Run is %s", status), func() {
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					waitlisted, err := submit.TryAcquire(ctx, h.RM.NotifyReadyForSubmission, rs.Run.ID, nil)
					So(waitlisted, ShouldBeFalse)
					return err
				}, nil), ShouldBeNil)
				rs.Run.Status = status
				res, err := h.OnReadyForSubmission(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				current, waitlist, err := submit.LoadCurrentAndWaitlist(ctx, rs.Run.ID)
				So(err, ShouldBeNil)
				So(current, ShouldBeEmpty)
				So(waitlist, ShouldBeEmpty)
			})
		}

		now := ct.Clock.Now().UTC()
		ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")
		Convey("When status is SUBMITTING", func() {
			rs.Run.Status = run.Status_SUBMITTING
			Convey("Continue submission if TaskID matches and within deadline", func() {
				rs.Run.Submission = &run.Submission{
					Deadline: timestamppb.New(now.Add(10 * time.Minute)), // within deadline
					TaskId:   "task-foo",                                 // same task ID as the current task
				}
				res, err := h.OnReadyForSubmission(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State.Run.Status, ShouldEqual, run.Status_SUBMITTING)
				So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
					Deadline: timestamppb.New(now.Add(10 * time.Minute)),
					TaskId:   "task-foo",
				}) // unchanged
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldNotBeNil)
			})

			Convey("Sends Poke if TaskID doesn't match and within deadline", func() {
				rs.Run.Submission = &run.Submission{
					Deadline: timestamppb.New(now.Add(10 * time.Minute)), // within deadline
					TaskId:   "task-bar",
				}
				res, err := h.OnReadyForSubmission(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				runtest.AssertReceivedPoke(ctx, rs.Run.ID, rs.Run.Submission.Deadline.AsTime())
			})

			Convey("Deadline has expired", func() {
				rs.Run.Submission = &run.Submission{
					Deadline: timestamppb.New(now.Add(-1 * time.Minute)), // expired
					Cls:      []int64{2, 1},
					TaskId:   "task-bar",
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					waitlisted, err := submit.TryAcquire(ctx, h.RM.NotifyReadyForSubmission, rid, nil)
					So(waitlisted, ShouldBeFalse)
					return err
				}, nil), ShouldBeNil)
				assertSubmitQueueReleased := func() {
					cur, err := submit.CurrentRun(ctx, lProject)
					So(err, ShouldBeNil)
					So(cur, ShouldBeEmpty)
				}
				Convey("None of the CLs are submitted", func() {
					res, err := h.OnReadyForSubmission(ctx, rs)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_FAILED)
					So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now())
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					for _, ci := range []*gerritpb.ChangeInfo{ci1, ci2} {
						ci := ct.GFake.GetChange(gHost, int(ci.GetNumber())).Info
						So(ci, gf.ShouldLastMessageContain, timeoutMsg)
						So(ci, gf.ShouldLastMessageContain, "None of the CLs in the Run were submitted by CV")
						So(ci, gf.ShouldLastMessageContain, "CLs: [https://x-review.example.com/2222, https://x-review.example.com/1111]")
						for _, vote := range ci.GetLabels()[trigger.CQLabelName].GetAll() {
							So(vote.GetValue(), ShouldEqual, 0)
						}
					}
					assertSubmitQueueReleased()
				})

				Convey("CLs partially submitted", func() {
					rs.Run.Submission.SubmittedCls = []int64{2}
					res, err := h.OnReadyForSubmission(ctx, rs)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_FAILED)
					So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now())
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					So(ct.GFake.GetChange(gHost, int(ci2.GetNumber())).Info, ShouldResembleProto, ci2) // ci2 untouched
					ci := ct.GFake.GetChange(gHost, int(ci1.GetNumber())).Info
					So(ci, gf.ShouldLastMessageContain, timeoutMsg)
					So(ci, gf.ShouldLastMessageContain, "CV partially submitted the CLs in the Run")
					So(ci, gf.ShouldLastMessageContain, "Not submitted: [https://x-review.example.com/1111]")
					So(ci, gf.ShouldLastMessageContain, "Submitted: [https://x-review.example.com/2222]")
					for _, vote := range ci.GetLabels()[trigger.CQLabelName].GetAll() {
						So(vote.GetValue(), ShouldEqual, 0)
					}
					assertSubmitQueueReleased()
				})

				Convey("CLs fully submitted", func() {
					rs.Run.Submission.SubmittedCls = []int64{2, 1}
					res, err := h.OnReadyForSubmission(ctx, rs)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_SUCCEEDED)
					So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now())
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					// both untouched
					So(ct.GFake.GetChange(gHost, int(ci1.GetNumber())).Info, ShouldResembleProto, ci1)
					So(ct.GFake.GetChange(gHost, int(ci2.GetNumber())).Info, ShouldResembleProto, ci2)
					assertSubmitQueueReleased()
				})
			})
		})

		for _, status := range []run.Status{run.Status_RUNNING, run.Status_WAITING_FOR_SUBMISSION} {
			Convey(fmt.Sprintf("Mark submitting when status is %s", status), func() {
				rs.Run.Status = status
				res, err := h.OnReadyForSubmission(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State.Run.Status, ShouldEqual, run.Status_SUBMITTING)
				So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
					Deadline: timestamppb.New(now.Add(20 * time.Minute)),
					Cls:      []int64{2, 1}, // in submission order
					TaskId:   "task-foo",
				})
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldNotBeNil)
			})
		}
	})
}

func TestSubmitter(t *testing.T) {
	Convey("Submitter", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject = "test_proj"
			gHost1   = "gerrit-1.example.com"
			gHost2   = "gerrit-2.example.com"
		)
		ci1 := gf.CI(1, gf.PS(3), gf.AllRevs(), gf.CQ(2))
		ci2 := gf.CI(2, gf.PS(5), gf.AllRevs(), gf.CQ(2))
		ct.GFake.AddFrom(gf.WithCIs(gHost1, gf.ACLRestricted(lProject), ci1))
		ct.GFake.AddFrom(gf.WithCIs(gHost2, gf.ACLRestricted(lProject), ci2))

		now := ct.Clock.Now().UTC()
		s := submitter{
			runID:    common.MakeRunID(lProject, now, 1, []byte("deadbeef")),
			deadline: now.Add(1 * time.Minute),
			clids:    common.CLIDs{1, 2},
			rm:       run.NewNotifier(ct.TQDispatcher),
		}
		So(datastore.Put(ctx,
			&run.Run{
				ID:         s.runID,
				Status:     run.Status_RUNNING,
				CreateTime: now,
				StartTime:  now,
				CLs:        s.clids,
			},
			&run.RunCL{
				ID:  1,
				Run: datastore.MakeKey(ctx, run.RunKind, string(s.runID)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost1,
							Info: ci1,
						},
					},
				},
			},
			&run.RunCL{
				ID:  2,
				Run: datastore.MakeKey(ctx, run.RunKind, string(s.runID)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost2,
							Info: ci2,
						},
					},
				},
			},
		), ShouldBeNil)
		So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			waitlisted, err := submit.TryAcquire(ctx, s.rm.NotifyReadyForSubmission, s.runID, nil)
			So(err, ShouldBeNil)
			So(waitlisted, ShouldBeFalse)
			return err
		}, nil), ShouldBeNil)

		verifyRunReleased := func(runID common.RunID) {
			current, waitlist, err := submit.LoadCurrentAndWaitlist(ctx, runID)
			So(err, ShouldBeNil)
			So(current, ShouldNotEqual, runID)
			So(waitlist.Index(runID), ShouldBeLessThan, 0) // doesn't exist
		}

		Convey("PreCondition Failure", func() {
			ctx = memlogger.Use(ctx)
			log := logging.Get(ctx).(*memlogger.MemLogger)
			Convey("Submit queue not acquired", func() {
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return submit.Release(ctx, s.rm.NotifyReadyForSubmission, s.runID)
				}, nil), ShouldBeNil)
				So(s.submit(ctx), ShouldBeNil)
				runtest.AssertReceivedSubmissionCompleted(ctx, s.runID,
					&eventpb.SubmissionCompleted{
						Result: eventpb.SubmissionResult_FAILED_PRECONDITION,
					},
				)
				So(log, memlogger.ShouldHaveLog, logging.Warning, "run no longer holds submit queue, currently held by")
			})
		})

		Convey("Submit successfully", func() {
			So(s.submit(ctx), ShouldBeNil)
			verifyRunReleased(s.runID)
			runtest.AssertReceivedCLSubmitted(ctx, s.runID, 1)
			So(ct.GFake.GetChange(gHost1, 1).Info.GetStatus(), ShouldEqual, gerritpb.ChangeStatus_MERGED)
			runtest.AssertReceivedCLSubmitted(ctx, s.runID, 2)
			So(ct.GFake.GetChange(gHost2, 2).Info.GetStatus(), ShouldEqual, gerritpb.ChangeStatus_MERGED)
			So(ct.GFake.Requests(), ShouldHaveLength, len(s.clids)) // len(s.clids) SubmitRevision calls
			runtest.AssertReceivedSubmissionCompleted(ctx, s.runID,
				&eventpb.SubmissionCompleted{
					Result: eventpb.SubmissionResult_SUCCEEDED,
				},
			)
		})

		// TODO(crbug/1199880): support flakiness for Gerrit fake to test submit
		// will retry individual CL on transient error and not release queue
		// for transient failure.
		// Also test that submission has exhausted the allocated time.

		Convey("Submit fails permanently when", func() {
			Convey("No submit privilege", func() {
				// Submit gHost1/1 successfully but lack of submission right to
				// gHost2/2.
				ct.GFake.MutateChange(gHost2, 2, func(c *gf.Change) {
					c.ACLs = gf.ACLGrant(gf.OpSubmit, codes.PermissionDenied, "another_project")
				})
				So(s.submit(ctx), ShouldBeNil)
				verifyRunReleased(s.runID)
				runtest.AssertReceivedCLSubmitted(ctx, s.runID, 1)
				So(ct.GFake.GetChange(gHost1, 1).Info.GetStatus(), ShouldEqual, gerritpb.ChangeStatus_MERGED)
				runtest.AssertNotReceivedCLSubmitted(ctx, s.runID, 2)
				So(ct.GFake.GetChange(gHost2, 2).Info.GetStatus(), ShouldEqual, gerritpb.ChangeStatus_NEW)
				runtest.AssertReceivedSubmissionCompleted(ctx, s.runID,
					&eventpb.SubmissionCompleted{
						Result: eventpb.SubmissionResult_FAILED_PERMANENT,
						FailureReason: &eventpb.SubmissionCompleted_ClFailure{
							ClFailure: &eventpb.SubmissionCompleted_CLSubmissionFailure{
								Clid:    2,
								Message: permDeniedMsg,
							},
						},
					},
				)
			})
			Convey("A new revision is uploaded ", func() {
				// gHost2/2 gets a new PS.
				ct.GFake.MutateChange(gHost2, 2, func(c *gf.Change) {
					c.Info = proto.Clone(ci2).(*gerritpb.ChangeInfo)
					gf.PS(6)(c.Info)
				})
				So(s.submit(ctx), ShouldBeNil)
				verifyRunReleased(s.runID)
				So(ct.GFake.GetChange(gHost1, 1).Info.GetStatus(), ShouldEqual, gerritpb.ChangeStatus_MERGED)
				runtest.AssertNotReceivedCLSubmitted(ctx, s.runID, 2)
				So(ct.GFake.GetChange(gHost2, 2).Info.GetStatus(), ShouldEqual, gerritpb.ChangeStatus_NEW)
				runtest.AssertReceivedSubmissionCompleted(ctx, s.runID,
					&eventpb.SubmissionCompleted{
						Result: eventpb.SubmissionResult_FAILED_PERMANENT,
						FailureReason: &eventpb.SubmissionCompleted_ClFailure{
							ClFailure: &eventpb.SubmissionCompleted_CLSubmissionFailure{
								Clid:    2,
								Message: fmt.Sprintf(failedPreconditionMsgFmt, fmt.Sprintf("rpc error: code = FailedPrecondition desc = revision %s is not current revision", ci2.GetCurrentRevision())),
							},
						},
					},
				)
			})
		})

		Convey("Change has already been merged", func() {
			ct.GFake.MutateChange(gHost1, 1, func(c *gf.Change) {
				c.Info = proto.Clone(ci1).(*gerritpb.ChangeInfo)
				gf.Status(gerritpb.ChangeStatus_MERGED)(c.Info)
			})
			// Submitter should receive FailedPrecondition failure from Gerrit
			// for Submit RPC. But the subsequent GetChange will figure out that
			// Change has been merged already and consider submission of gHost1/1
			// as a success.
			So(s.submit(ctx), ShouldBeNil)
			verifyRunReleased(s.runID)
			runtest.AssertReceivedCLSubmitted(ctx, s.runID, 1)
			So(ct.GFake.GetChange(gHost1, 1).Info.GetStatus(), ShouldEqual, gerritpb.ChangeStatus_MERGED)
			So(ct.GFake.Requests(), ShouldHaveLength, len(s.clids)+1) // 1 extra getChange call
			runtest.AssertReceivedSubmissionCompleted(ctx, s.runID,
				&eventpb.SubmissionCompleted{
					Result: eventpb.SubmissionResult_SUCCEEDED,
				},
			)
		})
	})
}

func TestOnCLSubmitted(t *testing.T) {
	t.Parallel()

	Convey("OnCLSubmitted", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		rid := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("deadbeef"))
		rs := &state.RunState{Run: run.Run{
			ID:         rid,
			Status:     run.Status_SUBMITTING,
			CreateTime: ct.Clock.Now().UTC().Add(-2 * time.Minute),
			StartTime:  ct.Clock.Now().UTC().Add(-1 * time.Minute),
			CLs:        common.CLIDs{1, 3, 5, 7},
			Submission: &run.Submission{
				Cls: []int64{3, 1, 7, 5}, // in submission order
			},
		}}

		h := &Impl{}
		Convey("Single", func() {
			res, err := h.OnCLSubmitted(ctx, rs, common.CLIDs{3})
			So(err, ShouldBeNil)
			So(res.State.Run.Submission.SubmittedCls, ShouldResemble, []int64{3})

		})
		Convey("Duplicate", func() {
			res, err := h.OnCLSubmitted(ctx, rs, common.CLIDs{3, 3, 3, 3, 1, 1, 1})
			So(err, ShouldBeNil)
			So(res.State.Run.Submission.SubmittedCls, ShouldResemble, []int64{3, 1})
		})
		Convey("Obey Submission order", func() {
			res, err := h.OnCLSubmitted(ctx, rs, common.CLIDs{1, 3, 5, 7})
			So(err, ShouldBeNil)
			So(res.State.Run.Submission.SubmittedCls, ShouldResemble, []int64{3, 1, 7, 5})
		})
		Convey("Merge to existing", func() {
			rs.Run.Submission.SubmittedCls = []int64{3, 1}
			// 1 should be deduped
			res, err := h.OnCLSubmitted(ctx, rs, common.CLIDs{1, 7})
			So(err, ShouldBeNil)
			So(res.State.Run.Submission.SubmittedCls, ShouldResemble, []int64{3, 1, 7})
		})
		Convey("Last cl arrives first", func() {
			res, err := h.OnCLSubmitted(ctx, rs, common.CLIDs{5})
			So(err, ShouldBeNil)
			So(res.State.Run.Submission.SubmittedCls, ShouldResemble, []int64{5})
			res, err = h.OnCLSubmitted(ctx, rs, common.CLIDs{1, 3})
			So(err, ShouldBeNil)
			So(res.State.Run.Submission.SubmittedCls, ShouldResemble, []int64{3, 1, 5})
			res, err = h.OnCLSubmitted(ctx, rs, common.CLIDs{7})
			So(err, ShouldBeNil)
			So(res.State.Run.Submission.SubmittedCls, ShouldResemble, []int64{3, 1, 7, 5})
		})
		Convey("Error for unknown CLs", func() {
			res, err := h.OnCLSubmitted(ctx, rs, common.CLIDs{1, 3, 5, 7, 9, 11})
			So(err, ShouldErrLike, "received CLSubmitted event for cls not belonging to this Run: [9 11]")
			So(res, ShouldBeNil)
		})
	})
}
