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
	"errors"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
	"go.chromium.org/luci/cv/internal/run/runtest"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(state.RunState{}))
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(run.Run{}))
}

func TestOnReadyForSubmission(t *testing.T) {
	t.Parallel()

	ftt.Run("OnReadyForSubmission", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "l_project"
		const gHost = "x-review.example.com"
		rid := common.MakeRunID(lProject, ct.Clock.Now().Add(-2*time.Minute), 1, []byte("deadbeef"))
		runCLs := common.CLIDs{1, 2}
		r := run.Run{
			ID:         rid,
			Mode:       run.FullRun,
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
							TreeName: lProject,
						},
					},
				},
			},
		}
		prjcfgtest.Create(ctx, rid.LUCIProject(), cg)
		meta, err := prjcfg.GetLatestMeta(ctx, rid.LUCIProject())
		assert.NoErr(t, err)
		assert.Loosely(t, meta.ConfigGroupIDs, should.HaveLength(1))
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
		assert.Loosely(t, datastore.Put(ctx,
			&run.RunCL{
				ID:         1,
				Run:        datastore.MakeKey(ctx, common.RunKind, string(rid)),
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
				Run:        datastore.MakeKey(ctx, common.RunKind, string(rid)),
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
		), should.BeNil)

		rs := &state.RunState{Run: r}

		h, deps := makeTestHandler(&ct)

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			t.Run(fmt.Sprintf("Release submit queue when Run is %s", status), func(t *ftt.Test) {
				assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					waitlisted, err := submit.TryAcquire(ctx, deps.rm.NotifyReadyForSubmission, rs.ID, nil)
					assert.Loosely(t, waitlisted, should.BeFalse)
					return err
				}, nil), should.BeNil)
				rs.Status = status
				res, err := h.OnReadyForSubmission(ctx, rs)
				assert.NoErr(t, err)
				expectedState := &state.RunState{
					Run: rs.Run,
					LogEntries: []*run.LogEntry{
						{
							Time: timestamppb.New(clock.Now(ctx)),
							Kind: &run.LogEntry_ReleasedSubmitQueue_{
								ReleasedSubmitQueue: &run.LogEntry_ReleasedSubmitQueue{},
							},
						},
					},
				}
				assert.That(t, res.State, should.Match(expectedState))
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				assert.Loosely(t, res.PostProcessFn, should.BeNil)
				current, waitlist, err := submit.LoadCurrentAndWaitlist(ctx, rs.ID)
				assert.NoErr(t, err)
				assert.Loosely(t, current, should.BeEmpty)
				assert.Loosely(t, waitlist, should.BeEmpty)
			})
		}

		t.Run("No-Op when status is SUBMITTING", func(t *ftt.Test) {
			rs.Status = run.Status_SUBMITTING
			res, err := h.OnReadyForSubmission(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State, should.Equal(rs))
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, res.PostProcessFn, should.BeNil)
		})

		t.Run("Do not submit if parent Run is not done yet.", func(t *ftt.Test) {
			const parentRun = common.RunID("parent/1-cow")
			assert.Loosely(t, datastore.Put(ctx,
				&run.Run{
					ID:     parentRun,
					Status: run.Status_RUNNING,
					CLs:    common.CLIDs{13},
				},
				&run.RunCL{
					ID:         13,
					Run:        datastore.MakeKey(ctx, common.RunKind, string(parentRun)),
					ExternalID: "gerrit/foo-review.googlesource.com/111",
				},
			), should.BeNil)
			rs.Status = run.Status_WAITING_FOR_SUBMISSION
			rs.DepRuns = common.RunIDs{parentRun}
			res, err := h.OnReadyForSubmission(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.LogEntries, should.HaveLength(1))
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, res.PostProcessFn, should.BeNil)
		})

		for _, status := range []run.Status{run.Status_RUNNING, run.Status_WAITING_FOR_SUBMISSION} {
			now := ct.Clock.Now().UTC()
			ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")
			t.Run(fmt.Sprintf("When status is %s", status), func(t *ftt.Test) {
				rs.Status = status
				t.Run("Mark submitting if Submit Queue is acquired and tree is open", func(t *ftt.Test) {
					ct.TreeFakeSrv.ModifyState(lProject, tspb.GeneralState_OPEN)
					res, err := h.OnReadyForSubmission(ctx, rs)
					assert.NoErr(t, err)
					assert.That(t, res.State.Status, should.Equal(run.Status_SUBMITTING))
					assert.That(t, res.State.Submission, should.Match(&run.Submission{
						Deadline:          timestamppb.New(now.Add(defaultSubmissionDuration)),
						Cls:               []int64{2, 1}, // in submission order
						TaskId:            "task-foo",
						TreeOpen:          true,
						LastTreeCheckTime: timestamppb.New(now),
					}))
					assert.That(t, res.State.SubmissionScheduled, should.BeTrue)
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.That(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.NotBeNil)
					assert.That(t, submit.MustCurrentRun(ctx, lProject), should.Equal(rid))
					runtest.AssertReceivedReadyForSubmission(t, ctx, rid, now.Add(10*time.Second))
					assert.Loosely(t, res.State.LogEntries, should.HaveLength(2))
					assert.Loosely(t, res.State.LogEntries[0].Kind, should.HaveType[*run.LogEntry_AcquiredSubmitQueue_])
					assert.Loosely(t, res.State.LogEntries[1].Kind.(*run.LogEntry_TreeChecked_).TreeChecked.Open, should.BeTrue)
					// SubmitQueue not yet released.
				})

				t.Run("Add Run to waitlist when Submit Queue is occupied", func(t *ftt.Test) {
					// another run has taken the current slot
					anotherRunID := common.MakeRunID(lProject, now, 1, []byte("cafecafe"))
					assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						_, err := submit.TryAcquire(ctx, deps.rm.NotifyReadyForSubmission, anotherRunID, nil)
						assert.NoErr(t, err)
						return nil
					}, nil), should.BeNil)
					assert.Loosely(t, submit.MustCurrentRun(ctx, lProject), should.Equal(anotherRunID))
					res, err := h.OnReadyForSubmission(ctx, rs)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_WAITING_FOR_SUBMISSION))
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.BeNil)
					_, waitlist, err := submit.LoadCurrentAndWaitlist(ctx, rid)
					assert.NoErr(t, err)
					assert.Loosely(t, waitlist.Index(rid), should.BeZero)
					assert.Loosely(t, res.State.LogEntries, should.HaveLength(1))
					assert.Loosely(t, res.State.LogEntries[0].Kind, should.HaveType[*run.LogEntry_Waitlisted_])
				})

				t.Run("Revisit after 1 mintues if tree is closed", func(t *ftt.Test) {
					ct.TreeFakeSrv.ModifyState(lProject, tspb.GeneralState_CLOSED)
					res, err := h.OnReadyForSubmission(ctx, rs)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_WAITING_FOR_SUBMISSION))
					assert.That(t, res.State.Submission, should.Match(&run.Submission{
						TreeOpen:          false,
						LastTreeCheckTime: timestamppb.New(now),
					}))
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.BeNil)
					runtest.AssertReceivedPoke(t, ctx, rid, now.Add(1*time.Minute))
					// The Run must not occupy the Submit Queue
					assert.Loosely(t, submit.MustCurrentRun(ctx, lProject), should.NotEqual(rid))
					assert.Loosely(t, res.State.LogEntries, should.HaveLength(3))
					assert.Loosely(t, res.State.LogEntries[0].Kind, should.HaveType[*run.LogEntry_AcquiredSubmitQueue_])
					assert.Loosely(t, res.State.LogEntries[1].Kind, should.HaveType[*run.LogEntry_TreeChecked_])
					assert.Loosely(t, res.State.LogEntries[2].Kind, should.HaveType[*run.LogEntry_ReleasedSubmitQueue_])
					assert.Loosely(t, res.State.LogEntries[1].Kind.(*run.LogEntry_TreeChecked_).TreeChecked.Open, should.BeFalse)
				})

				t.Run("Set TreeErrorSince on first failure", func(t *ftt.Test) {
					ct.TreeFakeSrv.InjectErr(lProject, errors.New("error while fetching tree status"))
					res, err := h.OnReadyForSubmission(ctx, rs)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_WAITING_FOR_SUBMISSION))
					assert.That(t, res.State.Submission, should.Match(&run.Submission{
						TreeOpen:          false,
						LastTreeCheckTime: timestamppb.New(now),
						TreeErrorSince:    timestamppb.New(now),
					}))
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.BeNil)
					runtest.AssertReceivedPoke(t, ctx, rid, now.Add(1*time.Minute))
					// The Run must not occupy the Submit Queue
					assert.Loosely(t, submit.MustCurrentRun(ctx, lProject), should.NotEqual(rid))
					assert.Loosely(t, res.State.LogEntries, should.HaveLength(2))
					assert.Loosely(t, res.State.LogEntries[0].Kind, should.HaveType[*run.LogEntry_AcquiredSubmitQueue_])
					assert.Loosely(t, res.State.LogEntries[1].Kind, should.HaveType[*run.LogEntry_ReleasedSubmitQueue_])
				})
			})
		}
	})
}

