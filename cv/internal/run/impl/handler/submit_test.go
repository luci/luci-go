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
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/cv/internal/tree"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOnReadyForSubmission(t *testing.T) {
	t.Parallel()

	Convey("OnReadyForSubmission", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		runNotifier := run.NewNotifier(ct.TQDispatcher)

		rid := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("deadbeef"))
		runCLs := common.CLIDs{1, 2}
		r := run.Run{
			ID:         rid,
			Status:     run.Status_RUNNING,
			CreateTime: ct.Clock.Now().UTC().Add(-2 * time.Minute),
			StartTime:  ct.Clock.Now().UTC().Add(-1 * time.Minute),
			CLs:        runCLs,
		}
		ct.Cfg.Create(ctx, rid.LUCIProject(), &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "main"},
			},
		})
		meta, err := config.GetLatestMeta(ctx, rid.LUCIProject())
		So(err, ShouldBeNil)
		So(meta.ConfigGroupIDs, ShouldHaveLength, 1)
		r.ConfigGroupID = meta.ConfigGroupIDs[0]
		So(datastore.Put(ctx, &r,
			&run.RunCL{
				ID:  runCLs[0],
				Run: datastore.MakeKey(ctx, run.RunKind, string(rid)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: "example.com",
							Info: gf.CI(1111),
						},
					},
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
					},
				},
			},
			&run.RunCL{
				ID:  runCLs[1],
				Run: datastore.MakeKey(ctx, run.RunKind, string(rid)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: "example.com",
							Info: gf.CI(2222),
						},
					},
				},
			},
		), ShouldBeNil)
		rs := &state.RunState{
			Run:         r,
			RunNotifier: runNotifier,
		}

		h := &Impl{}

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Release submit queue when Run is %s", status), func() {
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					waitlisted, err := submit.TryAcquire(ctx, rs.RunNotifier, rs.Run.ID, nil)
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
					Deadline:     timestamppb.New(now.Add(10 * time.Minute)), // within deadline
					AttemptCount: 1,
					TaskId:       "task-foo", // same task ID as the current task
				}
				res, err := h.OnReadyForSubmission(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State.Run.Status, ShouldEqual, run.Status_SUBMITTING)
				So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
					Deadline:     timestamppb.New(now.Add(10 * time.Minute)),
					AttemptCount: 1,
					TaskId:       "task-foo",
				}) // unchanged
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldNotBeNil)
			})

			Convey("Sends Poke if TaskID doesn't match and within deadline", func() {
				rs.Run.Submission = &run.Submission{
					Deadline:     timestamppb.New(now.Add(10 * time.Minute)), // within deadline
					AttemptCount: 1,
					TaskId:       "task-bar",
				}
				res, err := h.OnReadyForSubmission(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				runtest.AssertReceivedPoke(ctx, rs.Run.ID, rs.Run.Submission.Deadline.AsTime())
			})

			Convey("Re-acquire submit queue if deadline is exceeded", func() {
				rs.Run.Submission = &run.Submission{
					Deadline:     timestamppb.New(now.Add(-1 * time.Minute)), // expired
					AttemptCount: 1,
					TaskId:       "task-bar",
				}

				Convey("And if waitlisted, fall back to WAITING_FOR_SUBMISSION status", func() {
					// submit queue is taken by another run.
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						waitlisted, err := submit.TryAcquire(ctx, rs.RunNotifier, common.MakeRunID("infra", now, 1, []byte("another-run")), nil)
						So(waitlisted, ShouldBeFalse)
						return err
					}, nil), ShouldBeNil)
					res, err := h.OnReadyForSubmission(ctx, rs)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_WAITING_FOR_SUBMISSION)
					So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
						AttemptCount: 1,
					})
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
				})

				Convey("And if not waitlisted, try submitting again", func() {
					res, err := h.OnReadyForSubmission(ctx, rs)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_SUBMITTING)
					So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
						Deadline:     timestamppb.New(now.Add(submissionDuration)),
						AttemptCount: 2,
						TaskId:       "task-foo",
					})
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldNotBeNil)
					// event sent when successfully acquiring the submit queue.
					runtest.AssertReceivedReadyForSubmission(ctx, rs.Run.ID, now.Add(10*time.Second))
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
					Deadline:     timestamppb.New(now.Add(20 * time.Minute)),
					AttemptCount: 1,
					Cls:          []int64{2, 1}, // in submission order
					TaskId:       "task-foo",
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

		runNotifier := run.NewNotifier(ct.TQDispatcher)

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
			runID:       common.MakeRunID(lProject, now, 1, []byte("deadbeef")),
			deadline:    now.Add(1 * time.Minute),
			treeURL:     "https://tree.example.com",
			attempt:     2,
			clids:       common.CLIDs{1, 2},
			runNotifier: runNotifier,
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
			waitlisted, err := submit.TryAcquire(ctx, s.runNotifier, s.runID, nil)
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
					return submit.Release(ctx, s.runNotifier, s.runID)
				}, nil), ShouldBeNil)
				So(s.submit(ctx), ShouldBeNil)
				runtest.AssertReceivedSubmissionCompleted(ctx, s.runID,
					&eventpb.SubmissionCompleted{
						Result:  eventpb.SubmissionResult_FAILED_PRECONDITION,
						Attempt: 2,
					},
				)
				So(log, memlogger.ShouldHaveLog, logging.Warning, "run no longer holds submit queue, currently held by")
			})
			Convey("Tree closed", func() {
				ct.TreeFake.ModifyState(ctx, tree.Closed)
				So(s.submit(ctx), ShouldBeNil)
				verifyRunReleased(s.runID)
				runtest.AssertReceivedSubmissionCompleted(ctx, s.runID,
					&eventpb.SubmissionCompleted{
						Result:  eventpb.SubmissionResult_FAILED_PRECONDITION,
						Attempt: 2,
					},
				)
				So(log, memlogger.ShouldHaveLog, logging.Warning, "tree \"https://tree.example.com\" is closed when submission starts")
			})
			Convey("Deadline has expired", func() {
				s.deadline = now.Add(-1 * time.Minute)
				So(s.submit(ctx), ShouldBeNil)
				verifyRunReleased(s.runID)
				runtest.AssertReceivedSubmissionCompleted(ctx, s.runID,
					&eventpb.SubmissionCompleted{
						Result:  eventpb.SubmissionResult_FAILED_PRECONDITION,
						Attempt: 2,
					},
				)
				So(log, memlogger.ShouldHaveLog, logging.Warning, "submit deadline has already expired")
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
					Result:  eventpb.SubmissionResult_SUCCEEDED,
					Attempt: 2,
				},
			)
		})

		// TODO(crbug/1199880): support flakiness for Gerrit fake to test submit
		// will retry individual CL on transient error.

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
						Result:       eventpb.SubmissionResult_FAILED_PERMANENT,
						Attempt:      2,
						FatalMessage: permDeniedMsg,
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
						Result:       eventpb.SubmissionResult_FAILED_PERMANENT,
						Attempt:      2,
						FatalMessage: fmt.Sprintf(failedPreconditionMsgFmt, fmt.Sprintf("rpc error: code = FailedPrecondition desc = revision %s is not current revision", ci2.GetCurrentRevision())),
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
					Result:  eventpb.SubmissionResult_SUCCEEDED,
					Attempt: 2,
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
