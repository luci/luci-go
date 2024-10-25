// Copyright 2022 The LUCI Authors.
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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/tq"
	apipb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func TestTaskDef(t *testing.T) {
	ftt.Run("compute task slice", t, func(t *ftt.Test) {
		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Id: 123,
				SchedulingTimeout: &durationpb.Duration{
					Seconds: 3600,
				},
				ExecutionTimeout: &durationpb.Duration{
					Seconds: 4800,
				},
				GracePeriod: &durationpb.Duration{
					Seconds: 60,
				},
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{},
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Agent: &pb.BuildInfra_Buildbucket_Agent{
							Source: &pb.BuildInfra_Buildbucket_Agent_Source{
								DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
									Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
										Package: "infra/tools/luci/bbagent/${platform}",
										Version: "canary-version",
										Server:  "cipd server",
									},
								},
							},
							CipdClientCache: &pb.CacheEntry{
								Name: "cipd_client_hash",
								Path: "cipd_client",
							},
							CipdPackagesCache: &pb.CacheEntry{
								Name: "cipd_cache_hash",
								Path: "cipd_cache",
							},
						},
					},
				},
			},
		}
		t.Run("only base slice", func(t *ftt.Test) {
			b.Proto.Infra.Swarming = &pb.BuildInfra_Swarming{
				Caches: []*pb.BuildInfra_Swarming_CacheEntry{
					{Name: "shared_builder_cache", Path: "builder"},
				},
				TaskDimensions: []*pb.RequestedDimension{
					{Key: "pool", Value: "Chrome"},
				},
			}
			slices, err := computeTaskSlice(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(slices), should.Equal(1))
			assert.Loosely(t, slices[0].Properties.Caches, should.Resemble([]*apipb.CacheEntry{
				{
					Path: filepath.Join("cache", "builder"),
					Name: "shared_builder_cache",
				},
				{
					Path: filepath.Join("cache", "cipd_client"),
					Name: "cipd_client_hash",
				},
				{
					Path: filepath.Join("cache", "cipd_cache"),
					Name: "cipd_cache_hash",
				},
			}))
			assert.Loosely(t, slices[0].Properties.Dimensions, should.Resemble([]*apipb.StringPair{
				{
					Key:   "pool",
					Value: "Chrome",
				},
			}))
		})

		t.Run("multiple dimensions and cache fallback", func(t *ftt.Test) {
			// Creates 4 task_slices by modifying the buildercfg in 2 ways:
			//  - Add two named caches, one expiring at 60 seconds, one at 360 seconds.
			//  - Add an optional builder dimension, expiring at 120 seconds.
			//
			// This ensures the combination of these features works correctly, and that
			// multiple 'caches' dimensions can be injected.
			b.Proto.Infra.Swarming = &pb.BuildInfra_Swarming{
				Caches: []*pb.BuildInfra_Swarming_CacheEntry{
					{Name: "shared_builder_cache", Path: "builder", WaitForWarmCache: &durationpb.Duration{Seconds: 60}},
					{Name: "second_cache", Path: "second", WaitForWarmCache: &durationpb.Duration{Seconds: 360}},
				},
				TaskDimensions: []*pb.RequestedDimension{
					{Key: "a", Value: "1", Expiration: &durationpb.Duration{Seconds: 120}},
					{Key: "a", Value: "2", Expiration: &durationpb.Duration{Seconds: 120}},
					{Key: "pool", Value: "Chrome"},
				},
			}
			slices, err := computeTaskSlice(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(slices), should.Equal(4))

			// All slices properties fields have the same value except dimensions.
			for _, tSlice := range slices {
				assert.Loosely(t, tSlice.Properties.ExecutionTimeoutSecs, should.Equal(4800))
				assert.Loosely(t, tSlice.Properties.GracePeriodSecs, should.Equal(240))
				assert.Loosely(t, tSlice.Properties.Caches, should.Resemble([]*apipb.CacheEntry{
					{Path: filepath.Join("cache", "builder"), Name: "shared_builder_cache"},
					{Path: filepath.Join("cache", "second"), Name: "second_cache"},
					{Path: filepath.Join("cache", "cipd_client"), Name: "cipd_client_hash"},
					{Path: filepath.Join("cache", "cipd_cache"), Name: "cipd_cache_hash"},
				}))
				assert.Loosely(t, tSlice.Properties.Env, should.Resemble([]*apipb.StringPair{
					{Key: "BUILDBUCKET_EXPERIMENTAL", Value: "FALSE"},
				}))
			}

			assert.Loosely(t, slices[0].ExpirationSecs, should.Equal(60))
			// The dimensions are different. 'a' and 'caches' are injected.
			assert.Loosely(t, slices[0].Properties.Dimensions, should.Resemble([]*apipb.StringPair{
				{Key: "a", Value: "1"},
				{Key: "a", Value: "2"},
				{Key: "caches", Value: "second_cache"},
				{Key: "caches", Value: "shared_builder_cache"},
				{Key: "pool", Value: "Chrome"},
			}))

			// 120 - 60
			assert.Loosely(t, slices[1].ExpirationSecs, should.Equal(60))
			// The dimensions are different. 'a' and 'caches' are injected.
			assert.Loosely(t, slices[1].Properties.Dimensions, should.Resemble([]*apipb.StringPair{
				{Key: "a", Value: "1"},
				{Key: "a", Value: "2"},
				{Key: "caches", Value: "second_cache"},
				{Key: "pool", Value: "Chrome"},
			}))

			// 360 - 120
			assert.Loosely(t, slices[2].ExpirationSecs, should.Equal(240))
			// 'a' expired, one 'caches' remains.
			assert.Loosely(t, slices[2].Properties.Dimensions, should.Resemble([]*apipb.StringPair{
				{Key: "caches", Value: "second_cache"},
				{Key: "pool", Value: "Chrome"},
			}))

			// 3600-360
			assert.Loosely(t, slices[3].ExpirationSecs, should.Equal(3240))
			// # The cold fallback; the last 'caches' expired.
			assert.Loosely(t, slices[3].Properties.Dimensions, should.Resemble([]*apipb.StringPair{
				{Key: "pool", Value: "Chrome"},
			}))
		})
	})

	ftt.Run("compute bbagent command", t, func(t *ftt.Test) {
		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "bbhost.com",
					},
				},
			},
		}
		t.Run("bbagent_getbuild experiment", func(t *ftt.Test) {
			b.Experiments = []string{"+luci.buildbucket.bbagent_getbuild"}
			bbagentCmd := computeCommand(b)
			assert.Loosely(t, bbagentCmd, should.Resemble([]string{
				"bbagent${EXECUTABLE_SUFFIX}",
				"-host",
				"bbhost.com",
				"-build-id",
				"123",
			}))
		})

		t.Run("no bbagent_getbuild experiment", func(t *ftt.Test) {
			b.Experiments = []string{"-luci.buildbucket.bbagent_getbuild"}
			b.Proto.Infra.Bbagent = &pb.BuildInfra_BBAgent{
				CacheDir:    "cache",
				PayloadPath: "payload_path",
			}
			bbagentCmd := computeCommand(b)
			expectedEncoded := bbinput.Encode(&pb.BBAgentArgs{
				Build:       b.Proto,
				CacheDir:    "cache",
				PayloadPath: "payload_path",
			})
			assert.Loosely(t, bbagentCmd, should.Resemble([]string{
				"bbagent${EXECUTABLE_SUFFIX}",
				expectedEncoded,
			}))
		})
	})

	ftt.Run("compute env_prefixes", t, func(t *ftt.Test) {
		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{},
				},
			},
		}
		t.Run("empty swarming cache", func(t *ftt.Test) {
			prefixes := computeEnvPrefixes(b)
			assert.Loosely(t, prefixes, should.Resemble([]*apipb.StringListPair{}))
		})

		t.Run("normal", func(t *ftt.Test) {
			b.Proto.Infra.Swarming.Caches = []*pb.BuildInfra_Swarming_CacheEntry{
				{Path: "vpython", Name: "vpython", EnvVar: "VPYTHON_VIRTUALENV_ROOT"},
				{Path: "abc", Name: "abc", EnvVar: "ABC"},
			}
			prefixes := computeEnvPrefixes(b)
			assert.Loosely(t, prefixes, should.Resemble([]*apipb.StringListPair{
				{Key: "ABC", Value: []string{filepath.Join("cache", "abc")}},
				{Key: "VPYTHON_VIRTUALENV_ROOT", Value: []string{filepath.Join("cache", "vpython")}},
			}))
		})
	})

	ftt.Run("compute swarming new task req", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		ctx, _ = testclock.UseTime(ctx, time.Unix(1444945245, 0).UTC())
		b := &model.Build{
			ID:        123,
			Project:   "project",
			BucketID:  "bucket",
			BuilderID: "builder",
			Proto: &pb.Build{
				Id:     123,
				Number: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{
						Priority:           20,
						TaskServiceAccount: "abc",
						Hostname:           "swarm.com",
					},
					Bbagent: &pb.BuildInfra_BBAgent{},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app-id.appspot.com",
						Agent: &pb.BuildInfra_Buildbucket_Agent{
							Source: &pb.BuildInfra_Buildbucket_Agent_Source{
								DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
									Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
										Package: "infra/tools/luci/bbagent/${platform}",
										Version: "canary-version",
										Server:  "cipd server",
									},
								},
							},
						},
					},
				},
			},
		}

		req, err := computeSwarmingNewTaskReq(ctx, b)
		// Strip out TaskSlices. It has been tested in other tests
		req.TaskSlices = []*apipb.TaskSlice(nil)
		assert.Loosely(t, err, should.BeNil)
		ud, _ := json.Marshal(&userdata{
			BuildID:          123,
			CreatedTS:        1444945245000000,
			SwarmingHostname: "swarm.com",
		})
		expected := &apipb.NewTaskRequest{
			RequestUuid:      "203882df-ce4b-5012-b32a-2c1d29c321a7",
			Name:             "bb-123-builder-1",
			Realm:            "project:bucket",
			Tags:             []string{"buildbucket_bucket:bucket", "buildbucket_build_id:123", "buildbucket_hostname:app-id.appspot.com", "buildbucket_template_canary:0", "luci_project:project"},
			Priority:         int32(20),
			PubsubTopic:      "projects/app-id/topics/swarming-go",
			PubsubUserdata:   string(ud),
			ServiceAccount:   "abc",
			PoolTaskTemplate: apipb.NewTaskRequest_SKIP,
		}
		assert.Loosely(t, req, should.Resemble(expected))
	})
}

