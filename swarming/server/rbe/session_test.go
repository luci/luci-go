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

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/hmactoken"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSessionServer(t *testing.T) {
	t.Parallel()

	Convey("With server", t, func() {
		const (
			fakeRBEInstance   = "fake-rbe-instance"
			fakeBotID         = "fake-bot-id"
			fakeSessionID     = "fake-rbe-session"
			fakeFirstLeaseID  = "fake-first-lease-id"
			fakeSecondLeaseID = "fake-second-lease-id"
			fakeFirstTaskID   = "fake-first-task-id"
			fakeSecondTaskID  = "fake-second-task-id"
		)

		fakePollState := &internalspb.PollState{
			Id:          "fake-poll-token",
			RbeInstance: fakeRBEInstance,
			IpAllowlist: "fake-ip-allowlist",
		}

		fakeRequest := &botsrv.Request{
			BotID:     fakeBotID,
			SessionID: fakeSessionID,
			PollState: fakePollState,
			Dimensions: map[string][]string{
				"id":     {fakeBotID},
				"pool":   {"some-pool"},
				"extra1": {"a", "b"},
				"extra2": {"c", "d"},
			},
		}

		payload := func(taskID string) *anypb.Any {
			msg, err := anypb.New(&internalspb.TaskPayload{TaskId: taskID})
			So(err, ShouldBeNil)
			return msg
		}

		result := func() *anypb.Any {
			msg, err := anypb.New(&internalspb.TaskResult{})
			So(err, ShouldBeNil)
			return msg
		}

		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, now)

		rbe := &mockedBotsClient{}

		srv := &SessionServer{
			rbe: rbe,
			hmacSecret: hmactoken.NewStaticSecret(secrets.Secret{
				Active: []byte("secret"),
			}),
		}

		Convey("CreateBotSession works", func() {
			rbe.expectCreateBotSession(func(r *remoteworkers.CreateBotSessionRequest) (*remoteworkers.BotSession, error) {
				So(r, ShouldResembleProto, &remoteworkers.CreateBotSessionRequest{
					Parent: fakeRBEInstance,
					BotSession: &remoteworkers.BotSession{
						BotId:   fakeBotID,
						Status:  remoteworkers.BotStatus_INITIALIZING,
						Version: "bot-version",
						Worker: &remoteworkers.Worker{
							Properties: []*remoteworkers.Worker_Property{
								{Key: "rbePoolID", Value: "rbe-pool-id"},
								{Key: "rbePoolVersion", Value: "rbe-pool-version"},
							},
							Devices: []*remoteworkers.Device{
								{
									Handle: "primary",
									Properties: []*remoteworkers.Device_Property{
										{Key: "label:extra1", Value: "a"},
										{Key: "label:extra1", Value: "b"},
										{Key: "label:extra2", Value: "c"},
										{Key: "label:extra2", Value: "d"},
										{Key: "label:pool", Value: "some-pool"},
									},
								},
							},
						},
					},
				})
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_INITIALIZING,
				}, nil
			})

			resp, err := srv.CreateBotSession(ctx, &CreateBotSessionRequest{
				Dimensions: map[string][]string{
					"ignored": {""}, // uses validated botsrv.Request.Dimensions instead
				},
				BotVersion: "bot-version",
				WorkerProperties: &WorkerProperties{
					PoolID:      "rbe-pool-id",
					PoolVersion: "rbe-pool-version",
				},
			}, fakeRequest)

			So(err, ShouldBeNil)

			expectedExpiry := now.Add(sessionTokenExpiry).Round(time.Second)

			msg := resp.(*CreateBotSessionResponse)
			So(msg.SessionExpiry, ShouldEqual, expectedExpiry.Unix())
			So(msg.SessionID, ShouldEqual, fakeSessionID)

			session := &internalspb.BotSession{}
			So(srv.hmacSecret.ValidateToken(msg.SessionToken, session), ShouldBeNil)
			So(session, ShouldResembleProto, &internalspb.BotSession{
				RbeBotSessionId: fakeSessionID,
				PollState:       fakePollState,
				Expiry:          timestamppb.New(expectedExpiry),
			})
		})

		Convey("CreateBotSession propagates RBE error", func() {
			rbe.expectCreateBotSession(func(r *remoteworkers.CreateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return nil, status.Errorf(codes.FailedPrecondition, "boom")
			})
			_, err := srv.CreateBotSession(ctx, &CreateBotSessionRequest{}, fakeRequest)
			So(err, ShouldHaveGRPCStatus, codes.FailedPrecondition)
			So(err, ShouldErrLike, "boom")
		})

		Convey("UpdateBotSession IDLE", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				So(r, ShouldResembleProto, &remoteworkers.UpdateBotSessionRequest{
					Name: fakeSessionID,
					BotSession: &remoteworkers.BotSession{
						Name:    fakeSessionID,
						BotId:   fakeBotID,
						Status:  remoteworkers.BotStatus_OK,
						Version: "bot-version",
						Worker: &remoteworkers.Worker{
							Properties: []*remoteworkers.Worker_Property{
								{Key: "rbePoolID", Value: "rbe-pool-id"},
								{Key: "rbePoolVersion", Value: "rbe-pool-version"},
							},
							Devices: []*remoteworkers.Device{
								{
									Handle: "primary",
									Properties: []*remoteworkers.Device_Property{
										{Key: "label:extra1", Value: "a"},
										{Key: "label:extra1", Value: "b"},
										{Key: "label:extra2", Value: "c"},
										{Key: "label:extra2", Value: "d"},
										{Key: "label:pool", Value: "some-pool"},
									},
								},
							},
						},
					},
				})
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Dimensions: map[string][]string{
					"ignored": {""}, // uses validated botsrv.Request.Dimensions instead
				},
				BotVersion: "bot-version",
				WorkerProperties: &WorkerProperties{
					PoolID:      "rbe-pool-id",
					PoolVersion: "rbe-pool-version",
				},
			}, fakeRequest)

			So(err, ShouldBeNil)

			expectedExpiry := now.Add(sessionTokenExpiry).Round(time.Second)

			msg := resp.(*UpdateBotSessionResponse)
			So(msg.Status, ShouldEqual, "OK")
			So(msg.SessionExpiry, ShouldEqual, expectedExpiry.Unix())
			So(msg.Lease, ShouldBeNil)

			session := &internalspb.BotSession{}
			So(srv.hmacSecret.ValidateToken(msg.SessionToken, session), ShouldBeNil)
			So(session, ShouldResembleProto, &internalspb.BotSession{
				RbeBotSessionId: fakeSessionID,
				PollState:       fakePollState,
				Expiry:          timestamppb.New(expectedExpiry),
			})
		})

		Convey("UpdateBotSession IDLE + DEADLINE_EXCEEDED", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return nil, status.Errorf(codes.DeadlineExceeded, "boom")
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, fakeRequest)

			So(err, ShouldBeNil)

			expectedExpiry := now.Add(sessionTokenExpiry).Round(time.Second)

			msg := resp.(*UpdateBotSessionResponse)
			So(msg.Status, ShouldEqual, "OK")
			So(msg.SessionExpiry, ShouldEqual, expectedExpiry.Unix())
			So(msg.Lease, ShouldBeNil)
		})

		Convey("UpdateBotSession TERMINATING", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				So(r.BotSession.Status, ShouldEqual, remoteworkers.BotStatus_BOT_TERMINATING)
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
					Leases: []*remoteworkers.Lease{
						// Will be ignored.
						{
							Id:      fakeFirstLeaseID,
							Payload: payload(fakeFirstTaskID),
							State:   remoteworkers.LeaseState_PENDING,
						},
					},
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "BOT_TERMINATING",
			}, fakeRequest)

			So(err, ShouldBeNil)

			msg := resp.(*UpdateBotSessionResponse)
			So(msg.Status, ShouldEqual, "OK")
			So(msg.Lease, ShouldBeNil)
		})

		Convey("UpdateBotSession TERMINATING by RBE", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				So(r.BotSession.Status, ShouldEqual, remoteworkers.BotStatus_OK)
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_BOT_TERMINATING,
					Leases: []*remoteworkers.Lease{
						// Will be ignored.
						{
							Id:      fakeFirstLeaseID,
							Payload: payload(fakeFirstTaskID),
							State:   remoteworkers.LeaseState_PENDING,
						},
					},
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, fakeRequest)

			So(err, ShouldBeNil)

			msg := resp.(*UpdateBotSessionResponse)
			So(msg.Status, ShouldEqual, "BOT_TERMINATING")
			So(msg.Lease, ShouldBeNil)
		})

		Convey("UpdateBotSession IDLE => PENDING", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				So(r.BotSession.Status, ShouldEqual, remoteworkers.BotStatus_OK)
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
					Leases: []*remoteworkers.Lease{
						{
							Id:      fakeFirstLeaseID,
							Payload: payload(fakeFirstTaskID),
							State:   remoteworkers.LeaseState_PENDING,
						},
						// Will be ignored.
						{
							Id:      fakeSecondLeaseID,
							Payload: payload(fakeSecondTaskID),
							State:   remoteworkers.LeaseState_PENDING,
						},
					},
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, fakeRequest)

			So(err, ShouldBeNil)

			lease := resp.(*UpdateBotSessionResponse).Lease
			So(lease.ID, ShouldEqual, fakeFirstLeaseID)
			So(lease.State, ShouldEqual, "PENDING")
			So(lease.Payload, ShouldResembleProto, &internalspb.TaskPayload{
				TaskId: fakeFirstTaskID,
			})
		})

		Convey("UpdateBotSession ACTIVE", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				So(r.BotSession.Status, ShouldEqual, remoteworkers.BotStatus_OK)
				So(r.BotSession.Leases, ShouldResembleProto, []*remoteworkers.Lease{
					{
						Id:    fakeFirstLeaseID,
						State: remoteworkers.LeaseState_ACTIVE,
					},
				})
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
					Leases: []*remoteworkers.Lease{
						{
							Id:    fakeFirstLeaseID,
							State: remoteworkers.LeaseState_ACTIVE,
						},
						// Will be ignored.
						{
							Id:      fakeSecondLeaseID,
							Payload: payload(fakeSecondTaskID),
							State:   remoteworkers.LeaseState_PENDING,
						},
					},
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease: &Lease{
					ID:    fakeFirstLeaseID,
					State: "ACTIVE",
				},
			}, fakeRequest)

			So(err, ShouldBeNil)

			lease := resp.(*UpdateBotSessionResponse).Lease
			So(lease.ID, ShouldEqual, fakeFirstLeaseID)
			So(lease.State, ShouldEqual, "ACTIVE")
			So(lease.Payload, ShouldBeNil)
		})

		Convey("UpdateBotSession ACTIVE => CANCELLED", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				So(r.BotSession.Status, ShouldEqual, remoteworkers.BotStatus_OK)
				So(r.BotSession.Leases, ShouldResembleProto, []*remoteworkers.Lease{
					{
						Id:    fakeFirstLeaseID,
						State: remoteworkers.LeaseState_ACTIVE,
					},
				})
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
					Leases: []*remoteworkers.Lease{
						{
							Id:    fakeFirstLeaseID,
							State: remoteworkers.LeaseState_CANCELLED,
						},
						// Will be ignored.
						{
							Id:      fakeSecondLeaseID,
							Payload: payload(fakeSecondTaskID),
							State:   remoteworkers.LeaseState_PENDING,
						},
					},
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease: &Lease{
					ID:    fakeFirstLeaseID,
					State: "ACTIVE",
				},
			}, fakeRequest)

			So(err, ShouldBeNil)

			lease := resp.(*UpdateBotSessionResponse).Lease
			So(lease.ID, ShouldEqual, fakeFirstLeaseID)
			So(lease.State, ShouldEqual, "CANCELLED")
			So(lease.Payload, ShouldBeNil)
		})

		Convey("UpdateBotSession ACTIVE => IDLE", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				So(r.BotSession.Status, ShouldEqual, remoteworkers.BotStatus_OK)
				So(r.BotSession.Leases, ShouldResembleProto, []*remoteworkers.Lease{
					{
						Id:     fakeFirstLeaseID,
						State:  remoteworkers.LeaseState_COMPLETED,
						Status: &statuspb.Status{},
						Result: result(),
					},
				})
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease: &Lease{
					ID:     fakeFirstLeaseID,
					State:  "COMPLETED",
					Result: &internalspb.TaskResult{},
				},
			}, fakeRequest)

			So(err, ShouldBeNil)
			So(resp.(*UpdateBotSessionResponse).Lease, ShouldBeNil)
		})

		Convey("UpdateBotSession ACTIVE => PENDING", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				So(r.BotSession.Status, ShouldEqual, remoteworkers.BotStatus_OK)
				So(r.BotSession.Leases, ShouldResembleProto, []*remoteworkers.Lease{
					{
						Id:     fakeFirstLeaseID,
						State:  remoteworkers.LeaseState_COMPLETED,
						Status: &statuspb.Status{},
						Result: result(),
					},
				})
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
					Leases: []*remoteworkers.Lease{
						{
							Id:      fakeSecondLeaseID,
							Payload: payload(fakeSecondTaskID),
							State:   remoteworkers.LeaseState_PENDING,
						},
					},
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease: &Lease{
					ID:     fakeFirstLeaseID,
					State:  "COMPLETED",
					Result: &internalspb.TaskResult{},
				},
			}, fakeRequest)

			So(err, ShouldBeNil)

			lease := resp.(*UpdateBotSessionResponse).Lease
			So(lease.ID, ShouldEqual, fakeSecondLeaseID)
			So(lease.State, ShouldEqual, "PENDING")
			So(lease.Payload, ShouldResembleProto, &internalspb.TaskPayload{
				TaskId: fakeSecondTaskID,
			})
		})

		Convey("UpdateBotSession no session ID", func() {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, &botsrv.Request{})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "missing session ID")
		})

		Convey("UpdateBotSession expired session token", func() {
			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, &botsrv.Request{
				SessionTokenExpired: true,
			})
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &UpdateBotSessionResponse{
				Status: "BOT_TERMINATING",
			})
		})

		Convey("UpdateBotSession propagates RBE error", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return nil, status.Errorf(codes.FailedPrecondition, "boom")
			})
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, fakeRequest)
			So(err, ShouldHaveGRPCStatus, codes.FailedPrecondition)
			So(err, ShouldErrLike, "boom")
		})

		Convey("UpdateBotSession bad session status", func() {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "huh",
			}, fakeRequest)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "unrecognized session status")
		})

		Convey("UpdateBotSession missing session status", func() {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{}, fakeRequest)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "missing session status")
		})

		Convey("UpdateBotSession bad lease state", func() {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease:  &Lease{State: "huh"},
			}, fakeRequest)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "unrecognized lease state")
		})

		Convey("UpdateBotSession missing lease state", func() {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease:  &Lease{},
			}, fakeRequest)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "missing lease state")
		})

		Convey("UpdateBotSession unexpected lease state", func() {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease:  &Lease{State: "CANCELLED"},
			}, fakeRequest)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "unexpected lease state")
		})

		Convey("UpdateBotSession ACTIVE lease disappears", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
					Leases: []*remoteworkers.Lease{
						// Will be ignored.
						{
							Id:      fakeSecondLeaseID,
							Payload: payload(fakeSecondTaskID),
							State:   remoteworkers.LeaseState_PENDING,
						},
					},
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease: &Lease{
					ID:    fakeFirstLeaseID,
					State: "ACTIVE",
				},
			}, fakeRequest)

			So(err, ShouldBeNil)
			So(resp.(*UpdateBotSessionResponse).Lease, ShouldBeNil)
		})

		Convey("UpdateBotSession unexpected ACTIVE lease transition", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
					Leases: []*remoteworkers.Lease{
						{
							Id:    fakeFirstLeaseID,
							State: remoteworkers.LeaseState_PENDING,
						},
					},
				}, nil
			})

			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease: &Lease{
					ID:    fakeFirstLeaseID,
					State: "ACTIVE",
				},
			}, fakeRequest)

			So(err, ShouldHaveGRPCStatus, codes.Internal)
			So(err, ShouldErrLike, "unexpected ACTIVE lease state transition to PENDING")
		})

		Convey("UpdateBotSession unrecognized payload type", func() {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				wrong, _ := anypb.New(&timestamppb.Timestamp{})
				return &remoteworkers.BotSession{
					Name:   fakeSessionID,
					Status: remoteworkers.BotStatus_OK,
					Leases: []*remoteworkers.Lease{
						{
							Id:      fakeFirstLeaseID,
							Payload: wrong,
							State:   remoteworkers.LeaseState_PENDING,
						},
					},
				}, nil
			})

			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, fakeRequest)

			So(err, ShouldHaveGRPCStatus, codes.Internal)
			So(err, ShouldErrLike, "failed to unmarshal pending lease payload")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

