// Copyright 2025 The LUCI Authors.
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

package botinfo

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(
		datastore.Optional[time.Time, datastore.Unindexed]{},
	))
}

func TestBotInfoUpdate(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		var testTime = time.Date(2024, time.March, 3, 4, 5, 6, 0, time.UTC)

		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, tc := testclock.UseTime(ctx, testTime)

		tickOneSec := func() { tc.Add(time.Second) }

		submit := func(ev model.BotEventType, dedupKey string, dims []string, state *botstate.Dict, diffs ...any) *SubmittedUpdate {
			var healthInfo *HealthInfo
			var taskInfo *TaskInfo
			var effectiveIDInfo *RBEEffectiveBotIDInfo
			for _, diff := range diffs {
				switch val := diff.(type) {
				case *HealthInfo:
					if healthInfo != nil {
						panic("healthInfo given twice")
					}
					healthInfo = val
				case *TaskInfo:
					if taskInfo != nil {
						panic("taskInfo given twice")
					}
					taskInfo = val
				case *RBEEffectiveBotIDInfo:
					if effectiveIDInfo != nil {
						panic("effectiveIDInfo given twice")
					}
					effectiveIDInfo = val
				default:
					panic(fmt.Sprintf("unexpected diff %T", diff))
				}
			}

			update := Update{
				BotID: "bot-id",
				BotGroupDimensions: map[string][]string{
					"pool":      {"a", "b"},
					"something": {"c"},
				},
				EventType:     ev,
				EventDedupKey: dedupKey,
				TasksManager: &tasks.MockedManager{
					AbandonTxnMock: func(context.Context, *tasks.AbandonOp) (*tasks.AbandonOpOutcome, error) {
						return &tasks.AbandonOpOutcome{}, nil
					},
				},
				Dimensions: dims,
				State:      state,
				CallInfo: &CallInfo{
					SessionID:       "session-id",
					Version:         "version",
					ExternalIP:      "external-ip",
					AuthenticatedAs: "user:someone@example.com",
				},
				HealthInfo:         healthInfo,
				TaskInfo:           taskInfo,
				EffectiveBotIDInfo: effectiveIDInfo,
			}
			submitted, err := update.Submit(ctx)
			assert.NoErr(t, err)
			return submitted
		}

		check := func() (*model.BotInfo, []*model.BotEvent) {
			info := &model.BotInfo{Key: model.BotInfoKey(ctx, "bot-id")}
			events := []*model.BotEvent{}
			q := datastore.NewQuery("BotEvent").Ancestor(info.Key.Root()).Order("ts")
			err := datastore.Get(ctx, info)
			if errors.Is(err, datastore.ErrNoSuchEntity) {
				info = nil
			} else {
				assert.NoErr(t, err)
			}
			assert.NoErr(t, datastore.GetAll(ctx, q, &events))
			return info, events
		}

		summary := func(ev []*model.BotEvent) []string {
			var out []string
			for _, e := range ev {
				parts := []string{string(e.EventType)}
				if e.Message != "" {
					parts = append(parts, e.Message)
				}
				out = append(out, strings.Join(parts, " "))
			}
			return out
		}

		testState1 := &botstate.Dict{JSON: []byte(`{"some-state": 1}`)}
		testState2 := &botstate.Dict{JSON: []byte(`{"some-state": 2}`)}

		t.Run("New BotInfo entity", func(t *ftt.Test) {
			submit(model.BotEventConnected, "event-id", nil, testState1)

			botInfo, events := check()

			assert.That(t, botInfo, should.Match(&model.BotInfo{
				Key: botInfo.Key,
				Dimensions: []string{
					"id:bot-id",
					"pool:a",
					"pool:b",
					"something:c",
				},
				Composite: []model.BotStateEnum{
					model.BotStateNotInMaintenance,
					model.BotStateAlive,
					model.BotStateHealthy,
					model.BotStateBusy,
				},
				FirstSeen:         testTime,
				LastEventDedupKey: "bot_connected:event-id",
				BotCommon: model.BotCommon{
					State:           botstate.Dict{JSON: []byte(`{"some-state": 1}`)},
					SessionID:       "session-id",
					ExternalIP:      "external-ip",
					AuthenticatedAs: "user:someone@example.com",
					Version:         "version",
					LastSeen:        datastore.NewUnindexedOptional(testTime),
					ExpireAt:        testTime.Add(oldBotInfoCutoff),
				},
			}))

			assert.That(t, len(events), should.Equal(1))
			assert.That(t, events[0], should.Match(&model.BotEvent{
				Key:       events[0].Key,
				Timestamp: testTime,
				EventType: model.BotEventConnected,
				Dimensions: []string{
					"id:bot-id",
					"pool:a",
					"pool:b",
					"something:c",
				},
				BotCommon: model.BotCommon{
					State:           botstate.Dict{JSON: []byte(`{"some-state": 1}`)},
					SessionID:       "session-id",
					ExternalIP:      "external-ip",
					AuthenticatedAs: "user:someone@example.com",
					Version:         "version",
					LastSeen:        datastore.NewUnindexedOptional(testTime),
					ExpireAt:        testTime.Add(oldBotEventsCutOff),
				},
			}))
		})

		t.Run("Connect => Idle", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-1", []string{"dim:1", "id:bot-id"}, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-2", []string{"dim:1", "id:bot-id"}, testState2) // not recorded as "not interesting"
			tickOneSec()
			submit(model.BotEventIdle, "idle-2", []string{"dim:1", "id:bot-id"}, testState2) // complete ignored as a dup

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
			}))

			assert.That(t, botInfo.Dimensions, should.Match([]string{"dim:1", "id:bot-id"}))
			assert.That(t, botInfo.FirstSeen, should.Match(testTime))
			assert.That(t, botInfo.LastSeen, should.Match(datastore.NewUnindexedOptional(testTime.Add(2*time.Second))))
			assert.That(t, botInfo.IdleSince, should.Match(datastore.NewUnindexedOptional(testTime.Add(time.Second))))
			assert.That(t, botInfo.State, should.Match(*testState2))
			assert.That(t, botInfo.Composite, should.Match([]model.BotStateEnum{
				model.BotStateNotInMaintenance,
				model.BotStateAlive,
				model.BotStateHealthy,
				model.BotStateIdle,
			}))

			// Even though "idle-2" was not recorded as BotEvent, it is still stored
			// as the last processed event.
			assert.That(t, botInfo.LastEventDedupKey, should.Equal("bot_idle:idle-2"))
		})

		t.Run("Connect => Idle (no state)", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle", nil, nil) // do not pass state

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
			}))
			assert.That(t, botInfo.State, should.Match(*testState1))
		})

		t.Run("Connect => delete => delete", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			submitted := submit(model.BotEventDeleted, "deleted-1", nil, nil)
			assert.Loosely(t, submitted.BotInfo, should.NotBeNil)

			botInfo, events := check()
			assert.Loosely(t, botInfo, should.BeNil)
			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_deleted",
			}))

			submitted = submit(model.BotEventDeleted, "deleted-2", nil, nil)
			assert.Loosely(t, submitted.BotInfo, should.BeNil)

			botInfo, events = check()
			assert.Loosely(t, botInfo, should.BeNil)
			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_deleted", // still only one
			}))
		})

		t.Run("Connect => Idle => Dimension change", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-1", []string{"dim:1", "id:bot-id"}, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-2", []string{"dim:2", "id:bot-id"}, testState1)

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_idle",
			}))

			assert.That(t, botInfo.Dimensions, should.Match([]string{"dim:2", "id:bot-id"}))
			assert.That(t, events[1].Dimensions, should.Match([]string{"dim:1", "id:bot-id"}))
			assert.That(t, events[2].Dimensions, should.Match([]string{"dim:2", "id:bot-id"}))
		})

		t.Run("Connect => Idle => Maintenance", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-1", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-2", nil, testState1, &HealthInfo{Maintenance: "boom"})

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_idle boom",
			}))

			assert.That(t, botInfo.Composite, should.Match([]model.BotStateEnum{
				model.BotStateInMaintenance,
				model.BotStateAlive,
				model.BotStateHealthy,
				model.BotStateBusy, // implied by being in maintenance
			}))
		})

		t.Run("Connect => Idle => Quarantine", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-1", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-2", nil, testState1, &HealthInfo{Quarantined: "boom"})

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_idle boom",
			}))

			assert.That(t, botInfo.Composite, should.Match([]model.BotStateEnum{
				model.BotStateNotInMaintenance,
				model.BotStateAlive,
				model.BotStateQuarantined,
				model.BotStateBusy, // implied by being in quarantine
			}))
		})

		t.Run("Connect => Idle => Dead", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle", nil, testState1)
			tickOneSec()
			submit(model.BotEventMissing, "missing", nil, testState1)

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_missing",
			}))

			assert.That(t, botInfo.Composite, should.Match([]model.BotStateEnum{
				model.BotStateNotInMaintenance,
				model.BotStateDead,
				model.BotStateHealthy,
				model.BotStateIdle,
			}))
		})

		t.Run("Connect => Idle => Dead => Logging", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle", nil, testState1)
			tickOneSec()
			submit(model.BotEventMissing, "missing", nil, testState1)
			tickOneSec()
			submit(model.BotEventError, "error", nil, testState1)

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_missing",
				"bot_error",
			}))

			assert.That(t, botInfo.Composite, should.Match([]model.BotStateEnum{
				model.BotStateNotInMaintenance,
				model.BotStateDead, // still dead
				model.BotStateHealthy,
				model.BotStateIdle, // still idle
			}))
		})

		t.Run("Connect => Task => Task update => Idle", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventTask, "task", nil, testState1, &TaskInfo{TaskID: "task-id", TaskName: "task-name"})
			tickOneSec()
			submit(model.BotEventTaskUpdate, "update-1", nil, testState1)
			tickOneSec()
			submit(model.BotEventTaskUpdate, "update-2", nil, testState1)
			tickOneSec()

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"request_task",
			}))

			assert.That(t, botInfo.Composite, should.Match([]model.BotStateEnum{
				model.BotStateNotInMaintenance,
				model.BotStateAlive,
				model.BotStateHealthy,
				model.BotStateBusy,
			}))
			assert.That(t, botInfo.TaskID, should.Equal("task-id"))
			assert.That(t, botInfo.TaskName, should.Equal("task-name"))
			assert.That(t, events[1].TaskID, should.Equal("task-id"))

			// Finishes the task and becomes idle.
			submit(model.BotEventTaskCompleted, "completed", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle", nil, testState1)

			botInfo, events = check()

			assert.That(t, summary(events)[2:], should.Match([]string{
				"task_completed",
				"bot_idle",
			}))

			assert.That(t, botInfo.Composite, should.Match([]model.BotStateEnum{
				model.BotStateNotInMaintenance,
				model.BotStateAlive,
				model.BotStateHealthy,
				model.BotStateIdle,
			}))
			assert.That(t, botInfo.TaskID, should.Equal(""))
			assert.That(t, botInfo.TaskName, should.Equal(""))

			assert.That(t, events[2].TaskID, should.Equal("task-id")) // task_completed
			assert.That(t, events[3].TaskID, should.Equal(""))        // bot_idle
		})

		t.Run("Connect => Task => Missing => Connect", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventTask, "task", nil, testState1, &TaskInfo{TaskID: "task-id", TaskName: "task-name"})
			tickOneSec()
			submit(model.BotEventTaskUpdate, "update-2", nil, testState1)
			tickOneSec()
			submit(model.BotEventMissing, "missing", nil, testState1)
			tickOneSec()

			info, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"request_task",
				"bot_missing Abandoned task-id",
			}))

			// Actually retained TaskID.
			assert.That(t, info.TaskID, should.Equal("task-id"))

			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()

			info, events = check()

			// Reset TaskID, but no new "Abandoned ..." event emitted.
			assert.That(t, info.TaskID, should.Equal(""))
			assert.That(t, summary(events[3:]), should.Match([]string{
				"bot_connected",
			}))
		})

		t.Run("Connect => Task => Connect", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventTask, "task", nil, testState1, &TaskInfo{TaskID: "task-id", TaskName: "task-name"})
			tickOneSec()
			submit(model.BotEventTaskUpdate, "update-2", nil, testState1)
			tickOneSec()
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()

			_, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"request_task",
				"bot_connected Abandoned task-id",
			}))
		})

		t.Run("Connect => Task => Deleted", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventTask, "task", nil, testState1, &TaskInfo{TaskID: "task-id", TaskName: "task-name"})
			tickOneSec()
			submit(model.BotEventTaskUpdate, "update-2", nil, testState1)
			tickOneSec()
			submit(model.BotEventDeleted, "delete", nil, testState1)
			tickOneSec()

			_, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"request_task",
				"bot_deleted Abandoned task-id",
			}))
		})

		t.Run("Connect => TerminateBot => Connect => Missing", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect-1", nil, testState1)
			tickOneSec()
			submit(model.BotEventTerminate, "terminate", nil, testState1, &TaskInfo{
				TaskID:    "task-id",
				TaskName:  "task-name",
				TaskFlags: model.TaskFlagTermination,
			})
			tickOneSec()
			submit(model.BotEventTaskCompleted, "done", nil, testState1)
			tickOneSec()
			submit(model.BotEventShutdown, "dead", nil, testState1)
			tickOneSec()

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_terminate",
				"task_completed",
				"bot_shutdown",
			}))

			assert.That(t, botInfo.Composite, should.Match([]model.BotStateEnum{
				model.BotStateNotInMaintenance,
				model.BotStateDead,
				model.BotStateHealthy,
				model.BotStateIdle,
			}))
			assert.That(t, botInfo.TaskID, should.Equal(""))
			assert.That(t, botInfo.TaskName, should.Equal(""))
			assert.That(t, botInfo.TaskFlags, should.Equal(model.TaskFlags(0)))
			assert.That(t, botInfo.LastFinishedTask, should.Equal(model.LastTaskDetails{
				TaskID:      "task-id",
				TaskName:    "task-name",
				TaskFlags:   model.TaskFlagTermination,
				FinishedDue: model.BotEventTaskCompleted,
			}))
			assert.That(t, botInfo.TerminationTaskID, should.Equal("task-id"))

			// When it connects again, TerminationTaskID gets unset.
			submit(model.BotEventConnected, "connect-2", nil, testState1)
			tickOneSec()

			botInfo, _ = check()
			assert.That(t, botInfo.LastFinishedTask, should.Equal(model.LastTaskDetails{}))
			assert.That(t, botInfo.TerminationTaskID, should.Equal(""))

			// When it does ungracefully, TerminationTaskID is still unset.
			submit(model.BotEventMissing, "missing", nil, testState1)
			tickOneSec()

			botInfo, _ = check()
			assert.That(t, botInfo.TerminationTaskID, should.Equal(""))
		})

		t.Run("Connect => Idle => Effective bot ID change", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-1", []string{"dim:a", "id:bot-id"}, testState1)
			tickOneSec()
			submit(model.BotEventIdle, "idle-2", []string{"dim:a", "id:bot-id"}, testState1, &RBEEffectiveBotIDInfo{
				RBEEffectiveBotID: "effective-id",
			})

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				`bot_idle RBE effective bot ID: "" => "effective-id"`,
			}))

			assert.That(t, botInfo.RBEEffectiveBotID, should.Equal("effective-id"))
		})

		t.Run("Prepare + initial update", func(t *ftt.Test) {
			// The callback sees `nil` if there's no BotInfo yet.
			var saw []*model.BotInfo
			update := Update{
				BotID:     "bot-id",
				EventType: model.BotEventConnected,
				Prepare: func(ctx context.Context, bot *model.BotInfo) (proceed bool, err error) {
					saw = append(saw, bot)
					return false, nil
				},
			}
			submitted, err := update.Submit(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, submitted, should.BeNil)
			assert.That(t, saw, should.Match([]*model.BotInfo{nil}))

			// The entity is still actually missing.
			info := &model.BotInfo{Key: model.BotInfoKey(ctx, "bot-id")}
			assert.That(t, datastore.Get(ctx, info), should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("Prepare + normal update", func(t *ftt.Test) {
			submit(model.BotEventConnected, "connect", nil, testState1)

			var saw []*model.BotInfo
			update := Update{
				BotID:     "bot-id",
				EventType: model.BotEventIdle,
				Prepare: func(ctx context.Context, bot *model.BotInfo) (proceed bool, err error) {
					saw = append(saw, bot)
					return false, nil
				},
			}
			submitted, err := update.Submit(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, submitted, should.BeNil)
			assert.Loosely(t, saw, should.HaveLength(1))
			assert.Loosely(t, saw[0], should.NotBeNil)

			// Have only one event recorded.
			_, events := check()
			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
			}))
		})
	})
}
