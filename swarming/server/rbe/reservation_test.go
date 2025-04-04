// Copyright 2023 The LUCI Authors.
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

package rbe

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
	configpb "go.chromium.org/luci/swarming/proto/config"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func TestReservationServer(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		const rbeInstance = "projects/x/instances/y"
		const rbeReservation = "reservation-id"

		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		config := cfgtest.NewMockedConfigs()
		poolCfg := config.MockPool("some-pool", "project:realm")
		poolCfg.InformationalDimensionRe = []string{"label-*"}
		p := cfgtest.MockConfigs(ctx, config)
		rbe := mockedReservationClient{
			newState: remoteworkers.ReservationState_RESERVATION_PENDING,
		}
		internals := mockedInternalsClient{}
		srv := ReservationServer{
			rbe:           &rbe,
			internals:     &internals,
			serverProject: "cloud-proj",
			serverVersion: "go-version",
			cfg:           p,
		}

		expirationTimeout := time.Hour
		executionTimeout := 10 * time.Minute
		expiry := clock.Now(ctx).Add(expirationTimeout).UTC()

		enqueueTask := &internalspb.EnqueueRBETask{
			Payload: &internalspb.TaskPayload{
				ReservationId:  rbeReservation,
				TaskId:         "60b2ed0a43023110",
				TaskToRunShard: 14,
				TaskToRunId:    1,
				DebugInfo: &internalspb.TaskPayload_DebugInfo{
					PySwarmingVersion: "py-version",
				},
			},
			RbeInstance:      rbeInstance,
			Expiry:           timestamppb.New(expiry),
			ExecutionTimeout: durationpb.New(executionTimeout),
			RequestedBotId:   "some-bot-id",
			Constraints: []*internalspb.EnqueueRBETask_Constraint{
				{Key: "key1", AllowedValues: []string{"v1", "v2"}},
				{Key: "key2", AllowedValues: []string{"v3"}},
			},
			Priority:        123,
			WaitForCapacity: true,
		}

		taskReqKey, err := model.TaskIDToRequestKey(ctx, enqueueTask.Payload.TaskId)
		assert.NoErr(t, err)
		taskToRun := &model.TaskToRun{
			Key: model.TaskToRunKey(ctx, taskReqKey,
				enqueueTask.Payload.TaskToRunShard,
				enqueueTask.Payload.TaskToRunId,
			),
			Expiration: datastore.NewIndexedOptional(expiry),
		}
		assert.NoErr(t, datastore.Put(ctx, taskToRun))

		t.Run("handleEnqueueRBETask ok", func(t *ftt.Test) {
			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			assert.NoErr(t, err)

			expectedPayload, _ := anypb.New(&internalspb.TaskPayload{
				ReservationId:  rbeReservation,
				TaskId:         "60b2ed0a43023110",
				TaskToRunShard: 14,
				TaskToRunId:    1,
				DebugInfo: &internalspb.TaskPayload_DebugInfo{
					PySwarmingVersion: "py-version",
					GoSwarmingVersion: "go-version",
				},
			})

			assert.Loosely(t, rbe.reservation, should.Match(&remoteworkers.Reservation{
				Name:    fmt.Sprintf("%s/reservations/%s", rbeInstance, rbeReservation),
				State:   remoteworkers.ReservationState_RESERVATION_PENDING,
				Payload: expectedPayload,
				Constraints: []*remoteworkers.Constraint{
					{Key: "label:key1", AllowedValues: []string{"v1", "v2"}},
					{Key: "label:key2", AllowedValues: []string{"v3"}},
				},
				ExpireTime:       timestamppb.New(expiry.Add(executionTimeout)),
				QueuingTimeout:   durationpb.New(expirationTimeout),
				ExecutionTimeout: durationpb.New(executionTimeout),
				Priority:         123,
				RequestedBotId:   "some-bot-id",
				MatchTimeout:     durationpb.New(expirationTimeout),
			}))
		})

		t.Run("handleEnqueueRBETask TaskToRun is gone", func(t *ftt.Test) {
			assert.NoErr(t, datastore.Delete(ctx, datastore.KeyForObj(ctx, taskToRun)))

			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			assert.NoErr(t, err)

			// Didn't call RBE.
			assert.Loosely(t, rbe.reservation, should.BeNil)
		})

		t.Run("handleEnqueueRBETask TaskToRun is claimed", func(t *ftt.Test) {
			taskToRun.Expiration.Unset()
			assert.NoErr(t, datastore.Put(ctx, taskToRun))

			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			assert.NoErr(t, err)

			// Didn't call RBE.
			assert.Loosely(t, rbe.reservation, should.BeNil)
		})

		t.Run("handleEnqueueRBETask transient err", func(t *ftt.Test) {
			rbe.errCreate = status.Errorf(codes.Internal, "boom")
			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run("handleEnqueueRBETask already exists", func(t *ftt.Test) {
			rbe.errCreate = status.Errorf(codes.AlreadyExists, "boom")
			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			assert.NoErr(t, err)
		})

		t.Run("handleEnqueueRBETask fatal error", func(t *ftt.Test) {
			t.Run("expected error, report ok", func(t *ftt.Test) {
				rbe.errCreate = status.Errorf(codes.FailedPrecondition, "boom")
				internals.expireSlice = func(req *internalspb.ExpireSliceRequest) error {
					assert.Loosely(t, req, should.Match(&internalspb.ExpireSliceRequest{
						TaskId:         enqueueTask.Payload.TaskId,
						TaskToRunShard: enqueueTask.Payload.TaskToRunShard,
						TaskToRunId:    enqueueTask.Payload.TaskToRunId,
						Reason:         internalspb.ExpireSliceRequest_NO_RESOURCE,
						Details:        "rpc error: code = FailedPrecondition desc = boom",
					}))
					return nil
				}
				err := srv.handleEnqueueRBETask(ctx, enqueueTask)
				assert.Loosely(t, tq.Ignore.In(err), should.BeTrue)
			})

			t.Run("unexpected error, report ok", func(t *ftt.Test) {
				rbe.errCreate = status.Errorf(codes.PermissionDenied, "boom")
				internals.expireSlice = func(req *internalspb.ExpireSliceRequest) error {
					assert.Loosely(t, req, should.Match(&internalspb.ExpireSliceRequest{
						TaskId:         enqueueTask.Payload.TaskId,
						TaskToRunShard: enqueueTask.Payload.TaskToRunShard,
						TaskToRunId:    enqueueTask.Payload.TaskToRunId,
						Reason:         internalspb.ExpireSliceRequest_PERMISSION_DENIED,
						Details:        "rpc error: code = PermissionDenied desc = boom",
					}))
					return nil
				}
				err := srv.handleEnqueueRBETask(ctx, enqueueTask)
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})

			t.Run("expected, report failed", func(t *ftt.Test) {
				rbe.errCreate = status.Errorf(codes.FailedPrecondition, "boom")
				internals.expireSlice = func(_ *internalspb.ExpireSliceRequest) error {
					return status.Errorf(codes.InvalidArgument, "boom")
				}
				err := srv.handleEnqueueRBETask(ctx, enqueueTask)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, tq.Ignore.In(err), should.BeFalse)
				assert.Loosely(t, tq.Fatal.In(err), should.BeFalse)
			})
		})

		t.Run("handleCancelRBETask ok", func(t *ftt.Test) {
			err := srv.handleCancelRBETask(ctx, &internalspb.CancelRBETask{
				RbeInstance:   rbeInstance,
				ReservationId: rbeReservation,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, rbe.lastCancel, should.Match(&remoteworkers.CancelReservationRequest{
				Name:   fmt.Sprintf("%s/reservations/%s", rbeInstance, rbeReservation),
				Intent: remoteworkers.CancelReservationIntent_ANY,
			}))
		})

		t.Run("handleCancelRBETask not found", func(t *ftt.Test) {
			rbe.errCancel = status.Errorf(codes.NotFound, "boo")
			err := srv.handleCancelRBETask(ctx, &internalspb.CancelRBETask{
				RbeInstance:   rbeInstance,
				ReservationId: rbeReservation,
			})
			assert.Loosely(t, tq.Ignore.In(err), should.BeTrue)
		})

		t.Run("handleCancelRBETask internal", func(t *ftt.Test) {
			rbe.errCancel = status.Errorf(codes.Internal, "boo")
			err := srv.handleCancelRBETask(ctx, &internalspb.CancelRBETask{
				RbeInstance:   rbeInstance,
				ReservationId: rbeReservation,
			})
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run("ExpireSliceBasedOnReservation", func(t *ftt.Test) {
			const (
				reservationName = "projects/.../instances/.../reservations/..."
				taskSliceIndex  = 1
				taskID          = "637f8e221100aa10"
			)

			// This is checked when retrying the submission.
			reqKey, _ := model.TaskIDToRequestKey(ctx, taskID)
			req := &model.TaskRequest{
				Key:        reqKey,
				TaskSlices: make([]model.TaskSlice, taskSliceIndex+1), // empty, its fine for this test
			}
			assert.NoErr(t, datastore.Put(ctx, req))

			// Get the matching parameters of TaskToRun, they are derived from task ID
			// and the slice index.
			ttrKey, _ := model.TaskRequestToToRunKey(ctx, req, taskSliceIndex)
			taskToRunID := ttrKey.IntID()
			taskToRunShard := (&model.TaskToRun{Key: ttrKey}).MustShardIndex()

			var (
				expireSliceReason  internalspb.ExpireSliceRequest_Reason
				expireSliceDetails string
			)
			internals.expireSlice = func(r *internalspb.ExpireSliceRequest) error {
				assert.Loosely(t, r.TaskId, should.Equal(taskID))
				assert.Loosely(t, r.TaskToRunShard, should.Equal(taskToRunShard))
				assert.Loosely(t, r.TaskToRunId, should.Equal(taskToRunID))
				assert.Loosely(t, r.Reason, should.NotEqual(internalspb.ExpireSliceRequest_REASON_UNSPECIFIED))
				expireSliceReason = r.Reason
				expireSliceDetails = r.Details
				return nil
			}

			var enqueueNewErr error
			var resubmittedTTR []*model.TaskToRun
			srv.enqueueNew = func(_ context.Context, _ *tq.Dispatcher, tr *model.TaskRequest, ttr *model.TaskToRun, _ *cfg.Config) error {
				assert.That(t, tr.Key.Equal(ttr.Key.Parent()), should.BeTrue)
				if enqueueNewErr != nil {
					return enqueueNewErr
				}
				resubmittedTTR = append(resubmittedTTR, ttr)
				return nil
			}

			reservationID := func(retryCount int) string {
				return fmt.Sprintf("%s-%s-%d-%d", srv.serverProject, taskID, taskSliceIndex, retryCount)
			}

			prepTaskToRun := func(reapable bool, retryCount int) {
				var exp datastore.Optional[time.Time, datastore.Indexed]
				if reapable {
					exp.Set(testclock.TestRecentTimeUTC.Add(time.Hour))
				}
				taskReqKey, _ := model.TaskIDToRequestKey(ctx, taskID)
				assert.NoErr(t, datastore.Put(ctx, &model.TaskToRun{
					Key:            model.TaskToRunKey(ctx, taskReqKey, taskToRunShard, taskToRunID),
					Expiration:     exp,
					RetryCount:     int64(retryCount),
					RBEReservation: reservationID(retryCount),
				}))
			}

			prepReapableTaskToRun := func(retryCount int) { prepTaskToRun(true, retryCount) }
			prepClaimedTaskToRun := func() { prepTaskToRun(false, 0) }

			expireBasedOnReservation := func(state remoteworkers.ReservationState, reservationID string, statusErr error, result *internalspb.TaskResult) error {
				rbe.reservation = &remoteworkers.Reservation{
					Name:   reservationName,
					State:  state,
					Status: status.Convert(statusErr).Proto(),
				}

				taskPayload := &internalspb.TaskPayload{
					ReservationId:  reservationID,
					TaskId:         taskID,
					SliceIndex:     taskSliceIndex,
					TaskToRunShard: taskToRunShard,
					TaskToRunId:    taskToRunID,
				}

				if result != nil {
					rbe.reservation.Result, _ = anypb.New(result)
				}
				rbe.reservation.Payload, _ = anypb.New(taskPayload)
				expireSliceReason = internalspb.ExpireSliceRequest_REASON_UNSPECIFIED
				expireSliceDetails = ""
				return srv.expireSliceBasedOnReservation(ctx, reservationName)
			}

			expectNoExpireSlice := func() {
				assert.Loosely(t, expireSliceReason, should.Equal(internalspb.ExpireSliceRequest_REASON_UNSPECIFIED))
			}

			expectNoResubmit := func() {
				assert.Loosely(t, resubmittedTTR, should.HaveLength(0))
			}

			expectExpireSlice := func(r internalspb.ExpireSliceRequest_Reason, details string) {
				expectNoResubmit()
				assert.Loosely(t, expireSliceReason, should.Equal(r))
				assert.Loosely(t, expireSliceDetails, should.ContainSubstring(details))
			}

			expectResubmit := func(reservationID string) {
				expectNoExpireSlice()
				assert.Loosely(t, resubmittedTTR, should.HaveLength(1))
				assert.That(t, resubmittedTTR[0].RBEReservation, should.Equal(reservationID))
				resubmittedTTR = nil
			}

			t.Run("Still pending", func(t *ftt.Test) {
				prepReapableTaskToRun(0)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_PENDING,
					reservationID(0),
					nil,
					nil,
				))
				expectNoExpireSlice()
			})

			t.Run("Successful", func(t *ftt.Test) {
				prepClaimedTaskToRun()
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					nil,
					&internalspb.TaskResult{},
				))
				expectNoExpireSlice()
			})

			t.Run("Canceled #1", func(t *ftt.Test) {
				prepClaimedTaskToRun()
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.Canceled, "canceled"),
					nil,
				))
				expectNoExpireSlice()
			})

			t.Run("Canceled #2", func(t *ftt.Test) {
				prepClaimedTaskToRun()
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_CANCELLED,
					reservationID(0),
					nil,
					nil,
				))
				expectNoExpireSlice()
			})

			t.Run("Expired", func(t *ftt.Test) {
				prepReapableTaskToRun(0)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.DeadlineExceeded, "deadline"),
					nil,
				))
				expectExpireSlice(internalspb.ExpireSliceRequest_EXPIRED, "deadline")
			})

			t.Run("No resources", func(t *ftt.Test) {
				prepReapableTaskToRun(0)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.FailedPrecondition, "no bots"),
					nil,
				))
				expectExpireSlice(internalspb.ExpireSliceRequest_NO_RESOURCE, "no bots")
			})

			t.Run("Bot internal error, first attempt", func(t *ftt.Test) {
				prepReapableTaskToRun(0)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.DeadlineExceeded, "ignored"),
					&internalspb.TaskResult{
						BotInternalError: "boom",
					},
				))
				expectResubmit(reservationID(1))
			})

			t.Run("Bot internal error, last attempt", func(t *ftt.Test) {
				prepReapableTaskToRun(maxReservationRetryCount)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(maxReservationRetryCount),
					status.Errorf(codes.DeadlineExceeded, "ignored"),
					&internalspb.TaskResult{BotInternalError: "boom"},
				))
				expectExpireSlice(internalspb.ExpireSliceRequest_BOT_INTERNAL_ERROR, "boom")
			})

			t.Run("Aborted before claimed, first attempt", func(t *ftt.Test) {
				prepReapableTaskToRun(0)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.Aborted, "bot died"),
					nil,
				))
				expectResubmit(reservationID(1))
			})

			t.Run("Aborted before claimed, last attempt", func(t *ftt.Test) {
				prepReapableTaskToRun(maxReservationRetryCount)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(maxReservationRetryCount),
					status.Errorf(codes.Aborted, "bot died"),
					nil,
				))
				expectExpireSlice(internalspb.ExpireSliceRequest_BOT_INTERNAL_ERROR, "bot died")
			})

			t.Run("Unexpectedly successful reservations, first attempt", func(t *ftt.Test) {
				prepReapableTaskToRun(0)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					nil,
					nil,
				))
				expectResubmit(reservationID(1))
			})

			t.Run("Unexpectedly successful reservations, last attempt", func(t *ftt.Test) {
				prepReapableTaskToRun(maxReservationRetryCount)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(maxReservationRetryCount),
					nil,
					nil,
				))
				expectExpireSlice(internalspb.ExpireSliceRequest_BOT_INTERNAL_ERROR, "unexpectedly finished")
			})

			t.Run("Unexpectedly canceled reservations", func(t *ftt.Test) {
				prepReapableTaskToRun(0)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.Canceled, "ignored"),
					nil,
				))
				expectNoExpireSlice()
			})

			t.Run("Skips already claimed TaskToRun", func(t *ftt.Test) {
				prepClaimedTaskToRun()
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.FailedPrecondition, "no bots"),
					nil,
				))
				expectNoExpireSlice()
			})

			t.Run("Skips missing TaskToRun", func(t *ftt.Test) {
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.FailedPrecondition, "no bots"),
					nil,
				))
				expectNoExpireSlice()
			})

			t.Run("Skips stale notification", func(t *ftt.Test) {
				prepReapableTaskToRun(1)
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0), // stale attempt
					status.Errorf(codes.DeadlineExceeded, "deadline"),
					nil,
				))
				expectNoExpireSlice()
			})

			t.Run("No longer valid reservation", func(t *ftt.Test) {
				prepReapableTaskToRun(0)
				enqueueNewErr = errors.Annotate(ErrBadReservation, "boom").Err()
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.DeadlineExceeded, "ignored"),
					&internalspb.TaskResult{
						BotInternalError: "boom",
					},
				))
				expectExpireSlice(internalspb.ExpireSliceRequest_EXPIRED, "boom: the reservation cannot be submitted")
			})

			t.Run("Transient resubmit error", func(t *ftt.Test) {
				prepReapableTaskToRun(0)

				// First attempt.
				enqueueNewErr = errors.New("boom")
				err := expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.DeadlineExceeded, "ignored"),
					&internalspb.TaskResult{
						BotInternalError: "boom",
					},
				)
				assert.That(t, transient.Tag.In(err), should.BeTrue)

				// Retry succeeds.
				enqueueNewErr = nil
				assert.NoErr(t, expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					reservationID(0),
					status.Errorf(codes.DeadlineExceeded, "ignored"),
					&internalspb.TaskResult{
						BotInternalError: "boom",
					},
				))

				expectResubmit(reservationID(1))
			})
		})
	})
}

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

		rbe := mockedReservationClient{
			newState: remoteworkers.ReservationState_RESERVATION_PENDING,
		}
		internals := mockedInternalsClient{}
		srv := ReservationServer{
			rbe:           &rbe,
			internals:     &internals,
			serverVersion: "go-version",
			cfg:           p,
		}
		srv.RegisterTQTasks(tqt.Tasks)

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
				return EnqueueCancel(ctx, tqt.TQ, tr, ttr)
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

		t.Run("EnqueueNew", func(t *ftt.Test) {
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
					return EnqueueNew(ctx, tqt.TQ, tr, ttr, p.Cached(ctx))
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
					return EnqueueNew(ctx, tqt.TQ, tr, ttr, p.Cached(ctx))
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
					return EnqueueNew(ctx, tqt.TQ, tr, ttr, p.Cached(ctx))
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
					return EnqueueNew(ctx, tqt.TQ, tr, ttr, p.Cached(ctx))
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

type mockedReservationClient struct {
	lastCreate *remoteworkers.CreateReservationRequest
	lastGet    *remoteworkers.GetReservationRequest
	lastCancel *remoteworkers.CancelReservationRequest

	errCreate error
	errGet    error
	errCancel error

	newState    remoteworkers.ReservationState
	reservation *remoteworkers.Reservation
}

func (m *mockedReservationClient) CreateReservation(ctx context.Context, in *remoteworkers.CreateReservationRequest, opts ...grpc.CallOption) (*remoteworkers.Reservation, error) {
	m.lastCreate = in
	m.reservation = proto.Clone(in.Reservation).(*remoteworkers.Reservation)
	m.reservation.State = m.newState
	if m.errCreate != nil {
		return nil, m.errCreate
	}
	return m.reservation, nil
}

func (m *mockedReservationClient) GetReservation(ctx context.Context, in *remoteworkers.GetReservationRequest, opts ...grpc.CallOption) (*remoteworkers.Reservation, error) {
	m.lastGet = in
	if m.errGet != nil {
		return nil, m.errGet
	}
	return m.reservation, nil
}

func (m *mockedReservationClient) CancelReservation(ctx context.Context, in *remoteworkers.CancelReservationRequest, opts ...grpc.CallOption) (*remoteworkers.CancelReservationResponse, error) {
	m.lastCancel = in
	if m.errCancel != nil {
		return nil, m.errCancel
	}
	return &remoteworkers.CancelReservationResponse{}, nil
}

type mockedInternalsClient struct {
	expireSlice func(*internalspb.ExpireSliceRequest) error
}

func (m *mockedInternalsClient) ExpireSlice(ctx context.Context, in *internalspb.ExpireSliceRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.expireSlice == nil {
		panic("must not be called")
	}
	if err := m.expireSlice(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
