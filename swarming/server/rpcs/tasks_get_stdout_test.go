// Copyright 2024 The LUCI Authors.
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
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetStdout(t *testing.T) {
	t.Parallel()

	Convey("TestGetStdout", t, func() {
		ctx := memory.Use(context.Background())
		state := NewMockedRequestState()
		state.MockPerm("project:visible-realm", acls.PermTasksGet)
		ctx = MockRequestState(ctx, state)
		srv := TasksServer{}
		reqKey, err := model.TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)
		var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)
		tr := &model.TaskRequest{
			Key:     reqKey,
			TxnUUID: "txn-uuid",
			TaskSlices: []model.TaskSlice{
				model.TaskSlice{
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
				StdoutChunks:        1,
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

		Convey("ok; many chunks", func() {
			numChunks := 5
			trs.TaskResultCommon.StdoutChunks = int64(numChunks)
			So(datastore.Put(ctx, tr, trs), ShouldBeNil)
			model.PutMockTaskOutput(ctx, reqKey, numChunks)
			expectedOutputStr := ""
			for i := 0; i < numChunks; i++ {
				expectedOutputStr += strings.Repeat(strconv.Itoa(i), model.ChunkSize)
			}
			resp, err := srv.GetStdout(ctx, &apipb.TaskIdWithOffsetRequest{
				TaskId: "65aba3a3e6b99310",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &apipb.TaskOutputResponse{
				Output: []byte(expectedOutputStr),
				State:  apipb.TaskState_COMPLETED,
			})
		})

		Convey("ok; one chunks", func() {
			numChunks := 1
			trs.TaskResultCommon.StdoutChunks = int64(numChunks)
			So(datastore.Put(ctx, tr, trs), ShouldBeNil)
			model.PutMockTaskOutput(ctx, reqKey, numChunks)
			expectedOutputStr := ""
			for i := 0; i < numChunks; i++ {
				expectedOutputStr += strings.Repeat(strconv.Itoa(i), model.ChunkSize)
			}
			resp, err := srv.GetStdout(ctx, &apipb.TaskIdWithOffsetRequest{
				TaskId: "65aba3a3e6b99310",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &apipb.TaskOutputResponse{
				Output: []byte(expectedOutputStr),
				State:  apipb.TaskState_COMPLETED,
			})
		})

		Convey("ok; offset and length", func() {
			numChunks := 5
			trs.TaskResultCommon.StdoutChunks = int64(numChunks)
			So(datastore.Put(ctx, tr, trs), ShouldBeNil)
			model.PutMockTaskOutput(ctx, reqKey, numChunks)
			expectedOutputStr := ""
			for i := 0; i < numChunks; i++ {
				expectedOutputStr += strings.Repeat(strconv.Itoa(i), model.ChunkSize)
			}
			resp, err := srv.GetStdout(ctx, &apipb.TaskIdWithOffsetRequest{
				TaskId: "65aba3a3e6b99310",
				Offset: 100,
				Length: 1000,
			})
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &apipb.TaskOutputResponse{
				Output: []byte(expectedOutputStr[100:1100]),
				State:  apipb.TaskState_COMPLETED,
			})
		})

		Convey("ok; all missing chunks", func() {
			numChunks := 2
			trs.TaskResultCommon.StdoutChunks = int64(numChunks)
			So(datastore.Put(ctx, tr, trs), ShouldBeNil)
			resp, err := srv.GetStdout(ctx, &apipb.TaskIdWithOffsetRequest{
				TaskId: "65aba3a3e6b99310",
			})
			So(err, ShouldBeNil)
			expectedOutput := bytes.Join([][]byte{
				bytes.Repeat([]byte("\x00"), model.ChunkSize),
				bytes.Repeat([]byte("\x00"), model.ChunkSize),
			}, []byte(""))
			So(resp, ShouldResembleProto, &apipb.TaskOutputResponse{
				Output: expectedOutput,
				State:  apipb.TaskState_COMPLETED,
			})
		})

		Convey("not ok; no task_id", func() {
			resp, err := srv.GetStdout(ctx, &apipb.TaskIdWithOffsetRequest{})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "task_id is required")
			So(resp, ShouldBeNil)
		})

		Convey("not ok; error with task_id", func() {
			resp, err := srv.GetStdout(ctx, &apipb.TaskIdWithOffsetRequest{
				TaskId: "!",
			})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "task_id !: bad task ID: too small")
			So(resp, ShouldBeNil)
		})

		Convey("not ok; no such task", func() {
			resp, err := srv.GetStdout(ctx, &apipb.TaskIdWithOffsetRequest{
				TaskId: "65aba3a3e6b99320",
			})
			So(err, ShouldHaveGRPCStatus, codes.NotFound)
			So(err, ShouldErrLike, "no such task")
			So(resp, ShouldBeNil)
		})

		Convey("not ok; requestor does not have ACLs", func() {
			reqKey, err := model.TaskIDToRequestKey(ctx, "65aba3a3e6b99320")
			So(err, ShouldBeNil)
			trs := model.TaskResultSummary{
				Key:                  model.TaskResultSummaryKey(ctx, reqKey),
				RequestRealm:         "project:no-access-realm",
				RequestPool:          "no-access-pool",
				RequestBotID:         "da bot",
				RequestAuthenticated: "user:someone@notyou.com",
			}
			So(datastore.Put(ctx, &trs), ShouldBeNil)
			resp, err := srv.GetStdout(ctx, &apipb.TaskIdWithOffsetRequest{
				TaskId: "65aba3a3e6b99320",
			})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, "the caller \"user:test@example.com\" doesn't have permission \"swarming.tasks.get\" for the task \"65aba3a3e6b99320\"")
			So(resp, ShouldBeNil)
		})
	})
}
