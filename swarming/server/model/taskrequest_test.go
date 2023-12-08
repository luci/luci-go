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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/swarming/proto/config"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTaskRequest(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	Convey("With datastore", t, func() {
		ctx := memory.Use(context.Background())

		taskSlice := func(val string, exp time.Time) TaskSlice {
			return TaskSlice{
				Properties: TaskProperties{
					Idempotent: true,
					Dimensions: TaskDimensions{
						"d1": {"v1", "v2"},
						"d2": {val},
					},
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
				taskSlice("a", testTime.Add(10*time.Minute)),
				taskSlice("b", testTime.Add(20*time.Minute)),
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
	})
}
