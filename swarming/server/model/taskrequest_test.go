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

package model

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskRequest(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	Convey("With datastore", t, func() {
		ctx := memory.Use(context.Background())

		taskSlice := func(val string, exp time.Time, pool, botID string) TaskSlice {
			dims := TaskDimensions{
				"d1": {"v1", "v2"},
				"d2": {val},
			}
			if pool != "" {
				dims["pool"] = []string{pool}
			}
			if botID != "" {
				dims["id"] = []string{botID}
			}
			return TaskSlice{
				Properties: TaskProperties{
					Idempotent:           true,
					Dimensions:           dims,
					ExecutionTimeoutSecs: 123,
					GracePeriodSecs:      456,
					IOTimeoutSecs:        789,
					Command:              []string{"run", val},
					RelativeCwd:          "./rel/cwd",
					Env: Env{
						"k1": "v1",
						"k2": val,
					},
					EnvPrefixes: EnvPrefixes{
						"p1": {"v1", "v2"},
						"p2": {val},
					},
					Caches: []CacheEntry{
						{Name: "n1", Path: "p1"},
						{Name: "n2", Path: "p2"},
					},
					CASInputRoot: CASReference{
						CASInstance: "cas-inst",
						Digest: CASDigest{
							Hash:      "cas-hash",
							SizeBytes: 1234,
						},
					},
					CIPDInput: CIPDInput{
						Server: "server",
						ClientPackage: CIPDPackage{
							PackageName: "client-package",
							Version:     "client-version",
						},
						Packages: []CIPDPackage{
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
					Containment: Containment{
						LowerPriority:             true,
						ContainmentType:           123,
						LimitProcesses:            456,
						LimitTotalCommittedMemory: 789,
					},
				},
				ExpirationSecs:  int64(exp.Sub(testTime).Seconds()),
				WaitForCapacity: true,
			}
		}

		key, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		fullyPopulated := TaskRequest{
			Key:     key,
			TxnUUID: "txn-uuid",
			TaskSlices: []TaskSlice{
				taskSlice("a", testTime.Add(10*time.Minute), "pool", "botID"),
				taskSlice("b", testTime.Add(20*time.Minute), "pool", "botID"),
			},
			Created:              testTime,
			Expiration:           testTime.Add(20 * time.Minute),
			Name:                 "name",
			ParentTaskID:         datastore.NewIndexedNullable("parent-task-id"),
			Authenticated:        "authenticated",
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
			ResultDB:             ResultDBConfig{Enable: true},
			HasBuildTask:         true,
		}

		// Can round-trip.
		So(datastore.Put(ctx, &fullyPopulated), ShouldBeNil)
		loaded := TaskRequest{Key: key}
		So(datastore.Get(ctx, &loaded), ShouldBeNil)
		So(loaded, ShouldResemble, fullyPopulated)

		partiallyPopulated := TaskRequest{
			Key:     key,
			TxnUUID: "txn-uuid",
			TaskSlices: []TaskSlice{
				taskSlice("a", testTime.Add(10*time.Minute), "", ""),
				taskSlice("b", testTime.Add(20*time.Minute), "", ""),
			},
			Created:              testTime,
			Expiration:           testTime.Add(20 * time.Minute),
			Name:                 "name",
			ParentTaskID:         datastore.NewIndexedNullable("parent-task-id"),
			Authenticated:        "authenticated",
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
			ResultDB:             ResultDBConfig{Enable: true},
			HasBuildTask:         true,
		}

		Convey("TestGetters", func() {
			Convey("ok", func() {
				So(loaded.Pool(), ShouldEqual, "pool")
				So(loaded.BotID(), ShouldEqual, "botID")
			})
			Convey("nil", func() {
				So(partiallyPopulated.Pool(), ShouldEqual, "")
				So(partiallyPopulated.BotID(), ShouldEqual, "")
			})
		})
	})
}

func TestTaskRequestToProto(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	Convey("ToProto", t, func() {
		ctx := memory.Use(context.Background())

		key, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		tr := &TaskRequest{
			Key:     key,
			TxnUUID: "txn-uuid",
			TaskSlices: []TaskSlice{
				TaskSlice{
					Properties: TaskProperties{
						Idempotent: true,
						Dimensions: TaskDimensions{
							"d1": {"v1", "v2"},
							"d2": {"v1"},
						},
						ExecutionTimeoutSecs: 123,
						GracePeriodSecs:      456,
						IOTimeoutSecs:        789,
						Command:              []string{"run"},
						RelativeCwd:          "./rel/cwd",
						Env: Env{
							"k1": "v1",
							"k2": "val",
						},
						EnvPrefixes: EnvPrefixes{
							"p1": {"v1", "v2"},
							"p2": {"val"},
						},
						Caches: []CacheEntry{
							{Name: "n1", Path: "p1"},
							{Name: "n2", Path: "p2"},
						},
						CASInputRoot: CASReference{
							CASInstance: "cas-inst",
							Digest: CASDigest{
								Hash:      "cas-hash",
								SizeBytes: 1234,
							},
						},
						CIPDInput: CIPDInput{
							Server: "server",
							ClientPackage: CIPDPackage{
								PackageName: "client-package",
								Version:     "client-version",
							},
							Packages: []CIPDPackage{
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
						Containment: Containment{
							LowerPriority:             true,
							ContainmentType:           123,
							LimitProcesses:            456,
							LimitTotalCommittedMemory: 789,
						},
					},
					ExpirationSecs:  600,
					WaitForCapacity: true,
				},
			},
			Created:              testTime,
			Expiration:           testTime.Add(10 * time.Minute),
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
			ResultDB:             ResultDBConfig{Enable: true},
			HasBuildTask:         true,
		}
		resp := tr.ToProto()
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
			ExpirationSecs:       600,
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
}

func TestTaskSlice(t *testing.T) {
	t.Parallel()

	// This testcase will cover the TaskProperties.ToProto() function as well
	// since TaskProperties is a field in TaskSlice.
	Convey("ToProto", t, func() {
		Convey("null", func() {
			ts := TaskSlice{}
			So(ts.ToProto(), ShouldResembleProto, &apipb.TaskSlice{})
		})
		Convey("fullyPopulated", func() {
			ts := TaskSlice{
				Properties: TaskProperties{
					Idempotent:           true,
					Dimensions:           TaskDimensions{},
					ExecutionTimeoutSecs: 123,
					GracePeriodSecs:      456,
					IOTimeoutSecs:        789,
					Command:              []string{"run"},
					RelativeCwd:          "./rel/cwd",
					Env: Env{
						"k1": "v1",
						"k2": "v2",
					},
					EnvPrefixes: EnvPrefixes{
						"p1": {"v1", "v2"},
						"p2": {"v3"},
					},
					Caches: []CacheEntry{
						{Name: "n1", Path: "p1"},
						{Name: "n2", Path: "p2"},
					},
					CASInputRoot: CASReference{
						CASInstance: "cas-inst",
						Digest: CASDigest{
							Hash:      "cas-hash",
							SizeBytes: 1234,
						},
					},
					CIPDInput: CIPDInput{
						Server: "server",
						ClientPackage: CIPDPackage{
							PackageName: "client-package",
							Version:     "client-version",
						},
						Packages: []CIPDPackage{
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
					Outputs: []string{"o1", "o2"},
					Containment: Containment{
						LowerPriority:             true,
						ContainmentType:           123,
						LimitProcesses:            456,
						LimitTotalCommittedMemory: 789,
					},
				},
				ExpirationSecs:  2,
				WaitForCapacity: true,
			}
			expected := &apipb.TaskSlice{
				ExpirationSecs:  2,
				WaitForCapacity: true,
				Properties: &apipb.TaskProperties{
					Idempotent:           true,
					Dimensions:           []*apipb.StringPair{},
					ExecutionTimeoutSecs: 123,
					GracePeriodSecs:      456,
					IoTimeoutSecs:        789,
					Command:              []string{"run"},
					RelativeCwd:          "./rel/cwd",
					Outputs:              []string{"o1", "o2"},
					Containment: &apipb.Containment{
						ContainmentType: 123,
					},
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
					Env: []*apipb.StringPair{
						{Key: "k1", Value: "v1"},
						{Key: "k2", Value: "v2"},
					},
					EnvPrefixes: []*apipb.StringListPair{
						{Key: "p1", Value: []string{"v1", "v2"}},
						{Key: "p2", Value: []string{"v3"}},
					},
				},
			}
			Convey("HasSecretBytes is true", func() {
				ts.Properties.HasSecretBytes = true
				expected.Properties.SecretBytes = []byte("<REDACTED>")
				So(ts.ToProto(), ShouldResembleProto, expected)
			})
			Convey("HasSecretBytes is false", func() {
				ts.Properties.HasSecretBytes = false
				So(ts.ToProto(), ShouldResembleProto, expected)
			})
		})
	})
}

func TestTaskDimensions(t *testing.T) {
	t.Parallel()
	Convey("Hash", t, func() {
		Convey("dimensions keys are sorted when hash", func() {
			dims1 := TaskDimensions{
				"d1": {"v1", "v2"},
				"d2": {"v3"},
			}
			dims2 := TaskDimensions{
				"d2": {"v3"},
				"d1": {"v1", "v2"},
			}
			So(dims1.Hash(), ShouldEqual, dims2.Hash())
			So(dims1.Hash(), ShouldEqual, 2036451960)
		})

		Convey("empty dims", func() {
			var dims1 TaskDimensions
			dims2 := TaskDimensions{}
			So(dims1.Hash(), ShouldEqual, dims2.Hash())
		})

		Convey("dims with OR-ed dimensions", func() {
			dims1 := TaskDimensions{
				"d1": {"v1|v2"},
			}
			dims2 := TaskDimensions{
				"d1": {"v2|v1"},
			}
			So(dims1.Hash(), ShouldNotEqual, dims2.Hash())
		})
	})
}
