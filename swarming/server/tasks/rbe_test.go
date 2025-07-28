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

package tasks

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/swarming/proto/config"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func TestEnqueueTasks(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, tqt := tqtasks.TestingContext(ctx)

		cfg := cfgtest.NewMockedConfigs()
		poolCfg := cfg.MockPool("pool", "project:realm")
		poolCfg.RbeMigration = &configpb.Pool_RBEMigration{
			RbeInstance: "rbe-instance",
			BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
				{
					Mode:    configpb.Pool_RBEMigration_BotModeAllocation_RBE,
					Percent: 100,
				},
			},
			EffectiveBotIdDimension: "dut_id",
		}
		cfg.MockBot("bot1", "pool")

		cfg.MockPool("pool-2", "project:realm").RbeMigration = &configpb.Pool_RBEMigration{
			RbeInstance: "rbe-instance",
			BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
				{
					Mode:    configpb.Pool_RBEMigration_BotModeAllocation_RBE,
					Percent: 100,
				},
			},
			// No EffectiveBotIdDimension here.
		}
		cfg.MockBot("conflicting", "pool", "pool:pool-2")

		p := cfgtest.MockConfigs(ctx, cfg)

		t.Run("EnqueueCancel", func(t *ftt.Test) {
			tID := "65aba3a3e6b99310"
			reqKey, _ := model.TaskIDToRequestKey(ctx, tID)
			tr := &model.TaskRequest{
				Key:         reqKey,
				RBEInstance: "rbe-instance",
				TaskSlices: []model.TaskSlice{
					{
						Properties: model.TaskProperties{
							Dimensions: model.TaskDimensions{
								"d1": {"v1", "v2"},
							},
						},
					},
				},
				Name: "task",
			}
			ttrKey, _ := model.TaskRequestToToRunKey(ctx, tr, 0)
			ttr := &model.TaskToRun{
				Key:            ttrKey,
				RBEReservation: "rbe-reservation",
			}
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return EnqueueRBECancel(ctx, tqt.TQ, tr, ttr)
			}, nil)
			assert.NoErr(t, txErr)
			assert.Loosely(t, tqt.PendingAll(), should.HaveLength(1))

			expected := &internalspb.CancelRBETask{
				RbeInstance:   "rbe-instance",
				ReservationId: "rbe-reservation",
				DebugInfo: &internalspb.CancelRBETask_DebugInfo{
					Created:  timestamppb.New(now),
					TaskName: "task",
				},
			}
			actual := tqt.Payloads()[0].(*internalspb.CancelRBETask)
			assert.Loosely(t, actual, should.Match(expected))
		})

		t.Run("EnqueueRBENew", func(t *ftt.Test) {
			t.Run("OK", func(t *ftt.Test) {
				tID := "65aba3a3e6b69310"
				reqKey, _ := model.TaskIDToRequestKey(ctx, tID)
				tr := &model.TaskRequest{
					Key:         reqKey,
					RBEInstance: "rbe-instance",
					TaskSlices: []model.TaskSlice{
						{
							Properties: model.TaskProperties{
								Dimensions: model.TaskDimensions{
									"d1": {"v1", "v2|v3"},
									"id": {"bot1"},
								},
								ExecutionTimeoutSecs: 3600,
								GracePeriodSecs:      300,
							},
							WaitForCapacity: true,
						},
					},
					Name:                "task",
					Priority:            30,
					SchedulingAlgorithm: configpb.Pool_SCHEDULING_ALGORITHM_FIFO,
				}
				ttrKey, _ := model.TaskRequestToToRunKey(ctx, tr, 0)
				ttr := &model.TaskToRun{
					Key:            ttrKey,
					RBEReservation: "rbe-reservation",
				}
				txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return EnqueueRBENew(ctx, tqt.TQ, tr, ttr, p.Cached(ctx))
				}, nil)
				assert.NoErr(t, txErr)
				assert.Loosely(t, tqt.PendingAll(), should.HaveLength(1))

				expected := &internalspb.EnqueueRBETask{
					Payload: &internalspb.TaskPayload{
						ReservationId:  "rbe-reservation",
						TaskId:         tID,
						SliceIndex:     0,
						TaskToRunShard: 5,
						TaskToRunId:    1,
						DebugInfo: &internalspb.TaskPayload_DebugInfo{
							Created:  timestamppb.New(now),
							TaskName: "task",
						},
					},
					RbeInstance:    "rbe-instance",
					Expiry:         timestamppb.New(ttr.Expiration.Get()),
					RequestedBotId: "bot1",
					Constraints: []*internalspb.EnqueueRBETask_Constraint{
						{Key: "d1", AllowedValues: []string{"v1"}},
						{Key: "d1", AllowedValues: []string{"v2", "v3"}},
					},
					Priority:            30,
					SchedulingAlgorithm: configpb.Pool_SCHEDULING_ALGORITHM_FIFO,
					ExecutionTimeout:    durationpb.New(time.Duration(3930) * time.Second),
					WaitForCapacity:     true,
				}
				actual := tqt.Payloads()[0].(*internalspb.EnqueueRBETask)
				assert.Loosely(t, actual, should.Match(expected))
			})

			t.Run("Conflicting RBE config", func(t *ftt.Test) {
				tID := "65aba3a3e6b69310"
				reqKey, _ := model.TaskIDToRequestKey(ctx, tID)
				tr := &model.TaskRequest{
					Key:         reqKey,
					RBEInstance: "rbe-instance",
					TaskSlices: []model.TaskSlice{
						{
							Properties: model.TaskProperties{
								Dimensions: model.TaskDimensions{
									"d1": {"v1", "v2|v3"},
									"id": {"conflicting"},
								},
								ExecutionTimeoutSecs: 3600,
								GracePeriodSecs:      300,
							},
							WaitForCapacity: true,
						},
					},
					Name:                "task",
					Priority:            30,
					SchedulingAlgorithm: configpb.Pool_SCHEDULING_ALGORITHM_FIFO,
				}
				ttrKey, _ := model.TaskRequestToToRunKey(ctx, tr, 0)
				ttr := &model.TaskToRun{
					Key:            ttrKey,
					RBEReservation: "rbe-reservation",
				}
				txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return EnqueueRBENew(ctx, tqt.TQ, tr, ttr, p.Cached(ctx))
				}, nil)
				assert.That(t, errors.Is(txErr, ErrBadReservation), should.BeTrue)
				assert.That(t, txErr, should.ErrLike(`conflicting RBE config for bot "conflicting"`))
			})

			t.Run("OK_with_effective_bot_id", func(t *ftt.Test) {
				tID := "65aba3a3e6b69320"
				reqKey, _ := model.TaskIDToRequestKey(ctx, tID)
				tr := &model.TaskRequest{
					Key:         reqKey,
					RBEInstance: "rbe-instance",
					TaskSlices: []model.TaskSlice{
						{
							Properties: model.TaskProperties{
								Dimensions: model.TaskDimensions{
									"d1":     {"v1", "v2|v3"},
									"pool":   {"pool"},
									"dut_id": {"dut1"},
								},
								ExecutionTimeoutSecs: 3600,
								GracePeriodSecs:      300,
							},
						},
					},
					Name:                "task",
					Priority:            30,
					SchedulingAlgorithm: configpb.Pool_SCHEDULING_ALGORITHM_FIFO,
				}
				ttrKey, _ := model.TaskRequestToToRunKey(ctx, tr, 0)
				ttr := &model.TaskToRun{
					Key:            ttrKey,
					RBEReservation: "rbe-reservation",
				}
				txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return EnqueueRBENew(ctx, tqt.TQ, tr, ttr, p.Cached(ctx))
				}, nil)
				assert.NoErr(t, txErr)
				assert.Loosely(t, tqt.PendingAll(), should.HaveLength(1))

				actual := tqt.Payloads()[0].(*internalspb.EnqueueRBETask)
				assert.That(t, actual.RequestedBotId, should.Equal("pool:dut_id:dut1"))
			})

			t.Run("OK_with_effective_bot_id_from_bot", func(t *ftt.Test) {
				botInfo := &model.BotInfo{
					Key:               model.BotInfoKey(ctx, "bot1"),
					RBEEffectiveBotID: "pool:dut_id:dut1",
				}
				assert.NoErr(t, datastore.Put(ctx, botInfo))

				tID := "65aba3a3e6b69330"
				reqKey, _ := model.TaskIDToRequestKey(ctx, tID)
				tr := &model.TaskRequest{
					Key:         reqKey,
					RBEInstance: "rbe-instance",
					TaskSlices: []model.TaskSlice{
						{
							Properties: model.TaskProperties{
								Dimensions: model.TaskDimensions{
									"d1": {"v1", "v2|v3"},
									"id": {"bot1"},
								},
								ExecutionTimeoutSecs: 3600,
								GracePeriodSecs:      300,
							},
						},
					},
					Name:                "task",
					Priority:            30,
					SchedulingAlgorithm: configpb.Pool_SCHEDULING_ALGORITHM_FIFO,
				}
				ttrKey, _ := model.TaskRequestToToRunKey(ctx, tr, 0)
				ttr := &model.TaskToRun{
					Key:            ttrKey,
					RBEReservation: "rbe-reservation",
				}
				txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return EnqueueRBENew(ctx, tqt.TQ, tr, ttr, p.Cached(ctx))
				}, nil)
				assert.NoErr(t, txErr)
				assert.Loosely(t, tqt.PendingAll(), should.HaveLength(1))

				actual := tqt.Payloads()[0].(*internalspb.EnqueueRBETask)
				assert.That(t, actual.RequestedBotId, should.Equal("pool:dut_id:dut1"))
			})
		})
	})
}

