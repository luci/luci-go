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

package bq

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	bqpb "go.chromium.org/luci/swarming/proto/api"
	"go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func createSampleTaskRequest(key *datastore.Key, testTime time.Time) model.TaskRequest {
	taskSlice := func(val string, exp time.Time) model.TaskSlice {
		return model.TaskSlice{
			Properties: model.TaskProperties{
				Idempotent: true,
				Dimensions: model.TaskDimensions{
					"d1": {"v1", "v2"},
					"d2": {val},
				},
				ExecutionTimeoutSecs: 123,
				GracePeriodSecs:      456,
				IOTimeoutSecs:        789,
				Command:              []string{"run", val},
				RelativeCwd:          "./rel/cwd",
				Env: model.Env{
					"k1": "v1",
					"k2": val,
				},
				EnvPrefixes: model.EnvPrefixes{
					"p1": {"v1", "v2"},
					"p2": {val},
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
					ContainmentType:           apipb.ContainmentType_NOT_SPECIFIED,
					LimitProcesses:            456,
					LimitTotalCommittedMemory: 789,
				},
			},
			ExpirationSecs:  int64(exp.Sub(testTime).Seconds()),
			WaitForCapacity: true,
		}
	}
	return model.TaskRequest{
		Key:     key,
		TxnUUID: "txn-uuid",
		TaskSlices: []model.TaskSlice{
			taskSlice("a", testTime.Add(10*time.Minute)),
			taskSlice("b", testTime.Add(20*time.Minute)),
		},
		Created:              testTime,
		Expiration:           testTime.Add(20 * time.Minute),
		Name:                 "name",
		ParentTaskID:         datastore.Nullable[string, datastore.Indexed]{},
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
}

func createSampleBQTaskRequest(taskID string, testTime time.Time) *bqpb.TaskRequest {
	taskSlice := func(val string, exp time.Time) *bqpb.TaskSlice {
		return &bqpb.TaskSlice{
			Properties: &bqpb.TaskProperties{
				Idempotent: true,
				Dimensions: []*bqpb.StringListPair{
					{
						Key:    "d1",
						Values: []string{"v1", "v2"},
					},
					{
						Key:    "d2",
						Values: []string{val},
					},
				},
				ExecutionTimeout: seconds(123),
				GracePeriod:      seconds(456),
				IoTimeout:        seconds(789),
				Command:          []string{"run", val},
				RelativeCwd:      "./rel/cwd",
				Env: []*bqpb.StringPair{
					{
						Key:   "k1",
						Value: "v1",
					},
					{
						Key:   "k2",
						Value: val,
					},
				},
				EnvPaths: []*bqpb.StringListPair{
					{
						Key:    "p1",
						Values: []string{"v1", "v2"},
					},
					{
						Key:    "p2",
						Values: []string{val},
					},
				},
				NamedCaches: []*bqpb.NamedCacheEntry{
					{Name: "n1", DestPath: "p1"},
					{Name: "n2", DestPath: "p2"},
				},
				CasInputRoot: &bqpb.CASReference{
					CasInstance: "cas-inst",
					Digest: &bqpb.Digest{
						Hash:      "cas-hash",
						SizeBytes: 1234,
					},
				},
				CipdInputs: []*bqpb.CIPDPackage{
					{
						PackageName: "pkg1",
						Version:     "ver1",
						DestPath:    "path1",
					},
					{
						PackageName: "pkg2",
						Version:     "ver2",
						DestPath:    "path2",
					},
				},
				Outputs:        []string{"o1", "o2"},
				HasSecretBytes: true,
				Containment: &bqpb.Containment{
					ContainmentType: bqpb.Containment_NOT_SPECIFIED,
				},
			},
			Expiration:      seconds(int64(exp.Sub(testTime).Seconds())),
			WaitForCapacity: true,
		}
	}
	return &bqpb.TaskRequest{
		TaskId: taskID,
		TaskSlices: []*bqpb.TaskSlice{
			taskSlice("a", testTime.Add(10*time.Minute)),
			taskSlice("b", testTime.Add(20*time.Minute)),
		},
		Name:             "name",
		CreateTime:       timestamppb.New(testTime),
		ParentTaskId:     "",
		Authenticated:    "user:authenticated",
		User:             "user",
		Tags:             []string{"tag1", "tag2"},
		ServiceAccount:   "service-account",
		Realm:            "realm",
		Priority:         123,
		BotPingTolerance: seconds(456),
		PubsubNotification: &bqpb.PubSub{
			Topic:    "pubsub-topic",
			Userdata: "pubsub-user-data",
		},
		Resultdb: &bqpb.ResultDBCfg{Enable: true},
	}
}

func TestTaskRequestConversion(t *testing.T) {
	t.Parallel()
	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)
	Convey("Convert TaskRequest with empty parent task", t, func() {
		ctx := memory.Use(context.Background())
		taskID := "65aba3a3e6b99310"
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		So(err, ShouldBeNil)
		sampleRequest := createSampleTaskRequest(key, testTime)
		expected := createSampleBQTaskRequest(taskID, testTime)
		actual := taskRequestToBQ(&sampleRequest)
		So(actual, ShouldResembleProto, expected)
	})

	Convey("Converting empty EnvPrefixes works", t, func() {
		ctx := memory.Use(context.Background())
		taskID := "65aba3a3e6b99310"
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		So(err, ShouldBeNil)
		sampleRequest := createSampleTaskRequest(key, testTime)
		// Set this field to zero
		sampleRequest.TaskSlices[0].Properties.EnvPrefixes = make(model.EnvPrefixes)

		// Set this list to zero too
		expected := createSampleBQTaskRequest(taskID, testTime)
		expected.TaskSlices[0].Properties.EnvPaths = make([]*bqpb.StringListPair, 0)
		actual := taskRequestToBQ(&sampleRequest)
		So(err, ShouldBeNil)
		So(actual, ShouldResembleProto, expected)
	})
}
