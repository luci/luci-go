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
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/swarming/internal/remoteworkers"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestReservationServer(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		const rbeInstance = "projects/x/instances/y"
		const rbeReservation = "reservation-id"

		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		rbe := mockedReservationClient{
			newState: remoteworkers.ReservationState_RESERVATION_PENDING,
		}
		internals := mockedInternalsClient{}
		srv := ReservationServer{
			rbe:           &rbe,
			internals:     &internals,
			serverVersion: "go-version",
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
			Priority: 123,
		}

		taskReqKey, err := model.UnpackTaskRequestKey(ctx, enqueueTask.Payload.TaskId)
		So(err, ShouldBeNil)
		taskToRun := &model.TaskToRun{
			Key: model.TaskToRunKey(ctx, taskReqKey,
				enqueueTask.Payload.TaskToRunShard,
				enqueueTask.Payload.TaskToRunId,
			),
			Expiration: datastore.NewIndexedOptional(expiry),
		}
		So(datastore.Put(ctx, taskToRun), ShouldBeNil)

		Convey("handleEnqueueRBETask ok", func() {
			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			So(err, ShouldBeNil)

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

			So(rbe.reservation, ShouldResembleProto, &remoteworkers.Reservation{
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
			})
		})

		Convey("handleEnqueueRBETask TaskToRun is gone", func() {
			So(datastore.Delete(ctx, datastore.KeyForObj(ctx, taskToRun)), ShouldBeNil)

			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			So(err, ShouldBeNil)

			// Didn't call RBE.
			So(rbe.reservation, ShouldBeNil)
		})

		Convey("handleEnqueueRBETask TaskToRun is claimed", func() {
			taskToRun.Expiration.Unset()
			So(datastore.Put(ctx, taskToRun), ShouldBeNil)

			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			So(err, ShouldBeNil)

			// Didn't call RBE.
			So(rbe.reservation, ShouldBeNil)
		})

		Convey("handleEnqueueRBETask transient err", func() {
			rbe.errCreate = status.Errorf(codes.Internal, "boom")
			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			So(err, ShouldNotBeNil)
			So(transient.Tag.In(err), ShouldBeTrue)
		})

		Convey("handleEnqueueRBETask already exists", func() {
			rbe.errCreate = status.Errorf(codes.AlreadyExists, "boom")
			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			So(err, ShouldBeNil)
		})

		Convey("handleEnqueueRBETask fatal error", func() {
			Convey("expected error, report ok", func() {
				rbe.errCreate = status.Errorf(codes.FailedPrecondition, "boom")
				internals.expireSlice = func(req *internalspb.ExpireSliceRequest) error {
					So(req, ShouldResembleProto, &internalspb.ExpireSliceRequest{
						TaskId:         enqueueTask.Payload.TaskId,
						TaskToRunShard: enqueueTask.Payload.TaskToRunShard,
						TaskToRunId:    enqueueTask.Payload.TaskToRunId,
						Reason:         internalspb.ExpireSliceRequest_NO_RESOURCE,
						Details:        "rpc error: code = FailedPrecondition desc = boom",
					})
					return nil
				}
				err := srv.handleEnqueueRBETask(ctx, enqueueTask)
				So(tq.Ignore.In(err), ShouldBeTrue)
			})

			Convey("unexpected error, report ok", func() {
				rbe.errCreate = status.Errorf(codes.PermissionDenied, "boom")
				internals.expireSlice = func(req *internalspb.ExpireSliceRequest) error {
					So(req, ShouldResembleProto, &internalspb.ExpireSliceRequest{
						TaskId:         enqueueTask.Payload.TaskId,
						TaskToRunShard: enqueueTask.Payload.TaskToRunShard,
						TaskToRunId:    enqueueTask.Payload.TaskToRunId,
						Reason:         internalspb.ExpireSliceRequest_PERMISSION_DENIED,
						Details:        "rpc error: code = PermissionDenied desc = boom",
					})
					return nil
				}
				err := srv.handleEnqueueRBETask(ctx, enqueueTask)
				So(tq.Fatal.In(err), ShouldBeTrue)
			})

			Convey("expected, report failed", func() {
				rbe.errCreate = status.Errorf(codes.FailedPrecondition, "boom")
				internals.expireSlice = func(_ *internalspb.ExpireSliceRequest) error {
					return status.Errorf(codes.InvalidArgument, "boom")
				}
				err := srv.handleEnqueueRBETask(ctx, enqueueTask)
				So(err, ShouldNotBeNil)
				So(tq.Ignore.In(err), ShouldBeFalse)
				So(tq.Fatal.In(err), ShouldBeFalse)
			})
		})

		Convey("handleCancelRBETask ok", func() {
			err := srv.handleCancelRBETask(ctx, &internalspb.CancelRBETask{
				RbeInstance:   rbeInstance,
				ReservationId: rbeReservation,
			})
			So(err, ShouldBeNil)
			So(rbe.lastCancel, ShouldResembleProto, &remoteworkers.CancelReservationRequest{
				Name:   fmt.Sprintf("%s/reservations/%s", rbeInstance, rbeReservation),
				Intent: remoteworkers.CancelReservationIntent_ANY,
			})
		})

		Convey("handleCancelRBETask not found", func() {
			rbe.errCancel = status.Errorf(codes.NotFound, "boo")
			err := srv.handleCancelRBETask(ctx, &internalspb.CancelRBETask{
				RbeInstance:   rbeInstance,
				ReservationId: rbeReservation,
			})
			So(tq.Ignore.In(err), ShouldBeTrue)
		})

		Convey("handleCancelRBETask internal", func() {
			rbe.errCancel = status.Errorf(codes.Internal, "boo")
			err := srv.handleCancelRBETask(ctx, &internalspb.CancelRBETask{
				RbeInstance:   rbeInstance,
				ReservationId: rbeReservation,
			})
			So(transient.Tag.In(err), ShouldBeTrue)
		})

		Convey("ExpireSliceBasedOnReservation", func() {
			const (
				reservationName = "projects/.../instances/.../reservations/..."
				taskSliceIndex  = 1
				taskToRunShard  = 5
				taskToRunID     = 678
				taskID          = "637f8e221100aa10"
			)

			var (
				expireSliceReason  internalspb.ExpireSliceRequest_Reason
				expireSliceDetails string
			)
			internals.expireSlice = func(r *internalspb.ExpireSliceRequest) error {
				So(r.TaskId, ShouldEqual, taskID)
				So(r.TaskToRunShard, ShouldEqual, taskToRunShard)
				So(r.TaskToRunId, ShouldEqual, taskToRunID)
				So(r.Reason, ShouldNotEqual, internalspb.ExpireSliceRequest_REASON_UNSPECIFIED)
				expireSliceReason = r.Reason
				expireSliceDetails = r.Details
				return nil
			}

			prepTaskToRun := func(reapable bool) {
				var exp datastore.Optional[time.Time, datastore.Indexed]
				if reapable {
					exp.Set(testclock.TestRecentTimeUTC.Add(time.Hour))
				}
				taskReqKey, _ := model.UnpackTaskRequestKey(ctx, taskID)
				So(datastore.Put(ctx, &model.TaskToRun{
					Key:        model.TaskToRunKey(ctx, taskReqKey, taskToRunShard, taskToRunID),
					Expiration: exp,
				}), ShouldBeNil)
			}

			prepReapableTaskToRun := func() { prepTaskToRun(true) }
			prepClaimedTaskToRun := func() { prepTaskToRun(false) }

			expireBasedOnReservation := func(state remoteworkers.ReservationState, statusErr error, result *internalspb.TaskResult) {
				rbe.reservation = &remoteworkers.Reservation{
					Name:   reservationName,
					State:  state,
					Status: status.Convert(statusErr).Proto(),
				}
				rbe.reservation.Payload, _ = anypb.New(&internalspb.TaskPayload{
					ReservationId:  "",
					TaskId:         taskID,
					SliceIndex:     taskSliceIndex,
					TaskToRunShard: taskToRunShard,
					TaskToRunId:    taskToRunID,
				})
				if result != nil {
					rbe.reservation.Result, _ = anypb.New(result)
				}
				expireSliceReason = internalspb.ExpireSliceRequest_REASON_UNSPECIFIED
				expireSliceDetails = ""
				So(srv.ExpireSliceBasedOnReservation(ctx, reservationName), ShouldBeNil)
			}

			expectNoExpireSlice := func() {
				So(expireSliceReason, ShouldEqual, internalspb.ExpireSliceRequest_REASON_UNSPECIFIED)
			}

			expectExpireSlice := func(r internalspb.ExpireSliceRequest_Reason, details string) {
				So(expireSliceReason, ShouldEqual, r)
				So(expireSliceDetails, ShouldContainSubstring, details)
			}

			Convey("Still pending", func() {
				prepReapableTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_PENDING,
					nil,
					nil,
				)
				expectNoExpireSlice()
			})

			Convey("Successful", func() {
				prepClaimedTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					nil,
					&internalspb.TaskResult{},
				)
				expectNoExpireSlice()
			})

			Convey("Canceled #1", func() {
				prepClaimedTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					status.Errorf(codes.Canceled, "canceled"),
					nil,
				)
				expectNoExpireSlice()
			})

			Convey("Canceled #2", func() {
				prepClaimedTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_CANCELLED,
					nil,
					nil,
				)
				expectNoExpireSlice()
			})

			Convey("Expired", func() {
				prepReapableTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					status.Errorf(codes.DeadlineExceeded, "deadline"),
					nil,
				)
				expectExpireSlice(internalspb.ExpireSliceRequest_EXPIRED, "deadline")
			})

			Convey("No resources", func() {
				prepReapableTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					status.Errorf(codes.FailedPrecondition, "no bots"),
					nil,
				)
				expectExpireSlice(internalspb.ExpireSliceRequest_NO_RESOURCE, "no bots")
			})

			Convey("Bot internal error", func() {
				prepReapableTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					status.Errorf(codes.DeadlineExceeded, "ignored"),
					&internalspb.TaskResult{BotInternalError: "boom"},
				)
				expectExpireSlice(internalspb.ExpireSliceRequest_BOT_INTERNAL_ERROR, "boom")
			})

			Convey("Aborted before claimed", func() {
				prepReapableTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					status.Errorf(codes.Aborted, "bot died"),
					nil,
				)
				expectExpireSlice(internalspb.ExpireSliceRequest_BOT_INTERNAL_ERROR, "bot died")
			})

			Convey("Unexpectedly successful reservations", func() {
				prepReapableTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					nil,
					nil,
				)
				expectExpireSlice(internalspb.ExpireSliceRequest_BOT_INTERNAL_ERROR, "unexpectedly finished")
			})

			Convey("Unexpectedly canceled reservations", func() {
				prepReapableTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					status.Errorf(codes.Canceled, "ignored"),
					nil,
				)
				expectNoExpireSlice()
			})

			Convey("Skips already claimed TaskToRun", func() {
				prepClaimedTaskToRun()
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					status.Errorf(codes.FailedPrecondition, "no bots"),
					nil,
				)
				expectNoExpireSlice()
			})

			Convey("Skips missing TaskToRun", func() {
				expireBasedOnReservation(
					remoteworkers.ReservationState_RESERVATION_COMPLETED,
					status.Errorf(codes.FailedPrecondition, "no bots"),
					nil,
				)
				expectNoExpireSlice()
			})
		})
	})
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
