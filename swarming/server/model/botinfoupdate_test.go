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

package model

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

		submit := func(ev BotEventType, dedupKey string, dims []string, state *botstate.Dict, diffs ...any) *SubmittedBotInfoUpdate {
			var healthInfo *BotHealthInfo
			var taskInfo *BotEventTaskInfo
			var effectiveIDInfo *RBEEffectiveBotIDInfo
			for _, diff := range diffs {
				switch val := diff.(type) {
				case *BotHealthInfo:
					if healthInfo != nil {
						panic("healthInfo given twice")
					}
					healthInfo = val
				case *BotEventTaskInfo:
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

			update := BotInfoUpdate{
				BotID: "bot-id",
				BotGroupDimensions: map[string][]string{
					"pool":      {"a", "b"},
					"something": {"c"},
				},
				EventType:     ev,
				EventDedupKey: dedupKey,
				Dimensions:    dims,
				State:         state,
				CallInfo: &BotEventCallInfo{
					SessionID:       "session-id",
					Version:         "version",
					ExternalIP:      "external-ip",
					AuthenticatedAs: "user:someone@example.com",
				},
				HealthInfo:         healthInfo,
				TaskInfo:           taskInfo,
				EffectiveBotIDInfo: effectiveIDInfo,
			}
			submitted, err := update.Submit(ctx, nil)
			assert.NoErr(t, err)
			return submitted
		}

		check := func() (*BotInfo, []*BotEvent) {
			info := &BotInfo{Key: BotInfoKey(ctx, "bot-id")}
			events := []*BotEvent{}
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

		summary := func(ev []*BotEvent) []string {
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
			submit(BotEventConnected, "event-id", nil, testState1)

			botInfo, events := check()

			assert.That(t, botInfo, should.Match(&BotInfo{
				Key: botInfo.Key,
				Dimensions: []string{
					"id:bot-id",
					"pool:a",
					"pool:b",
					"something:c",
				},
				Composite: []BotStateEnum{
					BotStateNotInMaintenance,
					BotStateAlive,
					BotStateHealthy,
					BotStateBusy,
				},
				FirstSeen:         testTime,
				LastEventDedupKey: "bot_connected:event-id",
				BotCommon: BotCommon{
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
			assert.That(t, events[0], should.Match(&BotEvent{
				Key:       events[0].Key,
				Timestamp: testTime,
				EventType: BotEventConnected,
				Dimensions: []string{
					"id:bot-id",
					"pool:a",
					"pool:b",
					"something:c",
				},
				BotCommon: BotCommon{
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
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-1", []string{"dim:1", "id:bot-id"}, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-2", []string{"dim:1", "id:bot-id"}, testState2) // not recorded as "not interesting"
			tickOneSec()
			submit(BotEventIdle, "idle-2", []string{"dim:1", "id:bot-id"}, testState2) // complete ignored as a dup

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
			assert.That(t, botInfo.Composite, should.Match([]BotStateEnum{
				BotStateNotInMaintenance,
				BotStateAlive,
				BotStateHealthy,
				BotStateIdle,
			}))

			// Even though "idle-2" was not recorded as BotEvent, it is still stored
			// as the last processed event.
			assert.That(t, botInfo.LastEventDedupKey, should.Equal("bot_idle:idle-2"))
		})

		t.Run("Connect => Idle (no state)", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle", nil, nil) // do not pass state

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
			}))
			assert.That(t, botInfo.State, should.Match(*testState1))
		})

		t.Run("Connect => delete => delete", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			submitted := submit(BotEventDeleted, "deleted-1", nil, nil)
			assert.Loosely(t, submitted.BotInfo, should.NotBeNil)

			botInfo, events := check()
			assert.Loosely(t, botInfo, should.BeNil)
			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_deleted",
			}))

			submitted = submit(BotEventDeleted, "deleted-2", nil, nil)
			assert.Loosely(t, submitted.BotInfo, should.BeNil)

			botInfo, events = check()
			assert.Loosely(t, botInfo, should.BeNil)
			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_deleted", // still only one
			}))
		})

		t.Run("Connect => Idle => Dimension change", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-1", []string{"dim:1", "id:bot-id"}, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-2", []string{"dim:2", "id:bot-id"}, testState1)

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
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-1", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-2", nil, testState1, &BotHealthInfo{Maintenance: "boom"})

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_idle boom",
			}))

			assert.That(t, botInfo.Composite, should.Match([]BotStateEnum{
				BotStateInMaintenance,
				BotStateAlive,
				BotStateHealthy,
				BotStateBusy, // implied by being in maintenance
			}))
		})

		t.Run("Connect => Idle => Quarantine", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-1", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-2", nil, testState1, &BotHealthInfo{Quarantined: "boom"})

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_idle boom",
			}))

			assert.That(t, botInfo.Composite, should.Match([]BotStateEnum{
				BotStateNotInMaintenance,
				BotStateAlive,
				BotStateQuarantined,
				BotStateBusy, // implied by being in quarantine
			}))
		})

		t.Run("Connect => Idle => Dead", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle", nil, testState1)
			tickOneSec()
			submit(BotEventMissing, "missing", nil, testState1)

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_missing",
			}))

			assert.That(t, botInfo.Composite, should.Match([]BotStateEnum{
				BotStateNotInMaintenance,
				BotStateDead,
				BotStateHealthy,
				BotStateIdle,
			}))
		})

		t.Run("Connect => Idle => Dead => Logging", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle", nil, testState1)
			tickOneSec()
			submit(BotEventMissing, "missing", nil, testState1)
			tickOneSec()
			submit(BotEventError, "error", nil, testState1)

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_missing",
				"bot_error",
			}))

			assert.That(t, botInfo.Composite, should.Match([]BotStateEnum{
				BotStateNotInMaintenance,
				BotStateDead, // still dead
				BotStateHealthy,
				BotStateIdle, // still idle
			}))
		})

		t.Run("Connect => Task => Task update => Idle", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventTask, "task", nil, testState1, &BotEventTaskInfo{TaskID: "task-id", TaskName: "task-name"})
			tickOneSec()
			submit(BotEventTaskUpdate, "update-1", nil, testState1)
			tickOneSec()
			submit(BotEventTaskUpdate, "update-2", nil, testState1)
			tickOneSec()

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"request_task",
			}))

			assert.That(t, botInfo.Composite, should.Match([]BotStateEnum{
				BotStateNotInMaintenance,
				BotStateAlive,
				BotStateHealthy,
				BotStateBusy,
			}))
			assert.That(t, botInfo.TaskID, should.Equal("task-id"))
			assert.That(t, botInfo.TaskName, should.Equal("task-name"))
			assert.That(t, events[1].TaskID, should.Equal("task-id"))

			// Finishes the task and becomes idle.
			submit(BotEventTaskCompleted, "completed", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle", nil, testState1)

			botInfo, events = check()

			assert.That(t, summary(events)[2:], should.Match([]string{
				"task_completed",
				"bot_idle",
			}))

			assert.That(t, botInfo.Composite, should.Match([]BotStateEnum{
				BotStateNotInMaintenance,
				BotStateAlive,
				BotStateHealthy,
				BotStateIdle,
			}))
			assert.That(t, botInfo.TaskID, should.Equal(""))
			assert.That(t, botInfo.TaskName, should.Equal(""))

			assert.That(t, events[2].TaskID, should.Equal("task-id")) // task_completed
			assert.That(t, events[3].TaskID, should.Equal(""))        // bot_idle
		})

		t.Run("Connect => Task => Missing => Connect", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventTask, "task", nil, testState1, &BotEventTaskInfo{TaskID: "task-id", TaskName: "task-name"})
			tickOneSec()
			submit(BotEventTaskUpdate, "update-2", nil, testState1)
			tickOneSec()
			submit(BotEventMissing, "missing", nil, testState1)
			tickOneSec()

			info, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"request_task",
				"bot_missing Abandoned task-id",
			}))

			// Actually retained TaskID.
			assert.That(t, info.TaskID, should.Equal("task-id"))

			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()

			info, events = check()

			// Reset TaskID, but no new "Abandoned ..." event emitted.
			assert.That(t, info.TaskID, should.Equal(""))
			assert.That(t, summary(events[3:]), should.Match([]string{
				"bot_connected",
			}))
		})

		t.Run("Connect => Task => Connect", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventTask, "task", nil, testState1, &BotEventTaskInfo{TaskID: "task-id", TaskName: "task-name"})
			tickOneSec()
			submit(BotEventTaskUpdate, "update-2", nil, testState1)
			tickOneSec()
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()

			_, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"request_task",
				"bot_connected Abandoned task-id",
			}))
		})

		t.Run("Connect => Task => Deleted", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventTask, "task", nil, testState1, &BotEventTaskInfo{TaskID: "task-id", TaskName: "task-name"})
			tickOneSec()
			submit(BotEventTaskUpdate, "update-2", nil, testState1)
			tickOneSec()
			submit(BotEventDeleted, "delete", nil, testState1)
			tickOneSec()

			_, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"request_task",
				"bot_deleted Abandoned task-id",
			}))
		})

		t.Run("Connect => TerminateBot => Connect => Missing", func(t *ftt.Test) {
			submit(BotEventConnected, "connect-1", nil, testState1)
			tickOneSec()
			submit(BotEventTerminate, "terminate", nil, testState1, &BotEventTaskInfo{
				TaskID:    "task-id",
				TaskName:  "task-name",
				TaskFlags: TaskFlagTermination,
			})
			tickOneSec()
			submit(BotEventTaskCompleted, "done", nil, testState1)
			tickOneSec()
			submit(BotEventShutdown, "dead", nil, testState1)
			tickOneSec()

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_terminate",
				"task_completed",
				"bot_shutdown",
			}))

			assert.That(t, botInfo.Composite, should.Match([]BotStateEnum{
				BotStateNotInMaintenance,
				BotStateDead,
				BotStateHealthy,
				BotStateIdle,
			}))
			assert.That(t, botInfo.TaskID, should.Equal(""))
			assert.That(t, botInfo.TaskName, should.Equal(""))
			assert.That(t, botInfo.TaskFlags, should.Equal(TaskFlags(0)))
			assert.That(t, botInfo.LastFinishedTask, should.Equal(LastTaskDetails{
				TaskID:      "task-id",
				TaskName:    "task-name",
				TaskFlags:   TaskFlagTermination,
				FinishedDue: BotEventTaskCompleted,
			}))
			assert.That(t, botInfo.TerminationTaskID, should.Equal("task-id"))

			// When it connects again, TerminationTaskID gets unset.
			submit(BotEventConnected, "connect-2", nil, testState1)
			tickOneSec()

			botInfo, _ = check()
			assert.That(t, botInfo.LastFinishedTask, should.Equal(LastTaskDetails{}))
			assert.That(t, botInfo.TerminationTaskID, should.Equal(""))

			// When it does ungracefully, TerminationTaskID is still unset.
			submit(BotEventMissing, "missing", nil, testState1)
			tickOneSec()

			botInfo, _ = check()
			assert.That(t, botInfo.TerminationTaskID, should.Equal(""))
		})

		t.Run("Connect => Idle => Effective bot ID change", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-1", []string{"dim:a", "id:bot-id"}, testState1)
			tickOneSec()
			submit(BotEventIdle, "idle-2", []string{"dim:a", "id:bot-id"}, testState1, &RBEEffectiveBotIDInfo{
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
			var saw []*BotInfo
			update := BotInfoUpdate{
				BotID:     "bot-id",
				EventType: BotEventConnected,
				Prepare: func(ctx context.Context, bot *BotInfo) (proceed bool, err error) {
					saw = append(saw, bot)
					return false, nil
				},
			}
			submitted, err := update.Submit(ctx, nil)
			assert.NoErr(t, err)
			assert.Loosely(t, submitted, should.BeNil)
			assert.That(t, saw, should.Match([]*BotInfo{nil}))

			// The entity is still actually missing.
			info := &BotInfo{Key: BotInfoKey(ctx, "bot-id")}
			assert.That(t, datastore.Get(ctx, info), should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("Prepare + normal update", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, testState1)

			var saw []*BotInfo
			update := BotInfoUpdate{
				BotID:     "bot-id",
				EventType: BotEventIdle,
				Prepare: func(ctx context.Context, bot *BotInfo) (proceed bool, err error) {
					saw = append(saw, bot)
					return false, nil
				},
			}
			submitted, err := update.Submit(ctx, nil)
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
