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
	"encoding/hex"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
)

func createTaskSlice(val string, testTime, exp time.Time, pool, botID string) TaskSlice {
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

func TestTaskRequest(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		key, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		assert.NoErr(t, err)

		fullyPopulated := TaskRequest{
			Key:     key,
			TxnUUID: "txn-uuid",
			TaskSlices: []TaskSlice{
				createTaskSlice("a", testTime, testTime.Add(10*time.Minute), "pool", "botID"),
				createTaskSlice("b", testTime, testTime.Add(20*time.Minute), "pool", "botID"),
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
		assert.NoErr(t, datastore.Put(ctx, &fullyPopulated))
		loaded := TaskRequest{Key: key}
		assert.NoErr(t, datastore.Get(ctx, &loaded))
		assert.Loosely(t, loaded, should.Resemble(fullyPopulated))

		partiallyPopulated := TaskRequest{
			Key:     key,
			TxnUUID: "txn-uuid",
			TaskSlices: []TaskSlice{
				createTaskSlice("a", testTime, testTime.Add(10*time.Minute), "", ""),
				createTaskSlice("b", testTime, testTime.Add(20*time.Minute), "", ""),
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

		t.Run("TestGetters", func(t *ftt.Test) {
			t.Run("ok", func(t *ftt.Test) {
				assert.Loosely(t, loaded.Pool(), should.Equal("pool"))
				assert.Loosely(t, loaded.BotID(), should.Equal("botID"))
			})
			t.Run("nil", func(t *ftt.Test) {
				assert.Loosely(t, partiallyPopulated.Pool(), should.BeEmpty)
				assert.Loosely(t, partiallyPopulated.BotID(), should.BeEmpty)
			})
		})

		t.Run("TestExecutionDeadline", func(t *ftt.Test) {
			expected := testTime.Add(time.Duration(600+1200+123+456) * time.Second)
			assert.That(t, fullyPopulated.ExecutionDeadline(), should.Match(expected))
		})
	})

	ftt.Run("NewTaskRequestKey", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		now := time.Date(2025, time.January, 1, 2, 3, 4, 5, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)
		ctx = cryptorand.MockForTest(ctx, 0)

		key := NewTaskRequestKey(ctx)
		assert.That(t, RequestKeyToTaskID(key, AsRequest),
			should.Equal("6e386bafc012fa10"))
	})
}

func TestTaskRequestToProto(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	ftt.Run("ToProto", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		key, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		assert.NoErr(t, err)

		tr := &TaskRequest{
			Key:     key,
			TxnUUID: "txn-uuid",
			TaskSlices: []TaskSlice{
				{
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
		assert.Loosely(t, resp, should.Resemble(&apipb.TaskRequestResponse{
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
		}))
	})
}

func TestTaskSlice(t *testing.T) {
	t.Parallel()

	// This testcase will cover the TaskProperties.ToProto() function as well
	// since TaskProperties is a field in TaskSlice.
	ftt.Run("ToProto", t, func(t *ftt.Test) {
		t.Run("null", func(t *ftt.Test) {
			ts := TaskSlice{}
			assert.Loosely(t, ts.ToProto(), should.Resemble(&apipb.TaskSlice{}))
		})
		t.Run("fullyPopulated", func(t *ftt.Test) {
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
			t.Run("HasSecretBytes is true", func(t *ftt.Test) {
				ts.Properties.HasSecretBytes = true
				expected.Properties.SecretBytes = []byte("<REDACTED>")
				assert.Loosely(t, ts.ToProto(), should.Match(expected))
			})
			t.Run("HasSecretBytes is false", func(t *ftt.Test) {
				ts.Properties.HasSecretBytes = false
				assert.Loosely(t, ts.ToProto(), should.Match(expected))
			})
		})
	})

	ftt.Run("PrecalculatePropertiesHash", t, func(t *ftt.Test) {
		t.Run("return_exist", func(t *ftt.Test) {
			ts := &TaskSlice{
				PropertiesHash: []byte("hash_is_already_here"),
			}
			assert.NoErr(t, ts.PrecalculatePropertiesHash(nil))
			assert.That(t, ts.PropertiesHash, should.Match([]byte("hash_is_already_here")))
		})

		t.Run("properties_has_secret_bytes_while_none_given", func(t *ftt.Test) {
			ts := &TaskSlice{
				Properties: TaskProperties{
					HasSecretBytes: true,
				},
			}
			assert.That(
				t, ts.PrecalculatePropertiesHash(nil),
				should.ErrLike("properties should have secret bytes but none is provided or vice versa"))
		})

		t.Run("properties_does_not_have_secret_bytes_while_one_given", func(t *ftt.Test) {
			ts := &TaskSlice{}
			assert.That(
				t, ts.PrecalculatePropertiesHash(&SecretBytes{SecretBytes: []byte("secret")}),
				should.ErrLike("properties should have secret bytes but none is provided or vice versa"))
		})

		t.Run("works", func(t *ftt.Test) {
			t.Run("without_secret_bytes", func(t *ftt.Test) {
				ts := &TaskSlice{
					Properties: TaskProperties{
						GracePeriodSecs: 30,
						EnvPrefixes: EnvPrefixes{
							"p1": {"v2", "v1"},
						},
						Dimensions: TaskDimensions{
							"d1":   {"v1", "v2"},
							"pool": {"pool"},
						},
					},
				}
				assert.NoErr(t, ts.PrecalculatePropertiesHash(nil))
				expected := "079e5c6f0bb17d036cc3196c89d2d3ef15fea09d85e7a69a8ba7fe69cf5a7afd"
				assert.That(t, hex.EncodeToString(ts.PropertiesHash[:]), should.Equal(expected))
			})

			t.Run("with_secret_bytes", func(t *ftt.Test) {
				ts := &TaskSlice{
					Properties: TaskProperties{
						GracePeriodSecs: 30,
						EnvPrefixes: EnvPrefixes{
							"p1": {"v2", "v1"},
						},
						Dimensions: TaskDimensions{
							"d1":   {"v1", "v2"},
							"pool": {"pool"},
						},
						HasSecretBytes: true,
					},
				}
				sb := []byte("secret")
				assert.NoErr(t, ts.PrecalculatePropertiesHash(&SecretBytes{SecretBytes: sb}))

				expected := "b114a396a92ef47203df9f2927e73a90b1bf6e4573daa4e714c10d1394c8da05"
				assert.That(t, hex.EncodeToString(ts.PropertiesHash[:]), should.Equal(expected))
			})
		})
	})
}

func TestTaskProperties(t *testing.T) {
	t.Parallel()
	ftt.Run("IsTerminate", t, func(t *ftt.Test) {
		t.Run("no", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				p := TaskProperties{}
				assert.Loosely(t, p.IsTerminate(), should.BeFalse)
			})
			t.Run("with_timeout", func(t *ftt.Test) {
				p := TaskProperties{
					ExecutionTimeoutSecs: 123,
				}
				assert.Loosely(t, p.IsTerminate(), should.BeFalse)
			})
			t.Run("with_command", func(t *ftt.Test) {
				p := TaskProperties{
					Command: []string{"cmd"},
				}
				assert.Loosely(t, p.IsTerminate(), should.BeFalse)
			})
			t.Run("with_caches", func(t *ftt.Test) {
				p := TaskProperties{
					Caches: []CacheEntry{
						{
							Name: "name",
							Path: "path",
						},
					},
				}
				assert.Loosely(t, p.IsTerminate(), should.BeFalse)
			})
			t.Run("with_cipd_input", func(t *ftt.Test) {
				p := TaskProperties{
					CIPDInput: CIPDInput{
						Server: "server",
					},
				}
				assert.Loosely(t, p.IsTerminate(), should.BeFalse)
			})
			t.Run("has_secret_bytess", func(t *ftt.Test) {
				p := TaskProperties{
					HasSecretBytes: true,
				}
				assert.Loosely(t, p.IsTerminate(), should.BeFalse)
			})
			t.Run("with_non_id_dimension", func(t *ftt.Test) {
				p := TaskProperties{
					Dimensions: TaskDimensions{
						"pool": {"pool"},
					},
				}
				assert.Loosely(t, p.IsTerminate(), should.BeFalse)
			})
		})
		t.Run("yes", func(t *ftt.Test) {
			p := TaskProperties{
				Dimensions: TaskDimensions{
					"id": {"id"},
				},
			}
			assert.Loosely(t, p.IsTerminate(), should.BeTrue)
		})
	})
}

func TestTaskDimensions(t *testing.T) {
	t.Parallel()
	ftt.Run("Hash", t, func(t *ftt.Test) {
		t.Run("dimensions keys are sorted when hash", func(t *ftt.Test) {
			dims1 := TaskDimensions{
				"d1": {"v1", "v2"},
				"d2": {"v3"},
			}
			dims2 := TaskDimensions{
				"d2": {"v3"},
				"d1": {"v1", "v2"},
			}
			assert.Loosely(t, dims1.Hash(), should.Equal(dims2.Hash()))
			assert.Loosely(t, dims1.Hash(), should.Equal(2036451960))
		})

		t.Run("empty dims", func(t *ftt.Test) {
			var dims1 TaskDimensions
			dims2 := TaskDimensions{}
			assert.Loosely(t, dims1.Hash(), should.Equal(dims2.Hash()))
		})

		t.Run("dims with OR-ed dimensions", func(t *ftt.Test) {
			dims1 := TaskDimensions{
				"d1": {"v1|v2"},
			}
			dims2 := TaskDimensions{
				"d1": {"v2|v1"},
			}
			assert.Loosely(t, dims1.Hash(), should.NotEqual(dims2.Hash()))
		})
	})
}
