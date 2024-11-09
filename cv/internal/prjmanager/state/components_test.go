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

package state

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/itriager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runcreator"
	"go.chromium.org/luci/cv/internal/run/runquery"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestEarliestDecisionTime(t *testing.T) {
	t.Parallel()

	ftt.Run("earliestDecisionTime works", t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC
		t0 := now.Add(time.Hour)

		earliest := func(cs []*prjpb.Component) time.Time {
			ts, tsPB, asap := earliestDecisionTime(cs)
			if asap {
				return now
			}
			if ts.IsZero() {
				assert.Loosely(t, tsPB, should.BeNil)
			} else {
				assert.Loosely(t, tsPB.AsTime(), should.Resemble(ts))
			}
			return ts
		}

		cs := []*prjpb.Component{
			{DecisionTime: nil},
		}
		assert.Loosely(t, earliest(cs), should.Resemble(time.Time{}))

		cs = append(cs, &prjpb.Component{DecisionTime: timestamppb.New(t0.Add(time.Second))})
		assert.Loosely(t, earliest(cs), should.Resemble(t0.Add(time.Second)))

		cs = append(cs, &prjpb.Component{})
		assert.Loosely(t, earliest(cs), should.Resemble(t0.Add(time.Second)))

		cs = append(cs, &prjpb.Component{DecisionTime: timestamppb.New(t0.Add(time.Hour))})
		assert.Loosely(t, earliest(cs), should.Resemble(t0.Add(time.Second)))

		cs = append(cs, &prjpb.Component{DecisionTime: timestamppb.New(t0)})
		assert.Loosely(t, earliest(cs), should.Resemble(t0))

		cs = append(cs, &prjpb.Component{
			TriageRequired: true,
			// DecisionTime in this case doesn't matter.
			DecisionTime: timestamppb.New(t0.Add(10 * time.Hour)),
		})
		assert.Loosely(t, earliest(cs), should.Resemble(now))
	})
}