func TestSyncBuild(t *testing.T) {
	t.Parallel()
	ftt.Run("SyncBuild", t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		now := testclock.TestRecentTimeUTC
		mockSwarm := clients.NewMockSwarmingClient(ctl)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, &clients.MockSwarmingClientKey, mockSwarm)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx = metrics.WithBuilder(ctx, "proj", "bucket", "builder")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		store := &testsecrets.Store{
			Secrets: map[string]secrets.Secret{
				"key": {Active: []byte("stuff")},
			},
		}
		ctx = secrets.Use(ctx, store)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		metricsStore := tsmon.Store(ctx)

		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Id:         123,
				Status:     pb.Status_SCHEDULED,
				CreateTime: &timestamppb.Timestamp{Seconds: now.UnixNano() / 1000000000},
				SchedulingTimeout: &durationpb.Duration{
					Seconds: 3600,
				},
				ExecutionTimeout: &durationpb.Duration{
					Seconds: 4800,
				},
				GracePeriod: &durationpb.Duration{
					Seconds: 60,
				},
				Builder: &pb.BuilderID{
					Project: "proj",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		}
		inf := &model.BuildInfra{
			ID:    1,
			Build: datastore.KeyForObj(ctx, &model.Build{ID: 123}),
			Proto: &pb.BuildInfra{
				Swarming: &pb.BuildInfra_Swarming{
					Hostname: "swarm",
					Caches: []*pb.BuildInfra_Swarming_CacheEntry{
						{Name: "shared_builder_cache", Path: "builder", WaitForWarmCache: &durationpb.Duration{Seconds: 60}},
						{Name: "second_cache", Path: "second", WaitForWarmCache: &durationpb.Duration{Seconds: 360}},
					},
					TaskDimensions: []*pb.RequestedDimension{
						{Key: "a", Value: "1", Expiration: &durationpb.Duration{Seconds: 120}},
						{Key: "a", Value: "2", Expiration: &durationpb.Duration{Seconds: 120}},
						{Key: "pool", Value: "Chrome"},
					},
				},
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Source: &pb.BuildInfra_Buildbucket_Agent_Source{
							DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
								Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
									Package: "infra/tools/luci/bbagent/${platform}",
									Version: "canary-version",
									Server:  "cipd server",
								},
							},
						},
					},
				},
				Resultdb: &pb.BuildInfra_ResultDB{
					Hostname:   "rdbhost",
					Invocation: "inv",
				},
			},
		}
		bs := &model.BuildStatus{
			Build:  datastore.KeyForObj(ctx, b),
			Status: b.Proto.Status,
		}
		bldr := &model.Builder{
			ID:     "builder",
			Parent: model.BucketKey(ctx, "proj", "bucket"),
			Config: &pb.BuilderConfig{
				MaxConcurrentBuilds: 2,
			},
		}
		assert.Loosely(t, datastore.Put(ctx, b, inf, bs, bldr), should.BeNil)
		t.Run("swarming-build-create", func(t *ftt.Test) {

			t.Run("build not found", func(t *ftt.Test) {
				err := SyncBuild(ctx, 789, 0)
				assert.Loosely(t, err, should.ErrLike("build 789 or buildInfra not found"))
			})

			t.Run("build too old", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					ID:         111,
					CreateTime: now.AddDate(0, 0, -3),
					Proto: &pb.Build{
						Builder: &pb.BuilderID{},
					},
				}), should.BeNil)

				assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
					ID:    1,
					Build: datastore.KeyForObj(ctx, &model.Build{ID: 111}),
				}), should.BeNil)
				err := SyncBuild(ctx, 111, 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
			})

			t.Run("build ended", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					ID:     111,
					Status: pb.Status_SUCCESS,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{},
					},
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
					ID:    1,
					Build: datastore.KeyForObj(ctx, &model.Build{ID: 111}),
				}), should.BeNil)
				err := SyncBuild(ctx, 111, 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
			})

			t.Run("create swarming success", func(t *ftt.Test) {
				mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).Return(&apipb.TaskRequestMetadataResponse{
					TaskId: "task123",
				}, nil)
				err := SyncBuild(ctx, 123, 0)
				assert.Loosely(t, err, should.BeNil)
				updatedBuild := &model.Build{ID: 123}
				updatedInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, updatedBuild)}
				assert.Loosely(t, datastore.Get(ctx, updatedBuild), should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, updatedInfra), should.BeNil)
				assert.Loosely(t, updatedBuild.UpdateToken, should.NotBeEmpty)
				assert.Loosely(t, updatedInfra.Proto.Swarming.TaskId, should.Equal("task123"))
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			})

			t.Run("create swarming http 400 err", func(t *ftt.Test) {
				mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).Return(nil, status.Errorf(codes.PermissionDenied, "PermissionDenied"))
				err := SyncBuild(ctx, 123, 0)
				assert.Loosely(t, err, should.BeNil)
				failedBuild := &model.Build{ID: 123}
				bldStatus := &model.BuildStatus{
					Build: datastore.KeyForObj(ctx, &model.Build{ID: 123}),
				}
				assert.Loosely(t, datastore.Get(ctx, failedBuild, bldStatus), should.BeNil)
				assert.Loosely(t, failedBuild.Status, should.Equal(pb.Status_INFRA_FAILURE))
				assert.Loosely(t, failedBuild.Proto.SummaryMarkdown, should.ContainSubstring("failed to create a swarming task: rpc error: code = PermissionDenied desc = PermissionDenied"))
				assert.Loosely(t, sch.Tasks(), should.HaveLength(4))
				assert.Loosely(t, bldStatus.Status, should.Equal(pb.Status_INFRA_FAILURE))
			})

			t.Run("create swarming http 500 err", func(t *ftt.Test) {
				mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).Return(nil, status.Errorf(codes.Internal, "Server error"))
				err := SyncBuild(ctx, 123, 0)
				assert.Loosely(t, err, should.ErrLike("failed to create a swarming task"))
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
				bld := &model.Build{ID: 123}
				assert.Loosely(t, datastore.Get(ctx, bld), should.BeNil)
				assert.Loosely(t, bld.Status, should.Equal(pb.Status_SCHEDULED))
			})

			t.Run("create swarming http 500 err give up", func(t *ftt.Test) {
				ctx1, _ := testclock.UseTime(ctx, now.Add(swarmingCreateTaskGiveUpTimeout))
				mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).Return(nil, status.Errorf(codes.Internal, "Server error"))
				err := SyncBuild(ctx1, 123, 0)
				assert.Loosely(t, err, should.BeNil)
				failedBuild := &model.Build{ID: 123}
				assert.Loosely(t, datastore.Get(ctx, failedBuild), should.BeNil)
				assert.Loosely(t, failedBuild.Status, should.Equal(pb.Status_INFRA_FAILURE))
				assert.Loosely(t, failedBuild.Proto.SummaryMarkdown, should.ContainSubstring("failed to create a swarming task: rpc error: code = Internal desc = Server error"))
				assert.Loosely(t, sch.Tasks(), should.HaveLength(4))
			})

			t.Run("swarming task creation success but update build fail", func(t *ftt.Test) {
				mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, req *apipb.NewTaskRequest) (*apipb.TaskRequestMetadataResponse, error) {
					// Hack to make the build update fail when trying to update build with the new task ID.
					inf.Proto.Swarming.TaskId = "old task ID"
					assert.Loosely(t, datastore.Put(ctx, inf), should.BeNil)
					return &apipb.TaskRequestMetadataResponse{TaskId: "new task ID"}, nil
				})

				err := SyncBuild(ctx, 123, 0)
				assert.Loosely(t, err, should.ErrLike("failed to update build 123: build already has a task old task ID"))
				currentInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, &model.Build{
					ID: 123,
				})}
				assert.Loosely(t, datastore.Get(ctx, currentInfra), should.BeNil)
				assert.Loosely(t, currentInfra.Proto.Swarming.TaskId, should.Equal("old task ID"))
				assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
			})
		})

		t.Run("swarming sync", func(t *ftt.Test) {
			inf.Proto.Swarming.TaskId = "task_id"
			assert.Loosely(t, datastore.Put(ctx, inf), should.BeNil)

			t.Run("non-existing task ID", func(t *ftt.Test) {
				mockSwarm.EXPECT().GetTaskResult(ctx, "task_id").Return(nil, status.Errorf(codes.NotFound, "Not found"))
				err := syncBuildWithTaskResult(ctx, 123, "task_id", mockSwarm)
				assert.Loosely(t, err, should.BeNil)
				failedBuild := &model.Build{ID: 123}
				assert.Loosely(t, datastore.Get(ctx, failedBuild), should.BeNil)
				assert.Loosely(t, failedBuild.Status, should.Equal(pb.Status_INFRA_FAILURE))
				assert.Loosely(t, failedBuild.Proto.SummaryMarkdown, should.ContainSubstring("invalid swarming task task_id"))
			})

			t.Run("swarming server 500", func(t *ftt.Test) {
				mockSwarm.EXPECT().GetTaskResult(ctx, "task_id").Return(nil, status.Errorf(codes.Internal, "Server error"))
				err := syncBuildWithTaskResult(ctx, 123, "task_id", mockSwarm)
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			})

			t.Run("empty task result", func(t *ftt.Test) {
				mockSwarm.EXPECT().GetTaskResult(ctx, "task_id").Return(nil, nil)
				err := syncBuildWithTaskResult(ctx, 123, "task_id", mockSwarm)
				assert.Loosely(t, err, should.BeNil)
				failedBuild := &model.Build{ID: 123}
				assert.Loosely(t, datastore.Get(ctx, failedBuild), should.BeNil)
				assert.Loosely(t, failedBuild.Status, should.Equal(pb.Status_INFRA_FAILURE))
				assert.Loosely(t, failedBuild.Proto.SummaryMarkdown, should.ContainSubstring("Swarming task task_id unexpectedly disappeared"))
			})

			t.Run("invalid task result state", func(t *ftt.Test) {
				// syncBuildWithTaskResult should return Fatal error
				mockSwarm.EXPECT().GetTaskResult(ctx, "task_id").Return(&apipb.TaskResultResponse{State: apipb.TaskState_INVALID}, nil)
				err := syncBuildWithTaskResult(ctx, 123, "task_id", mockSwarm)
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
				bb := &model.Build{ID: 123}
				assert.Loosely(t, datastore.Get(ctx, bb), should.BeNil)
				assert.Loosely(t, bb.Status, should.Equal(pb.Status_SCHEDULED)) // build status should not been impacted

				// The swarming-build-sync flow shouldn't bubble up the Fatal error.
				// It should ignore and enqueue the next generation of sync task.
				mockSwarm.EXPECT().GetTaskResult(ctx, "task_id").Return(&apipb.TaskResultResponse{State: apipb.TaskState_INVALID}, nil)
				err = SyncBuild(ctx, 123, 1)
				assert.Loosely(t, err, should.BeNil)
				bb = &model.Build{ID: 123}
				assert.Loosely(t, datastore.Get(ctx, bb), should.BeNil)
				assert.Loosely(t, bb.Status, should.Equal(pb.Status_SCHEDULED))
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
				assert.Loosely(t, sch.Tasks().Payloads()[0], should.Resemble(&taskdefs.SyncSwarmingBuildTask{
					BuildId:    123,
					Generation: 2,
				}))
			})

			t.Run("cancel incomplete steps for an ended build", func(t *ftt.Test) {
				mockSwarm.EXPECT().GetTaskResult(ctx, "task_id").Return(&apipb.TaskResultResponse{
					State:       apipb.TaskState_BOT_DIED,
					StartedTs:   &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
					CompletedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
				}, nil)
				steps := model.BuildSteps{
					ID:    1,
					Build: datastore.KeyForObj(ctx, &model.Build{ID: 123}),
				}
				assert.Loosely(t, steps.FromProto([]*pb.Step{
					{Name: "step1", Status: pb.Status_SUCCESS},
					{Name: "step2", Status: pb.Status_STARTED},
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(ctx, &steps), should.BeNil)

				err := syncBuildWithTaskResult(ctx, 123, "task_id", mockSwarm)
				assert.Loosely(t, err, should.BeNil)
				failedBuild := &model.Build{ID: 123}
				assert.Loosely(t, datastore.Get(ctx, failedBuild), should.BeNil)
				assert.Loosely(t, failedBuild.Status, should.Equal(pb.Status_INFRA_FAILURE))
				allSteps := &model.BuildSteps{
					ID:    1,
					Build: datastore.KeyForObj(ctx, &model.Build{ID: 123}),
				}
				assert.Loosely(t, datastore.Get(ctx, allSteps), should.BeNil)
				mSteps, err := allSteps.ToProto(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mSteps, should.Resemble([]*pb.Step{
					{
						Name:   "step1",
						Status: pb.Status_SUCCESS,
					},
					{
						Name:    "step2",
						Status:  pb.Status_CANCELED,
						EndTime: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
				}))
			})

			t.Run("build has output status set to FAILURE", func(t *ftt.Test) {
				fakeTaskResult := &apipb.TaskResultResponse{
					State:   apipb.TaskState_COMPLETED,
					Failure: true,
				}
				mockSwarm.EXPECT().GetTaskResult(ctx, "task567").Return(fakeTaskResult, nil)
				b := &model.Build{
					ID: 567,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreateTime: &timestamppb.Timestamp{Seconds: now.UnixNano() / 1000000000},
						Status:     pb.Status_STARTED,
						Output: &pb.Build_Output{
							Status: pb.Status_FAILURE,
						},
					},
				}
				inf := &model.BuildInfra{
					ID:    1,
					Build: datastore.KeyForObj(ctx, &model.Build{ID: 567}),
					Proto: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{
							Hostname: "swarm",
							TaskId:   "task567",
						},
					},
				}
				bs := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, b),
					Status: b.Proto.Status,
				}
				assert.Loosely(t, datastore.Put(ctx, b, inf, bs), should.BeNil)
				err := SyncBuild(ctx, 567, 1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
				assert.Loosely(t, b.Proto.Status, should.Equal(pb.Status_FAILURE))
			})

			t.Run("build has output status set to CANCELED", func(t *ftt.Test) {
				fakeTaskResult := &apipb.TaskResultResponse{
					State:   apipb.TaskState_COMPLETED,
					Failure: true,
				}
				mockSwarm.EXPECT().GetTaskResult(ctx, "task567").Return(fakeTaskResult, nil)
				b := &model.Build{
					ID: 567,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreateTime: &timestamppb.Timestamp{Seconds: now.UnixNano() / 1000000000},
						Status:     pb.Status_STARTED,
						Output: &pb.Build_Output{
							Status: pb.Status_CANCELED,
						},
					},
				}
				inf := &model.BuildInfra{
					ID:    1,
					Build: datastore.KeyForObj(ctx, &model.Build{ID: 567}),
					Proto: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{
							Hostname: "swarm",
							TaskId:   "task567",
						},
					},
				}
				bs := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, b),
					Status: b.Proto.Status,
				}
				assert.Loosely(t, datastore.Put(ctx, b, inf, bs), should.BeNil)
				err := SyncBuild(ctx, 567, 1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
				assert.Loosely(t, b.Proto.Status, should.Equal(pb.Status_CANCELED))
			})

			t.Run("build has output status set to CANCELED while swarming task succeeded", func(t *ftt.Test) {
				fakeTaskResult := &apipb.TaskResultResponse{
					State: apipb.TaskState_COMPLETED,
				}
				mockSwarm.EXPECT().GetTaskResult(ctx, "task567").Return(fakeTaskResult, nil)
				b := &model.Build{
					ID: 567,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreateTime: &timestamppb.Timestamp{Seconds: now.UnixNano() / 1000000000},
						Status:     pb.Status_STARTED,
						Output: &pb.Build_Output{
							Status: pb.Status_CANCELED,
						},
					},
				}
				inf := &model.BuildInfra{
					ID:    1,
					Build: datastore.KeyForObj(ctx, &model.Build{ID: 567}),
					Proto: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{
							Hostname: "swarm",
							TaskId:   "task567",
						},
					},
				}
				bs := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, b),
					Status: b.Proto.Status,
				}
				assert.Loosely(t, datastore.Put(ctx, b, inf, bs), should.BeNil)
				err := SyncBuild(ctx, 567, 1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
				assert.Loosely(t, b.Proto.Status, should.Equal(pb.Status_CANCELED))
			})

			t.Run("task has no resource", func(t *ftt.Test) {
				fakeTaskResult := &apipb.TaskResultResponse{
					State:       apipb.TaskState_NO_RESOURCE,
					AbandonedTs: &timestamppb.Timestamp{Seconds: now.UnixNano() / 1000000000},
				}
				mockSwarm.EXPECT().GetTaskResult(ctx, "task567").Return(fakeTaskResult, nil)
				b := &model.Build{
					ID: 567,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreateTime: &timestamppb.Timestamp{Seconds: now.UnixNano() / 1000000000},
						Status:     pb.Status_SCHEDULED,
					},
				}
				inf := &model.BuildInfra{
					ID:    1,
					Build: datastore.KeyForObj(ctx, &model.Build{ID: 567}),
					Proto: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{
							Hostname: "swarm",
							TaskId:   "task567",
						},
					},
				}
				bs := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, b),
					Status: b.Proto.Status,
				}
				assert.Loosely(t, datastore.Put(ctx, b, inf, bs), should.BeNil)
				err := SyncBuild(ctx, 567, 1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
				assert.Loosely(t, b.Proto.Status, should.Equal(pb.Status_INFRA_FAILURE))
				assert.Loosely(t, b.Proto.StartTime, should.BeNil)
			})

			var cases = []struct {
				fakeTaskResult *apipb.TaskResultResponse
				expected       *expectedBuildFields
			}{
				{
					fakeTaskResult: &apipb.TaskResultResponse{State: apipb.TaskState_PENDING},
					expected: &expectedBuildFields{
						status: pb.Status_SCHEDULED,
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:     apipb.TaskState_RUNNING,
						StartedTs: &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
					},
					expected: &expectedBuildFields{
						status: pb.Status_STARTED,
						startT: &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:       apipb.TaskState_COMPLETED,
						StartedTs:   &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						CompletedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
					expected: &expectedBuildFields{
						status: pb.Status_SUCCESS,
						startT: &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						endT:   &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:       apipb.TaskState_COMPLETED,
						StartedTs:   &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						CompletedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
						BotDimensions: []*apipb.StringListPair{
							{
								Key:   "os",
								Value: []string{"Ubuntu", "Trusty"},
							},
							{
								Key:   "pool",
								Value: []string{"luci.chromium.try"},
							},
							{
								Key:   "id",
								Value: []string{"bot1"},
							},
							{
								Key: "empty",
							},
						},
					},
					expected: &expectedBuildFields{
						status: pb.Status_SUCCESS,
						startT: &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						endT:   &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
						botDimensions: []*pb.StringPair{
							{Key: "id", Value: "bot1"},
							{Key: "os", Value: "Trusty"},
							{Key: "os", Value: "Ubuntu"},
							{Key: "pool", Value: "luci.chromium.try"},
						},
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:       apipb.TaskState_COMPLETED,
						Failure:     true,
						StartedTs:   &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						CompletedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
					expected: &expectedBuildFields{
						status: pb.Status_INFRA_FAILURE,
						startT: &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						endT:   &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:       apipb.TaskState_BOT_DIED,
						StartedTs:   &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						CompletedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
					expected: &expectedBuildFields{
						status: pb.Status_INFRA_FAILURE,
						startT: &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						endT:   &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:       apipb.TaskState_TIMED_OUT,
						StartedTs:   &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						CompletedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
					expected: &expectedBuildFields{
						status:    pb.Status_INFRA_FAILURE,
						isTimeOut: true,
						startT:    &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
						endT:      &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:       apipb.TaskState_EXPIRED,
						AbandonedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
					expected: &expectedBuildFields{
						status:               pb.Status_INFRA_FAILURE,
						isResourceExhaustion: true,
						isTimeOut:            true,
						endT:                 &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:       apipb.TaskState_KILLED,
						AbandonedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
					expected: &expectedBuildFields{
						status: pb.Status_CANCELED,
						endT:   &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:       apipb.TaskState_NO_RESOURCE,
						AbandonedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
					expected: &expectedBuildFields{
						status:               pb.Status_INFRA_FAILURE,
						isResourceExhaustion: true,
						endT:                 &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
					},
				},
				{
					fakeTaskResult: &apipb.TaskResultResponse{
						State:       apipb.TaskState_NO_RESOURCE,
						AbandonedTs: &timestamppb.Timestamp{Seconds: now.UnixNano() / 1000000000},
					},
					expected: &expectedBuildFields{
						status:               pb.Status_INFRA_FAILURE,
						isResourceExhaustion: true,
						endT:                 &timestamppb.Timestamp{Seconds: now.UnixNano() / 1000000000},
					},
				},
			}
			for i, tCase := range cases {
				t.Run(fmt.Sprintf("test %d - task %s", i, tCase.fakeTaskResult.State), func(t *ftt.Test) {
					mockSwarm.EXPECT().GetTaskResult(ctx, "task_id").Return(tCase.fakeTaskResult, nil)
					err := SyncBuild(ctx, 123, 1)
					assert.Loosely(t, err, should.BeNil)
					syncedBuild := &model.Build{ID: 123}
					assert.Loosely(t, datastore.Get(ctx, syncedBuild), should.BeNil)
					assert.Loosely(t, syncedBuild.Status, should.Equal(tCase.expected.status))
					if tCase.expected.isResourceExhaustion {
						assert.Loosely(t, syncedBuild.Proto.StatusDetails.ResourceExhaustion, should.Resemble(&pb.StatusDetails_ResourceExhaustion{}))
					} else {
						assert.Loosely(t, syncedBuild.Proto.StatusDetails.GetResourceExhaustion(), should.BeNil)
					}
					if tCase.expected.isTimeOut {
						assert.Loosely(t, syncedBuild.Proto.StatusDetails.Timeout, should.Resemble(&pb.StatusDetails_Timeout{}))
					} else {
						assert.Loosely(t, syncedBuild.Proto.StatusDetails.GetTimeout(), should.BeNil)
					}
					if tCase.expected.startT != nil {
						assert.Loosely(t, syncedBuild.Proto.StartTime, should.Resemble(tCase.expected.startT))
					}
					if tCase.expected.endT != nil {
						assert.Loosely(t, syncedBuild.Proto.EndTime, should.Resemble(tCase.expected.endT))
					}
					if tCase.expected.botDimensions != nil {
						syncedInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, syncedBuild)}
						assert.Loosely(t, datastore.Get(ctx, syncedInfra), should.BeNil)
						assert.Loosely(t, syncedInfra.Proto.Swarming.BotDimensions, should.Resemble(tCase.expected.botDimensions))
					}
					if protoutil.IsEnded(syncedBuild.Status) {
						// FinalizeResultDB, ExportBigQuery, NotifyPubSubGoProxy, PopPendingBuilds and a continuation sync task.
						assert.Loosely(t, sch.Tasks(), should.HaveLength(5))

						v2fs := []any{pb.Status_name[int32(syncedBuild.Status)], "None"}
						assert.Loosely(t, metricsStore.Get(ctx, metrics.V2.BuildCountCompleted, v2fs), should.Equal(1))

					} else if syncedBuild.Status == pb.Status_STARTED {
						// NotifyPubSubGoProxy and a continuation sync task.
						assert.Loosely(t, sch.Tasks(), should.HaveLength(2))
						assert.Loosely(t, metricsStore.Get(ctx, metrics.V2.BuildCountStarted, []any{"None"}), should.Equal(1))
					}
					syncedBuildStatus := &model.BuildStatus{Build: datastore.KeyForObj(ctx, syncedBuild)}
					assert.Loosely(t, datastore.Get(ctx, syncedBuildStatus), should.BeNil)
					assert.Loosely(t, syncedBuildStatus.Status, should.Equal(syncedBuild.Proto.Status))
				})
			}
		})
	})

}

