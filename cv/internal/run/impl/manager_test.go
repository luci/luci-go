// Copyright 2020 The LUCI Authors.
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

package impl

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/quota"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/handler"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/rdb"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestRunManager(t *testing.T) {
	t.Parallel()

	ftt.Run("RunManager", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const runID = "chromium/222-1-deadbeef"
		const initialEVersion = 10
		assert.Loosely(t, datastore.Put(ctx, &run.Run{
			ID:       runID,
			Status:   run.Status_RUNNING,
			EVersion: initialEVersion,
		}), should.BeNil)

		currentRun := func(ctx context.Context) *run.Run {
			ret := &run.Run{ID: runID}
			assert.Loosely(t, datastore.Get(ctx, ret), should.BeNil)
			return ret
		}

		notifier := run.NewNotifier(ct.TQDispatcher)
		pm := prjmanager.NewNotifier(ct.TQDispatcher)
		tjNotifier := tryjob.NewNotifier(ct.TQDispatcher)
		clMutator := changelist.NewMutator(ct.TQDispatcher, pm, notifier, tjNotifier)
		clUpdater := changelist.NewUpdater(ct.TQDispatcher, clMutator)
		cf := rdb.NewMockRecorderClientFactory(ct.GoMockCtl)
		qm := quota.NewManager(ct.GFactory())
		_ = New(notifier, pm, tjNotifier, clMutator, clUpdater, ct.GFactory(), ct.BuildbucketFake.NewClientFactory(), ct.TreeFake.Client(), ct.BQFake, cf, qm, ct.Env)

		// sorted by the order of execution.
		eventTestcases := []struct {
			event                *eventpb.Event
			sendFn               func(context.Context) error
			invokedHandlerMethod string
		}{
			{
				&eventpb.Event{
					Event: &eventpb.Event_LongOpCompleted{
						LongOpCompleted: &eventpb.LongOpCompleted{
							OperationId: "1-1",
						},
					},
				},
				func(ctx context.Context) error {
					return notifier.SendNow(ctx, runID, &eventpb.Event{
						Event: &eventpb.Event_LongOpCompleted{
							LongOpCompleted: &eventpb.LongOpCompleted{
								OperationId: "1-1",
							},
						},
					})
				},
				"OnLongOpCompleted",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_Cancel{
						Cancel: &eventpb.Cancel{
							Reason: "user request",
						},
					},
				},
				func(ctx context.Context) error {
					return notifier.Cancel(ctx, runID, "user request")
				},
				"Cancel",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_Start{
						Start: &eventpb.Start{},
					},
				},
				func(ctx context.Context) error {
					return notifier.Start(ctx, runID)
				},
				"Start",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_NewConfig{
						NewConfig: &eventpb.NewConfig{
							Hash:     "deadbeef",
							Eversion: 2,
						},
					},
				},
				func(ctx context.Context) error {
					return notifier.UpdateConfig(ctx, runID, "deadbeef", 2)
				},
				"UpdateConfig",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_ClsUpdated{
						ClsUpdated: &changelist.CLUpdatedEvents{
							Events: []*changelist.CLUpdatedEvent{
								{
									Clid:     int64(1),
									Eversion: int64(2),
								},
							},
						},
					},
				},
				func(ctx context.Context) error {
					return notifier.NotifyCLsUpdated(ctx, runID, &changelist.CLUpdatedEvents{
						Events: []*changelist.CLUpdatedEvent{
							{
								Clid:     int64(1),
								Eversion: int64(2),
							},
						},
					})
				},
				"OnCLsUpdated",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_TryjobsUpdated{
						TryjobsUpdated: &tryjob.TryjobUpdatedEvents{
							Events: []*tryjob.TryjobUpdatedEvent{
								{TryjobId: 10},
							},
						},
					},
				},
				func(ctx context.Context) error {
					return notifier.SendNow(ctx, runID, &eventpb.Event{
						Event: &eventpb.Event_TryjobsUpdated{
							TryjobsUpdated: &tryjob.TryjobUpdatedEvents{
								Events: []*tryjob.TryjobUpdatedEvent{
									{TryjobId: 10},
								},
							},
						},
					})
				},
				"OnTryjobsUpdated",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_ClsSubmitted{
						ClsSubmitted: &eventpb.CLsSubmitted{
							Clids: []int64{1, 2},
						},
					},
				},
				func(ctx context.Context) error {
					return notifier.SendNow(ctx, runID, &eventpb.Event{
						Event: &eventpb.Event_ClsSubmitted{
							ClsSubmitted: &eventpb.CLsSubmitted{
								Clids: []int64{1, 2},
							},
						},
					})
				},
				"OnCLsSubmitted",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_SubmissionCompleted{
						SubmissionCompleted: &eventpb.SubmissionCompleted{
							Result: eventpb.SubmissionResult_SUCCEEDED,
						},
					},
				},
				func(ctx context.Context) error {
					return notifier.SendNow(ctx, runID, &eventpb.Event{
						Event: &eventpb.Event_SubmissionCompleted{
							SubmissionCompleted: &eventpb.SubmissionCompleted{
								Result: eventpb.SubmissionResult_SUCCEEDED,
							},
						},
					})
				},
				"OnSubmissionCompleted",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_ReadyForSubmission{
						ReadyForSubmission: &eventpb.ReadyForSubmission{},
					},
				},
				func(ctx context.Context) error {
					return notifier.SendNow(ctx, runID, &eventpb.Event{
						Event: &eventpb.Event_ReadyForSubmission{
							ReadyForSubmission: &eventpb.ReadyForSubmission{},
						},
					})
				},
				"OnReadyForSubmission",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_Poke{
						Poke: &eventpb.Poke{},
					},
				},
				func(ctx context.Context) error {
					return notifier.PokeNow(ctx, runID)
				},
				"Poke",
			},
			{
				&eventpb.Event{
					Event: &eventpb.Event_ParentRunCompleted{
						ParentRunCompleted: &eventpb.ParentRunCompleted{},
					},
				},
				func(ctx context.Context) error {
					return notifier.SendNow(ctx, runID, &eventpb.Event{
						Event: &eventpb.Event_ParentRunCompleted{
							ParentRunCompleted: &eventpb.ParentRunCompleted{},
						},
					})
				},
				"OnParentRunCompleted",
			},
		}
		for _, et := range eventTestcases {
			t.Run(fmt.Sprintf("Can process Event %T", et.event.GetEvent()), func(t *ftt.Test) {
				fh := &fakeHandler{}
				ctx = context.WithValue(ctx, &fakeHandlerKey, fh)
				assert.Loosely(t, et.sendFn(ctx), should.BeNil)
				runtest.AssertInEventbox(t, ctx, runID, et.event)
				assert.Loosely(t, runtest.Runs(ct.TQ.Tasks()), should.Resemble(common.RunIDs{runID}))
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
				assert.Loosely(t, fh.invocations[0], should.Equal(et.invokedHandlerMethod))
				assert.Loosely(t, currentRun(ctx).EVersion, should.Equal(initialEVersion+1))
				runtest.AssertNotInEventbox(t, ctx, runID, et.event) // consumed
			})
		}

		t.Run("Process Events in order", func(t *ftt.Test) {
			fh := &fakeHandler{}
			ctx = context.WithValue(ctx, &fakeHandlerKey, fh)

			var expectInvokedMethods []string
			for _, et := range eventTestcases {
				// skipping Cancel because when Start and Cancel are both present.
				// only Cancel will execute. See next test
				if et.event.GetCancel() == nil {
					expectInvokedMethods = append(expectInvokedMethods, et.invokedHandlerMethod)
				}
			}
			rand.Shuffle(len(eventTestcases), func(i, j int) {
				eventTestcases[i], eventTestcases[j] = eventTestcases[j], eventTestcases[i]
			})
			for _, etc := range eventTestcases {
				if etc.event.GetCancel() == nil {
					assert.Loosely(t, etc.sendFn(ctx), should.BeNil)
				}
			}
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
			expectInvokedMethods = append(expectInvokedMethods, "TryResumeSubmission") // always invoked
			assert.Loosely(t, fh.invocations, should.Resemble(expectInvokedMethods))
			assert.Loosely(t, currentRun(ctx).EVersion, should.Equal(initialEVersion+1))
		})

		t.Run("Don't Start if received both Cancel and Start Event", func(t *ftt.Test) {
			fh := &fakeHandler{}
			ctx = context.WithValue(ctx, &fakeHandlerKey, fh)
			notifier.Start(ctx, runID)
			notifier.Cancel(ctx, runID, "user request")
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
			assert.Loosely(t, fh.invocations[0], should.Equal("Cancel"))
			for _, inv := range fh.invocations[1:] {
				assert.Loosely(t, inv, should.NotEqual("Start"))
			}
			assert.Loosely(t, currentRun(ctx).EVersion, should.Equal(initialEVersion+1))
			runtest.AssertNotInEventbox(t, ctx, runID, &eventpb.Event{
				Event: &eventpb.Event_Cancel{
					Cancel: &eventpb.Cancel{
						Reason: "user request",
					},
				},
			},
				&eventpb.Event{
					Event: &eventpb.Event_Start{
						Start: &eventpb.Start{},
					},
				},
			)
		})

		t.Run("Can Preserve events", func(t *ftt.Test) {
			fh := &fakeHandler{preserveEvents: true}
			ctx = context.WithValue(ctx, &fakeHandlerKey, fh)
			assert.Loosely(t, notifier.Start(ctx, runID), should.BeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
			assert.Loosely(t, currentRun(ctx).EVersion, should.Equal(initialEVersion+1))
			runtest.AssertInEventbox(t, ctx, runID,
				&eventpb.Event{
					Event: &eventpb.Event_Start{
						Start: &eventpb.Start{},
					},
				},
			)
		})

		t.Run("Can save RunLog", func(t *ftt.Test) {
			fh := &fakeHandler{startAddsLogEntries: []*run.LogEntry{
				{
					Time: timestamppb.New(clock.Now(ctx)),
					Kind: &run.LogEntry_Created_{Created: &run.LogEntry_Created{
						ConfigGroupId: "deadbeef/main",
					}},
				},
			}}
			ctx = context.WithValue(ctx, &fakeHandlerKey, fh)
			assert.Loosely(t, notifier.Start(ctx, runID), should.BeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
			assert.Loosely(t, currentRun(ctx).EVersion, should.Equal(initialEVersion+1))
			entries, err := run.LoadRunLogEntries(ctx, runID)
			assert.NoErr(t, err)
			assert.Loosely(t, entries, should.Resemble(fh.startAddsLogEntries))
		})

		t.Run("Can run PostProcessFn", func(t *ftt.Test) {
			var postProcessFnExecuted bool
			fh := &fakeHandler{
				postProcessFn: func(c context.Context) error {
					postProcessFnExecuted = true
					return nil
				},
			}
			ctx = context.WithValue(ctx, &fakeHandlerKey, fh)
			assert.Loosely(t, notifier.Start(ctx, runID), should.BeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
			assert.Loosely(t, postProcessFnExecuted, should.BeTrue)
		})
	})

	ftt.Run("Poke", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const (
			lProject   = "chromium"
			dryRunners = "dry-runner-group"
			runID      = lProject + "/222-1-deadbeef"
		)
		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
							DryRunAccessList: []string{dryRunners},
						},
					},
				},
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)

		tCreate := ct.Clock.Now().UTC().Add(-2 * time.Minute)
		assert.Loosely(t, datastore.Put(ctx, &run.Run{
			ID:            runID,
			Status:        run.Status_RUNNING,
			CreateTime:    tCreate,
			StartTime:     tCreate.Add(1 * time.Minute),
			EVersion:      10,
			ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
		}), should.BeNil)

		notifier := run.NewNotifier(ct.TQDispatcher)
		pm := prjmanager.NewNotifier(ct.TQDispatcher)
		tjNotifier := tryjob.NewNotifier(ct.TQDispatcher)
		clMutator := changelist.NewMutator(ct.TQDispatcher, pm, notifier, tjNotifier)
		clUpdater := changelist.NewUpdater(ct.TQDispatcher, clMutator)
		cf := rdb.NewMockRecorderClientFactory(ct.GoMockCtl)
		qm := quota.NewManager(ct.GFactory())
		_ = New(notifier, pm, tjNotifier, clMutator, clUpdater, ct.GFactory(), ct.BuildbucketFake.NewClientFactory(), ct.TreeFake.Client(), ct.BQFake, cf, qm, ct.Env)

		t.Run("Recursive", func(t *ftt.Test) {
			assert.Loosely(t, notifier.PokeNow(ctx, runID), should.BeNil)
			assert.Loosely(t, runtest.Runs(ct.TQ.Tasks()), should.Resemble(common.RunIDs{runID}))
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
			for i := 0; i < 10; i++ {
				now := clock.Now(ctx)
				runtest.AssertInEventbox(t, ctx, runID, &eventpb.Event{
					Event: &eventpb.Event_Poke{
						Poke: &eventpb.Poke{},
					},
					ProcessAfter: timestamppb.New(now.Add(pokeInterval)),
				})
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
			}

			t.Run("Stops after Run is finalized", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &run.Run{
					ID:         runID,
					Status:     run.Status_CANCELLED,
					CreateTime: tCreate,
					StartTime:  tCreate.Add(1 * time.Minute),
					EndTime:    ct.Clock.Now().UTC(),
					EVersion:   11,
				}), should.BeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
				runtest.AssertEventboxEmpty(t, ctx, runID)
			})
		})

		t.Run("Existing event due during the interval", func(t *ftt.Test) {
			assert.Loosely(t, notifier.PokeNow(ctx, runID), should.BeNil)
			assert.Loosely(t, notifier.PokeAfter(ctx, runID, 30*time.Second), should.BeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))

			runtest.AssertNotInEventbox(t, ctx, runID, &eventpb.Event{
				Event: &eventpb.Event_Poke{
					Poke: &eventpb.Poke{},
				},
				ProcessAfter: timestamppb.New(clock.Now(ctx).Add(pokeInterval)),
			})
			assert.Loosely(t, runtest.Tasks(ct.TQ.Tasks()), should.HaveLength(1))
			task := runtest.Tasks(ct.TQ.Tasks())[0]
			assert.Loosely(t, task.ETA, should.Resemble(clock.Now(ctx).UTC().Add(30*time.Second)))
			assert.Loosely(t, task.Payload, should.Resemble(&eventpb.ManageRunTask{RunId: string(runID)}))
		})

		t.Run("Run is missing", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Delete(ctx, &run.Run{ID: runID}), should.BeNil)
			assert.Loosely(t, notifier.PokeNow(ctx, runID), should.BeNil)
			ctx = memlogger.Use(ctx)
			log := logging.Get(ctx).(*memlogger.MemLogger)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunTaskClass))
			assert.Loosely(t, log, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, fmt.Sprintf("run %s is missing from datastore but got manage-run task", runID)))
		})
	})
}

