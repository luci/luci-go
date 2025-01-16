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
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/cfg"
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

		submit := func(ev BotEventType, dedupKey string, dims []string, healthInfo *BotHealthInfo, taskInfo *BotEventTaskInfo) {
			update := BotInfoUpdate{
				BotID: "bot-id",
				BotGroup: &cfg.BotGroup{
					Dimensions: map[string][]string{
						"pool":      {"a", "b"},
						"something": {"c"},
					},
				},
				EventType:     ev,
				EventDedupKey: dedupKey,
				Dimensions:    dims,
				CallInfo: &BotEventCallInfo{
					SessionID:       "session-id",
					Version:         "version",
					State:           botstate.Dict{JSON: []byte(`{"some-state": 1}`)},
					ExternalIP:      "external-ip",
					AuthenticatedAs: "user:someone@example.com",
				},
				HealthInfo: healthInfo,
				TaskInfo:   taskInfo,
			}
			_, err := update.Submit(ctx)
			assert.NoErr(t, err)
		}

		check := func() (*BotInfo, []*BotEvent) {
			info := &BotInfo{Key: BotInfoKey(ctx, "bot-id")}
			events := []*BotEvent{}
			q := datastore.NewQuery("BotEvent").Ancestor(info.Key.Root()).Order("ts")
			assert.NoErr(t, datastore.Get(ctx, info))
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

		t.Run("New BotInfo entity", func(t *ftt.Test) {
			submit(BotEventConnected, "event-id", nil, nil, nil)

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
			submit(BotEventConnected, "connect", nil, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle-1", []string{"dim:1"}, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle-2", []string{"dim:1"}, nil, nil) // not recorded as "not interesting"
			tickOneSec()
			submit(BotEventIdle, "idle-2", []string{"dim:1"}, nil, nil) // complete ignored as a dup

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
			}))

			assert.That(t, botInfo.Dimensions, should.Match([]string{"dim:1"}))
			assert.That(t, botInfo.FirstSeen, should.Match(testTime))
			assert.That(t, botInfo.LastSeen, should.Match(datastore.NewUnindexedOptional(testTime.Add(2*time.Second))))
			assert.That(t, botInfo.IdleSince, should.Match(datastore.NewUnindexedOptional(testTime.Add(time.Second))))
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

		t.Run("Connect => Idle => Dimension change", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle-1", []string{"dim:1"}, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle-2", []string{"dim:2"}, nil, nil)

			botInfo, events := check()

			assert.That(t, summary(events), should.Match([]string{
				"bot_connected",
				"bot_idle",
				"bot_idle",
			}))

			assert.That(t, botInfo.Dimensions, should.Match([]string{"dim:2"}))
			assert.That(t, events[1].Dimensions, should.Match([]string{"dim:1"}))
			assert.That(t, events[2].Dimensions, should.Match([]string{"dim:2"}))
		})

		t.Run("Connect => Idle => Maintenance", func(t *ftt.Test) {
			submit(BotEventConnected, "connect", nil, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle-1", nil, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle-2", nil, &BotHealthInfo{Maintenance: "boom"}, nil)

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
			submit(BotEventConnected, "connect", nil, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle-1", nil, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle-2", nil, &BotHealthInfo{Quarantined: "boom"}, nil)

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
			submit(BotEventConnected, "connect", nil, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle", nil, nil, nil)
			tickOneSec()
			submit(BotEventMissing, "missing", nil, nil, nil)

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
			submit(BotEventConnected, "connect", nil, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle", nil, nil, nil)
			tickOneSec()
			submit(BotEventMissing, "missing", nil, nil, nil)
			tickOneSec()
			submit(BotEventError, "error", nil, nil, nil)

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
			submit(BotEventConnected, "connect", nil, nil, nil)
			tickOneSec()
			submit(BotEventTask, "task", nil, nil, &BotEventTaskInfo{TaskID: "task-id", TaskName: "task-name"})
			tickOneSec()
			submit(BotEventTaskUpdate, "update-1", nil, nil, nil)
			tickOneSec()
			submit(BotEventTaskUpdate, "update-2", nil, nil, nil)
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
			submit(BotEventTaskCompleted, "completed", nil, nil, nil)
			tickOneSec()
			submit(BotEventIdle, "idle", nil, nil, &BotEventTaskInfo{})

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
	})
}