func TestHandleCancelSwarmingTask(t *testing.T) {
	t.Parallel()
	ftt.Run("HandleCancelSwarmingTask", t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		now := testclock.TestRecentTimeUTC
		mockSwarm := clients.NewMockSwarmingClient(ctl)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, &clients.MockSwarmingClientKey, mockSwarm)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("wrong", func(t *ftt.Test) {
			t.Run("empty hostname", func(t *ftt.Test) {
				err := HandleCancelSwarmingTask(ctx, "", "task123", "project:bucket")
				assert.Loosely(t, err, should.ErrLike("hostname is empty"))
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})

			t.Run("empty taskID", func(t *ftt.Test) {
				err := HandleCancelSwarmingTask(ctx, "hostname", "", "project:bucket")
				assert.Loosely(t, err, should.ErrLike("taskID is empty"))
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})

			t.Run("wrong realm", func(t *ftt.Test) {
				err := HandleCancelSwarmingTask(ctx, "hostname", "task123", "bad_realm")
				assert.Loosely(t, err, should.ErrLike(`bad global realm name "bad_realm"`))
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})

			t.Run("swarming http 500", func(t *ftt.Test) {
				mockSwarm.EXPECT().CancelTask(ctx, gomock.Any()).Return(nil, status.Errorf(codes.Internal, "swarming internal error"))
				err := HandleCancelSwarmingTask(ctx, "hostname", "task123", "project:bucket")

				assert.Loosely(t, err, should.ErrLike("transient error in cancelling the task task123"))
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			})

			t.Run("swarming http <500", func(t *ftt.Test) {
				mockSwarm.EXPECT().CancelTask(ctx, gomock.Any()).Return(nil, status.Errorf(codes.InvalidArgument, "bad request"))
				err := HandleCancelSwarmingTask(ctx, "hostname", "task123", "project:bucket")

				assert.Loosely(t, err, should.ErrLike("fatal error in cancelling the task task123"))
				assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			})
		})

		t.Run("success", func(t *ftt.Test) {
			t.Run("response.ok", func(t *ftt.Test) {
				mockSwarm.EXPECT().CancelTask(ctx, gomock.Any()).Return(&apipb.CancelResponse{Canceled: true}, nil)
				assert.Loosely(t, HandleCancelSwarmingTask(ctx, "hostname", "task123", "project:bucket"), should.BeNil)
			})

			t.Run("!response.ok", func(t *ftt.Test) {
				mockSwarm.EXPECT().CancelTask(ctx, gomock.Any()).Return(&apipb.CancelResponse{Canceled: false, WasRunning: false}, nil)
				assert.Loosely(t, HandleCancelSwarmingTask(ctx, "hostname", "task123", "project:bucket"), should.BeNil)
			})
		})
	})
}

