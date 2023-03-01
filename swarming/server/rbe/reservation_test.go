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
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

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
		ctx := memory.Use(context.Background())
		rbe := mockedReservationClient{
			newState: remoteworkers.ReservationState_RESERVATION_PENDING,
		}
		internals := mockedInternalsClient{}
		srv := ReservationServer{
			rbe:           &rbe,
			internals:     &internals,
			serverVersion: "go-version",
		}

		expiry := testclock.TestRecentTimeUTC

		enqueueTask := &internalspb.EnqueueRBETask{
			Payload: &internalspb.TaskPayload{
				ReservationId:  "reservation-id",
				TaskId:         "60b2ed0a43023110",
				TaskToRunShard: 14,
				TaskToRunId:    1,
				DebugInfo: &internalspb.TaskPayload_DebugInfo{
					PySwarmingVersion: "py-version",
				},
			},
			RbeInstance:    "projects/x/instances/y",
			Expiry:         timestamppb.New(expiry),
			RequestedBotId: "some-bot-id",
			Constraints: []*internalspb.EnqueueRBETask_Constraint{
				{Key: "key1", AllowedValues: []string{"v1", "v2"}},
				{Key: "key2", AllowedValues: []string{"v3"}},
			},
			Priority: 123,
		}

		taskReqKey, err := model.TaskRequestKey(ctx, enqueueTask.Payload.TaskId)
		So(err, ShouldBeNil)
		taskToRun := &model.TaskToRun{
			Kind:       model.TaskToRunKind(enqueueTask.Payload.TaskToRunShard),
			ID:         enqueueTask.Payload.TaskToRunId,
			Parent:     taskReqKey,
			Expiration: testclock.TestRecentTimeUTC,
		}
		So(datastore.Put(ctx, taskToRun), ShouldBeNil)

		Convey("handleEnqueueRBETask ok", func() {
			err := srv.handleEnqueueRBETask(ctx, enqueueTask)
			So(err, ShouldBeNil)

			expectedPayload, _ := anypb.New(&internalspb.TaskPayload{
				ReservationId:  "reservation-id",
				TaskId:         "60b2ed0a43023110",
				TaskToRunShard: 14,
				TaskToRunId:    1,
				DebugInfo: &internalspb.TaskPayload_DebugInfo{
					PySwarmingVersion: "py-version",
					GoSwarmingVersion: "go-version",
				},
			})

			So(rbe.reservation, ShouldResembleProto, &remoteworkers.Reservation{
				Name:    "projects/x/instances/y/reservations/reservation-id",
				State:   remoteworkers.ReservationState_RESERVATION_PENDING,
				Payload: expectedPayload,
				Constraints: []*remoteworkers.Constraint{
					{Key: "label:key1", AllowedValues: []string{"v1", "v2"}},
					{Key: "label:key2", AllowedValues: []string{"v3"}},
				},
				ExpireTime:     timestamppb.New(expiry),
				Priority:       123,
				RequestedBotId: "some-bot-id",
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
			taskToRun.Expiration = time.Time{}
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
	})
}

type mockedReservationClient struct {
	lastCreate *remoteworkers.CreateReservationRequest
	lastGet    *remoteworkers.GetReservationRequest

	errCreate error
	errGet    error

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
	panic("unused yet")
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