func TestComponentsActions(t *testing.T) {
	t.Parallel()

	ftt.Run("Component actions logic work in the abstract", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		now := ct.Clock.Now()

		const lProject = "luci-project"

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{Name: "main"}}})
		meta := prjcfgtest.MustExist(ctx, lProject)
		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		runNotifier := run.NewNotifier(ct.TQDispatcher)
		tjNotifier := tryjob.NewNotifier(ct.TQDispatcher)
		h := Handler{
			PMNotifier:  pmNotifier,
			RunNotifier: runNotifier,
			CLMutator:   changelist.NewMutator(ct.TQDispatcher, pmNotifier, runNotifier, tjNotifier),
		}
		state := &State{
			PB: &prjpb.PState{
				LuciProject: lProject,
				Status:      prjpb.Status_STARTED,
				ConfigHash:  meta.Hash(),
				Pcls: []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2},
					{Clid: 3},
					{Clid: 999},
				},
				Components: []*prjpb.Component{
					{Clids: []int64{999}}, // never sees any action.
					{Clids: []int64{1}, DecisionTime: timestamppb.New(now.Add(1 * time.Minute))},
					{Clids: []int64{2}, DecisionTime: timestamppb.New(now.Add(2 * time.Minute))},
					{Clids: []int64{3}, DecisionTime: timestamppb.New(now.Add(3 * time.Minute))},
				},
				NextEvalTime: timestamppb.New(now.Add(1 * time.Minute)),
			},
		}

		pb := backupPB(state)

		markComponentsForTriage := func(indexes ...int) {
			for _, i := range indexes {
				state.PB.GetComponents()[i].TriageRequired = true
			}
			pb = backupPB(state)
		}

		markTriaged := func(c *prjpb.Component) *prjpb.Component {
			if !c.GetTriageRequired() {
				panic(fmt.Errorf("must required triage"))
			}
			o := c.CloneShallow()
			o.TriageRequired = false
			return o
		}

		calledOn := make(chan *prjpb.Component, len(state.PB.Components))
		collectCalledOn := func() []int {
			var out []int
		loop:
			for {
				select {
				case c := <-calledOn:
					out = append(out, int(c.GetClids()[0]))
				default:
					break loop
				}
			}
			sort.Ints(out)
			return out
		}

		t.Run("noop at triage", func(t *ftt.Test) {
			h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
				calledOn <- c
				return itriager.Result{}, nil
			}
			actions, saveForDebug, err := h.triageComponents(ctx, state)
			assert.NoErr(t, err)
			assert.Loosely(t, saveForDebug, should.BeFalse)
			assert.Loosely(t, actions, should.BeNil)
			assert.Loosely(t, state.PB, should.Resemble(pb))
			assert.Loosely(t, collectCalledOn(), should.BeEmpty)

			t.Run("ExecDeferred", func(t *ftt.Test) {
				state2, sideEffect, err := h.ExecDeferred(ctx, state)
				assert.NoErr(t, err)
				assert.Loosely(t, state.PB, should.Resemble(pb))
				assert.Loosely(t, state2, should.Equal(state)) // pointer comparison
				assert.Loosely(t, sideEffect, should.BeNil)
				// Always creates new task iff there is NextEvalTime.
				assert.Loosely(t, pmtest.ETAsOF(ct.TQ.Tasks(), lProject), should.NotBeEmpty)
			})
		})

		t.Run("triage called on TriageRequired components or when decision time is <= now", func(t *ftt.Test) {
			ct.Clock.Set(state.PB.Components[1].DecisionTime.AsTime())
			c1next := state.PB.Components[1].DecisionTime.AsTime().Add(time.Hour)
			markComponentsForTriage(3)
			h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
				calledOn <- c
				switch c.GetClids()[0] {
				case 1:
					c = c.CloneShallow()
					c.DecisionTime = timestamppb.New(c1next)
					return itriager.Result{NewValue: c}, nil
				case 3:
					return itriager.Result{NewValue: markTriaged(c)}, nil
				}
				panic("unreachable")
			}
			actions, saveForDebug, err := h.triageComponents(ctx, state)
			assert.NoErr(t, err)
			assert.Loosely(t, saveForDebug, should.BeFalse)
			assert.Loosely(t, actions, should.HaveLength(2))
			assert.Loosely(t, collectCalledOn(), should.Resemble([]int{1, 3}))

			t.Run("ExecDeferred", func(t *ftt.Test) {
				state2, sideEffect, err := h.ExecDeferred(ctx, state)
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				pb.NextEvalTime = timestamppb.New(now.Add(2 * time.Minute))
				pb.Components[1].DecisionTime = timestamppb.New(c1next)
				pb.Components[3].TriageRequired = false
				assert.Loosely(t, state2.PB, should.Resemble(pb))
				assert.Loosely(t, pmtest.ETAsWithin(ct.TQ.Tasks(), lProject, time.Second, now.Add(2*time.Minute)), should.NotBeEmpty)
			})
		})

		t.Run("purges CLs", func(t *ftt.Test) {
			markComponentsForTriage(1, 2, 3)
			h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
				switch clid := c.GetClids()[0]; clid {
				case 1, 3:
					return itriager.Result{CLsToPurge: []*prjpb.PurgeCLTask{{
						PurgingCl: &prjpb.PurgingCL{Clid: clid,
							ApplyTo: &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
						},
						PurgeReasons: []*prjpb.PurgeReason{{
							ClError: &changelist.CLError{
								Kind: &changelist.CLError_OwnerLacksEmail{
									OwnerLacksEmail: true,
								},
							},
							ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
						}},
					}}}, nil
				case 2:
					return itriager.Result{}, nil
				}
				panic("unreachable")
			}
			actions, saveForDebug, err := h.triageComponents(ctx, state)
			assert.NoErr(t, err)
			assert.Loosely(t, saveForDebug, should.BeFalse)
			assert.Loosely(t, actions, should.HaveLength(3))
			assert.Loosely(t, state.PB, should.Resemble(pb))

			t.Run("ExecDeferred", func(t *ftt.Test) {
				state2, sideEffects, err := h.ExecDeferred(ctx, state)
				assert.NoErr(t, err)
				expectedDeadline := timestamppb.New(now.Add(maxPurgingCLDuration))
				assert.Loosely(t, state2.PB.GetPurgingCls(), should.Resemble([]*prjpb.PurgingCL{
					{Clid: 1, OperationId: "1580640000-1", Deadline: expectedDeadline,
						ApplyTo: &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
					},
					{Clid: 3, OperationId: "1580640000-3", Deadline: expectedDeadline,
						ApplyTo: &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
					},
				}))

				sideEffect := sideEffects.(*SideEffects).items[0]
				assert.Loosely(t, sideEffect, should.HaveType[*TriggerPurgeCLTasks])
				ps := sideEffect.(*TriggerPurgeCLTasks).payloads
				assert.Loosely(t, ps, should.HaveLength(2))
				// Unlike PB.PurgingCls, the tasks aren't necessarily sorted.
				sort.Slice(ps, func(i, j int) bool { return ps[i].GetPurgingCl().GetClid() < ps[j].GetPurgingCl().GetClid() })
				assert.Loosely(t, ps[0].GetPurgingCl(), should.Resemble(state2.PB.GetPurgingCls()[0])) // CL#1
				assert.Loosely(t, ps[0].GetLuciProject(), should.Equal(lProject))
				assert.Loosely(t, ps[1].GetPurgingCl(), should.Resemble(state2.PB.GetPurgingCls()[1])) // CL#3
			})
		})

		t.Run("trigger CL Deps", func(t *ftt.Test) {
			markComponentsForTriage(1, 2, 3)
			h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
				switch clid := c.GetClids()[0]; clid {
				case 3:
					return itriager.Result{CLsToTriggerDeps: []*prjpb.TriggeringCLDeps{{
						OriginClid: 3,
						DepClids:   []int64{1, 2},
					}}}, nil
				case 1, 2:
					return itriager.Result{}, nil
				}
				panic("unreachable")
			}
			actions, saveForDebug, err := h.triageComponents(ctx, state)
			assert.NoErr(t, err)
			assert.Loosely(t, saveForDebug, should.BeFalse)
			assert.Loosely(t, actions, should.HaveLength(3))
			assert.Loosely(t, state.PB, should.Resemble(pb))

			t.Run("ExecDeferred", func(t *ftt.Test) {
				state2, sideEffects, err := h.ExecDeferred(ctx, state)
				assert.NoErr(t, err)
				expectedDeadline := timestamppb.New(now.Add(prjpb.MaxTriggeringCLDepsDuration))
				assert.Loosely(t, state2.PB.GetTriggeringClDeps(), should.HaveLength(1))
				assert.Loosely(t, state2.PB.GetTriggeringClDeps()[0], should.Resemble(&prjpb.TriggeringCLDeps{
					OriginClid:  3,
					DepClids:    []int64{1, 2},
					OperationId: fmt.Sprintf("%d-3", expectedDeadline.AsTime().Unix()),
					Deadline:    expectedDeadline,
				}))

				sideEffect := sideEffects.(*SideEffects).items[0]
				assert.Loosely(t, sideEffect, should.HaveType[*ScheduleTriggeringCLDepsTasks])
				ts := sideEffect.(*ScheduleTriggeringCLDepsTasks).payloads
				assert.Loosely(t, ts, should.HaveLength(1))
				// Sort tasks. Tasks aren't necessarily sorted.
				sort.Slice(ts, func(i, j int) bool {
					lhs := ts[i].GetTriggeringClDeps().GetOriginClid()
					rhs := ts[j].GetTriggeringClDeps().GetOriginClid()
					return lhs < rhs
				})
				assert.Loosely(t, ts, should.HaveLength(1))
				assert.Loosely(t, ts[0].GetLuciProject(), should.Equal(lProject))
				assert.Loosely(t, ts[0].GetTriggeringClDeps(), should.Resemble(
					state2.PB.GetTriggeringClDeps()[0]))
			})
		})

		t.Run("partial failure in triage", func(t *ftt.Test) {
			markComponentsForTriage(1, 2, 3)
			h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
				switch c.GetClids()[0] {
				case 1:
					return itriager.Result{}, errors.New("oops1")
				case 2, 3:
					return itriager.Result{NewValue: markTriaged(c)}, nil
				}
				panic("unreachable")
			}
			actions, saveForDebug, err := h.triageComponents(ctx, state)
			assert.NoErr(t, err)
			assert.Loosely(t, saveForDebug, should.BeFalse)
			assert.Loosely(t, actions, should.HaveLength(2))
			assert.Loosely(t, state.PB, should.Resemble(pb))

			t.Run("ExecDeferred", func(t *ftt.Test) {
				// Execute slightly after #1 component decision time.
				ct.Clock.Set(pb.Components[1].DecisionTime.AsTime().Add(time.Microsecond))
				state2, sideEffect, err := h.ExecDeferred(ctx, state)
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				pb.Components[2].TriageRequired = false
				pb.Components[3].TriageRequired = false
				pb.NextEvalTime = timestamppb.New(ct.Clock.Now()) // re-triage ASAP.
				assert.Loosely(t, state2.PB, should.Resemble(pb))
				// Self-poke task must be scheduled for earliest possible from now.
				assert.Loosely(t, pmtest.ETAsWithin(ct.TQ.Tasks(), lProject, time.Second, ct.Clock.Now().Add(prjpb.PMTaskInterval)), should.NotBeEmpty)
			})
		})

		t.Run("outdated PMState detected during triage", func(t *ftt.Test) {
			markComponentsForTriage(1, 2, 3)
			h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
				switch c.GetClids()[0] {
				case 1:
					return itriager.Result{}, errors.Annotate(itriager.ErrOutdatedPMState, "smth changed").Err()
				case 2, 3:
					return itriager.Result{NewValue: markTriaged(c)}, nil
				}
				panic("unreachable")
			}
			actions, saveForDebug, err := h.triageComponents(ctx, state)
			assert.NoErr(t, err)
			assert.Loosely(t, saveForDebug, should.BeFalse)
			assert.Loosely(t, actions, should.HaveLength(2))
			assert.Loosely(t, state.PB, should.Resemble(pb))

			t.Run("ExecDeferred", func(t *ftt.Test) {
				state2, sideEffect, err := h.ExecDeferred(ctx, state)
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				pb.Components[2].TriageRequired = false
				pb.Components[3].TriageRequired = false
				pb.NextEvalTime = timestamppb.New(ct.Clock.Now()) // re-triage ASAP.
				assert.Loosely(t, state2.PB, should.Resemble(pb))
				// Self-poke task must be scheduled for earliest possible from now.
				assert.Loosely(t, pmtest.ETAsWithin(ct.TQ.Tasks(), lProject, time.Second, ct.Clock.Now().Add(prjpb.PMTaskInterval)), should.NotBeEmpty)
			})
		})

		t.Run("100% failure in triage", func(t *ftt.Test) {
			markComponentsForTriage(1, 2)
			h.ComponentTriage = func(_ context.Context, _ *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
				return itriager.Result{}, errors.New("oops")
			}
			_, _, err := h.triageComponents(ctx, state)
			assert.Loosely(t, err, should.ErrLike("failed to triage 2 components"))
			assert.Loosely(t, state.PB, should.Resemble(pb))

			t.Run("ExecDeferred", func(t *ftt.Test) {
				state2, sideEffect, err := h.ExecDeferred(ctx, state)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.Loosely(t, state2, should.BeNil)
			})
		})

		t.Run("Catches panic in triage", func(t *ftt.Test) {
			markComponentsForTriage(1)
			h.ComponentTriage = func(_ context.Context, _ *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
				panic(errors.New("oops"))
			}
			_, _, err := h.ExecDeferred(ctx, state)
			assert.Loosely(t, err, should.ErrLike(errCaughtPanic))
			assert.Loosely(t, state.PB, should.Resemble(pb))
		})

		t.Run("With Run Creation", func(t *ftt.Test) {
			// Run creation requires ProjectStateOffload entity to exist.
			assert.Loosely(t, datastore.Put(ctx, &prjmanager.ProjectStateOffload{
				ConfigHash: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0].Hash(),
				Project:    datastore.MakeKey(ctx, prjmanager.ProjectKind, lProject),
				Status:     prjpb.Status_STARTED,
			}), should.BeNil)

			makeRunCreator := func(clid int64, fail bool) *runcreator.Creator {
				cfgGroups, err := prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)
				if err != nil {
					panic(err)
				}
				ci := gf.CI(int(clid), gf.PS(1), gf.Owner("user-1"), gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")))
				cl := &changelist.CL{
					ID:       common.CLID(clid),
					EVersion: 1,
					Snapshot: &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: "gerrit-review.example.com",
						Info: ci,
					}}},
				}
				if fail {
					// Simulate EVersion mismatch to fail run creation.
					cl.EVersion = 2
				}
				err = datastore.Put(ctx, cl)
				if err != nil {
					panic(err)
				}
				cl.EVersion = 1
				return &runcreator.Creator{
					LUCIProject:   lProject,
					ConfigGroupID: cfgGroups[0].ID,
					Mode:          run.DryRun,
					OperationID:   fmt.Sprintf("op-%d-%t", clid, fail),
					Owner:         identity.Identity("user:user-1@example.com"),
					CreatedBy:     identity.Identity("user:user-2@example.com"),
					BilledTo:      identity.Identity("user:user-2@example.com"),
					Options:       &run.Options{},
					InputCLs: []runcreator.CL{{
						ID:               common.CLID(clid),
						ExpectedEVersion: 1,
						Snapshot:         cl.Snapshot,
						TriggerInfo: trigger.Find(&trigger.FindInput{
							ChangeInfo:  ci,
							ConfigGroup: cfgGroups[0].Content,
						}).GetCqVoteTrigger(),
					}},
				}
			}

			findRunOf := func(clid int) *run.Run {
				switch runs, _, err := (runquery.CLQueryBuilder{CLID: common.CLID(clid)}).LoadRuns(ctx); {
				case err != nil:
					panic(err)
				case len(runs) == 0:
					return nil
				case len(runs) > 1:
					panic(fmt.Errorf("%d Runs for given CL", len(runs)))
				default:
					return runs[0]
				}
			}

			t.Run("100% success", func(t *ftt.Test) {
				markComponentsForTriage(1)
				h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
					rc := makeRunCreator(1, false /* succeed */)
					return itriager.Result{NewValue: markTriaged(c), RunsToCreate: []*runcreator.Creator{rc}}, nil
				}

				state2, sideEffect, err := h.ExecDeferred(ctx, state)
				assert.NoErr(t, err)
				assert.Loosely(t, sideEffect, should.BeNil)
				pb.Components[1].TriageRequired = false // must be saved, since Run Creation succeeded.
				assert.Loosely(t, state2.PB, should.Resemble(pb))
				assert.Loosely(t, findRunOf(1), should.NotBeNil)
			})

			t.Run("100% failure", func(t *ftt.Test) {
				markComponentsForTriage(1)
				h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
					rc := makeRunCreator(1, true /* fail */)
					return itriager.Result{NewValue: markTriaged(c), RunsToCreate: []*runcreator.Creator{rc}}, nil
				}

				_, sideEffect, err := h.ExecDeferred(ctx, state)
				assert.Loosely(t, err, should.ErrLike("failed to actOnComponents"))
				assert.Loosely(t, sideEffect, should.BeNil)
				assert.Loosely(t, findRunOf(1), should.BeNil)
			})

			t.Run("Partial failure", func(t *ftt.Test) {
				markComponentsForTriage(1, 2, 3)
				h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
					clid := c.GetClids()[0]
					// Set up each component trying to create a Run,
					// and #2 and #3 additionally purging a CL,
					// but #2 failing to create a Run.
					failIf := clid == 2
					rc := makeRunCreator(clid, failIf)
					res := itriager.Result{NewValue: markTriaged(c), RunsToCreate: []*runcreator.Creator{rc}}
					if clid != 1 {
						// Contrived example, since in practice purging a CL concurrently
						// with Run creation in the same component ought to happen only iff
						// there are several CLs and presumably on different CLs.
						res.CLsToPurge = []*prjpb.PurgeCLTask{
							{
								PurgingCl: &prjpb.PurgingCL{
									Clid:    clid,
									ApplyTo: &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
								},
								PurgeReasons: []*prjpb.PurgeReason{{
									ClError: &changelist.CLError{
										Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true},
									},
									ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
								}},
							},
						}
					}
					return res, nil
				}

				state2, sideEffects, err := h.ExecDeferred(ctx, state)
				assert.NoErr(t, err)
				// Only #3 component purge must be a SideEffect.
				sideEffect := sideEffects.(*SideEffects).items[0]
				assert.Loosely(t, sideEffect, should.HaveType[*TriggerPurgeCLTasks])
				ps := sideEffect.(*TriggerPurgeCLTasks).payloads
				assert.Loosely(t, ps, should.HaveLength(1))
				assert.Loosely(t, ps[0].GetPurgingCl().GetClid(), should.Equal(3))

				assert.Loosely(t, findRunOf(1), should.NotBeNil)
				pb.Components[1].TriageRequired = false
				// Component #2 must remain unchanged.
				assert.Loosely(t, findRunOf(3), should.NotBeNil)
				pb.Components[3].TriageRequired = false
				pb.PurgingCls = []*prjpb.PurgingCL{
					{
						Clid: 3, OperationId: "1580640000-3",
						Deadline: timestamppb.New(ct.Clock.Now().Add(maxPurgingCLDuration)),
						ApplyTo:  &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
					},
				}
				pb.NextEvalTime = timestamppb.New(ct.Clock.Now()) // re-triage ASAP.
				assert.Loosely(t, state2.PB, should.Resemble(pb))
			})

			t.Run("Catches panic", func(t *ftt.Test) {
				markComponentsForTriage(1)
				h.ComponentTriage = func(_ context.Context, c *prjpb.Component, _ itriager.PMState) (itriager.Result, error) {
					rc := makeRunCreator(1, false)
					rc.LUCIProject = "" // causes panic because of incorrect usage.
					return itriager.Result{NewValue: markTriaged(c), RunsToCreate: []*runcreator.Creator{rc}}, nil
				}

				_, _, err := h.ExecDeferred(ctx, state)
				assert.Loosely(t, err, should.ErrLike(errCaughtPanic))
				assert.Loosely(t, state.PB, should.Resemble(pb))
			})
		})
	})
}