func TestSubNotify(t *testing.T) {
	t.Parallel()
	ftt.Run("SubNotify", t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		now := testclock.TestRecentTimeUTC
		mockSwarm := clients.NewMockSwarmingClient(ctl)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, &clients.MockSwarmingClientKey, mockSwarm)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx = metrics.WithBuilder(ctx, "proj", "bucket", "builder")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)
		ctx, _ = tq.TestingContext(ctx, nil)
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			"swarming-pubsub-msg-id": cachingtest.NewBlobCache(),
		})
		ctx, sch := tq.TestingContext(ctx, nil)

		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Id:         123,
				Status:     pb.Status_SCHEDULED,
				CreateTime: &timestamppb.Timestamp{Seconds: now.UnixNano() / 1000000000},
				SchedulingTimeout: &durationpb.Duration{
					Seconds: 3600,
				},
				ExecutionTimeout: &durationpb.Duration{
					Seconds: 4800,
				},
				GracePeriod: &durationpb.Duration{
					Seconds: 60,
				},
				Builder: &pb.BuilderID{
					Project: "proj",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		}
		inf := &model.BuildInfra{
			ID:    1,
			Build: datastore.KeyForObj(ctx, &model.Build{ID: 123}),
			Proto: &pb.BuildInfra{
				Swarming: &pb.BuildInfra_Swarming{
					Hostname: "swarm",
					TaskId:   "task123",
					Caches: []*pb.BuildInfra_Swarming_CacheEntry{
						{Name: "shared_builder_cache", Path: "builder", WaitForWarmCache: &durationpb.Duration{Seconds: 60}},
						{Name: "second_cache", Path: "second", WaitForWarmCache: &durationpb.Duration{Seconds: 360}},
					},
					TaskDimensions: []*pb.RequestedDimension{
						{Key: "a", Value: "1", Expiration: &durationpb.Duration{Seconds: 120}},
						{Key: "a", Value: "2", Expiration: &durationpb.Duration{Seconds: 120}},
						{Key: "pool", Value: "Chrome"},
					},
				},
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Source: &pb.BuildInfra_Buildbucket_Agent_Source{
							DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
								Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
									Package: "infra/tools/luci/bbagent/${platform}",
									Version: "canary-version",
									Server:  "cipd server",
								},
							},
						},
					},
				},
			},
		}
		bs := &model.BuildStatus{
			Build:  datastore.KeyForObj(ctx, b),
			Status: b.Proto.Status,
		}
		bldr := &model.Builder{
			ID:     "builder",
			Parent: model.BucketKey(ctx, "proj", "bucket"),
			Config: &pb.BuilderConfig{
				MaxConcurrentBuilds: 2,
			},
		}
		assert.Loosely(t, datastore.Put(ctx, b, inf, bs, bldr), should.BeNil)

		t.Run("bad msg data", func(t *ftt.Test) {
			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          999,
				CreatedTS:        1448841600000,
				SwarmingHostname: "swarm",
			}, "", "msg1")
			err := SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("task_id not found in message data"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)

			body = makeSwarmingPubsubMsg(&userdata{
				CreatedTS:        1448841600000,
				SwarmingHostname: "swarm",
			}, "task123", "msg1")
			err = SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("invalid build_id 0"))

			body = makeSwarmingPubsubMsg(&userdata{
				BuildID:          999,
				SwarmingHostname: "swarm",
			}, "task123", "msg1")
			err = SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("invalid created_ts 0"))

			body = makeSwarmingPubsubMsg(&userdata{
				BuildID:          999,
				CreatedTS:        1448841600000,
				SwarmingHostname: " ",
			}, "task123", "msg1")
			err = SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("swarming hostname not found in userdata"))

			body = makeSwarmingPubsubMsg(&userdata{
				BuildID:          999,
				CreatedTS:        1448841600000,
				SwarmingHostname: "https://swarm.com",
			}, "task123", "msg1")
			err = SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("swarming hostname https://swarm.com must not contain '://'"))
		})

		t.Run("build not found", func(t *ftt.Test) {
			old := now.Add(-time.Minute).UnixNano() / int64(time.Microsecond)
			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          999,
				CreatedTS:        old,
				SwarmingHostname: "swarm",
			}, "task123", "msg1")
			err := SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("Build 999 or BuildInfra for task https://swarm/task?id=task123 not found"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)

			recent := now.Add(-50*time.Second).UnixNano() / int64(time.Microsecond)
			body = makeSwarmingPubsubMsg(&userdata{
				BuildID:          999,
				CreatedTS:        recent,
				SwarmingHostname: "swarm",
			}, "task123", "msg1")
			err = SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("Build 999 or BuildInfra for task https://swarm/task?id=task123 not found yet"))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run("different swarming hostname", func(t *ftt.Test) {

			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          123,
				CreatedTS:        1517260502000000,
				SwarmingHostname: "swarm2",
			}, "task123", "msg1")
			err := SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("swarming_hostname swarm of build 123 does not match swarm2"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run("different task id", func(t *ftt.Test) {
			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          123,
				CreatedTS:        1517260502000000,
				SwarmingHostname: "swarm",
			}, "task345", "msg1")
			err := SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("swarming_task_id task123 of build 123 does not match task345"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run("swarming 500s error", func(t *ftt.Test) {
			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          123,
				CreatedTS:        1517260502000000,
				SwarmingHostname: "swarm",
			}, "task123", "msg1")
			mockSwarm.EXPECT().GetTaskResult(ctx, "task123").Return(nil, status.Errorf(codes.Internal, "swarming internal error"))
			err := SubNotify(ctx, body)
			assert.Loosely(t, err, should.ErrLike("rpc error: code = Internal desc = swarming internal error"))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)

			cache := caching.GlobalCache(ctx, "swarming-pubsub-msg-id")
			_, err = cache.Get(ctx, "msg1")
			assert.Loosely(t, err, should.Equal(caching.ErrCacheMiss))
		})

		t.Run("status already ended", func(t *ftt.Test) {
			b.Proto.Status = pb.Status_SUCCESS
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)

			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          123,
				CreatedTS:        1517260502000000,
				SwarmingHostname: "swarm",
			}, "task123", "msg1")
			err := SubNotify(ctx, body)
			assert.Loosely(t, err, should.BeNil)
			mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).Times(0)

			assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
		})

		t.Run("status changed to success", func(t *ftt.Test) {
			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          123,
				CreatedTS:        1517260502000000,
				SwarmingHostname: "swarm",
			}, "task123", "msg1")
			mockSwarm.EXPECT().GetTaskResult(ctx, "task123").Return(&apipb.TaskResultResponse{
				State:       apipb.TaskState_COMPLETED,
				StartedTs:   &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
				CompletedTs: &timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000},
				BotDimensions: []*apipb.StringListPair{
					{
						Key:   "new_key",
						Value: []string{"new_val"},
					},
				},
			}, nil)
			err := SubNotify(ctx, body)
			assert.Loosely(t, err, should.BeNil)
			syncedBuild := &model.Build{ID: 123}
			assert.Loosely(t, datastore.Get(ctx, syncedBuild), should.BeNil)
			assert.Loosely(t, syncedBuild.Status, should.Equal(pb.Status_SUCCESS))
			assert.Loosely(t, syncedBuild.Proto.StartTime, should.Resemble(&timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000}))
			assert.Loosely(t, syncedBuild.Proto.EndTime, should.Resemble(&timestamppb.Timestamp{Seconds: 1517271318, Nanos: 162860000}))

			syncedInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, syncedBuild)}
			assert.Loosely(t, datastore.Get(ctx, syncedInfra), should.BeNil)
			assert.Loosely(t, syncedInfra.Proto.Swarming.BotDimensions, should.Resemble([]*pb.StringPair{
				{
					Key:   "new_key",
					Value: "new_val",
				},
			}))
			syncedBuildStatus := &model.BuildStatus{Build: datastore.KeyForObj(ctx, syncedBuild)}
			assert.Loosely(t, datastore.Get(ctx, syncedBuildStatus), should.BeNil)
			assert.Loosely(t, syncedBuildStatus.Status, should.Equal(pb.Status_SUCCESS))

			cache := caching.GlobalCache(ctx, "swarming-pubsub-msg-id")
			cached, err := cache.Get(ctx, "msg1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cached, should.Resemble([]byte{1}))

			// ExportBigQuery, NotifyPubSub, NotifyPubSubGoProxy, PopPendingBuilds tasks.
			assert.Loosely(t, sch.Tasks(), should.HaveLength(3))

			// BuildCompleted metric should be set to 1 with SUCCESS.
			v2fs := []any{pb.Status_name[int32(syncedBuild.Status)], "None"}
			assert.Loosely(t, store.Get(ctx, metrics.V2.BuildCountCompleted, v2fs), should.Equal(1))
		})

		t.Run("status unchanged(in STARTED) while bot dimensions changed", func(t *ftt.Test) {
			b.Proto.Status = pb.Status_STARTED
			bs.Status = b.Proto.Status
			assert.Loosely(t, datastore.Put(ctx, b, bs), should.BeNil)
			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          123,
				CreatedTS:        1517260502000000,
				SwarmingHostname: "swarm",
			}, "task123", "msg1")
			mockSwarm.EXPECT().GetTaskResult(ctx, "task123").Return(&apipb.TaskResultResponse{
				State:     apipb.TaskState_RUNNING,
				StartedTs: &timestamppb.Timestamp{Seconds: 1517260502, Nanos: 649750000},
				BotDimensions: []*apipb.StringListPair{
					{
						Key:   "new_key",
						Value: []string{"new_val"},
					},
				},
			}, nil)
			err := SubNotify(ctx, body)
			assert.Loosely(t, err, should.BeNil)
			syncedBuild := &model.Build{ID: 123}
			assert.Loosely(t, datastore.Get(ctx, syncedBuild), should.BeNil)
			assert.Loosely(t, syncedBuild.Status, should.Equal(pb.Status_STARTED))

			syncedInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, syncedBuild)}
			assert.Loosely(t, datastore.Get(ctx, syncedInfra), should.BeNil)
			assert.Loosely(t, syncedInfra.Proto.Swarming.BotDimensions, should.Resemble([]*pb.StringPair{{
				Key:   "new_key",
				Value: "new_val",
			}}))
			syncedBuildStatus := &model.BuildStatus{Build: datastore.KeyForObj(ctx, syncedBuild)}
			assert.Loosely(t, datastore.Get(ctx, syncedBuildStatus), should.BeNil)
			assert.Loosely(t, syncedBuildStatus.Status, should.Equal(pb.Status_STARTED))

			assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
		})

		t.Run("status unchanged(not in STARTED) while bot dimensions changed", func(t *ftt.Test) {
			b.Proto.Status = pb.Status_STARTED
			bs.Status = b.Proto.Status
			assert.Loosely(t, datastore.Put(ctx, b, bs), should.BeNil)
			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          123,
				CreatedTS:        1517260502000000,
				SwarmingHostname: "swarm",
			}, "task123", "msg1")
			mockSwarm.EXPECT().GetTaskResult(ctx, "task123").Return(&apipb.TaskResultResponse{
				State: apipb.TaskState_PENDING,
				BotDimensions: []*apipb.StringListPair{
					{
						Key:   "new_key",
						Value: []string{"new_val"},
					},
				},
			}, nil)
			err := SubNotify(ctx, body)
			assert.Loosely(t, err, should.BeNil)
			syncedBuild := &model.Build{ID: 123}
			assert.Loosely(t, datastore.Get(ctx, syncedBuild), should.BeNil)
			assert.Loosely(t, syncedBuild.Status, should.Equal(pb.Status_STARTED))

			currentInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, syncedBuild)}
			assert.Loosely(t, datastore.Get(ctx, currentInfra), should.BeNil)
			assert.Loosely(t, currentInfra.Proto.Swarming.BotDimensions, should.BeEmpty)

			syncedBuildStatus := &model.BuildStatus{Build: datastore.KeyForObj(ctx, syncedBuild)}
			assert.Loosely(t, datastore.Get(ctx, syncedBuildStatus), should.BeNil)
			assert.Loosely(t, syncedBuildStatus.Status, should.Equal(pb.Status_STARTED))

			assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
		})

		t.Run("duplicate message", func(t *ftt.Test) {
			cache := caching.GlobalCache(ctx, "swarming-pubsub-msg-id")
			err := cache.Set(ctx, "msg123", []byte{1}, 0*time.Second)
			assert.Loosely(t, err, should.BeNil)

			body := makeSwarmingPubsubMsg(&userdata{
				BuildID:          123,
				CreatedTS:        1517260502000000,
				SwarmingHostname: "swarm",
			}, "task123", "msg123")
			mockSwarm.EXPECT().GetTaskResult(ctx, "task_id").Times(0)
			err = SubNotify(ctx, body)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func makeSwarmingPubsubMsg(userdata *userdata, taskID string, msgID string) io.Reader {
	ud, _ := json.Marshal(userdata)
	data := struct {
		TaskID   string `json:"task_id"`
		Userdata string `json:"userdata"`
	}{TaskID: taskID, Userdata: string(ud)}
	bd, _ := json.Marshal(data)
	msg := struct {
		Message struct {
			Data      string
			MessageID string
		}
	}{struct {
		Data      string
		MessageID string
	}{Data: base64.StdEncoding.EncodeToString(bd), MessageID: msgID}}
	jmsg, _ := json.Marshal(msg)
	return bytes.NewReader(jmsg)
}

type expectedBuildFields struct {
	status               pb.Status
	startT               *timestamppb.Timestamp
	endT                 *timestamppb.Timestamp
	isTimeOut            bool
	isResourceExhaustion bool
	botDimensions        []*pb.StringPair
}