type (
	expectedCreate func(*remoteworkers.CreateBotSessionRequest) (*remoteworkers.BotSession, error)
	expectedUpdate func(*remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error)
)

type mockedBotsClient struct {
	expected []any // either expectedCreate or expectedUpdate
}

func (m *mockedBotsClient) expectCreateBotSession(cb expectedCreate) {
	m.expected = append(m.expected, cb)
}

func (m *mockedBotsClient) expectUpdateBotSession(cb expectedUpdate) {
	m.expected = append(m.expected, cb)
}

func (m *mockedBotsClient) popExpected() (cb any) {
	So(m.expected, ShouldNotBeEmpty)
	cb, m.expected = m.expected[0], m.expected[1:]
	return cb
}

func (m *mockedBotsClient) CreateBotSession(ctx context.Context, in *remoteworkers.CreateBotSessionRequest, opts ...grpc.CallOption) (*remoteworkers.BotSession, error) {
	cb := m.popExpected()
	So(cb, ShouldHaveSameTypeAs, expectedCreate(nil))
	return cb.(expectedCreate)(in)
}

func (m *mockedBotsClient) UpdateBotSession(ctx context.Context, in *remoteworkers.UpdateBotSessionRequest, opts ...grpc.CallOption) (*remoteworkers.BotSession, error) {
	cb := m.popExpected()
	So(cb, ShouldHaveSameTypeAs, expectedUpdate(nil))
	return cb.(expectedUpdate)(in)
}