func TestOnSubmissionCompleted(t *testing.T) {
	t.Parallel()

	ftt.Run("OnSubmissionCompleted", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		const gHost = "x-review.example.com"
		rid := common.MakeRunID(lProject, ct.Clock.Now().Add(-2*time.Minute), 1, []byte("deadbeef"))
		runCLs := common.CLIDs{1, 2}
		r := run.Run{
			ID:         rid,
			Mode:       run.FullRun,
			Status:     run.Status_SUBMITTING,
			CreateTime: ct.Clock.Now().UTC().Add(-2 * time.Minute),
			StartTime:  ct.Clock.Now().UTC().Add(-1 * time.Minute),
			CLs:        runCLs,
		}
		assert.NoErr(t, datastore.Put(ctx, &r))
		cg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "main"},
			},
		}
		prjcfgtest.Create(ctx, rid.LUCIProject(), cg)
		meta, err := prjcfg.GetLatestMeta(ctx, rid.LUCIProject())
		assert.NoErr(t, err)
		assert.Loosely(t, meta.ConfigGroupIDs, should.HaveLength(1))
		r.ConfigGroupID = meta.ConfigGroupIDs[0]

		genCL := func(clid common.CLID, change int, deps ...common.CLID) (*gerritpb.ChangeInfo, *changelist.CL, *run.RunCL) {
			ci := gf.CI(
				change, gf.PS(2),
				gf.Owner("user-99"),
				gf.CQ(1, ct.Clock.Now().Add(-5*time.Minute), gf.U("user-101")),
				gf.CQ(2, ct.Clock.Now().Add(-2*time.Minute), gf.U("user-100")),
				gf.Updated(clock.Now(ctx).Add(-1*time.Minute)))
			triggers := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: cg.ConfigGroups[0]})
			assert.That(t, triggers.GetCqVoteTrigger(), should.Match(&run.Trigger{
				Time:            timestamppb.New(ct.Clock.Now().Add(-2 * time.Minute)),
				Mode:            string(run.FullRun),
				Email:           "user-100@example.com",
				GerritAccountId: 100,
			}))
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
				Run:        datastore.MakeKey(ctx, common.RunKind, string(rid)),
				ExternalID: changelist.MustGobID(gHost, ci.GetNumber()),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: proto.Clone(ci).(*gerritpb.ChangeInfo),
						},
					},
				},
				Trigger: triggers.GetCqVoteTrigger(),
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
		assert.NoErr(t, datastore.Put(ctx, cl1, cl2, runCL1, runCL2))

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
		h, deps := makeTestHandler(&ct)

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			t.Run(fmt.Sprintf("Release submit queue when Run is %s", status), func(t *ftt.Test) {
				assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					waitlisted, err := submit.TryAcquire(ctx, deps.rm.NotifyReadyForSubmission, rs.ID, nil)
					assert.Loosely(t, waitlisted, should.BeFalse)
					return err
				}, nil), should.BeNil)
				rs.Status = status
				res, err := h.OnSubmissionCompleted(ctx, rs, nil)
				assert.NoErr(t, err)
				expectedState := &state.RunState{
					Run: rs.Run,
					LogEntries: []*run.LogEntry{
						{
							Time: timestamppb.New(clock.Now(ctx)),
							Kind: &run.LogEntry_ReleasedSubmitQueue_{
								ReleasedSubmitQueue: &run.LogEntry_ReleasedSubmitQueue{},
							},
						},
					},
				}
				assert.That(t, res.State, should.Match(expectedState))
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				assert.Loosely(t, res.PostProcessFn, should.BeNil)
				current, waitlist, err := submit.LoadCurrentAndWaitlist(ctx, rs.ID)
				assert.NoErr(t, err)
				assert.Loosely(t, current, should.BeEmpty)
				assert.Loosely(t, waitlist, should.BeEmpty)
			})
		}

		ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")
		t.Run("Succeeded", func(t *ftt.Test) {
			sc := &eventpb.SubmissionCompleted{
				Result: eventpb.SubmissionResult_SUCCEEDED,
			}
			res, err := h.OnSubmissionCompleted(ctx, rs, sc)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUCCEEDED))
			assert.That(t, res.State.EndTime, should.Match(ct.Clock.Now().UTC()))
			assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, res.PostProcessFn, should.BeNil)
		})

		selfSetReviewRequests := func() (ret []*gerritpb.SetReviewRequest) {
			for _, req := range ct.GFake.Requests() {
				switch r, ok := req.(*gerritpb.SetReviewRequest); {
				case !ok:
				case r.GetOnBehalfOf() != 0:
				default:
					ret = append(ret, r)
				}
			}
			sort.SliceStable(ret, func(i, j int) bool {
				return ret[i].Number < ret[j].Number
			})
			return
		}
		assertNotify := func(req *gerritpb.SetReviewRequest, accts ...int64) {
			assert.Loosely(t, req, should.NotBeNil)
			assert.Loosely(t, req.GetNotify(), should.Equal(gerritpb.Notify_NOTIFY_NONE))
			assert.That(t, req.GetNotifyDetails(), should.Match(&gerritpb.NotifyDetails{
				Recipients: []*gerritpb.NotifyDetails_Recipient{
					{
						RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
						Info: &gerritpb.NotifyDetails_Info{
							Accounts: accts,
						},
					},
				},
			}))
		}
		assertAttentionSet := func(req *gerritpb.SetReviewRequest, reason string, accs ...int64) {
			assert.Loosely(t, req, should.NotBeNil)
			expected := []*gerritpb.AttentionSetInput{}
			for _, a := range accs {
				expected = append(
					expected,
					&gerritpb.AttentionSetInput{
						User:   strconv.FormatInt(a, 10),
						Reason: "ps#2: " + reason,
					},
				)
			}
			actual := req.GetAddToAttentionSet()
			sort.SliceStable(actual, func(i, j int) bool {
				lhs, _ := strconv.Atoi(actual[i].User)
				rhs, _ := strconv.Atoi(actual[j].User)
				return lhs < rhs
			})
			assert.That(t, actual, should.Match(expected))
		}

		t.Run("Transient failure", func(t *ftt.Test) {
			sc := &eventpb.SubmissionCompleted{
				Result: eventpb.SubmissionResult_FAILED_TRANSIENT,
			}
			t.Run("When deadline is not exceeded", func(t *ftt.Test) {
				rs.Submission = &run.Submission{
					Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(10 * time.Minute)),
				}

				t.Run("Resume submission if TaskID matches", func(t *ftt.Test) {
					rs.Submission.TaskId = "task-foo" // same task ID as the current task
					res, err := h.OnSubmissionCompleted(ctx, rs, sc)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUBMITTING))
					assert.That(t, res.State.Submission, should.Match(&run.Submission{
						Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(10 * time.Minute)),
						TaskId:   "task-foo",
					})) // unchanged
					assert.Loosely(t, res.State.SubmissionScheduled, should.BeTrue)
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.NotBeNil)
				})

				t.Run("Invoke RM at deadline if TaskID doesn't match", func(t *ftt.Test) {
					ctx, rmDispatcher := runtest.MockDispatch(ctx)
					rs.Submission.TaskId = "another-task"
					res, err := h.OnSubmissionCompleted(ctx, rs, sc)
					assert.NoErr(t, err)
					expectedState := &state.RunState{
						Run: rs.Run,
						LogEntries: []*run.LogEntry{
							{
								Time: timestamppb.New(clock.Now(ctx)),
								Kind: &run.LogEntry_SubmissionFailure_{
									SubmissionFailure: &run.LogEntry_SubmissionFailure{
										Event: &eventpb.SubmissionCompleted{Result: eventpb.SubmissionResult_FAILED_TRANSIENT},
									},
								},
							},
						},
					}
					assert.That(t, res.State, should.Match(expectedState))
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeTrue)
					assert.Loosely(t, res.PostProcessFn, should.BeNil)
					assert.Loosely(t, rmDispatcher.LatestETAof(string(rid)), should.HappenOnOrAfter(rs.Submission.Deadline.AsTime()))
				})
			})

			t.Run("When deadline is exceeded", func(t *ftt.Test) {
				rs.Submission = &run.Submission{
					Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(-10 * time.Minute)),
					TaskId:   "task-foo",
				}
				assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					waitlisted, err := submit.TryAcquire(ctx, deps.rm.NotifyReadyForSubmission, rid, nil)
					assert.Loosely(t, waitlisted, should.BeFalse)
					return err
				}, nil), should.BeNil)
				runAndVerify := func(expectedMsgs []struct {
					clid int64
					msg  string
				}) {
					res, err := h.OnSubmissionCompleted(ctx, rs, sc)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUBMITTING))
					for i, f := range sc.GetClFailures().GetFailures() {
						assert.Loosely(t, res.State.Submission.GetFailedCls()[i], should.Equal(f.GetClid()))
					}
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.BeNil)
					assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
					for i, f := range sc.GetClFailures().GetFailures() {
						assert.Loosely(t, res.State.Submission.GetFailedCls()[i], should.Equal(f.GetClid()))
					}
					for _, op := range res.State.OngoingLongOps.GetOps() {
						assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
						expectedRequests := make([]*run.OngoingLongOps_Op_ResetTriggers_Request, len(expectedMsgs))
						for i, expectedMsg := range expectedMsgs {
							expectedRequests[i] = &run.OngoingLongOps_Op_ResetTriggers_Request{
								Clid:    expectedMsg.clid,
								Message: expectedMsg.msg,
								Notify: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
								AddToAttention: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
								AddToAttentionReason: submissionFailureAttentionReason,
							}
						}
						assert.That(t, op.GetResetTriggers().GetRequests(), should.Match(expectedRequests))
						assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
					}
					assert.Loosely(t, submit.MustCurrentRun(ctx, lProject), should.NotEqual(rs.ID))
				}

				t.Run("Single CL Run", func(t *ftt.Test) {
					rs.Submission.Cls = []int64{2}
					t.Run("Not submitted", func(t *ftt.Test) {
						t.Run("CL failure", func(t *ftt.Test) {
							sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
								ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
									Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
										{Clid: 2, Message: "some transient failure"},
									},
								},
							}
							runAndVerify([]struct {
								clid int64
								msg  string
							}{
								{
									clid: 2,
									msg:  "CL failed to submit because of transient failure: some transient failure. However, submission is running out of time to retry.",
								},
							})
						})
						t.Run("Unclassified failure", func(t *ftt.Test) {
							runAndVerify([]struct {
								clid int64
								msg  string
							}{
								{
									clid: 2,
									msg:  timeoutMsg,
								},
							})
						})
					})
					t.Run("Submitted", func(t *ftt.Test) {
						rs.Submission.SubmittedCls = []int64{2}
						res, err := h.OnSubmissionCompleted(ctx, rs, sc)
						assert.NoErr(t, err)
						assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUCCEEDED))
						assert.That(t, res.State.EndTime, should.Match(ct.Clock.Now()))
						for _, op := range res.State.OngoingLongOps.GetOps() {
							if op.GetExecutePostAction() == nil {
								if !check.Loosely(t, op.GetWork(), should.BeNil) {
									t.Fatal("should not contain any long op other than post action")
								}
							}
						}
						assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
						assert.Loosely(t, res.PreserveEvents, should.BeFalse)
						assert.Loosely(t, res.PostProcessFn, should.BeNil)
						assert.That(t, ct.GFake.GetChange(gHost, int(ci2.GetNumber())).Info, should.Match(ci2)) // unchanged
						assert.Loosely(t, submit.MustCurrentRun(ctx, lProject), should.NotEqual(rs.ID))
					})
				})

				t.Run("Multi CLs Run", func(t *ftt.Test) {
					rs.Submission.Cls = []int64{2, 1}
					t.Run("None of the CLs are submitted", func(t *ftt.Test) {
						t.Run("CL failure", func(t *ftt.Test) {
							sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
								ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
									Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
										{Clid: 2, Message: "some transient failure"},
									},
								},
							}
							t.Run("With root CL", func(t *ftt.Test) {
								rs.RootCL = 1
								runAndVerify([]struct {
									clid int64
									msg  string
								}{
									{
										clid: 1,
										msg:  "Failed to submit the following CL(s):\n* https://x-review.example.com/c/2222: CL failed to submit because of transient failure: some transient failure. However, submission is running out of time to retry.\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
									},
								})
							})
							t.Run("Without root CL", func(t *ftt.Test) {
								runAndVerify([]struct {
									clid int64
									msg  string
								}{
									{
										clid: 1,
										msg:  "This CL is not submitted because submission has failed for the following CL(s) which this CL depends on.\n* https://x-review.example.com/c/2222\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
									},
									{
										clid: 2,
										msg:  "CL failed to submit because of transient failure: some transient failure. However, submission is running out of time to retry.\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
									},
								})
							})
						})
						t.Run("Unclassified failure", func(t *ftt.Test) {
							t.Run("With root CL", func(t *ftt.Test) {
								rs.RootCL = 1
								runAndVerify([]struct {
									clid int64
									msg  string
								}{
									{
										clid: 1,
										msg:  timeoutMsg + "\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
									},
								})
							})
							t.Run("Without root CL", func(t *ftt.Test) {
								runAndVerify([]struct {
									clid int64
									msg  string
								}{
									{
										clid: 1,
										msg:  timeoutMsg + "\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
									},
									{
										clid: 2,
										msg:  timeoutMsg + "\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
									},
								})
							})
						})
					})

					t.Run("CLs partially submitted", func(t *ftt.Test) {
						rs.Submission.SubmittedCls = []int64{2}
						ct.GFake.MutateChange(gHost, int(ci2.GetNumber()), func(c *gf.Change) {
							gf.PS(int(ci2.GetRevisions()[ci2.GetCurrentRevision()].GetNumber()) + 1)(c.Info)
							gf.Status(gerritpb.ChangeStatus_MERGED)(c.Info)
						})

						t.Run("CL failure", func(t *ftt.Test) {
							sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
								ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
									Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
										{Clid: 1, Message: "some transient failure"},
									},
								},
							}
							t.Run("With root CL", func(t *ftt.Test) {
								rs.RootCL = 1
								runAndVerify([]struct {
									clid int64
									msg  string
								}{
									{
										clid: 1,
										msg:  "CL failed to submit because of transient failure: some transient failure. However, submission is running out of time to retry.\n\nCLs in the Run have been submitted partially.\nNot submitted:\n* https://x-review.example.com/c/1111\nSubmitted:\n* https://x-review.example.com/c/2222\nPlease, use your judgement to determine if already submitted CLs have to be reverted, or if the remaining CLs could be manually submitted. If you think the partially submitted CLs may have broken the tip-of-tree of your project, consider notifying your infrastructure team/gardeners/sheriffs.",
									},
								})
								// Not posting message to any other CLs at all.
								reqs := selfSetReviewRequests()
								assert.Loosely(t, reqs, should.BeEmpty)
							})
							t.Run("Without root CL", func(t *ftt.Test) {
								runAndVerify([]struct {
									clid int64
									msg  string
								}{
									{
										clid: 1,
										msg:  "CL failed to submit because of transient failure: some transient failure. However, submission is running out of time to retry.\n\nCLs in the Run have been submitted partially.\nNot submitted:\n* https://x-review.example.com/c/1111\nSubmitted:\n* https://x-review.example.com/c/2222\nPlease, use your judgement to determine if already submitted CLs have to be reverted, or if the remaining CLs could be manually submitted. If you think the partially submitted CLs may have broken the tip-of-tree of your project, consider notifying your infrastructure team/gardeners/sheriffs.",
									},
								})
								// Verify posting message to the submitted CL about the failure
								// on the dependent CLs
								reqs := selfSetReviewRequests()
								assert.Loosely(t, reqs, should.HaveLength(1))
								assert.Loosely(t, reqs[0].GetNumber(), should.Equal(ci2.GetNumber()))
								assertNotify(reqs[0], 99, 100, 101)
								assertAttentionSet(reqs[0], "failed to submit dependent CLs", 99, 100, 101)
								assert.Loosely(t, reqs[0].Message, should.ContainSubstring("This CL is submitted. However, submission has failed for the following CL(s) which depend on this CL."))
							})
						})
						t.Run("Unclassified failure", func(t *ftt.Test) {
							t.Run("With root CL", func(t *ftt.Test) {
								rs.RootCL = 1
								runAndVerify([]struct {
									clid int64
									msg  string
								}{
									{
										clid: 1,
										msg:  timeoutMsg + "\n\nCLs in the Run have been submitted partially.\nNot submitted:\n* https://x-review.example.com/c/1111\nSubmitted:\n* https://x-review.example.com/c/2222\nPlease, use your judgement to determine if already submitted CLs have to be reverted, or if the remaining CLs could be manually submitted. If you think the partially submitted CLs may have broken the tip-of-tree of your project, consider notifying your infrastructure team/gardeners/sheriffs.",
									},
								})
							})
							t.Run("Without root CL", func(t *ftt.Test) {
								runAndVerify([]struct {
									clid int64
									msg  string
								}{
									{
										clid: 1,
										msg:  timeoutMsg + "\n\nCLs in the Run have been submitted partially.\nNot submitted:\n* https://x-review.example.com/c/1111\nSubmitted:\n* https://x-review.example.com/c/2222\nPlease, use your judgement to determine if already submitted CLs have to be reverted, or if the remaining CLs could be manually submitted. If you think the partially submitted CLs may have broken the tip-of-tree of your project, consider notifying your infrastructure team/gardeners/sheriffs.",
									},
								})
							})

						})
					})

					t.Run("CLs fully submitted", func(t *ftt.Test) {
						rs.Submission.SubmittedCls = []int64{2, 1}
						res, err := h.OnSubmissionCompleted(ctx, rs, sc)
						assert.NoErr(t, err)
						assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUCCEEDED))
						assert.That(t, res.State.EndTime, should.Match(ct.Clock.Now()))
						assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
						assert.Loosely(t, res.PreserveEvents, should.BeFalse)
						assert.Loosely(t, res.PostProcessFn, should.BeNil)
						assert.Loosely(t, submit.MustCurrentRun(ctx, lProject), should.NotEqual(rs.ID))
					})
				})
			})
		})

		t.Run("Permanent failure", func(t *ftt.Test) {
			sc := &eventpb.SubmissionCompleted{
				Result: eventpb.SubmissionResult_FAILED_PERMANENT,
			}
			rs.Submission = &run.Submission{
				Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(10 * time.Minute)),
				TaskId:   "task-foo",
			}
			runAndVerify := func(expectedMsgs []struct {
				clid int64
				msg  string
			}) {
				res, err := h.OnSubmissionCompleted(ctx, rs, sc)
				assert.NoErr(t, err)
				assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUBMITTING))
				for i, f := range sc.GetClFailures().GetFailures() {
					assert.Loosely(t, res.State.Submission.GetFailedCls()[i], should.Equal(f.GetClid()))
				}
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				assert.Loosely(t, res.PostProcessFn, should.BeNil)
				assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
				for i, f := range sc.GetClFailures().GetFailures() {
					assert.Loosely(t, res.State.Submission.GetFailedCls()[i], should.Equal(f.GetClid()))
				}
				for _, op := range res.State.OngoingLongOps.GetOps() {
					assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
					expectedRequests := make([]*run.OngoingLongOps_Op_ResetTriggers_Request, len(expectedMsgs))
					for i, expectedMsg := range expectedMsgs {
						expectedRequests[i] = &run.OngoingLongOps_Op_ResetTriggers_Request{
							Clid:    expectedMsg.clid,
							Message: expectedMsg.msg,
							Notify: gerrit.Whoms{
								gerrit.Whom_OWNER,
								gerrit.Whom_CQ_VOTERS,
							},
							AddToAttention: gerrit.Whoms{
								gerrit.Whom_OWNER,
								gerrit.Whom_CQ_VOTERS,
							},
							AddToAttentionReason: submissionFailureAttentionReason,
						}
					}
					assert.That(t, op.GetResetTriggers().GetRequests(), should.Match(expectedRequests))
					assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
				}
			}

			t.Run("Single CL Run", func(t *ftt.Test) {
				rs.Submission.Cls = []int64{2}
				t.Run("CL Submission failure", func(t *ftt.Test) {
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
					runAndVerify([]struct {
						clid int64
						msg  string
					}{
						{
							clid: 2,
							msg:  "CV failed to submit this CL because of merge conflict",
						},
					})
				})

				t.Run("Unclassified failure", func(t *ftt.Test) {
					runAndVerify([]struct {
						clid int64
						msg  string
					}{
						{
							clid: 2,
							msg:  defaultMsg,
						},
					})
				})
			})

			t.Run("Multi CLs Run", func(t *ftt.Test) {
				rs.Submission.Cls = []int64{2, 1}
				t.Run("None of the CLs are submitted", func(t *ftt.Test) {
					t.Run("CL Submission failure", func(t *ftt.Test) {
						sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
							ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
								Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
									{
										Clid:    2,
										Message: "Failed to submit this CL because of merge conflict",
									},
								},
							},
						}
						t.Run("With root CL", func(t *ftt.Test) {
							rs.RootCL = 1
							runAndVerify([]struct {
								clid int64
								msg  string
							}{
								{
									clid: 1,
									msg:  "Failed to submit the following CL(s):\n* https://x-review.example.com/c/2222: Failed to submit this CL because of merge conflict\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
								},
							})
						})
						t.Run("Without root CL", func(t *ftt.Test) {
							runAndVerify([]struct {
								clid int64
								msg  string
							}{
								{
									clid: 1,
									msg:  "This CL is not submitted because submission has failed for the following CL(s) which this CL depends on.\n* https://x-review.example.com/c/2222\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
								},
								{
									clid: 2,
									msg:  "Failed to submit this CL because of merge conflict\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
								},
							})
						})
					})

					t.Run("Unclassified failure", func(t *ftt.Test) {
						t.Run("With root CL", func(t *ftt.Test) {
							rs.RootCL = 1
							runAndVerify([]struct {
								clid int64
								msg  string
							}{
								{
									clid: 1,
									msg:  defaultMsg + "\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
								},
							})
						})
						t.Run("Without root CL", func(t *ftt.Test) {
							runAndVerify([]struct {
								clid int64
								msg  string
							}{
								{
									clid: 1,
									msg:  defaultMsg + "\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
								},
								{
									clid: 2,
									msg:  defaultMsg + "\n\nNone of the CLs in the Run has been submitted. CLs:\n* https://x-review.example.com/c/2222\n* https://x-review.example.com/c/1111",
								},
							})
						})
					})
				})

				t.Run("CLs partially submitted", func(t *ftt.Test) {
					rs.Submission.SubmittedCls = []int64{2}
					ct.GFake.MutateChange(gHost, int(ci2.GetNumber()), func(c *gf.Change) {
						gf.PS(int(ci2.GetRevisions()[ci2.GetCurrentRevision()].GetNumber()) + 1)(c.Info)
						gf.Status(gerritpb.ChangeStatus_MERGED)(c.Info)
					})

					t.Run("CL Submission failure", func(t *ftt.Test) {
						sc.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
							ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
								Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
									{
										Clid:    1,
										Message: "Failed to submit this CL because of merge conflict",
									},
								},
							},
						}
						runAndVerify([]struct {
							clid int64
							msg  string
						}{
							{
								clid: 1,
								msg:  "Failed to submit this CL because of merge conflict\n\nCLs in the Run have been submitted partially.\nNot submitted:\n* https://x-review.example.com/c/1111\nSubmitted:\n* https://x-review.example.com/c/2222\nPlease, use your judgement to determine if already submitted CLs have to be reverted, or if the remaining CLs could be manually submitted. If you think the partially submitted CLs may have broken the tip-of-tree of your project, consider notifying your infrastructure team/gardeners/sheriffs.",
							},
						})

						// Verify posting message to the submitted CL about the failure
						// on the dependent CLs
						reqs := selfSetReviewRequests()
						assert.Loosely(t, reqs, should.HaveLength(1))
						assert.Loosely(t, reqs[0].GetNumber(), should.Equal(ci2.GetNumber()))
						assertNotify(reqs[0], 99, 100, 101)
						assertAttentionSet(reqs[0], "failed to submit dependent CLs", 99, 100, 101)
						assert.Loosely(t, reqs[0].Message, should.ContainSubstring("This CL is submitted. However, submission has failed for the following CL(s) which depend on this CL."))
					})

					t.Run("don't attempt posting dependent failure message if posted already", func(t *ftt.Test) {
						ct.GFake.MutateChange(gHost, int(ci2.GetNumber()), func(c *gf.Change) {
							msgs := c.Info.GetMessages()
							msgs = append(msgs, &gerritpb.ChangeMessageInfo{
								Message: partiallySubmittedMsgForSubmittedCLs,
							})
							gf.Messages(msgs...)(c.Info)
						})
						runAndVerify([]struct {
							clid int64
							msg  string
						}{
							{
								clid: 1,
								msg:  defaultMsg + "\n\nCLs in the Run have been submitted partially.\nNot submitted:\n* https://x-review.example.com/c/1111\nSubmitted:\n* https://x-review.example.com/c/2222\nPlease, use your judgement to determine if already submitted CLs have to be reverted, or if the remaining CLs could be manually submitted. If you think the partially submitted CLs may have broken the tip-of-tree of your project, consider notifying your infrastructure team/gardeners/sheriffs.",
							},
						})
						reqs := selfSetReviewRequests()
						assert.Loosely(t, reqs, should.BeEmpty) // no request to ci2
					})

					t.Run("Unclassified failure", func(t *ftt.Test) {
						runAndVerify([]struct {
							clid int64
							msg  string
						}{
							{
								clid: 1,
								msg:  defaultMsg + "\n\nCLs in the Run have been submitted partially.\nNot submitted:\n* https://x-review.example.com/c/1111\nSubmitted:\n* https://x-review.example.com/c/2222\nPlease, use your judgement to determine if already submitted CLs have to be reverted, or if the remaining CLs could be manually submitted. If you think the partially submitted CLs may have broken the tip-of-tree of your project, consider notifying your infrastructure team/gardeners/sheriffs.",
							},
						})
					})
				})
			})
		})
	})
}

