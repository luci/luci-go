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

package botapi

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"

	configpb "go.chromium.org/luci/swarming/proto/config"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botinfo"
	"go.chromium.org/luci/swarming/server/botsession"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(
		datastore.Nullable[string, datastore.Indexed]{},
	))
}

func TestClaim(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
	var idleSince = testTime.Add(-time.Hour)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, testTime)

		taskID := "65aba3a3e6b99310"
		runID := "65aba3a3e6b99311"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		ttrID := model.TaskToRunID(0)
		ttrShard := int32(5)

		secret := hmactoken.NewStaticSecret(secrets.Secret{
			Active: []byte("secret"),
		})

		srv := BotAPIServer{
			cfg:        cfgtest.MockConfigs(ctx, cfgtest.NewMockedConfigs()),
			hmacSecret: secret,
			version:    "server-ver",
		}

		prepTask := func(dims model.TaskDimensions, claimID *string) (*model.TaskRequest, *model.TaskToRun) {
			var claim datastore.Optional[string, datastore.Unindexed]
			var exp datastore.Optional[time.Time, datastore.Indexed]
			if claimID != nil {
				claim.Set(*claimID)
			} else {
				exp.Set(testTime.Add(time.Hour))
			}
			req := &model.TaskRequest{
				Key:  reqKey,
				Name: "task-name",
				TaskSlices: []model.TaskSlice{
					{
						Properties: model.TaskProperties{
							Dimensions: dims,
						},
					},
				},
			}
			ttr := &model.TaskToRun{
				Key:        model.TaskToRunKey(ctx, reqKey, ttrShard, ttrID),
				Dimensions: dims,
				Expiration: exp,
				ClaimID:    claim,
			}
			assert.NoErr(t, datastore.Put(ctx, req, ttr))
			return req, ttr
		}

		call := func(req *ClaimRequest) (*ClaimResponse, error) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "bot:bot-id",
			})
			resp, err := srv.Claim(ctx, req, &botsrv.Request{
				Session: &internalspb.Session{
					BotId: "bot-id",
					BotConfig: &internalspb.BotConfig{
						LogsCloudProject: "logs-cloud-project",
					},
					SessionId: "session-id",
				},
				Dimensions: []string{
					"id:bot-id",
					"pool:bot-pool",
				},
			})
			if err != nil {
				return nil, err
			}
			return resp.(*ClaimResponse), nil
		}

		t.Run("Bad request", func(t *ftt.Test) {
			calls := []ClaimRequest{
				{ClaimID: "", TaskID: taskID, TaskToRunID: ttrID},
				{ClaimID: "claim-id", TaskID: "", TaskToRunID: ttrID},
				{ClaimID: "claim-id", TaskID: taskID},
				{ClaimID: "claim-id", TaskID: "huh", TaskToRunID: ttrID},
			}
			for _, c := range calls {
				_, err := call(&c)
				assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
			}
		})

		t.Run("No task", func(t *ftt.Test) {
			resp, err := call(&ClaimRequest{
				ClaimID:     "claim-id",
				TaskID:      taskID,
				TaskToRunID: ttrID,
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&ClaimResponse{
				Cmd:    ClaimSkip,
				Reason: "No such task",
			}))
		})

		t.Run("Ignores broken state", func(t *ftt.Test) {
			_, err := call(&ClaimRequest{
				ClaimID:     "claim-id",
				TaskID:      taskID,
				TaskToRunID: ttrID,
				State:       botstate.Dict{JSON: []byte("broken")},
			})
			assert.NoErr(t, err)
		})

		t.Run("Dimensions mismatch", func(t *ftt.Test) {
			prepTask(model.TaskDimensions{"pool": {"another-pool"}}, nil)
			resp, err := call(&ClaimRequest{
				ClaimID:        "new-claim-id",
				TaskID:         taskID,
				TaskToRunShard: ttrShard,
				TaskToRunID:    ttrID,
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&ClaimResponse{
				Cmd:    ClaimSkip,
				Reason: "Dimensions mismatch",
			}))
		})

		t.Run("Expired", func(t *ftt.Test) {
			existingClaimID := "" // means the slice expired
			prepTask(model.TaskDimensions{"pool": {"bot-pool"}}, &existingClaimID)
			resp, err := call(&ClaimRequest{
				ClaimID:        "new-claim-id",
				TaskID:         taskID,
				TaskToRunShard: ttrShard,
				TaskToRunID:    ttrID,
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&ClaimResponse{
				Cmd:    ClaimSkip,
				Reason: "The task slice has expired",
			}))
		})

		t.Run("Claimed by someone else", func(t *ftt.Test) {
			existingClaimID := "another-bot:new-claim-id"
			prepTask(model.TaskDimensions{"pool": {"bot-pool"}}, &existingClaimID)
			resp, err := call(&ClaimRequest{
				ClaimID:        "new-claim-id",
				TaskID:         taskID,
				TaskToRunShard: ttrShard,
				TaskToRunID:    ttrID,
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&ClaimResponse{
				Cmd:    ClaimSkip,
				Reason: `Already claimed by "another-bot:new-claim-id"`,
			}))
		})

		t.Run("Claimed by us already", func(t *ftt.Test) {
			existingClaimID := "bot-id:new-claim-id"
			prepTask(model.TaskDimensions{"pool": {"bot-pool"}}, &existingClaimID)
			resp, err := call(&ClaimRequest{
				ClaimID:        "new-claim-id",
				TaskID:         taskID,
				TaskToRunShard: ttrShard,
				TaskToRunID:    ttrID,
			})
			assert.NoErr(t, err)
			// Note: detailed tests for this response are in TestClaimTaskResponse.
			assert.That(t, resp, should.Match(&ClaimResponse{
				Cmd:     ClaimRun,
				TaskID:  runID,
				Session: resp.Session,
				Manifest: &TaskManifest{
					TaskID:     runID,
					Dimensions: model.TaskDimensions{"pool": {"bot-pool"}},
					ServiceAccounts: TaskServiceAccounts{
						System: TaskServiceAccount{ServiceAccount: "none"},
						Task:   TaskServiceAccount{ServiceAccount: "none"},
					},
					BotID:              "bot-id",
					BotDimensions:      map[string][]string{"id": {"bot-id"}, "pool": {"bot-pool"}},
					BotAuthenticatedAs: "bot:bot-id",
				},
			}))
		})

		t.Run("Claim txn OK", func(t *ftt.Test) {
			var botInfoUpdate *botinfo.Update
			srv.submitUpdate = func(ctx context.Context, u *botinfo.Update) error {
				u.PanicIfInvalid()
				botInfoUpdate = u
				return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					proceed, err := u.Prepare(ctx, &model.BotInfo{
						Key: model.BotInfoKey(ctx, u.BotID),
						BotCommon: model.BotCommon{
							Version:   "bot-version",
							IdleSince: datastore.NewUnindexedOptional(idleSince),
						},
					})
					assert.NoErr(t, err)
					assert.That(t, proceed, should.BeTrue)
					return nil
				}, nil)
			}

			var claimOp *tasks.ClaimOp
			var botDetails *tasks.BotDetails

			srv.taskWriteOp = &tasks.TaskWriteOpForTests{
				MockedClaimTxn: func(ctx context.Context, op *tasks.ClaimOp, bot *tasks.BotDetails) (*tasks.ClaimTxnOutcome, error) {
					claimOp = op
					botDetails = bot
					return &tasks.ClaimTxnOutcome{Claimed: true}, nil
				},
				MockedFinishClaimOp: func(ctx context.Context, op *tasks.ClaimOp, outcome *tasks.ClaimTxnOutcome) {
					assert.That(t, outcome.Claimed, should.BeTrue)
				},
			}

			req, ttr := prepTask(model.TaskDimensions{"pool": {"bot-pool"}}, nil)
			resp, err := call(&ClaimRequest{
				ClaimID:        "new-claim-id",
				TaskID:         taskID,
				TaskToRunShard: ttrShard,
				TaskToRunID:    ttrID,
				State:          botstate.Dict{JSON: []byte(`{"some": "state"}`)},
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&ClaimResponse{
				Cmd:      ClaimRun,
				TaskID:   runID,
				Session:  resp.Session,
				Manifest: resp.Manifest,
			}))

			assert.That(t, botInfoUpdate, should.Match(&botinfo.Update{
				BotID:         "bot-id",
				EventType:     model.BotEventTask,
				EventDedupKey: "new-claim-id",
				Prepare:       botInfoUpdate.Prepare,
				State:         &botstate.Dict{JSON: []byte(`{"some": "state"}`)},
				CallInfo: &botinfo.CallInfo{
					SessionID:       "session-id",
					ExternalIP:      "127.0.0.1",
					AuthenticatedAs: "bot:bot-id",
				},
				TaskInfo: &botinfo.TaskInfo{
					TaskID:    runID,
					TaskName:  "task-name",
					TaskFlags: 0,
				},
			}))

			assert.That(t, claimOp, should.Match(&tasks.ClaimOp{
				Request:       req,
				TaskToRunKey:  ttr.Key,
				ClaimID:       "bot-id:new-claim-id",
				ServerVersion: "server-ver",
			}))

			assert.That(t, botDetails, should.Match(&tasks.BotDetails{
				Dimensions:       model.BotDimensions{"id": {"bot-id"}, "pool": {"bot-pool"}},
				Version:          "bot-version",
				LogsCloudProject: "logs-cloud-project",
				IdleSince:        idleSince,
			}))
		})

		t.Run("Claim txn unavailable", func(t *ftt.Test) {
			srv.submitUpdate = func(ctx context.Context, u *botinfo.Update) error {
				u.PanicIfInvalid()
				return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					proceed, err := u.Prepare(ctx, &model.BotInfo{
						Key: model.BotInfoKey(ctx, u.BotID),
						BotCommon: model.BotCommon{
							Version:   "bot-version",
							IdleSince: datastore.NewUnindexedOptional(idleSince),
						},
					})
					assert.NoErr(t, err)
					assert.That(t, proceed, should.BeFalse)
					return nil
				}, nil)
			}

			srv.taskWriteOp = &tasks.TaskWriteOpForTests{
				MockedClaimTxn: func(ctx context.Context, op *tasks.ClaimOp, bot *tasks.BotDetails) (*tasks.ClaimTxnOutcome, error) {
					return &tasks.ClaimTxnOutcome{Unavailable: "unavailable"}, nil
				},
				MockedFinishClaimOp: func(ctx context.Context, op *tasks.ClaimOp, outcome *tasks.ClaimTxnOutcome) {
					assert.That(t, outcome.Claimed, should.BeFalse)
				},
			}

			prepTask(model.TaskDimensions{"pool": {"bot-pool"}}, nil)
			resp, err := call(&ClaimRequest{
				ClaimID:        "new-claim-id",
				TaskID:         taskID,
				TaskToRunShard: ttrShard,
				TaskToRunID:    ttrID,
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&ClaimResponse{
				Cmd:    ClaimSkip,
				Reason: "unavailable",
			}))
		})

		t.Run("Claim txn already claimed", func(t *ftt.Test) {
			srv.submitUpdate = func(ctx context.Context, u *botinfo.Update) error {
				u.PanicIfInvalid()
				return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					proceed, err := u.Prepare(ctx, &model.BotInfo{
						Key: model.BotInfoKey(ctx, u.BotID),
						BotCommon: model.BotCommon{
							Version:   "bot-version",
							IdleSince: datastore.NewUnindexedOptional(idleSince),
						},
					})
					assert.NoErr(t, err)
					assert.That(t, proceed, should.BeFalse)
					return nil
				}, nil)
			}

			srv.taskWriteOp = &tasks.TaskWriteOpForTests{
				MockedClaimTxn: func(ctx context.Context, op *tasks.ClaimOp, bot *tasks.BotDetails) (*tasks.ClaimTxnOutcome, error) {
					return &tasks.ClaimTxnOutcome{}, nil
				},
				MockedFinishClaimOp: func(ctx context.Context, op *tasks.ClaimOp, outcome *tasks.ClaimTxnOutcome) {
					assert.That(t, outcome.Claimed, should.BeFalse)
				},
			}

			prepTask(model.TaskDimensions{"pool": {"bot-pool"}}, nil)
			resp, err := call(&ClaimRequest{
				ClaimID:        "new-claim-id",
				TaskID:         taskID,
				TaskToRunShard: ttrShard,
				TaskToRunID:    ttrID,
			})
			assert.NoErr(t, err)
			assert.That(t, resp, should.Match(&ClaimResponse{
				Cmd:      ClaimRun,
				TaskID:   runID,
				Session:  resp.Session,
				Manifest: resp.Manifest,
			}))
		})
	})
}

