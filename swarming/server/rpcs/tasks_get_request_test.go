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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskRequestToResponse(t *testing.T) {
	t.Parallel()

	Convey("TestTaskRequestToResponse", t, func() {
		ctx := memory.Use(context.Background())

		key, err := model.TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		tr := &model.TaskRequest{
			Key:     key,
			TxnUUID: "txn-uuid",
			TaskSlices: []model.TaskSlice{
				model.TaskSlice{
					Properties: model.TaskProperties{
						Idempotent: true,
						Dimensions: model.TaskDimensions{
							"d1": {"v1", "v2"},
							"d2": {"v1"},
						},
						ExecutionTimeoutSecs: 123,
						GracePeriodSecs:      456,
						IOTimeoutSecs:        789,
						Command:              []string{"run"},
						RelativeCwd:          "./rel/cwd",
						Env: model.Env{
							"k1": "v1",
							"k2": "val",
						},
						EnvPrefixes: model.EnvPrefixes{
							"p1": {"v1", "v2"},
							"p2": {"val"},
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
					ExpirationSecs:  int64(TestTime.Add(10 * time.Minute).Sub(TestTime).Seconds()),
					WaitForCapacity: true,
				},
			},
			Created:              TestTime,
			Expiration:           TestTime.Add(20 * time.Minute),
			Name:                 "name",
			ParentTaskID:         datastore.NewIndexedNullable("parent-task-id"),
			Authenticated:        "user:authenticated",
			User:                 "user",
			Tags:                 []string{"tag1", "tag2"},
			ManualTags:           []string{"tag1"},
			ServiceAccount:       "service-account",
			Realm:                "realm",
			RealmsEnabled:        true,
			SchedulingAlgorithm:  configpb.Pool_SCHEDULING_ALGORITHM_FIFO,
			Priority:             123,
			BotPingToleranceSecs: 456,
			RBEInstance:          "rbe-instance",
			PubSubTopic:          "pubsub-topic",
			PubSubAuthToken:      "pubsub-auth-token",
			PubSubUserData:       "pubsub-user-data",
			ResultDBUpdateToken:  "resultdb-update-token",
			ResultDB:             model.ResultDBConfig{Enable: true},
			HasBuildTask:         true,
		}
		apiReq := &apipb.TaskIdRequest{
			TaskId: "65aba3a3e6b99310",
		}
		Convey("ok", func() {
			So(datastore.Put(ctx, tr), ShouldBeNil)
			resp, err := taskRequestToResponse(ctx, apiReq, tr)
			So(err, ShouldBeNil)
			expectedProperties := &apipb.TaskProperties{
				Caches: []*apipb.CacheEntry{
					{Name: "n1", Path: "p1"},
					{Name: "n2", Path: "p2"},
				},
				CasInputRoot: &apipb.CASReference{
					CasInstance: "cas-inst",
					Digest: &apipb.Digest{
						Hash:      "cas-hash",
						SizeBytes: 1234,
					},
				},
				CipdInput: &apipb.CipdInput{
					Server: "server",
					ClientPackage: &apipb.CipdPackage{
						PackageName: "client-package",
						Version:     "client-version",
					},
					Packages: []*apipb.CipdPackage{
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
				Command:     []string{"run"},
				Containment: &apipb.Containment{ContainmentType: 123},
				Dimensions: []*apipb.StringPair{
					{Key: "d1", Value: "v1"},
					{Key: "d1", Value: "v2"},
					{Key: "d2", Value: "v1"},
				},
				Env: []*apipb.StringPair{
					{Key: "k1", Value: "v1"},
					{Key: "k2", Value: "val"},
				},
				EnvPrefixes: []*apipb.StringListPair{
					{Key: "p1", Value: []string{"v1", "v2"}},
					{Key: "p2", Value: []string{"val"}},
				},
				ExecutionTimeoutSecs: 123,
				GracePeriodSecs:      456,
				Idempotent:           true,
				IoTimeoutSecs:        789,
				Outputs:              []string{"o1", "o2"},
				RelativeCwd:          "./rel/cwd",
				SecretBytes:          []byte("<REDACTED>"),
			}
			So(resp, ShouldResembleProto, &apipb.TaskRequestResponse{
				Authenticated:        "user:authenticated",
				BotPingToleranceSecs: 456,
				CreatedTs:            timestamppb.New(time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)),
				ExpirationSecs:       4,
				Name:                 "name",
				ParentTaskId:         "parent-task-id",
				Priority:             123,
				Properties:           expectedProperties,
				PubsubTopic:          "pubsub-topic",
				PubsubUserdata:       "pubsub-user-data",
				RbeInstance:          "rbe-instance",
				Realm:                "realm",
				Resultdb:             &apipb.ResultDBCfg{Enable: true},
				ServiceAccount:       "service-account",
				Tags:                 []string{"tag1", "tag2"},
				TaskId:               "65aba3a3e6b99310",
				TaskSlices: []*apipb.TaskSlice{
					{
						ExpirationSecs:  600,
						Properties:      expectedProperties,
						WaitForCapacity: true,
					},
				},
				User: "user",
			})
		})
	})
}