func TestOnCLsSubmitted(t *testing.T) {
	t.Parallel()

	ftt.Run("OnCLsSubmitted", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
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

		h, _ := makeTestHandler(&ct)
		t.Run("Single", func(t *ftt.Test) {
			res, err := h.OnCLsSubmitted(ctx, rs, common.CLIDs{3})
			assert.NoErr(t, err)
			assert.That(t, res.State.Submission.SubmittedCls, should.Match([]int64{3}))

		})
		t.Run("Duplicate", func(t *ftt.Test) {
			res, err := h.OnCLsSubmitted(ctx, rs, common.CLIDs{3, 3, 3, 3, 1, 1, 1})
			assert.NoErr(t, err)
			assert.That(t, res.State.Submission.SubmittedCls, should.Match([]int64{3, 1}))
		})
		t.Run("Obey Submission order", func(t *ftt.Test) {
			res, err := h.OnCLsSubmitted(ctx, rs, common.CLIDs{1, 3, 5, 7})
			assert.NoErr(t, err)
			assert.That(t, res.State.Submission.SubmittedCls, should.Match([]int64{3, 1, 7, 5}))
		})
		t.Run("Merge to existing", func(t *ftt.Test) {
			rs.Submission.SubmittedCls = []int64{3, 1}
			// 1 should be deduped
			res, err := h.OnCLsSubmitted(ctx, rs, common.CLIDs{1, 7})
			assert.NoErr(t, err)
			assert.That(t, res.State.Submission.SubmittedCls, should.Match([]int64{3, 1, 7}))
		})
		t.Run("Last cl arrives first", func(t *ftt.Test) {
			res, err := h.OnCLsSubmitted(ctx, rs, common.CLIDs{5})
			assert.NoErr(t, err)
			assert.That(t, res.State.Submission.SubmittedCls, should.Match([]int64{5}))
			rs = res.State
			res, err = h.OnCLsSubmitted(ctx, rs, common.CLIDs{1, 3})
			assert.NoErr(t, err)
			assert.That(t, res.State.Submission.SubmittedCls, should.Match([]int64{3, 1, 5}))
			rs = res.State
			res, err = h.OnCLsSubmitted(ctx, rs, common.CLIDs{7})
			assert.NoErr(t, err)
			assert.That(t, res.State.Submission.SubmittedCls, should.Match([]int64{3, 1, 7, 5}))
		})
		t.Run("Error for unknown CLs", func(t *ftt.Test) {
			res, err := h.OnCLsSubmitted(ctx, rs, common.CLIDs{1, 3, 5, 7, 9, 11})
			assert.ErrIsLike(t, err, "received CLsSubmitted event for cls not belonging to this Run: [9 11]")
			assert.Loosely(t, res, should.BeNil)
		})
	})
}
