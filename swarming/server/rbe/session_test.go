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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botsession"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/hmactoken"
)

func TestSessionServer(t *testing.T) {
	t.Parallel()

	ftt.Run("With server", t, func(t *ftt.Test) {
		const (
			fakeRBEInstance   = "fake-rbe-instance"
			fakeBotID         = "fake-bot-id"
			fakeSessionID     = "fake-session-id"
			fakeRBESessionID  = "fake-rbe-session"
			fakeFirstLeaseID  = "fake-first-lease-id"
			fakeSecondLeaseID = "fake-second-lease-id"
			fakeFirstTaskID   = "fake-first-task-id"
			fakeSecondTaskID  = "fake-second-task-id"
		)

		fakeSession := &internalspb.Session{
			BotId:     fakeBotID,
			SessionId: fakeSessionID,
			BotConfig: &internalspb.BotConfig{
				RbeInstance: fakeRBEInstance,
			},
			RbeBotSessionId: fakeRBESessionID,
		}

		fakeRequest := &botsrv.Request{
			Session: fakeSession,
			Dimensions: []string{
				"id:" + fakeBotID,
				"extra1:a",
				"extra1:b",
				"extra2:c",
				"extra2:d",
				"pool:some-pool",
			},
		}

		payload := func(taskID string) *anypb.Any {
			msg, err := anypb.New(&internalspb.TaskPayload{TaskId: taskID})
			assert.NoErr(t, err)
			return msg
		}

		result := func() *anypb.Any {
			msg, err := anypb.New(&internalspb.TaskResult{})
			assert.NoErr(t, err)
			return msg
		}

		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, now)

		rbe := &mockedBotsClient{}

		cfg := cfgtest.NewMockedConfigs()
		poolCfg := cfg.MockPool("some-pool", "project:realm")
		poolCfg.InformationalDimensionRe = []string{"label-*"}
		srv := &SessionServer{
			rbe: rbe,
			hmacSecret: hmactoken.NewStaticSecret(secrets.Secret{
				Active: []byte("secret"),
			}),
			backendVer: "backend-ver",
			cfg:        cfgtest.MockConfigs(ctx, cfg),
		}

		t.Run("CreateBotSession works", func(t *ftt.Test) {
			rbe.expectCreateBotSession(func(r *remoteworkers.CreateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r, should.Match(&remoteworkers.CreateBotSessionRequest{
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
				}))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
					Status: remoteworkers.BotStatus_INITIALIZING,
				}, nil
			})

			resp, err := srv.CreateBotSession(ctx, &CreateBotSessionRequest{
				BotVersion: "bot-version",
				WorkerProperties: &WorkerProperties{
					PoolID:      "rbe-pool-id",
					PoolVersion: "rbe-pool-version",
				},
			}, fakeRequest)

			assert.NoErr(t, err)

			msg := resp.(*CreateBotSessionResponse)
			assert.Loosely(t, msg.SessionID, should.Equal(fakeRBESessionID))

			session, err := botsession.Unmarshal(msg.Session, srv.hmacSecret)
			assert.NoErr(t, err)
			assert.That(t, session, should.Match(&internalspb.Session{
				BotId:     fakeBotID,
				SessionId: fakeSessionID,
				Expiry:    timestamppb.New(now.Add(botsession.Expiry)),
				DebugInfo: &internalspb.DebugInfo{
					Created:         timestamppb.New(now),
					SwarmingVersion: srv.backendVer,
					RequestId:       "00000000000000000000000000000000",
				},
				BotConfig: &internalspb.BotConfig{
					RbeInstance: fakeRBEInstance,
				},
				RbeBotSessionId: fakeRBESessionID,
			}))
		})

		t.Run("informational dims are excluded", func(t *ftt.Test) {
			req := &botsrv.Request{
				Session: fakeSession,
				Dimensions: []string{
					"id:" + fakeBotID,
					"extra1:a",
					"extra1:b",
					"extra2:c",
					"extra2:d",
					"label-info:shouldskip",
					"pool:some-pool",
				},
			}
			rbe.expectCreateBotSession(func(r *remoteworkers.CreateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r.BotSession.Worker.Devices, should.HaveLength(1))
				assert.That(t, r.BotSession.Worker.Devices[0].Properties, should.Match([]*remoteworkers.Device_Property{
					{Key: "label:extra1", Value: "a"},
					{Key: "label:extra1", Value: "b"},
					{Key: "label:extra2", Value: "c"},
					{Key: "label:extra2", Value: "d"},
					{Key: "label:pool", Value: "some-pool"},
				}))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
					Status: remoteworkers.BotStatus_INITIALIZING,
				}, nil
			})

			_, err := srv.CreateBotSession(ctx, &CreateBotSessionRequest{
				BotVersion: "bot-version",
				WorkerProperties: &WorkerProperties{
					PoolID:      "rbe-pool-id",
					PoolVersion: "rbe-pool-version",
				},
			}, req)

			assert.NoErr(t, err)
		})

		t.Run("effective bot id is used", func(t *ftt.Test) {
			req := &botsrv.Request{
				Session: &internalspb.Session{
					BotId:     fakeBotID,
					SessionId: fakeSessionID,
					BotConfig: &internalspb.BotConfig{
						RbeInstance:                fakeRBEInstance,
						RbeEffectiveBotId:          "effective-bot-id",
						RbeEffectiveBotIdDimension: "dut_id",
					},
					RbeBotSessionId: fakeRBESessionID,
				},
				Dimensions: []string{
					"id:" + fakeBotID,
					"dut_id:effective-bot-id",
					"pool:some-pool",
				},
			}
			rbe.expectCreateBotSession(func(r *remoteworkers.CreateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.That(t, r.BotSession.BotId, should.Equal("effective-bot-id"))
				assert.Loosely(t, r.BotSession.Worker.Devices, should.HaveLength(1))
				assert.That(t, r.BotSession.Worker.Devices[0].Properties, should.Match([]*remoteworkers.Device_Property{
					{Key: "label:pool", Value: "some-pool"},
				}))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
					Status: remoteworkers.BotStatus_INITIALIZING,
				}, nil
			})

			_, err := srv.CreateBotSession(ctx, &CreateBotSessionRequest{
				BotVersion: "bot-version",
				WorkerProperties: &WorkerProperties{
					PoolID:      "rbe-pool-id",
					PoolVersion: "rbe-pool-version",
				},
			}, req)

			assert.NoErr(t, err)
		})

		t.Run("CreateBotSession propagates RBE error", func(t *ftt.Test) {
			rbe.expectCreateBotSession(func(r *remoteworkers.CreateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return nil, status.Errorf(codes.FailedPrecondition, "boom")
			})
			_, err := srv.CreateBotSession(ctx, &CreateBotSessionRequest{}, fakeRequest)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("boom"))
		})

		t.Run("UpdateBotSession IDLE", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r, should.Match(&remoteworkers.UpdateBotSessionRequest{
					Name: fakeRBESessionID,
					BotSession: &remoteworkers.BotSession{
						Name:    fakeRBESessionID,
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
				}))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
					Status: remoteworkers.BotStatus_OK,
				}, nil
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status:     "OK",
				BotVersion: "bot-version",
				WorkerProperties: &WorkerProperties{
					PoolID:      "rbe-pool-id",
					PoolVersion: "rbe-pool-version",
				},
			}, fakeRequest)

			assert.NoErr(t, err)

			msg := resp.(*UpdateBotSessionResponse)
			assert.Loosely(t, msg.Status, should.Equal("OK"))
			assert.Loosely(t, msg.Lease, should.BeNil)

			session, err := botsession.Unmarshal(msg.Session, srv.hmacSecret)
			assert.NoErr(t, err)
			assert.That(t, session, should.Match(&internalspb.Session{
				BotId:     fakeBotID,
				SessionId: fakeSessionID,
				Expiry:    timestamppb.New(now.Add(botsession.Expiry)),
				DebugInfo: &internalspb.DebugInfo{
					Created:         timestamppb.New(now),
					SwarmingVersion: srv.backendVer,
					RequestId:       "00000000000000000000000000000000",
				},
				BotConfig: &internalspb.BotConfig{
					RbeInstance: fakeRBEInstance,
				},
				RbeBotSessionId: fakeRBESessionID,
			}))
		})

		t.Run("UpdateBotSession IDLE + DEADLINE_EXCEEDED", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return nil, status.Errorf(codes.DeadlineExceeded, "boom")
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, fakeRequest)

			assert.NoErr(t, err)

			msg := resp.(*UpdateBotSessionResponse)
			assert.Loosely(t, msg.Status, should.Equal("OK"))
			assert.Loosely(t, msg.Lease, should.BeNil)
		})

		t.Run("UpdateBotSession TERMINATING", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r.BotSession.Status, should.Equal(remoteworkers.BotStatus_BOT_TERMINATING))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.NoErr(t, err)

			msg := resp.(*UpdateBotSessionResponse)
			assert.Loosely(t, msg.Status, should.Equal("OK"))
			assert.Loosely(t, msg.Lease, should.BeNil)
		})

		t.Run("UpdateBotSession TERMINATING by RBE", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r.BotSession.Status, should.Equal(remoteworkers.BotStatus_OK))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.NoErr(t, err)

			msg := resp.(*UpdateBotSessionResponse)
			assert.Loosely(t, msg.Status, should.Equal("BOT_TERMINATING"))
			assert.Loosely(t, msg.Lease, should.BeNil)
		})

		t.Run("UpdateBotSession IDLE => PENDING", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r.BotSession.Status, should.Equal(remoteworkers.BotStatus_OK))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.NoErr(t, err)

			lease := resp.(*UpdateBotSessionResponse).Lease
			assert.Loosely(t, lease.ID, should.Equal(fakeFirstLeaseID))
			assert.Loosely(t, lease.State, should.Equal("PENDING"))
			assert.Loosely(t, lease.Payload, should.Match(&internalspb.TaskPayload{
				TaskId: fakeFirstTaskID,
			}))
		})

		t.Run("UpdateBotSession ACTIVE", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r.BotSession.Status, should.Equal(remoteworkers.BotStatus_OK))
				assert.Loosely(t, r.BotSession.Leases, should.Match([]*remoteworkers.Lease{
					{
						Id:    fakeFirstLeaseID,
						State: remoteworkers.LeaseState_ACTIVE,
					},
				}))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.NoErr(t, err)

			lease := resp.(*UpdateBotSessionResponse).Lease
			assert.Loosely(t, lease.ID, should.Equal(fakeFirstLeaseID))
			assert.Loosely(t, lease.State, should.Equal("ACTIVE"))
			assert.Loosely(t, lease.Payload, should.BeNil)
		})

		t.Run("UpdateBotSession ACTIVE => CANCELLED", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r.BotSession.Status, should.Equal(remoteworkers.BotStatus_OK))
				assert.Loosely(t, r.BotSession.Leases, should.Match([]*remoteworkers.Lease{
					{
						Id:    fakeFirstLeaseID,
						State: remoteworkers.LeaseState_ACTIVE,
					},
				}))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.NoErr(t, err)

			lease := resp.(*UpdateBotSessionResponse).Lease
			assert.Loosely(t, lease.ID, should.Equal(fakeFirstLeaseID))
			assert.Loosely(t, lease.State, should.Equal("CANCELLED"))
			assert.Loosely(t, lease.Payload, should.BeNil)
		})

		t.Run("UpdateBotSession ACTIVE => IDLE", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r.BotSession.Status, should.Equal(remoteworkers.BotStatus_OK))
				assert.Loosely(t, r.BotSession.Leases, should.Match([]*remoteworkers.Lease{
					{
						Id:     fakeFirstLeaseID,
						State:  remoteworkers.LeaseState_COMPLETED,
						Status: &statuspb.Status{},
						Result: result(),
					},
				}))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.NoErr(t, err)
			assert.Loosely(t, resp.(*UpdateBotSessionResponse).Lease, should.BeNil)
		})

		t.Run("UpdateBotSession ACTIVE => PENDING", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				assert.Loosely(t, r.BotSession.Status, should.Equal(remoteworkers.BotStatus_OK))
				assert.Loosely(t, r.BotSession.Leases, should.Match([]*remoteworkers.Lease{
					{
						Id:     fakeFirstLeaseID,
						State:  remoteworkers.LeaseState_COMPLETED,
						Status: &statuspb.Status{},
						Result: result(),
					},
				}))
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.NoErr(t, err)

			lease := resp.(*UpdateBotSessionResponse).Lease
			assert.Loosely(t, lease.ID, should.Equal(fakeSecondLeaseID))
			assert.Loosely(t, lease.State, should.Equal("PENDING"))
			assert.Loosely(t, lease.Payload, should.Match(&internalspb.TaskPayload{
				TaskId: fakeSecondTaskID,
			}))
		})

		t.Run("UpdateBotSession no session ID", func(t *ftt.Test) {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, &botsrv.Request{
				Session: &internalspb.Session{},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("no active RBE session"))
		})

		t.Run("UpdateBotSession BotId changed", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return nil, status.Errorf(codes.FailedPrecondition, `cannot change bot ID: expected "old_bot_id", got %q`, fakeBotID)
			})

			resp, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, fakeRequest)

			assert.NoErr(t, err)

			msg := resp.(*UpdateBotSessionResponse)
			assert.Loosely(t, msg.Status, should.Equal("UNHEALTHY"))
			assert.Loosely(t, msg.Lease, should.BeNil)
		})
		t.Run("UpdateBotSession propagates RBE error", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return nil, status.Errorf(codes.FailedPrecondition, "boom")
			})
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
			}, fakeRequest)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("boom"))
		})

		t.Run("UpdateBotSession bad session status", func(t *ftt.Test) {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "huh",
			}, fakeRequest)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("unrecognized session status"))
		})

		t.Run("UpdateBotSession missing session status", func(t *ftt.Test) {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{}, fakeRequest)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("missing session status"))
		})

		t.Run("UpdateBotSession bad lease state", func(t *ftt.Test) {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease:  &Lease{State: "huh"},
			}, fakeRequest)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("unrecognized lease state"))
		})

		t.Run("UpdateBotSession missing lease state", func(t *ftt.Test) {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease:  &Lease{},
			}, fakeRequest)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("missing lease state"))
		})

		t.Run("UpdateBotSession unexpected lease state", func(t *ftt.Test) {
			_, err := srv.UpdateBotSession(ctx, &UpdateBotSessionRequest{
				Status: "OK",
				Lease:  &Lease{State: "CANCELLED"},
			}, fakeRequest)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("unexpected lease state"))
		})

		t.Run("UpdateBotSession ACTIVE lease disappears", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.NoErr(t, err)
			assert.Loosely(t, resp.(*UpdateBotSessionResponse).Lease, should.BeNil)
		})

		t.Run("UpdateBotSession unexpected ACTIVE lease transition", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
			assert.Loosely(t, err, should.ErrLike("unexpected ACTIVE lease state transition to PENDING"))
		})

		t.Run("UpdateBotSession unrecognized payload type", func(t *ftt.Test) {
			rbe.expectUpdateBotSession(func(r *remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error) {
				wrong, _ := anypb.New(&timestamppb.Timestamp{})
				return &remoteworkers.BotSession{
					Name:   fakeRBESessionID,
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

			assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
			assert.Loosely(t, err, should.ErrLike("failed to unmarshal pending lease payload"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

type (
	expectedCreate func(*remoteworkers.CreateBotSessionRequest) (*remoteworkers.BotSession, error)
	expectedUpdate func(*remoteworkers.UpdateBotSessionRequest) (*remoteworkers.BotSession, error)
)

type mockedBotsClient struct {
	t testing.TB

	expected []any // either expectedCreate or expectedUpdate
}

func (m *mockedBotsClient) expectCreateBotSession(cb expectedCreate) {
	m.expected = append(m.expected, cb)
}

func (m *mockedBotsClient) expectUpdateBotSession(cb expectedUpdate) {
	m.expected = append(m.expected, cb)
}

func (m *mockedBotsClient) popExpected() (cb any) {
	assert.Loosely(m.t, m.expected, should.NotBeEmpty)
	cb, m.expected = m.expected[0], m.expected[1:]
	return cb
}

func (m *mockedBotsClient) CreateBotSession(ctx context.Context, in *remoteworkers.CreateBotSessionRequest, opts ...grpc.CallOption) (*remoteworkers.BotSession, error) {
	cb := m.popExpected()
	assert.Loosely(m.t, cb, should.HaveType[expectedCreate])
	return cb.(expectedCreate)(in)
}

func (m *mockedBotsClient) UpdateBotSession(ctx context.Context, in *remoteworkers.UpdateBotSessionRequest, opts ...grpc.CallOption) (*remoteworkers.BotSession, error) {
	cb := m.popExpected()
	assert.Loosely(m.t, cb, should.HaveType[expectedUpdate])
	return cb.(expectedUpdate)(in)
}
