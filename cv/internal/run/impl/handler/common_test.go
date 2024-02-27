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
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apipb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/bq"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/postaction"
	"go.chromium.org/luci/cv/internal/run/pubsub"
	"go.chromium.org/luci/cv/internal/run/rdb"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEndRun(t *testing.T) {
	t.Parallel()

	Convey("EndRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			clid     = 1
			lProject = "infra"
		)
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					PostActions: []*cfgpb.ConfigGroup_PostAction{
						{
							Name: "run-verification-label",
							Conditions: []*cfgpb.ConfigGroup_PostAction_TriggeringCondition{
								{
									Mode:     string(run.DryRun),
									Statuses: []apipb.Run_Status{apipb.Run_FAILED},
								},
							},
						},
					},
				},
			},
		})
		cgs, err := prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)
		So(err, ShouldBeNil)
		cg := cgs[0]

		// mock a CL with two onoging Runs.
		rids := common.RunIDs{
			common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef")),
			common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("cafecafe")),
		}
		sort.Sort(rids)
		cl := changelist.CL{
			ID:             clid,
			EVersion:       3,
			IncompleteRuns: rids,
			UpdateTime:     ct.Clock.Now().UTC(),
		}
		So(datastore.Put(ctx, &cl), ShouldBeNil)

		// mock some child runs of rids[0]
		childRunID := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("child"))
		childRun := run.Run{
			ID:      childRunID,
			DepRuns: common.RunIDs{rids[0]},
		}
		finChildRunID := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("finchild"))
		finChildRun := run.Run{
			ID:      finChildRunID,
			DepRuns: common.RunIDs{rids[0]},
			Status:  run.Status_FAILED,
		}
		So(datastore.Put(ctx, &childRun, &finChildRun), ShouldBeNil)

		rs := &state.RunState{
			Run: run.Run{
				ID:            rids[0],
				Status:        run.Status_RUNNING,
				ConfigGroupID: cg.ID,
				CreateTime:    ct.Clock.Now().Add(-2 * time.Minute),
				StartTime:     ct.Clock.Now().Add(-1 * time.Minute),
				Mode:          run.DryRun,
				CLs:           common.CLIDs{1},
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						"11-22": {
							CancelRequested: false,
							Work:            &run.OngoingLongOps_Op_PostStartMessage{PostStartMessage: true},
						},
					},
				},
			},
		}

		impl, deps := makeImpl(&ct)
		se := impl.endRun(ctx, rs, run.Status_FAILED, cg, []*run.Run{&childRun, &finChildRun})
		So(rs.Status, ShouldEqual, run.Status_FAILED)
		So(rs.EndTime, ShouldEqual, ct.Clock.Now())
		So(datastore.RunInTransaction(ctx, se, nil), ShouldBeNil)

		Convey("removeRunFromCLs", func() {
			// fetch the updated CL entity.
			cl = changelist.CL{ID: clid}
			So(datastore.Get(ctx, &cl), ShouldBeNil)

			// it should have removed the ended Run, but not the other
			// ongoing Run from the CL entity.
			So(cl.IncompleteRuns, ShouldResemble, common.RunIDs{rids[1]})
			Convey("schedule CLUpdate for the removed Run", func() {
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(changelist.BatchOnCLUpdatedTaskClass))
				pmtest.AssertReceivedRunFinished(ctx, rids[0], rs.Status)
				pmtest.AssertReceivedCLsNotified(ctx, rids[0].LUCIProject(), []*changelist.CL{&cl})
				So(deps.clUpdater.refreshedCLs, ShouldResemble, common.MakeCLIDs(clid))
			})
		})

		Convey("child runs get ParentRunCompleted events.", func() {
			runtest.AssertReceivedParentRunCompleted(ctx, childRunID)
			runtest.AssertNotReceivedParentRunCompleted(ctx, finChildRunID)
		})

		Convey("cancel ongoing LongOps", func() {
			So(rs.OngoingLongOps.GetOps()["11-22"].GetCancelRequested(), ShouldBeTrue)
		})

		Convey("populate metrics for run events", func() {
			fset1 := []any{
				lProject, "main", string(run.DryRun),
				apipb.Run_FAILED.String(), true,
			}
			fset2 := fset1[0 : len(fset1)-1]
			So(ct.TSMonSentValue(ctx, metrics.Public.RunEnded, fset1...), ShouldEqual, 1)
			So(ct.TSMonSentDistr(ctx, metrics.Public.RunDuration, fset2...).Sum(),
				ShouldAlmostEqual, (1 * time.Minute).Seconds())
			So(ct.TSMonSentDistr(ctx, metrics.Public.RunTotalDuration, fset1...).Sum(),
				ShouldAlmostEqual, (2 * time.Minute).Seconds())
		})

		Convey("publish RunEnded event", func() {
			var task *pubsub.PublishRunEndedTask
			for _, t := range ct.TQ.Tasks() {
				if p, ok := t.Payload.(*pubsub.PublishRunEndedTask); ok {
					task = p
					break
				}
			}
			So(task, ShouldResembleProto, &pubsub.PublishRunEndedTask{
				PublicId:    rs.ID.PublicID(),
				LuciProject: rs.ID.LUCIProject(),
				Status:      rs.Status,
				Eversion:    int64(rs.EVersion + 1),
			})
		})

		Convey("enqueue long-ops for PostAction", func() {
			postActions := make([]*run.OngoingLongOps_Op_ExecutePostActionPayload, 0, len(rs.OngoingLongOps.GetOps()))
			for _, op := range rs.OngoingLongOps.GetOps() {
				if act := op.GetExecutePostAction(); act != nil {
					d := timestamppb.New(ct.Clock.Now().UTC().Add(maxPostActionExecutionDuration))
					So(op.GetDeadline(), ShouldResembleProto, d)
					So(op.GetCancelRequested(), ShouldBeFalse)
					postActions = append(postActions, act)
				}
			}
			sort.Slice(postActions, func(i, j int) bool {
				return strings.Compare(postActions[i].GetName(), postActions[j].GetName()) < 0
			})

			So(postActions, ShouldResembleProto, []*run.OngoingLongOps_Op_ExecutePostActionPayload{
				{
					Name: postaction.CreditRunQuotaPostActionName,
					Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_CreditRunQuota_{
						CreditRunQuota: &run.OngoingLongOps_Op_ExecutePostActionPayload_CreditRunQuota{},
					},
				},
				{
					Name: "run-verification-label",
					Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_ConfigAction{
						ConfigAction: &cfgpb.ConfigGroup_PostAction{
							Name: "run-verification-label",
							Conditions: []*cfgpb.ConfigGroup_PostAction_TriggeringCondition{
								{
									Mode:     string(run.DryRun),
									Statuses: []apipb.Run_Status{apipb.Run_FAILED},
								},
							},
						},
					},
				},
			})
		})
	})
}