func TestFetchClaimDetails(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		taskID := "65aba3a3e6b99310"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		srv := BotAPIServer{
			cfg: cfgtest.MockConfigs(ctx, cfgtest.NewMockedConfigs()),
		}
		botReq := botsrv.Request{
			Session: &internalspb.Session{
				LastSeenConfig: timestamppb.New(time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)),
			},
		}

		storeTaskRequest := func(slices []model.TaskSlice) {
			assert.NoErr(t, datastore.Put(ctx, &model.TaskRequest{
				Key:        reqKey,
				TaskSlices: slices,
			}))
		}

		storeSecretBytes := func(blob []byte) {
			assert.NoErr(t, datastore.Put(ctx, &model.SecretBytes{
				Key:         model.SecretBytesKey(ctx, reqKey),
				SecretBytes: blob,
			}))
		}

		storedNamedCacheHint := func(os, pool, cache string, val int64) {
			assert.NoErr(t, datastore.Put(ctx, &model.NamedCacheStats{
				Key: model.NamedCacheStatsKey(ctx, pool, cache),
				OS: []model.PerOSEntry{
					{
						Name: os,
						Size: val,
					},
				},
			}))
		}

		ttr := func(sliceIdx int) *model.TaskToRun {
			return &model.TaskToRun{
				Key: model.TaskToRunKey(ctx, reqKey, 0, model.TaskToRunID(sliceIdx)),
			}
		}

		t.Run("Missing task", func(t *ftt.Test) {
			// Note: no TaskRequest in the datastore.
			d, err := srv.fetchClaimDetails(ctx, ttr(0), &botReq)
			assert.NoErr(t, err)
			assert.Loosely(t, d, should.BeNil)
		})

		t.Run("Termination task", func(t *ftt.Test) {
			storeTaskRequest([]model.TaskSlice{
				{Properties: model.TaskProperties{
					Dimensions: map[string][]string{"id": {"bot"}},
				}},
			})
			d, err := srv.fetchClaimDetails(ctx, ttr(0), &botReq)
			assert.NoErr(t, err)
			assert.Loosely(t, d.req, should.NotBeNil)
		})

		t.Run("Missing slice", func(t *ftt.Test) {
			storeTaskRequest([]model.TaskSlice{
				{Properties: model.TaskProperties{
					Command: []string{"boo"},
				}},
			})
			d, err := srv.fetchClaimDetails(ctx, ttr(1), &botReq)
			assert.NoErr(t, err)
			assert.Loosely(t, d, should.BeNil)
		})

		t.Run("OK minimal", func(t *ftt.Test) {
			storeTaskRequest([]model.TaskSlice{
				{Properties: model.TaskProperties{
					Command: []string{"boo 1"},
					Dimensions: map[string][]string{
						"pool": {"test-pool"},
					},
				}},
				{Properties: model.TaskProperties{
					Command: []string{"boo 2"},
					Dimensions: map[string][]string{
						"pool": {"test-pool"},
					},
				}},
			})
			d, err := srv.fetchClaimDetails(ctx, ttr(1), &botReq)
			assert.NoErr(t, err)
			assert.Loosely(t, d.req, should.NotBeNil)
			assert.That(t, d.slice, should.Equal(1))
			assert.That(t, d.secret, should.Match([]byte(nil)))
			assert.That(t, d.caches, should.Match([]TaskCache(nil)))
			assert.Loosely(t, d.settings, should.NotBeNil)
		})

		t.Run("OK full", func(t *ftt.Test) {
			botReq.Dimensions = botsrv.BotDimensions{
				"os:Linux",
				"pool:test-pool",
			}

			storeTaskRequest([]model.TaskSlice{
				{Properties: model.TaskProperties{
					Command: []string{"boo 1"},
					Dimensions: map[string][]string{
						"pool": {"test-pool"},
					},
				}},
				{Properties: model.TaskProperties{
					Command: []string{"boo 2"},
					Dimensions: map[string][]string{
						"pool": {"test-pool"},
					},
					Caches: []model.CacheEntry{
						{Name: "cache1", Path: "path1"},
						{Name: "cache2", Path: "path2"},
						{Name: "cache3", Path: "path3"},
					},
					HasSecretBytes: true,
				}},
			})
			storeSecretBytes([]byte("hi"))
			storedNamedCacheHint("Linux", "test-pool", "cache1", 123)
			storedNamedCacheHint("Linux", "test-pool", "cache2", 456)

			d, err := srv.fetchClaimDetails(ctx, ttr(1), &botReq)
			assert.NoErr(t, err)

			assert.That(t, d.secret, should.Match([]byte("hi")))
			assert.That(t, d.caches, should.Match([]TaskCache{
				{
					Name: "cache1",
					Path: "path1",
					Hint: "123",
				},
				{
					Name: "cache2",
					Path: "path2",
					Hint: "456",
				},
				{
					Name: "cache3",
					Path: "path3",
					Hint: "-1",
				},
			}))
		})

		t.Run("OK missing SecretBytes", func(t *ftt.Test) {
			storeTaskRequest([]model.TaskSlice{
				{Properties: model.TaskProperties{
					Command: []string{"boo 1"},
					Dimensions: map[string][]string{
						"pool": {"test-pool"},
					},
					HasSecretBytes: true,
				}},
			})
			d, err := srv.fetchClaimDetails(ctx, ttr(0), &botReq)
			assert.NoErr(t, err)
			assert.That(t, d.secret, should.Match([]byte(nil)))
		})
	})
}

