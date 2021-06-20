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
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
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
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						TreeStatus: &cfgpb.Verifiers_TreeStatus{
							Url: "tree.example.com",
						},
					},
				},
			},
		}
		prjcfgtest.Create(ctx, rid.LUCIProject(), cg)
		meta, err := prjcfg.GetLatestMeta(ctx, rid.LUCIProject())
		So(err, ShouldBeNil)
		So(meta.ConfigGroupIDs, ShouldHaveLength, 1)
		r.ConfigGroupID = meta.ConfigGroupIDs[0]

		// 1 depends on 2
		ci1 := gf.CI(
			1111, gf.PS(2),
			gf.CQ(2, ct.Clock.Now().Add(-2*time.Minute), gf.U("user-100")),
			gf.Updated(clock.Now(ctx).Add(-1*time.Minute)))
		ci2 := gf.CI(
			2222, gf.PS(3),
			gf.CQ(2, ct.Clock.Now().Add(-2*time.Minute), gf.U("user-100")),
			gf.Updated(clock.Now(ctx).Add(-1*time.Minute)))
		So(datastore.Put(ctx,
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
			},
		), ShouldBeNil)

		rs := &state.RunState{Run: r}

		h := &Impl{
			RM:         run.NewNotifier(ct.TQDispatcher),
			TreeClient: ct.TreeFake.Client(),
			CLUpdater:  &clUpdaterMock{},
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

		Convey("No-Op when status is SUBMITTING", func() {
			rs.Run.Status = run.Status_SUBMITTING
			res, err := h.OnReadyForSubmission(ctx, rs)
			So(err, ShouldBeNil)
			So(res.State, ShouldEqual, rs)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.PostProcessFn, ShouldBeNil)
		})

		for _, status := range []run.Status{run.Status_RUNNING, run.Status_WAITING_FOR_SUBMISSION} {
			now := ct.Clock.Now().UTC()
			ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")
			Convey(fmt.Sprintf("When status is %s", status), func() {
				rs.Run.Status = status
				Convey("Mark submitting if Submit Queue is acquired and tree is open", func() {
					res, err := h.OnReadyForSubmission(ctx, rs)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_SUBMITTING)
					So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
						Deadline:          timestamppb.New(now.Add(submissionDuration)),
						Cls:               []int64{2, 1}, // in submission order
						TaskId:            "task-foo",
						TreeOpen:          true,
						LastTreeCheckTime: timestamppb.New(now),
					})
					So(res.State.SubmissionScheduled, ShouldBeTrue)
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldNotBeNil)
					So(submit.MustCurrentRun(ctx, lProject), ShouldEqual, rid)
					runtest.AssertReceivedReadyForSubmission(ctx, rid, now.Add(10*time.Second))
				})

				Convey("Add Run to waitlist when Submit Queue is occupied", func() {
					// another run has taken the current slot
					anotherRunID := common.MakeRunID(lProject, now, 1, []byte("cafecafe"))
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						_, err := submit.TryAcquire(ctx, h.RM.NotifyReadyForSubmission, anotherRunID, nil)
						So(err, ShouldBeNil)
						return nil
					}, nil), ShouldBeNil)
					So(submit.MustCurrentRun(ctx, lProject), ShouldEqual, anotherRunID)
					res, err := h.OnReadyForSubmission(ctx, rs)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_WAITING_FOR_SUBMISSION)
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					_, waitlist, err := submit.LoadCurrentAndWaitlist(ctx, rid)
					So(err, ShouldBeNil)
					So(waitlist.Index(rid), ShouldEqual, 0)
				})

				Convey("Revisit after 1 mintues if tree is closed", func() {
					ct.TreeFake.ModifyState(ctx, tree.Closed)
					res, err := h.OnReadyForSubmission(ctx, rs)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_WAITING_FOR_SUBMISSION)
					So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
						TreeOpen:          false,
						LastTreeCheckTime: timestamppb.New(now),
					})
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					runtest.AssertReceivedPoke(ctx, rid, now.Add(1*time.Minute))
					// Must not occupy the Submit Queue
					So(submit.MustCurrentRun(ctx, lProject), ShouldNotEqual, rid)
				})
			})
		}
	})
}

