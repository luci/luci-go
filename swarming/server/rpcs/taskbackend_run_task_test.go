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

package rpcs

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/tasks"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func TestTaskBackendRunTask(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	now := testclock.TestRecentTimeUTC
	ctx, _ = testclock.UseTime(ctx, now)
	ctx, tqt := tqtasks.TestingContext(ctx)

	srv := &TaskBackend{
		BuildbucketTarget:       "swarming://target",
		BuildbucketAccount:      "ignored-in-the-test",
		DisableBuildbucketCheck: true,
		StatusPageLink: func(taskID string) string {
			return "http://fake.example.com/" + taskID
		},
		TasksServer: &TasksServer{
			TasksManager: tasks.NewManager(tqt.Tasks, "swarming", "version", nil, false),
		},
	}

	ftt.Run("validate", t, func(t *ftt.Test) {
		t.Run("no_id", func(t *ftt.Test) {
			_, err := srv.RunTask(ctx, &bbpb.RunTaskRequest{})
			assert.That(t, err, should.ErrLike("build_id is required"))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("no_pubsub_topic", func(t *ftt.Test) {
			req := &bbpb.RunTaskRequest{
				BuildId: "12345678",
			}
			_, err := srv.RunTask(ctx, req)
			assert.That(t, err, should.ErrLike("pubsub_topic is required"))
		})

		t.Run("no_target", func(t *ftt.Test) {
			req := &bbpb.RunTaskRequest{
				BuildId:     "12345678",
				PubsubTopic: "pubsub_topic",
			}
			_, err := srv.RunTask(ctx, req)
			assert.That(t, err, should.ErrLike("target: required"))
		})

		t.Run("wrong_target", func(t *ftt.Test) {
			req := &bbpb.RunTaskRequest{
				BuildId:     "12345678",
				PubsubTopic: "pubsub_topic",
				Target:      "wrong",
			}
			_, err := srv.RunTask(ctx, req)
			assert.That(t, err, should.ErrLike(`Expected "swarming://target", got "wrong"`))
		})

		t.Run("past_start_deadline", func(t *ftt.Test) {
			req := &bbpb.RunTaskRequest{
				BuildId:       "12345678",
				PubsubTopic:   "pubsub_topic",
				Target:        "swarming://target",
				StartDeadline: timestamppb.New(now.Add(-1 * time.Minute)),
			}
			_, err := srv.RunTask(ctx, req)
			assert.That(t, err, should.ErrLike("start_deadline must be in the future"))
		})

		t.Run("backend_config", func(t *ftt.Test) {
			call := func(cfgMap any) (*bbpb.RunTaskResponse, error) {
				req := &bbpb.RunTaskRequest{
					BuildId:       "12345678",
					PubsubTopic:   "pubsub_topic",
					Target:        srv.BuildbucketTarget,
					StartDeadline: timestamppb.New(now.Add(10 * time.Minute)),
					BackendConfig: toBackendCfg(cfgMap),
				}
				return srv.RunTask(ctx, req)
			}

			cases := []struct {
				name string
				cfg  any
				errs []string
			}{
				{
					name: "fail_ingest_config",
					cfg: map[string]any{
						"random": 500,
					},
					errs: []string{"unknown field"},
				},
				{
					name: "missing_fields",
					cfg:  nil,
					errs: []string{
						"agent_binary_cipd_filename: required",
					},
				},
			}

			for _, c := range cases {
				t.Run(c.name, func(t *ftt.Test) {
					_, err := call(c.cfg)
					for _, e := range c.errs {
						assert.That(t, err, should.ErrLike(e))
					}
				})
			}
		})
	})

	ftt.Run("failed_to_convert_request", t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), now)

		req := validRequest(now)
		req.Caches = append(req.Caches, &bbpb.CacheEntry{
			Name:             "cache2",
			Path:             "path2",
			WaitForWarmCache: durationpb.New(37 * time.Second),
		})
		_, err := srv.RunTask(ctx, req)
		assert.That(t, err, should.ErrLike("cache 1: wait_for_warm_cache must be a multiple of a minute"))
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("RunTask", t, func(t *ftt.Test) {
		t.Run("fail_to_create_task", func(t *ftt.Test) {
			req := validRequest(now)
			req.Dimensions = append(req.Dimensions, &bbpb.RequestedDimension{
				Key:   "pool",
				Value: "pool2",
			})
			_, err := srv.RunTask(ctx, req)
			assert.That(t, err, should.ErrLike("pool cannot be specified more than once"))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("OK", func(t *ftt.Test) {
			state := NewMockedRequestState()
			state.Configs.MockPool("pool", "project:pool-realm")
			state.MockPerm("project:pool-realm", acls.PermPoolsCreateTask)
			state.MockPerm("project:bucket", acls.PermTasksCreateInRealm)
			state.MockPermWithIdentity("project:bucket",
				identity.Identity("user:sa@service-accounts.com"), acls.PermTasksActAs)
			ctx = MockRequestState(ctx, state)
			ctx = cryptorand.MockForTest(ctx, 0)

			req := validRequest(now)
			res, err := srv.RunTask(ctx, req)
			assert.NoErr(t, err)

			expected := &bbpb.RunTaskResponse{
				Task: &bbpb.Task{
					Id: &bbpb.TaskID{
						Id:     "2cbe1fa55012fa10",
						Target: srv.BuildbucketTarget,
					},
					Link:     "http://fake.example.com/2cbe1fa55012fa10",
					Status:   bbpb.Status_SCHEDULED,
					UpdateId: now.UnixNano(),
				},
			}
			assert.That(t, res, should.Match(expected))
		})
	})
}

func TestComputeDimsByExp(t *testing.T) {
	t.Parallel()

	t.Run("invalid_wait_for_warm_cache", func(t *testing.T) {
		req := &bbpb.RunTaskRequest{
			Caches: []*bbpb.CacheEntry{
				{
					Name:             "cache",
					Path:             "path",
					WaitForWarmCache: durationpb.New(15 * time.Second),
				},
			},
		}
		_, _, err := computeDimsByExp(req)
		assert.That(t, err, should.ErrLike("cache 0: wait_for_warm_cache must be a multiple of a minute"))
	})

	t.Run("no_dims", func(t *testing.T) {
		dimsByExp, exps, err := computeDimsByExp(&bbpb.RunTaskRequest{})
		assert.NoErr(t, err)
		assert.That(t, len(dimsByExp), should.Equal(0))
		assert.That(t, len(exps), should.Equal(0))
	})

	t.Run("only_base", func(t *testing.T) {
		req := &bbpb.RunTaskRequest{
			Dimensions: []*bbpb.RequestedDimension{
				{
					Key:   "pool",
					Value: "pool",
				},
			},
			Caches: []*bbpb.CacheEntry{
				{
					Name: "cache",
					Path: "path",
				},
			},
		}
		dimsByExp, exps, err := computeDimsByExp(req)
		assert.NoErr(t, err)

		expectedMap := map[time.Duration][]*apipb.StringPair{
			0: {
				{Key: "pool", Value: "pool"},
			},
		}
		assert.That(t, dimsByExp, should.Match(expectedMap))
		assert.That(t, exps, should.Match([]time.Duration{0}))
	})

	t.Run("no_base", func(t *testing.T) {
		req := &bbpb.RunTaskRequest{
			Dimensions: []*bbpb.RequestedDimension{
				{
					Key:        "pool",
					Value:      "pool",
					Expiration: durationpb.New(time.Minute),
				},
				{
					Key:        "dim",
					Value:      "dim2",
					Expiration: durationpb.New(5 * time.Minute),
				},
				{
					Key:        "dim",
					Value:      "dim1",
					Expiration: durationpb.New(time.Minute),
				},
			},
			Caches: []*bbpb.CacheEntry{
				{
					Name:             "cache",
					Path:             "path",
					WaitForWarmCache: durationpb.New(10 * time.Minute),
				},
			},
		}
		dimsByExp, exps, err := computeDimsByExp(req)
		assert.NoErr(t, err)

		expectedMap := map[time.Duration][]*apipb.StringPair{
			time.Minute: {
				{Key: "caches", Value: "cache"},
				{Key: "dim", Value: "dim1"},
				{Key: "dim", Value: "dim2"},
				{Key: "pool", Value: "pool"},
			},
			5 * time.Minute: {
				{Key: "caches", Value: "cache"},
				{Key: "dim", Value: "dim2"},
			},
			10 * time.Minute: {
				{Key: "caches", Value: "cache"},
			},
		}
		assert.That(t, dimsByExp, should.Match(expectedMap))
		assert.That(t, exps, should.Match([]time.Duration{time.Minute, 5 * time.Minute, 10 * time.Minute}))
	})

	t.Run("full_example", func(t *testing.T) {
		req := &bbpb.RunTaskRequest{
			Dimensions: []*bbpb.RequestedDimension{
				{
					Key:   "pool",
					Value: "pool",
				},
				{
					Key:        "dim",
					Value:      "dim2",
					Expiration: durationpb.New(5 * time.Minute),
				},
				{
					Key:        "dim",
					Value:      "dim1",
					Expiration: durationpb.New(time.Minute),
				},
			},
			Caches: []*bbpb.CacheEntry{
				{
					Name:             "cache",
					Path:             "path",
					WaitForWarmCache: durationpb.New(10 * time.Minute),
				},
			},
		}
		dimsByExp, exps, err := computeDimsByExp(req)
		assert.NoErr(t, err)

		expectedMap := map[time.Duration][]*apipb.StringPair{
			0: {
				{Key: "pool", Value: "pool"},
			},
			time.Minute: {
				{Key: "caches", Value: "cache"},
				{Key: "dim", Value: "dim1"},
				{Key: "dim", Value: "dim2"},
				{Key: "pool", Value: "pool"},
			},
			5 * time.Minute: {
				{Key: "caches", Value: "cache"},
				{Key: "dim", Value: "dim2"},
				{Key: "pool", Value: "pool"},
			},
			10 * time.Minute: {
				{Key: "caches", Value: "cache"},
				{Key: "pool", Value: "pool"},
			},
		}
		assert.That(t, dimsByExp, should.Match(expectedMap))
		assert.That(t, exps, should.Match([]time.Duration{0, time.Minute, 5 * time.Minute, 10 * time.Minute}))
	})
}

func TestComputeNewTaskRequest(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeUTC
	ctx, _ := testclock.UseTime(context.Background(), now)

	t.Run("no_dims", func(t *testing.T) {
		req := validRequest(now)
		req.Dimensions = nil
		req.Caches = nil

		cfg, err := ingestBackendConfigWithDefaults(req.GetBackendConfig())
		assert.NoErr(t, err)

		_, err = computeNewTaskRequest(ctx, req, cfg)
		assert.That(t, err, should.ErrLike("dimensions are required"))
	})

	t.Run("OK", func(t *testing.T) {
		req := validRequest(now)

		cfg, err := ingestBackendConfigWithDefaults(req.GetBackendConfig())
		assert.NoErr(t, err)

		newReq, err := computeNewTaskRequest(ctx, req, cfg)
		assert.NoErr(t, err)

		sb, err := hex.DecodeString("1a05746f6b656e")
		assert.NoErr(t, err)

		slice := func(dims []*apipb.StringPair, exp time.Duration, waitForCapacity bool) *apipb.TaskSlice {
			return &apipb.TaskSlice{
				ExpirationSecs: int32(exp.Seconds()),
				Properties: &apipb.TaskProperties{
					GracePeriodSecs:      int32(req.GracePeriod.Seconds),
					ExecutionTimeoutSecs: int32(req.ExecutionTimeout.Seconds),
					Command:              []string{"bbagent", "arg1", "arg2"},
					Caches: []*apipb.CacheEntry{
						{Name: "cache", Path: "path"},
					},
					Dimensions: dims,
					CipdInput: &apipb.CipdInput{
						Packages: []*apipb.CipdPackage{
							{
								PackageName: cfg.AgentBinaryCipdPkg,
								Version:     cfg.AgentBinaryCipdVers,
								Path:        ".",
							},
						},
						Server: cfg.AgentBinaryCipdServer,
					},
					SecretBytes: sb,
				},
				WaitForCapacity: waitForCapacity,
			}
		}

		dims1 := []*apipb.StringPair{
			{Key: "caches", Value: "cache"},
			{Key: "dim", Value: "dim1"},
			{Key: "pool", Value: "pool"},
		}
		dims2 := []*apipb.StringPair{
			{Key: "pool", Value: "pool"},
		}
		expected := &apipb.NewTaskRequest{
			Name:                 "bb-12345678",
			Priority:             50,
			ServiceAccount:       "sa@service-accounts.com",
			BotPingToleranceSecs: 1200,
			Tags: []string{
				"tag1:v1",
				"tag2:v2",
			},
			Realm:       "project:bucket",
			RequestUuid: "req-bb-12345678",
			TaskSlices: []*apipb.TaskSlice{
				slice(dims1, time.Minute, false),
				slice(dims2, 240*time.Second, true),
			},
		}
		assert.That(t, newReq, should.Match(expected))
	})
}

func toBackendCfg(raw any) *structpb.Struct {
	cfgJSON, _ := json.Marshal(raw)
	cfgStruct := &structpb.Struct{}
	_ = cfgStruct.UnmarshalJSON(cfgJSON)
	return cfgStruct
}

func validRequest(now time.Time) *bbpb.RunTaskRequest {
	cfgMap := map[string]any{
		"priority":                   50,
		"service_account":            "sa@service-accounts.com",
		"agent_binary_cipd_pkg":      "pkg",
		"agent_binary_cipd_vers":     "latest",
		"agent_binary_cipd_filename": "bbagent",
		"agent_binary_cipd_server":   "https://cipd.example.com",
		"tags": []any{
			"tag1:v1",
			"tag2:v2",
		},
		"wait_for_capacity": true,
	}
	return &bbpb.RunTaskRequest{
		Target:           "swarming://target",
		BuildId:          "12345678",
		BuildbucketHost:  "buildbucket.example.com",
		BackendConfig:    toBackendCfg(cfgMap),
		GracePeriod:      durationpb.New(time.Minute),
		ExecutionTimeout: durationpb.New(10 * time.Minute),
		StartDeadline:    timestamppb.New(now.Add(5 * time.Minute)),
		Realm:            "project:bucket",
		PubsubTopic:      "pubsub_topic",
		AgentArgs: []string{
			"arg1",
			"arg2",
		},
		Dimensions: []*bbpb.RequestedDimension{
			{
				Key:   "pool",
				Value: "pool",
			},
			{
				Key:        "dim",
				Value:      "dim1",
				Expiration: durationpb.New(time.Minute),
			},
		},
		Caches: []*bbpb.CacheEntry{
			{
				Name:             "cache",
				Path:             "path",
				WaitForWarmCache: durationpb.New(time.Minute),
			},
		},
		Secrets: &bbpb.BuildSecrets{
			StartBuildToken: "token",
		},
		RequestId: "req-bb-12345678",
	}
}