func TestDimsToBotIDAndConstraints(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	bot2 := "bot2"
	bot2Info := &model.BotInfo{
		Key:               model.BotInfoKey(ctx, bot2),
		RBEEffectiveBotID: "pool:dut_id:bot2",
	}

	bot3 := "bot3"
	bot3Info := &model.BotInfo{
		Key: model.BotInfoKey(ctx, bot3),
	}
	assert.NoErr(t, datastore.Put(ctx, bot2Info, bot3Info))

	cases := []struct {
		name                 string
		dims                 model.TaskDimensions
		botID                string
		constraints          []*internalspb.EnqueueRBETask_Constraint
		rbeEffectiveBotIDDim string
		pool                 string
		err                  string
	}{
		{
			name: "ok_with_bot_id",
			dims: model.TaskDimensions{
				"id":  {"bot1"},
				"key": {"value"},
			},
			botID: "bot1",
			constraints: []*internalspb.EnqueueRBETask_Constraint{
				{Key: "key", AllowedValues: []string{"value"}},
			},
		},
		{
			name: "ok_with_no_id",
			dims: model.TaskDimensions{
				"key": {"value"},
			},
			botID: "",
			constraints: []*internalspb.EnqueueRBETask_Constraint{
				{Key: "key", AllowedValues: []string{"value"}},
			},
		},
		{
			name: "ok_to_translate_id_with_effective_one",
			dims: model.TaskDimensions{
				"id": {"bot2"},
			},
			botID:                "pool:dut_id:bot2",
			rbeEffectiveBotIDDim: "dut_id",
		},
		{
			name: "ok_with_effective_bot_id",
			dims: model.TaskDimensions{
				"dut_id": {"value"},
			},
			botID:                "pool:dut_id:value",
			rbeEffectiveBotIDDim: "dut_id",
			pool:                 "pool",
		},
		{
			name: "ok_with_id_and_effective_bot_id_if_not_confilicting",
			dims: model.TaskDimensions{
				"id":     {"bot2"},
				"dut_id": {"bot2"},
			},
			botID:                "pool:dut_id:bot2",
			rbeEffectiveBotIDDim: "dut_id",
			pool:                 "pool",
		},
		{
			name: "use_bot_id_if_failed_to_get_effective_bot_id",
			dims: model.TaskDimensions{
				"id": {"bot1"},
			},
			botID:                "bot1",
			rbeEffectiveBotIDDim: "dut_id",
			pool:                 "pool",
		},
		{
			name: "use_bot_id_if_bot_does_not_have_effective_bot_id",
			dims: model.TaskDimensions{
				"id": {"bot3"},
			},
			botID:                "bot3",
			rbeEffectiveBotIDDim: "dut_id",
			pool:                 "pool",
		},
		{
			name: "confilicting_effective_bot_id",
			dims: model.TaskDimensions{
				"id":     {"bot2"},
				"dut_id": {"value"},
			},
			rbeEffectiveBotIDDim: "dut_id",
			pool:                 "pool",
			err:                  `conflicting effective bot IDs: "pool:dut_id:bot2" (according to bot "bot2") and "pool:dut_id:value" (according to task)`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			botID, constraints, err := dimsToBotIDAndConstraints(ctx, tc.dims, tc.rbeEffectiveBotIDDim, tc.pool)
			assert.That(t, botID, should.Equal(tc.botID))
			assert.That(t, constraints, should.Match(tc.constraints))
			if tc.err == "" {
				assert.NoErr(t, err)
			} else {
				assert.That(t, errors.Is(err, ErrBadReservation), should.BeTrue)
				assert.That(t, err, should.ErrLike(tc.err))
			}
		})
	}
}