func TestOnSubmissionCompleted(t *testing.T) {
	t.Parallel()

	Convey("OnSubmissionCompleted", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "x-review.example.com"
		rid := common.MakeRunID(lProject, ct.Clock.Now().Add(-2*time.Minute), 1, []byte("deadbeef"))
		runCLs := common.CLIDs{1, 2}
		r := run.Run{
			ID:         rid,
			Status:     run.Status_SUBMITTING,
			CreateTime: ct.Clock.Now().UTC().Add(-2 * time.Minute),
			StartTime:  ct.Clock.Now().UTC().Add(-1 * time.Minute),
			CLs:        runCLs,
		}
		So(datastore.Put(ctx, &r), ShouldBeNil)
		cg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "main"},
			},
		}
		prjcfgtest.Create(ctx, rid.LUCIProject(), cg)
		meta, err := prjcfg.GetLatestMeta(ctx, rid.LUCIProject())
		So(err, ShouldBeNil)
		So(meta.ConfigGroupIDs, ShouldHaveLength, 1)
		r.ConfigGroupID = meta.ConfigGroupIDs[0]

		genCL := func(clid common.CLID, change int, deps ...common.CLID) (*gerritpb.ChangeInfo, *changelist.CL, *run.RunCL) {
			ci := gf.CI(
				change, gf.PS(2),
				gf.CQ(2, ct.Clock.Now().Add(-2*time.Minute), gf.U("user-100")),
				gf.Updated(clock.Now(ctx).Add(-1*time.Minute)))
			t := trigger.Find(ci, cg.ConfigGroups[0])
			So(t.GetGerritAccountId(), ShouldEqual, 100)
			cl := &changelist.CL{
				ID:         clid,
				ExternalID: changelist.MustGobID(gHost, ci.GetNumber()),
				EVersion:   10,
				Snapshot: &changelist.Snapshot{
					ExternalUpdateTime:    timestamppb.New(clock.Now(ctx).Add(-1 * time.Minute)),
					LuciProject:           lProject,
					Patchset:              2,
					MinEquivalentPatchset: 1,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: proto.Clone(ci).(*gerritpb.ChangeInfo),
						},
					},
				},
			}
			runCL := &run.RunCL{
				ID:         clid,
				Run:        datastore.MakeKey(ctx, run.RunKind, string(rid)),
				ExternalID: changelist.MustGobID(gHost, ci.GetNumber()),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: proto.Clone(ci).(*gerritpb.ChangeInfo),
						},
					},
				},
				Trigger: t,
			}
			if len(deps) > 0 {
				cl.Snapshot.Deps = make([]*changelist.Dep, len(deps))
				runCL.Detail.Deps = make([]*changelist.Dep, len(deps))
				for i, dep := range deps {
					cl.Snapshot.Deps[i] = &changelist.Dep{
						Clid: int64(dep),
						Kind: changelist.DepKind_HARD,
					}
					runCL.Detail.Deps[i] = &changelist.Dep{
						Clid: int64(dep),
						Kind: changelist.DepKind_HARD,
					}
				}
			}
			return ci, cl, runCL
		}

		ci1, cl1, runCL1 := genCL(1, 1111, 2)
		ci2, cl2, runCL2 := genCL(2, 2222)
		So(datastore.Put(ctx, cl1, cl2, runCL1, runCL2), ShouldBeNil)

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
		ct.GFake.SetDependsOn(gHost, ci1, ci2)

		rs := &state.RunState{Run: r}
		h := &Impl{
			RM:        run.NewNotifier(ct.TQDispatcher),
			CLUpdater: &clUpdaterMock{},
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
				res, err := h.OnSubmissionCompleted(ctx, rs, nil)
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

		ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")
		Convey("Succeeded", func() {
			sc := &eventpb.SubmissionCompleted{
				Result: eventpb.SubmissionResult_SUCCEEDED,
			}
			res, err := h.OnSubmissionCompleted(ctx, rs, sc)
			So(err, ShouldBeNil)
			So(res.State.Run.Status, ShouldEqual, run.Status_SUCCEEDED)
			So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now().UTC())
			So(res.SideEffectFn, ShouldNotBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.PostProcessFn, ShouldBeNil)
		})

		Convey("Transient failure", func() {
			sc := &eventpb.SubmissionCompleted{
				Result: eventpb.SubmissionResult_FAILED_TRANSIENT,
			}
			Convey("When deadline is not exceeded", func() {
				rs.Run.Submission = &run.Submission{
					Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(10 * time.Minute)),
				}

				Convey("Resume submission if TaskID matches", func() {
					rs.Run.Submission.TaskId = "task-foo" // same task ID as the current task
					res, err := h.OnSubmissionCompleted(ctx, rs, sc)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_SUBMITTING)
					So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
						Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(10 * time.Minute)),
						TaskId:   "task-foo",
					}) // unchanged
					So(res.State.SubmissionScheduled, ShouldBeTrue)
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldNotBeNil)
				})

				Convey("Invoke RM at deadline if TaskID doesn't match", func() {
					ctx, rmDispatcher := runtest.MockDispatch(ctx)
					rs.Run.Submission.TaskId = "another-task"
					res, err := h.OnSubmissionCompleted(ctx, rs, sc)
					So(err, ShouldBeNil)
					So(res.State, ShouldEqual, rs)
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeTrue)
					So(res.PostProcessFn, ShouldBeNil)
					So(rmDispatcher.LatestETAof(string(rid)), ShouldHappenOnOrAfter, rs.Run.Submission.Deadline.AsTime())
				})
			})

			Convey("When deadline is exceeded", func() {
				rs.Run.Submission = &run.Submission{
					Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(-10 * time.Minute)),
					TaskId:   "task-foo",
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					waitlisted, err := submit.TryAcquire(ctx, h.RM.NotifyReadyForSubmission, rid, nil)
					So(waitlisted, ShouldBeFalse)
					return err
				}, nil), ShouldBeNil)

				Convey("Single CL Run", func() {
					rs.Run.Submission.Cls = []int64{2}
					Convey("Not submitted", func() {
						runAndVerify := func(verifyMsgFn func(lastMsg string)) {
							res, err := h.OnSubmissionCompleted(ctx, rs, sc)
							So(err, ShouldBeNil)
							So(res.State.Run.Status, ShouldEqual, run.Status_FAILED)
							So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now())
							for i, f := range sc.GetClFailures().GetFailures() {
								So(res.State.Run.Submission.GetFailedCls()[i], ShouldEqual, f.GetClid())
							}
							So(res.SideEffectFn, ShouldNotBeNil)
							So(res.PreserveEvents, ShouldBeFalse)
							So(res.PostProcessFn, ShouldBeNil)
							ci := ct.GFake.GetChange(gHost, int(ci2.GetNumber())).Info
							verifyMsgFn(gf.LastMessage(ci).GetMessage())
							for _, vote := range ci.GetLabels()[trigger.CQLabelName].GetAll() {
								So(vote.GetValue(), ShouldEqual, 0)
							}
							So(submit.MustCurrentRun(ctx, lProject), ShouldNotEqual, rs.Run.ID)
						}
						Convey("CL failure", func() {
							sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
								ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
									Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
										{Clid: 2, Message: "some transient failure"},
									},
								},
							}
							runAndVerify(func(lastMsg string) {
								So(lastMsg, ShouldContainSubstring, "CL failed to submit because of transient failure: some transient failure. However, CV is running out of time to retry.")
								So(lastMsg, ShouldNotContainSubstring, "None of the CLs in the Run were submitted by CV")
							})
						})
						Convey("Unclassified failure", func() {
							runAndVerify(func(lastMsg string) {
								So(lastMsg, ShouldContainSubstring, timeoutMsg)
								So(lastMsg, ShouldNotContainSubstring, "None of the CLs in the Run were submitted by CV")
							})
						})
					})
					Convey("Submitted", func() {
						rs.Run.Submission.SubmittedCls = []int64{2}
						res, err := h.OnSubmissionCompleted(ctx, rs, sc)
						So(err, ShouldBeNil)
						So(res.State.Run.Status, ShouldEqual, run.Status_SUCCEEDED)
						So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now())
						So(res.SideEffectFn, ShouldNotBeNil)
						So(res.PreserveEvents, ShouldBeFalse)
						So(res.PostProcessFn, ShouldBeNil)
						So(ct.GFake.GetChange(gHost, int(ci2.GetNumber())).Info, ShouldResembleProto, ci2) // unchanged
						So(submit.MustCurrentRun(ctx, lProject), ShouldNotEqual, rs.Run.ID)
					})
				})

				Convey("Multi CLs Run", func() {
					rs.Run.Submission.Cls = []int64{2, 1}
					runAndVerify := func(verifyMsgFn func(changeNum int64, lastMsg string)) {
						res, err := h.OnSubmissionCompleted(ctx, rs, sc)
						So(err, ShouldBeNil)
						So(res.State.Run.Status, ShouldEqual, run.Status_FAILED)
						So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now())
						for i, f := range sc.GetClFailures().GetFailures() {
							So(res.State.Run.Submission.GetFailedCls()[i], ShouldEqual, f.GetClid())
						}
						So(res.SideEffectFn, ShouldNotBeNil)
						So(res.PreserveEvents, ShouldBeFalse)
						So(res.PostProcessFn, ShouldBeNil)
						for _, ci := range []*gerritpb.ChangeInfo{ci1, ci2} {
							ci := ct.GFake.GetChange(gHost, int(ci.GetNumber())).Info
							verifyMsgFn(ci.GetNumber(), gf.LastMessage(ci).GetMessage())
							if ct.GFake.GetChange(gHost, int(ci.GetNumber())).Info.GetStatus() != gerritpb.ChangeStatus_MERGED {
								for _, vote := range ci.GetLabels()[trigger.CQLabelName].GetAll() {
									So(vote.GetValue(), ShouldEqual, 0)
								}
							}
						}
						So(submit.MustCurrentRun(ctx, lProject), ShouldNotEqual, rs.Run.ID)
					}

					Convey("None of the CLs are submitted", func() {
						Convey("CL failure", func() {
							sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
								ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
									Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
										{Clid: 2, Message: "some transient failure"},
									},
								},
							}
							runAndVerify(func(changeNum int64, lastMsg string) {
								switch changeNum {
								case ci1.GetNumber():
									So(lastMsg, ShouldContainSubstring, "CV didn't attempt to submit this CL because CV failed to submit its dependent CL(s):\n  https://x-review.example.com/2222")
								case ci2.GetNumber():
									So(lastMsg, ShouldContainSubstring, "CL failed to submit because of transient failure: some transient failure. However, CV is running out of time to retry.")
								default:
									panic(fmt.Errorf("unknown change: %d", changeNum))
								}
								So(lastMsg, ShouldContainSubstring, "None of the CLs in the Run were submitted by CV")
								So(lastMsg, ShouldContainSubstring, "CLs: [https://x-review.example.com/2222, https://x-review.example.com/1111]")
							})
						})
						Convey("Unclassified failure", func() {
							runAndVerify(func(_ int64, lastMsg string) {
								So(lastMsg, ShouldContainSubstring, timeoutMsg)
								So(lastMsg, ShouldContainSubstring, "None of the CLs in the Run were submitted by CV")
								So(lastMsg, ShouldContainSubstring, "CLs: [https://x-review.example.com/2222, https://x-review.example.com/1111]")
							})
						})
					})

					Convey("CLs partially submitted", func() {
						rs.Run.Submission.SubmittedCls = []int64{2}
						ct.GFake.MutateChange(gHost, int(ci2.GetNumber()), func(c *gf.Change) {
							gf.PS(int(ci2.GetRevisions()[ci2.GetCurrentRevision()].GetNumber()) + 1)(c.Info)
							gf.Status(gerritpb.ChangeStatus_MERGED)(c.Info)
						})

						Convey("CL failure", func() {
							sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
								ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
									Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
										{Clid: 1, Message: "some transient failure"},
									},
								},
							}
							runAndVerify(func(changeNum int64, lastMsg string) {
								switch changeNum {
								case ci1.GetNumber():
									So(lastMsg, ShouldContainSubstring, "CL failed to submit because of transient failure: some transient failure. However, CV is running out of time to retry.")
									So(lastMsg, ShouldContainSubstring, "CV partially submitted the CLs in the Run")
									So(lastMsg, ShouldContainSubstring, "Not submitted: [https://x-review.example.com/1111]")
									So(lastMsg, ShouldContainSubstring, "Submitted: [https://x-review.example.com/2222]")
								case ci2.GetNumber():
								default:
									panic(fmt.Errorf("unknown change: %d", changeNum))
								}
							})
						})
						Convey("Unclassified failure", func() {
							runAndVerify(func(changeNum int64, lastMsg string) {
								switch changeNum {
								case ci1.GetNumber():
									So(lastMsg, ShouldContainSubstring, timeoutMsg)
									So(lastMsg, ShouldContainSubstring, "CV partially submitted the CLs in the Run")
									So(lastMsg, ShouldContainSubstring, "Not submitted: [https://x-review.example.com/1111]")
									So(lastMsg, ShouldContainSubstring, "Submitted: [https://x-review.example.com/2222]")
								case ci2.GetNumber():
								default:
									panic(fmt.Errorf("unknown change: %d", changeNum))
								}
							})
						})
					})

					Convey("CLs fully submitted", func() {
						rs.Run.Submission.SubmittedCls = []int64{2, 1}
						res, err := h.OnSubmissionCompleted(ctx, rs, sc)
						So(err, ShouldBeNil)
						So(res.State.Run.Status, ShouldEqual, run.Status_SUCCEEDED)
						So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now())
						So(res.SideEffectFn, ShouldNotBeNil)
						So(res.PreserveEvents, ShouldBeFalse)
						So(res.PostProcessFn, ShouldBeNil)
						// both untouched
						So(ct.GFake.GetChange(gHost, int(ci1.GetNumber())).Info, ShouldResembleProto, ci1)
						So(ct.GFake.GetChange(gHost, int(ci2.GetNumber())).Info, ShouldResembleProto, ci2)
						So(submit.MustCurrentRun(ctx, lProject), ShouldNotEqual, rs.Run.ID)
					})
				})
			})
		})

		Convey("Permanent failure", func() {
			sc := &eventpb.SubmissionCompleted{
				Result: eventpb.SubmissionResult_FAILED_PERMANENT,
			}
			rs.Run.Submission = &run.Submission{
				Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(10 * time.Minute)),
				TaskId:   "task-foo",
			}

			Convey("Single CL Run", func() {
				rs.Run.Submission.Cls = []int64{2}
				runAndVerify := func(verifyMsgFn func(lastMsg string)) {
					res, err := h.OnSubmissionCompleted(ctx, rs, sc)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_FAILED)
					So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now())
					for i, f := range sc.GetClFailures().GetFailures() {
						So(res.State.Run.Submission.GetFailedCls()[i], ShouldEqual, f.GetClid())
					}
					So(res.SideEffectFn, ShouldNotBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					ci := ct.GFake.GetChange(gHost, int(ci2.GetNumber())).Info
					verifyMsgFn(gf.LastMessage(ci).GetMessage())
					for _, vote := range ci.GetLabels()[trigger.CQLabelName].GetAll() {
						So(vote.GetValue(), ShouldEqual, 0)
					}
				}

				Convey("CL Submission failure", func() {
					sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
						ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
							Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
								{
									Clid:    2,
									Message: "CV failed to submit this CL because of merge conflict",
								},
							},
						},
					}
					runAndVerify(func(lastMsg string) {
						So(lastMsg, ShouldContainSubstring, "CV failed to submit this CL because of merge conflict")
						So(lastMsg, ShouldNotContainSubstring, "None of the CLs in the Run were submitted by CV")
					})
				})

				Convey("Unclassified failure", func() {
					runAndVerify(func(lastMsg string) {
						So(lastMsg, ShouldContainSubstring, defaultMsg)
						So(lastMsg, ShouldNotContainSubstring, "None of the CLs in the Run were submitted by CV")
					})
				})
			})

			Convey("Multi CLs Run", func() {
				rs.Run.Submission.Cls = []int64{2, 1}
				runAndVerify := func(verifyMsgFn func(changeNum int64, lastMsg string)) {
					res, err := h.OnSubmissionCompleted(ctx, rs, sc)
					So(err, ShouldBeNil)
					So(res.State.Run.Status, ShouldEqual, run.Status_FAILED)
					So(res.State.Run.EndTime, ShouldEqual, ct.Clock.Now())
					So(res.SideEffectFn, ShouldNotBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					for _, ci := range []*gerritpb.ChangeInfo{ci1, ci2} {
						ci := ct.GFake.GetChange(gHost, int(ci.GetNumber())).Info
						verifyMsgFn(ci.GetNumber(), gf.LastMessage(ci).GetMessage())
						if ct.GFake.GetChange(gHost, int(ci.GetNumber())).Info.GetStatus() != gerritpb.ChangeStatus_MERGED {
							for _, vote := range ci.GetLabels()[trigger.CQLabelName].GetAll() {
								So(vote.GetValue(), ShouldEqual, 0)
							}
						}
					}
				}
				Convey("None of the CLs are submitted", func() {
					Convey("CL Submission failure", func() {
						sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
							ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
								Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
									{
										Clid:    2,
										Message: "CV failed to submit this CL because of merge conflict",
									},
								},
							},
						}
						runAndVerify(func(changeNum int64, lastMsg string) {
							switch changeNum {
							case 1111:
								So(lastMsg, ShouldContainSubstring, "CV didn't attempt to submit this CL because CV failed to submit its dependent CL(s):\n  https://x-review.example.com/2222")
							case 2222:
								So(lastMsg, ShouldContainSubstring, "CV failed to submit this CL because of merge conflict")
							}
							So(lastMsg, ShouldContainSubstring, "None of the CLs in the Run were submitted by CV")
							So(lastMsg, ShouldContainSubstring, "CLs: [https://x-review.example.com/2222, https://x-review.example.com/1111]")
						})
					})

					Convey("Unclassified failure", func() {
						runAndVerify(func(changeNum int64, lastMsg string) {
							So(lastMsg, ShouldContainSubstring, defaultMsg)
							So(lastMsg, ShouldContainSubstring, "None of the CLs in the Run were submitted by CV")
							So(lastMsg, ShouldContainSubstring, "CLs: [https://x-review.example.com/2222, https://x-review.example.com/1111]")
						})
					})
				})

				Convey("CLs partially submitted", func() {
					rs.Run.Submission.SubmittedCls = []int64{2}
					ct.GFake.MutateChange(gHost, int(ci2.GetNumber()), func(c *gf.Change) {
						gf.PS(int(ci2.GetRevisions()[ci2.GetCurrentRevision()].GetNumber()) + 1)(c.Info)
						gf.Status(gerritpb.ChangeStatus_MERGED)(c.Info)
					})

					Convey("CL Submission failure", func() {
						sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
							ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
								Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
									{
										Clid:    1,
										Message: "CV failed to submit this CL because of merge conflict",
									},
								},
							},
						}
						runAndVerify(func(changeNum int64, lastMsg string) {
							if changeNum == ci1.GetNumber() {
								So(lastMsg, ShouldContainSubstring, "CV failed to submit this CL because of merge conflict")
							}
						})
					})

					Convey("Unclassified failure", func() {
						runAndVerify(func(changeNum int64, lastMsg string) {
							if changeNum == ci1.GetNumber() {
								So(lastMsg, ShouldContainSubstring, defaultMsg)
							}
						})
					})
				})
			})
		})
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
			Convey("Not holding Submit Queue", func() {
				ctx = memlogger.Use(ctx)
				log := logging.Get(ctx).(*memlogger.MemLogger)
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return submit.Release(ctx, s.rm.NotifyReadyForSubmission, s.runID)
				}, nil), ShouldBeNil)
				So(s.submit(ctx), ShouldBeNil)
				runtest.AssertReceivedSubmissionCompleted(ctx, s.runID,
					&eventpb.SubmissionCompleted{
						Result: eventpb.SubmissionResult_FAILED_PERMANENT,
					},
				)
				So(log, memlogger.ShouldHaveLog, logging.Error, "BUG: run no longer holds submit queue, currently held by")
			})

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
						FailureReason: &eventpb.SubmissionCompleted_ClFailures{
							ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
								Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
									{Clid: 2, Message: permDeniedMsg},
								},
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
						FailureReason: &eventpb.SubmissionCompleted_ClFailures{
							ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
								Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
									{
										Clid:    2,
										Message: fmt.Sprintf(failedPreconditionMsgFmt, fmt.Sprintf("rpc error: code = FailedPrecondition desc = revision %s is not current revision", ci2.GetCurrentRevision())),
									},
								},
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