func TestCheckRunCreate(t *testing.T) {
	t.Parallel()
	Convey("CheckRunCreate", t, func() {
		ct := &cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		const clid = 1
		const gHost = "x-review.example.com"
		const gRepo = "luci-go"
		const gChange = 123
		const lProject = "infra"

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Gerrit: []*cfgpb.ConfigGroup_Gerrit{{
						Url: "https://" + gHost,
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{Name: gRepo, RefRegexp: []string{"refs/heads/.+"}},
						},
					}},
				},
			},
		})
		cgs, err := prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)

		cg := cgs[0]

		ci := gf.CI(gChange, gf.CQ(+2))
		rid := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("deadbeef"))
		rs := &state.RunState{
			Run: run.Run{
				ID:            rid,
				Status:        run.Status_RUNNING,
				ConfigGroupID: prjcfg.MakeConfigGroupID("deadbeef", "main"),
				CreateTime:    ct.Clock.Now().Add(-2 * time.Minute),
				StartTime:     ct.Clock.Now().Add(-1 * time.Minute),
				CLs:           common.CLIDs{1},
			},
		}
		cl := changelist.CL{
			ID:             clid,
			ExternalID:     changelist.MustGobID(gHost, gChange),
			IncompleteRuns: common.RunIDs{rid},
			EVersion:       3,
			UpdateTime:     ct.Clock.Now().UTC(),
			Snapshot: &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: gHost,
					Info: ci,
				}},
				LuciProject:        lProject,
				ExternalUpdateTime: timestamppb.New(ct.Clock.Now()),
			},
		}
		triggers := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: cg.Content})
		So(triggers.GetCqVoteTrigger(), ShouldResembleProto, &run.Trigger{
			Time:            timestamppb.New(testclock.TestRecentTimeUTC.Add(10 * time.Hour)),
			Mode:            string(run.FullRun),
			Email:           "user-1@example.com",
			GerritAccountId: 1,
		})
		rcl := run.RunCL{
			ID:         clid,
			Run:        datastore.MakeKey(ctx, common.RunKind, string(rid)),
			ExternalID: cl.ExternalID,
			Detail:     cl.Snapshot,
			Trigger:    triggers.GetCqVoteTrigger(),
		}
		So(datastore.Put(ctx, &cl, &rcl), ShouldBeNil)
		rcls, cls := []*run.RunCL{&rcl}, []*changelist.CL{&cl}
		So(err, ShouldBeNil)

		Convey("Returns empty metas for new patchset run", func() {
			rs.Mode = run.NewPatchsetRun
			ok, err := checkRunCreate(ctx, rs, cg, rcls, cls)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
			So(rs.OngoingLongOps.Ops, ShouldHaveLength, 1)
			So(rs.OngoingLongOps.Ops, ShouldContainKey, "1-1")
			reqs := rs.OngoingLongOps.Ops["1-1"].GetResetTriggers().GetRequests()
			So(reqs, ShouldHaveLength, 1)
			So(reqs[0].Message, ShouldEqual, "")
			So(reqs[0].AddToAttention, ShouldBeEmpty)
		})
		Convey("Populates metas for other modes", func() {
			rs.Mode = run.DryRun
			ok, err := checkRunCreate(ctx, rs, cg, rcls, cls)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
			So(rs.OngoingLongOps.Ops, ShouldHaveLength, 1)
			So(rs.OngoingLongOps.Ops, ShouldContainKey, "1-1")
			reqs := rs.OngoingLongOps.Ops["1-1"].GetResetTriggers().GetRequests()
			So(reqs, ShouldHaveLength, 1)
			So(reqs[0].Message, ShouldEqual, "CV cannot start a Run for `user-1@example.com` because the user is neither the CL owner nor a committer.")
			So(reqs[0].AddToAttention, ShouldResemble, []gerrit.Whom{
				gerrit.Whom_OWNER,
				gerrit.Whom_CQ_VOTERS})
		})
	})
}