func TestClaimTaskResponse(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)

	ctx := memory.Use(context.Background())
	ctx, _ = testclock.UseTime(ctx, testTime)

	taskID := "65aba3a3e6b99310"
	runID := "65aba3a3e6b99311"
	reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
	assert.NoErr(t, err)

	secret := hmactoken.NewStaticSecret(secrets.Secret{
		Active: []byte("secret"),
	})

	srv := BotAPIServer{
		project:    "server-proj",
		version:    "server-ver",
		hmacSecret: secret,
	}

	call := func(d *claimDetails) *ClaimResponse {
		ctx := auth.WithState(ctx, &authtest.FakeState{
			Identity: "bot:some-bot",
		})
		resp, err := srv.claimTask(ctx, d, &botsrv.Request{
			Session: &internalspb.Session{
				BotId:     "some-bot",
				SessionId: "session-id",
				BotConfig: &internalspb.BotConfig{
					SystemServiceAccount: "system@example.com",
					LogsCloudProject:     "logs-project",
				},
			},
			Dimensions: []string{
				"id:some-bot",
				"pool:some-pool",
				"os:Linux",
				"os:Ubuntu",
			},
		})
		assert.NoErr(t, err)
		return resp.(*ClaimResponse)
	}

	t.Run("Termination", func(t *testing.T) {
		resp := call(&claimDetails{
			req: &model.TaskRequest{
				Key: reqKey,
				TaskSlices: []model.TaskSlice{
					{Properties: model.TaskProperties{
						Dimensions: model.TaskDimensions{"id": {"bot"}},
					}},
				},
			},
		})
		assert.That(t, resp, should.Match(&ClaimResponse{
			Cmd:    ClaimTerminate,
			TaskID: runID,
		}))
	})

	t.Run("Minimal", func(t *testing.T) {
		resp := call(&claimDetails{
			req: &model.TaskRequest{
				Key: reqKey,
				TaskSlices: []model.TaskSlice{
					{Properties: model.TaskProperties{
						Dimensions: model.TaskDimensions{"skip": {"this"}},
					}},
					{Properties: model.TaskProperties{
						Dimensions:           model.TaskDimensions{"pool": {"some-pool"}},
						ExecutionTimeoutSecs: 120,
						GracePeriodSecs:      15,
					}},
				},
			},
			slice: 1,
		})
		assert.That(t, resp, should.Match(&ClaimResponse{
			Cmd:     ClaimRun,
			TaskID:  runID,
			Session: resp.Session,
			Manifest: &TaskManifest{
				TaskID:          runID,
				Dimensions:      model.TaskDimensions{"pool": {"some-pool"}},
				GracePeriodSecs: 15,
				HardTimeoutSecs: 120,
				ServiceAccounts: TaskServiceAccounts{
					System: TaskServiceAccount{ServiceAccount: "system@example.com"},
					Task:   TaskServiceAccount{ServiceAccount: "none"},
				},
				BotID: "some-bot",
				BotDimensions: map[string][]string{
					"id":   {"some-bot"},
					"os":   {"Linux", "Ubuntu"},
					"pool": {"some-pool"},
				},
				BotAuthenticatedAs: "bot:some-bot",
			},
		}))

		session, err := botsession.Unmarshal(resp.Session, secret)
		assert.NoErr(t, err)
		assert.That(t, session, should.Match(&internalspb.Session{
			BotId:     "some-bot",
			SessionId: "session-id",
			BotConfig: &internalspb.BotConfig{
				SystemServiceAccount: "system@example.com",
				LogsCloudProject:     "logs-project",
				Expiry:               timestamppb.New(testTime.Add(120*time.Second + 15*time.Second + 300*time.Second)),
			},
			DebugInfo: &internalspb.DebugInfo{
				Created:         timestamppb.New(testTime),
				SwarmingVersion: "server-ver",
				RequestId:       "00000000000000000000000000000000",
			},
			Expiry: timestamppb.New(testTime.Add(botsession.Expiry)),
		}))
	})

	t.Run("Full", func(t *testing.T) {
		resp := call(&claimDetails{
			req: &model.TaskRequest{
				Key:                 reqKey,
				ResultDBUpdateToken: "update-token",
				Realm:               "some:realm",
				ServiceAccount:      "task@example.com",
				TaskSlices: []model.TaskSlice{
					{Properties: model.TaskProperties{
						Dimensions: model.TaskDimensions{"skip": {"this"}},
					}},
					{Properties: model.TaskProperties{
						Dimensions: model.TaskDimensions{
							"pool": {"some-pool"},
							"os":   {"Linux", "Ubuntu"},
						},
						ExecutionTimeoutSecs: 120,
						GracePeriodSecs:      15,
						IOTimeoutSecs:        20,
						Caches: []model.CacheEntry{
							{Name: "cache1", Path: "path1"},
							{Name: "cache2", Path: "path2"},
						},
						CIPDInput: model.CIPDInput{
							Server: "https://cipd.example.com",
							ClientPackage: model.CIPDPackage{
								PackageName: "client-pkg",
								Version:     "client-ver",
							},
							Packages: []model.CIPDPackage{
								{
									PackageName: "cipd/pkg1",
									Version:     "ver1",
									Path:        "path1",
								},
								{
									PackageName: "cipd/pkg2",
									Version:     "ver2",
									Path:        "path2",
								},
							},
						},
						Command:     []string{"echo", "hi"},
						Containment: model.Containment{ContainmentType: 1},
						Env:         model.Env{"A": "B"},
						EnvPrefixes: model.EnvPrefixes{"P": {"X", "Y"}},
						CASInputRoot: model.CASReference{
							CASInstance: "cas-instance",
							Digest: model.CASDigest{
								Hash:      "cas-hash",
								SizeBytes: 1234,
							},
						},
						Outputs:     []string{"o1", "o2"},
						RelativeCwd: "./dir",
					}},
				},
			},
			slice: 1,
			caches: []TaskCache{
				{Name: "cache1", Path: "path1", Hint: "123"},
				{Name: "cache2", Path: "path2", Hint: "456"},
			},
			secret: []byte("hi"),
			settings: &configpb.SettingsCfg{
				Resultdb: &configpb.ResultDBSettings{
					Server: "https://resultdb.example.com",
				},
			},
		})
		assert.That(t, resp, should.Match(&ClaimResponse{
			Cmd:     ClaimRun,
			TaskID:  runID,
			Session: resp.Session,
			Manifest: &TaskManifest{
				TaskID: runID,
				Caches: []TaskCache{
					{Name: "cache1", Path: "path1", Hint: "123"},
					{Name: "cache2", Path: "path2", Hint: "456"},
				},
				CIPDInput: &model.CIPDInput{
					Server:        "https://cipd.example.com",
					ClientPackage: model.CIPDPackage{PackageName: "client-pkg", Version: "client-ver"},
					Packages: []model.CIPDPackage{
						{PackageName: "cipd/pkg1", Version: "ver1", Path: "path1"},
						{PackageName: "cipd/pkg2", Version: "ver2", Path: "path2"},
					},
				},
				Command:     []string{"echo", "hi"},
				Containment: &model.Containment{ContainmentType: 1},
				Dimensions: model.TaskDimensions{
					"os":   {"Linux", "Ubuntu"},
					"pool": {"some-pool"},
				},
				Env:             model.Env{"A": "B"},
				EnvPrefixes:     model.EnvPrefixes{"P": {"X", "Y"}},
				GracePeriodSecs: 15,
				HardTimeoutSecs: 120,
				IOTimeoutSecs:   20,
				SecretBytes:     []byte("hi"),
				CASInputRoot: &model.CASReference{
					CASInstance: "cas-instance",
					Digest:      model.CASDigest{Hash: "cas-hash", SizeBytes: 1234},
				},
				Outputs:     []string{"o1", "o2"},
				Realm:       &TaskRealm{Name: "some:realm"},
				RelativeCwd: "./dir",
				ResultDB: &TaskResultDB{
					Hostname: "resultdb.example.com",
					CurrentInvocation: TaskInvocation{
						Name:        "invocations/task-server-proj.appspot.com-65aba3a3e6b99311",
						UpdateToken: "update-token",
					},
				},
				ServiceAccounts: TaskServiceAccounts{
					System: TaskServiceAccount{ServiceAccount: "system@example.com"},
					Task:   TaskServiceAccount{ServiceAccount: "task@example.com"},
				},
				BotID: "some-bot",
				BotDimensions: map[string][]string{
					"id":   {"some-bot"},
					"os":   {"Linux", "Ubuntu"},
					"pool": {"some-pool"},
				},
				BotAuthenticatedAs: "bot:some-bot",
			},
		}))
	})
}