type fakeHandler struct {
	invocations         []string
	preserveEvents      bool
	postProcessFn       eventbox.PostProcessFn
	startAddsLogEntries []*run.LogEntry
}

var _ handler.Handler = &fakeHandler{}

func (fh *fakeHandler) Start(ctx context.Context, rs *state.RunState) (*handler.Result, error) {
	fh.addInvocation("Start")
	rs = rs.ShallowCopy()
	if len(fh.startAddsLogEntries) > 0 {
		rs.LogEntries = append(rs.LogEntries, fh.startAddsLogEntries...)
	}
	return &handler.Result{
		State:          rs,
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) Cancel(ctx context.Context, rs *state.RunState, reasons []string) (*handler.Result, error) {
	fh.addInvocation("Cancel")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) OnCLsUpdated(ctx context.Context, rs *state.RunState, _ common.CLIDs) (*handler.Result, error) {
	fh.addInvocation("OnCLsUpdated")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) OnReadyForSubmission(ctx context.Context, rs *state.RunState) (*handler.Result, error) {
	fh.addInvocation("OnReadyForSubmission")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

// OnCLsSubmitted records provided CLs have been submitted.
func (fh *fakeHandler) OnCLsSubmitted(ctx context.Context, rs *state.RunState, clids common.CLIDs) (*handler.Result, error) {
	fh.addInvocation("OnCLsSubmitted")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) OnSubmissionCompleted(ctx context.Context, rs *state.RunState, sc *eventpb.SubmissionCompleted) (*handler.Result, error) {
	fh.addInvocation("OnSubmissionCompleted")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) OnLongOpCompleted(ctx context.Context, rs *state.RunState, result *eventpb.LongOpCompleted) (*handler.Result, error) {
	fh.addInvocation("OnLongOpCompleted")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) OnTryjobsUpdated(ctx context.Context, rs *state.RunState, tryjobs common.TryjobIDs) (*handler.Result, error) {
	fh.addInvocation("OnTryjobsUpdated")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) TryResumeSubmission(ctx context.Context, rs *state.RunState) (*handler.Result, error) {
	fh.addInvocation("TryResumeSubmission")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) Poke(ctx context.Context, rs *state.RunState) (*handler.Result, error) {
	fh.addInvocation("Poke")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) UpdateConfig(ctx context.Context, rs *state.RunState, hash string) (*handler.Result, error) {
	fh.addInvocation("UpdateConfig")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}

func (fh *fakeHandler) addInvocation(method string) {
	fh.invocations = append(fh.invocations, method)
}

func (fh *fakeHandler) OnParentRunCompleted(ctx context.Context, rs *state.RunState) (*handler.Result, error) {
	fh.addInvocation("OnParentRunCompleted")
	return &handler.Result{
		State:          rs.ShallowCopy(),
		PreserveEvents: fh.preserveEvents,
		PostProcessFn:  fh.postProcessFn,
	}, nil
}
