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

package rpcs

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

func TestGetResult(t *testing.T) {
	t.Parallel()

	ftt.Run("TestGetResult", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		state := NewMockedRequestState()
		state.MockPerm("project:visible-realm", acls.PermTasksGet)
		ctx = MockRequestState(ctx, state)
		srv := TasksServer{}
		reqKey, err := model.TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		assert.NoErr(t, err)
		var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)
		tr := &model.TaskRequest{
			Key:     reqKey,
			TxnUUID: "txn-uuid",
			TaskSlices: []model.TaskSlice{
				{
					Properties: model.TaskProperties{
						Idempotent: true,
						Dimensions: model.TaskDimensions{
							"d1":   {"v1", "v2"},
							"d2":   {"v3"},
							"pool": {"pool"},
							"id":   {"bot123"},
						},
						ExecutionTimeoutSecs: 123,
						GracePeriodSecs:      456,
						IOTimeoutSecs:        789,
						Command:              []string{"run"},
						RelativeCwd:          "./rel/cwd",
						Env: model.Env{
							"k1": "v1",
							"k2": "v2",
						},
						EnvPrefixes: model.EnvPrefixes{
							"p1": {"v1", "v2"},
							"p2": {"v2"},
						},
						Caches: []model.CacheEntry{
							{Name: "n1", Path: "p1"},
							{Name: "n2", Path: "p2"},
						},
						CASInputRoot: model.CASReference{
							CASInstance: "cas-inst",
							Digest: model.CASDigest{
								Hash:      "cas-hash",
								SizeBytes: 1234,
							},
						},
						CIPDInput: model.CIPDInput{
							Server: "server",
							ClientPackage: model.CIPDPackage{
								PackageName: "client-package",
								Version:     "client-version",
							},
							Packages: []model.CIPDPackage{
								{
									PackageName: "pkg1",
									Version:     "ver1",
									Path:        "path1",
								},
								{
									PackageName: "pkg2",
									Version:     "ver2",
									Path:        "path2",
								},
							},
						},
						Outputs:        []string{"o1", "o2"},
						HasSecretBytes: true,
						Containment: model.Containment{
							LowerPriority:             true,
							ContainmentType:           123,
							LimitProcesses:            456,
							LimitTotalCommittedMemory: 789,
						},
					},
					ExpirationSecs:  15 * 60,
					WaitForCapacity: true,
				},
			},
			Created:              testTime,
			Expiration:           testTime.Add(20 * time.Minute),
			Name:                 "name",
			ParentTaskID:         datastore.NewIndexedNullable("parent-task-id"),
			Authenticated:        "authenticated-user@example.com",
			User:                 "user@example.com",
			Tags:                 []string{"tag1", "tag2"},
			ManualTags:           []string{"tag1"},
			ServiceAccount:       "service-account",
			Realm:                "project:visible-realm",
			RealmsEnabled:        true,
			SchedulingAlgorithm:  configpb.Pool_SCHEDULING_ALGORITHM_FIFO,
			Priority:             50,
			BotPingToleranceSecs: 456,
			RBEInstance:          "rbe-instance",
			PubSubTopic:          "pubsub-topic",
			PubSubAuthToken:      "pubsub-auth-token",
			PubSubUserData:       "pubsub-user-data",
			ResultDBUpdateToken:  "resultdb-update-token",
			ResultDB:             model.ResultDBConfig{Enable: true},
			HasBuildTask:         true,
		}
		trs := &model.TaskResultSummary{
			TaskResultCommon: model.TaskResultCommon{
				State:               apipb.TaskState_COMPLETED,
				Modified:            testTime,
				BotVersion:          "bot_version_123",
				BotDimensions:       model.BotDimensions{"os": []string{"linux"}, "cpu": []string{"x86_64"}},
				BotIdleSince:        datastore.NewUnindexedOptional(testTime.Add(-30 * time.Minute)),
				BotLogsCloudProject: "example-cloud-project",
				ServerVersions:      []string{"v1.0"},
				CurrentTaskSlice:    1,
				Started:             datastore.NewIndexedNullable(testTime.Add(-1 * time.Hour)),
				Completed:           datastore.NewIndexedNullable(testTime),
				DurationSecs:        datastore.NewUnindexedOptional(3600.0),
				ExitCode:            datastore.NewUnindexedOptional(int64(0)),
				Failure:             false,
				InternalFailure:     false,
				StdoutChunks:        10,
				CASOutputRoot: model.CASReference{
					CASInstance: "cas-instance",
					Digest: model.CASDigest{
						Hash:      "cas-hash",
						SizeBytes: 1024,
					},
				},
				CIPDPins: model.CIPDInput{
					Server: "https://example.cipd.server",
					ClientPackage: model.CIPDPackage{
						PackageName: "client_pkg",
						Version:     "1.0.0",
						Path:        "client",
					},
				},
				ResultDBInfo: model.ResultDBInfo{
					Hostname:   "results.api.example.dev",
					Invocation: "inv123",
				},
			},
			Key:                  model.TaskResultSummaryKey(ctx, reqKey),
			BotID:                datastore.NewUnindexedOptional("bot123"),
			Created:              testTime.Add(-2 * time.Hour),
			Tags:                 []string{"tag1", "tag2"},
			RequestName:          "example-request",
			RequestUser:          "user@example.com",
			RequestPriority:      50,
			RequestAuthenticated: "authenticated-user@example.com",
			RequestRealm:         "project:visible-realm",
			RequestPool:          "pool",
			RequestBotID:         "bot123",
			PropertiesHash:       datastore.NewIndexedOptional([]byte("prop-hash")),
			TryNumber:            datastore.NewIndexedNullable(int64(1)),
			CostUSD:              0.05,
			CostSavedUSD:         0.00,
			DedupedFrom:          "",
			ExpirationDelay:      datastore.NewUnindexedOptional(0.0),
		}

		assert.NoErr(t, datastore.Put(ctx, tr, trs))

		t.Run("with performance stats", func(t *ftt.Test) {
			ps := &model.PerformanceStats{
				Key:                  model.PerformanceStatsKey(ctx, reqKey),
				BotOverheadSecs:      float64(200),
				CacheTrim:            model.OperationStats{DurationSecs: float64(1)},
				PackageInstallation:  model.OperationStats{DurationSecs: float64(2)},
				NamedCachesInstall:   model.OperationStats{DurationSecs: float64(3)},
				NamedCachesUninstall: model.OperationStats{DurationSecs: float64(4)},
			}
			assert.NoErr(t, datastore.Put(ctx, ps))
			req := &apipb.TaskIdWithPerfRequest{TaskId: "65aba3a3e6b99310", IncludePerformanceStats: true}
			resp, err := srv.GetResult(ctx, req)
			assert.NoErr(t, err)
			assert.Loosely(t, resp, should.Match(&apipb.TaskResultResponse{
				BotDimensions: []*apipb.StringListPair{
					{Key: "cpu", Value: []string{"x86_64"}},
					{Key: "os", Value: []string{"linux"}},
				},
				BotId:               "bot123",
				BotLogsCloudProject: "example-cloud-project",
				BotVersion:          "bot_version_123",
				BotIdleSinceTs:      timestamppb.New(testTime.Add(-30 * time.Minute)),
				CasOutputRoot: &apipb.CASReference{
					CasInstance: "cas-instance",
					Digest: &apipb.Digest{
						Hash:      "cas-hash",
						SizeBytes: 1024,
					},
				},
				CipdPins: &apipb.CipdPins{
					ClientPackage: &apipb.CipdPackage{
						PackageName: "client_pkg",
						Version:     "1.0.0",
						Path:        "client",
					},
				},
				CompletedTs:      timestamppb.New(testTime),
				CostsUsd:         []float32{0.05},
				CreatedTs:        timestamppb.New(testTime.Add(-2 * time.Hour)),
				CurrentTaskSlice: 1,
				Duration:         float32(3600),
				ModifiedTs:       timestamppb.New(testTime),
				Name:             "example-request",
				PerformanceStats: &apipb.PerformanceStats{
					BotOverhead:          200,
					PackageInstallation:  &apipb.OperationStats{Duration: 2},
					CacheTrim:            &apipb.OperationStats{Duration: 1},
					NamedCachesInstall:   &apipb.OperationStats{Duration: 3},
					NamedCachesUninstall: &apipb.OperationStats{Duration: 4},
				},
				ResultdbInfo: &apipb.ResultDBInfo{
					Hostname:   "results.api.example.dev",
					Invocation: "inv123",
				},
				RunId:          "65aba3a3e6b99311",
				ServerVersions: []string{"v1.0"},
				StartedTs:      timestamppb.New(testTime.Add(-1 * time.Hour)),
				State:          apipb.TaskState_COMPLETED,
				Tags:           []string{"tag1", "tag2"},
				TaskId:         "65aba3a3e6b99310",
				User:           "user@example.com",
			}))
		})

		t.Run("no performance stats", func(t *ftt.Test) {
			req := &apipb.TaskIdWithPerfRequest{TaskId: "65aba3a3e6b99310", IncludePerformanceStats: false}
			resp, err := srv.GetResult(ctx, req)
			assert.NoErr(t, err)
			assert.Loosely(t, resp, should.Match(&apipb.TaskResultResponse{
				BotDimensions: []*apipb.StringListPair{
					{Key: "cpu", Value: []string{"x86_64"}},
					{Key: "os", Value: []string{"linux"}},
				},
				BotId:               "bot123",
				BotLogsCloudProject: "example-cloud-project",
				BotVersion:          "bot_version_123",
				BotIdleSinceTs:      timestamppb.New(testTime.Add(-30 * time.Minute)),
				CasOutputRoot: &apipb.CASReference{
					CasInstance: "cas-instance",
					Digest: &apipb.Digest{
						Hash:      "cas-hash",
						SizeBytes: 1024,
					},
				},
				CipdPins: &apipb.CipdPins{
					ClientPackage: &apipb.CipdPackage{
						PackageName: "client_pkg",
						Version:     "1.0.0",
						Path:        "client",
					},
				},
				CompletedTs:      timestamppb.New(testTime),
				CostsUsd:         []float32{0.05},
				CreatedTs:        timestamppb.New(testTime.Add(-2 * time.Hour)),
				CurrentTaskSlice: 1,
				Duration:         float32(3600),
				ModifiedTs:       timestamppb.New(testTime),
				Name:             "example-request",
				ResultdbInfo: &apipb.ResultDBInfo{
					Hostname:   "results.api.example.dev",
					Invocation: "inv123",
				},
				RunId:          "65aba3a3e6b99311",
				ServerVersions: []string{"v1.0"},
				StartedTs:      timestamppb.New(testTime.Add(-1 * time.Hour)),
				State:          apipb.TaskState_COMPLETED,
				Tags:           []string{"tag1", "tag2"},
				TaskId:         "65aba3a3e6b99310",
				User:           "user@example.com",
			}))
		})

		t.Run("no task_id", func(t *ftt.Test) {
			req := &apipb.TaskIdWithPerfRequest{TaskId: "", IncludePerformanceStats: false}
			_, err := srv.GetResult(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("task_id is required"))
		})

		t.Run("error with task_id", func(t *ftt.Test) {
			req := &apipb.TaskIdWithPerfRequest{TaskId: "1", IncludePerformanceStats: false}
			_, err := srv.GetResult(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("task_id 1: bad task ID: too small"))
		})

		t.Run("no such task", func(t *ftt.Test) {
			req := &apipb.TaskIdWithPerfRequest{TaskId: "65aba3a3e6b99320", IncludePerformanceStats: false}
			_, err := srv.GetResult(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such task"))
		})

		t.Run("requestor does not have ACLs", func(t *ftt.Test) {
			reqKey, err := model.TaskIDToRequestKey(ctx, "65aba3a3e6b99320")
			assert.NoErr(t, err)
			trs := model.TaskResultSummary{
				Key:                  model.TaskResultSummaryKey(ctx, reqKey),
				RequestRealm:         "project:no-access-realm",
				RequestPool:          "no-access-pool",
				RequestBotID:         "da bot",
				RequestAuthenticated: "user:someone@notyou.com",
			}
			assert.NoErr(t, datastore.Put(ctx, &trs))
			req := &apipb.TaskIdWithPerfRequest{TaskId: "65aba3a3e6b99320", IncludePerformanceStats: false}
			_, err = srv.GetResult(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("the caller \"user:test@example.com\" doesn't have permission \"swarming.tasks.get\" for the task \"65aba3a3e6b99320\""))
		})
	})
}