type dependencies struct {
	pm         *prjmanager.Notifier
	rm         *run.Notifier
	qm         *quotaManagerMock
	tjNotifier *tryjobNotifierMock
	clUpdater  *clUpdaterMock
}

type testHandler struct {
	inner Handler
}

func validateStateMutation(passed, initialCopy, result *state.RunState) {
	switch {
	case cvtesting.SafeShouldResemble(result, initialCopy) == "":
		// No state change; doesn't matter whether shallow copy is created or not.
		return
	case passed == result:
		So(errors.New("handler mutated the input state but doesn't create a shallow copy before mutation"), ShouldBeNil)
	case cvtesting.SafeShouldResemble(initialCopy, passed) != "":
		So(errors.New("handler created a shallow copy but modified addressable property in place; forgot to clone a proto?"), ShouldBeNil)
	}
}

func (t *testHandler) Start(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.Start(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) Cancel(ctx context.Context, rs *state.RunState, reasons []string) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.Cancel(ctx, rs, reasons)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnCLsUpdated(ctx context.Context, rs *state.RunState, cls common.CLIDs) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnCLsUpdated(ctx, rs, cls)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) UpdateConfig(ctx context.Context, rs *state.RunState, ver string) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.UpdateConfig(ctx, rs, ver)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnReadyForSubmission(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnReadyForSubmission(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnCLsSubmitted(ctx context.Context, rs *state.RunState, cls common.CLIDs) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnCLsSubmitted(ctx, rs, cls)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnSubmissionCompleted(ctx context.Context, rs *state.RunState, sc *eventpb.SubmissionCompleted) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnSubmissionCompleted(ctx, rs, sc)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnLongOpCompleted(ctx context.Context, rs *state.RunState, result *eventpb.LongOpCompleted) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnLongOpCompleted(ctx, rs, result)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnTryjobsUpdated(ctx context.Context, rs *state.RunState, tryjobs common.TryjobIDs) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnTryjobsUpdated(ctx, rs, tryjobs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) TryResumeSubmission(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.TryResumeSubmission(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) Poke(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.Poke(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnParentRunCompleted(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnParentRunCompleted(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(rs, initialCopy, res.State)
	return res, err
}

func makeTestHandler(ct *cvtesting.Test) (Handler, dependencies) {
	handler, dependencies := makeImpl(ct)
	return &testHandler{inner: handler}, dependencies
}

// makeImpl should only be used to test common functions. For testing handler,
// please use makeTestHandler instead.
func makeImpl(ct *cvtesting.Test) (*Impl, dependencies) {
	deps := dependencies{
		pm:         prjmanager.NewNotifier(ct.TQDispatcher),
		rm:         run.NewNotifier(ct.TQDispatcher),
		qm:         &quotaManagerMock{},
		tjNotifier: &tryjobNotifierMock{},
		clUpdater:  &clUpdaterMock{},
	}
	cf := rdb.NewMockRecorderClientFactory(ct.GoMockCtl)
	impl := &Impl{
		PM:          deps.pm,
		RM:          deps.rm,
		TN:          deps.tjNotifier,
		CLMutator:   changelist.NewMutator(ct.TQDispatcher, deps.pm, deps.rm, nil),
		CLUpdater:   deps.clUpdater,
		TreeClient:  ct.TreeFake.Client(),
		GFactory:    ct.GFactory(),
		BQExporter:  bq.NewExporter(ct.TQDispatcher, ct.BQFake, ct.Env),
		RdbNotifier: rdb.NewNotifier(ct.TQDispatcher, cf),
		Publisher:   pubsub.NewPublisher(ct.TQDispatcher, ct.Env),
		QM:          deps.qm,
		Env:         ct.Env,
	}
	return impl, deps
}

type clUpdaterMock struct {
	m            sync.Mutex
	refreshedCLs common.CLIDs
}

func (c *clUpdaterMock) ScheduleBatch(ctx context.Context, luciProject string, cls []*changelist.CL, requester changelist.UpdateCLTask_Requester) error {
	c.m.Lock()
	for _, cl := range cls {
		c.refreshedCLs = append(c.refreshedCLs, cl.ID)
	}
	c.m.Unlock()
	return nil
}

type tryjobNotifierMock struct {
	m               sync.Mutex
	updateScheduled common.TryjobIDs
}

func (t *tryjobNotifierMock) ScheduleUpdate(ctx context.Context, id common.TryjobID, _ tryjob.ExternalID) error {
	t.m.Lock()
	t.updateScheduled = append(t.updateScheduled, id)
	t.m.Unlock()
	return nil
}

type quotaManagerMock struct {
	runQuotaOp  *quotapb.OpResult
	userLimit   *cfgpb.UserLimit
	runQuotaErr error

	debitRunQuotaCalls  int
	creditRunQuotaCalls int
}

func (qm *quotaManagerMock) DebitRunQuota(ctx context.Context, r *run.Run) (*quotapb.OpResult, *cfgpb.UserLimit, error) {
	qm.debitRunQuotaCalls++
	return qm.runQuotaOp, qm.userLimit, qm.runQuotaErr
}

func (qm *quotaManagerMock) CreditRunQuota(ctx context.Context, r *run.Run) (*quotapb.OpResult, *cfgpb.UserLimit, error) {
	qm.creditRunQuotaCalls++
	return qm.runQuotaOp, qm.userLimit, qm.runQuotaErr
}

func (qm *quotaManagerMock) RunQuotaAccountID(r *run.Run) *quotapb.AccountID {
	return &quotapb.AccountID{
		AppId:        "cv",
		Realm:        r.ID.LUCIProject(),
		Namespace:    r.ConfigGroupID.Name(),
		Name:         r.CreatedBy.Email(),
		ResourceType: "runs",
	}
}
