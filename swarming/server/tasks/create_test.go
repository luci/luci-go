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

package tasks

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/resultdb"
)

func TestCreation(t *testing.T) {
	t.Parallel()

	ftt.Run("Creation", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = cryptorand.MockForTest(ctx, 0)
		lt := MockTQTasks()
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		globalStore := tsmon.Store(ctx)

		t.Run("duplicate_with_request_id", func(t *ftt.Test) {
			t.Run("error_fetching_result", func(t *ftt.Test) {
				tri := &model.TaskRequestID{
					Key:    model.TaskRequestIDKey(ctx, "bad_task_id"),
					TaskID: "not_a_task_id",
				}
				assert.NoErr(t, datastore.Put(ctx, tri))

				s := &Creation{
					RequestID: "bad_task_id",
				}
				_, err := s.Run(ctx)
				assert.That(t, err, should.ErrLike("unexpectedly invalid task_id not_a_task_id"))
			})
			t.Run("missing_entity", func(t *ftt.Test) {
				id := "65aba3a3e6b99310"
				reqKey, err := model.TaskIDToRequestKey(ctx, id)
				assert.NoErr(t, err)
				tr := &model.TaskRequest{
					Key: reqKey,
				}
				tri := &model.TaskRequestID{
					Key:    model.TaskRequestIDKey(ctx, "exist"),
					TaskID: id,
				}
				assert.NoErr(t, datastore.Put(ctx, tr, tri))

				c := &Creation{
					RequestID: "exist",
				}
				_, err = c.Run(ctx)
				assert.That(t, err, should.ErrLike("no such task"))
			})
			t.Run("found_duplicate", func(t *ftt.Test) {
				id := "65aba3a3e6b99310"
				reqKey, err := model.TaskIDToRequestKey(ctx, id)
				assert.NoErr(t, err)
				tr := &model.TaskRequest{
					Key: reqKey,
				}
				trs := &model.TaskResultSummary{
					Key: model.TaskResultSummaryKey(ctx, reqKey),
					TaskResultCommon: model.TaskResultCommon{
						State: apipb.TaskState_COMPLETED,
					},
				}
				tri := &model.TaskRequestID{
					Key:    model.TaskRequestIDKey(ctx, "exist"),
					TaskID: id,
				}
				assert.NoErr(t, datastore.Put(ctx, tr, trs, tri))

				c := &Creation{
					RequestID: "exist",
				}
				res, err := c.Run(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, res, should.NotBeNil)
				assert.That(t, res.Result.ToProto(), should.Match(trs.ToProto()))
			})
			t.Run("found_duplicate_for_build_task", func(t *ftt.Test) {
				id := "65aba3a3e6b99310"
				reqKey, err := model.TaskIDToRequestKey(ctx, id)
				assert.NoErr(t, err)
				tr := &model.TaskRequest{
					Key: reqKey,
				}
				trs := &model.TaskResultSummary{
					Key: model.TaskResultSummaryKey(ctx, reqKey),
					TaskResultCommon: model.TaskResultCommon{
						State: apipb.TaskState_COMPLETED,
					},
				}
				bt := &model.BuildTask{
					Key:      model.BuildTaskKey(ctx, reqKey),
					UpdateID: int64(3),
				}
				tri := &model.TaskRequestID{
					Key:    model.TaskRequestIDKey(ctx, "exist"),
					TaskID: id,
				}
				assert.NoErr(t, datastore.Put(ctx, tr, trs, bt, tri))

				c := &Creation{
					RequestID: "exist",
					BuildTask: &model.BuildTask{},
				}
				res, err := c.Run(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, res, should.NotBeNil)
				assert.That(t, res.Result.ToProto(), should.Match(trs.ToProto()))
				assert.That(t, res.BuildTask.UpdateID, should.Equal(int64(3)))
			})
		})

		t.Run("duplicate_with_properties_hash", func(t *ftt.Test) {
			now := testclock.TestRecentTimeUTC
			ctx, _ = testclock.UseTime(ctx, now)

			mockCfg := &cfgtest.MockedConfigs{
				Settings: &configpb.SettingsCfg{
					ReusableTaskAgeSecs: 3600,
					Resultdb: &configpb.ResultDBSettings{
						Server: "https://rdbhost.example.com",
					},
				},
			}
			p := cfgtest.MockConfigs(ctx, mockCfg)
			cfg := p.Cached(ctx)

			t.Run("found_duplicate", func(t *ftt.Test) {
				hash, err := hex.DecodeString("d4a3d498ade525d956d71b2356117647741e5618c31763a7d4323e46df9d5390")
				assert.NoErr(t, err)

				id := "65aba3a3e6b99310"
				reqKey, err := model.TaskIDToRequestKey(ctx, id)
				assert.NoErr(t, err)
				tr := &model.TaskRequest{
					Key: reqKey,
				}
				trs := &model.TaskResultSummary{
					Key: model.TaskResultSummaryKey(ctx, reqKey),
					TaskResultCommon: model.TaskResultCommon{
						State: apipb.TaskState_COMPLETED,
						ServerVersions: []string{
							"v1",
						},
						ResultDBInfo: model.ResultDBInfo{
							Hostname:   "resultdb.example.com",
							Invocation: "invocations/task-65aba3a3e6b99310",
						},
					},
					PropertiesHash: datastore.NewIndexedOptional(hash),
					Created:        now.Add(-30 * time.Minute),
					CostUSD:        10.0,
				}
				secrets := &model.SecretBytes{
					Key:         model.SecretBytesKey(ctx, reqKey),
					SecretBytes: []byte("secret"),
				}
				assert.That(t, datastore.Put(ctx, tr, trs, secrets), should.ErrLike(nil))

				c := Creation{
					Request: &model.TaskRequest{
						TaskSlices: []model.TaskSlice{
							{
								Properties: model.TaskProperties{
									HasSecretBytes: true,
								},
							},
							{
								Properties: model.TaskProperties{
									Idempotent:     true,
									HasSecretBytes: true,
								},
							},
						},
						PubSubTopic: "pubsub-topic",
						Tags: []string{
							"project:project",
							"subproject:subproject",
							"pool:pool",
							"rbe:rbe-instance",
							"spec_name:spec",
						},
					},
					SecretBytes: &model.SecretBytes{
						SecretBytes: []byte("secret"),
					},
					ServerVersion:  "v2",
					Config:         cfg,
					LifecycleTasks: lt,
				}

				res, err := c.Run(ctx)
				assert.NoErr(t, err)
				trs = res.Result
				assert.That(t, trs.State, should.Equal(apipb.TaskState_COMPLETED))
				assert.That(t, trs.ResultDBInfo, should.Match(trs.ResultDBInfo))
				assert.That(t, trs.CurrentTaskSlice, should.Equal(int64(1)))
				assert.That(t, trs.TryNumber.Get(), should.Equal(datastore.NewIndexedNullable(int64(0)).Get()))
				assert.That(t, trs.CostSavedUSD, should.Equal(10.0))
				assert.That(t, trs.CostUSD, should.Equal(0.0))
				assert.That(t, trs.ServerVersions, should.Match([]string{"v1", "v2"}))
				assert.That(t, trs.PropertiesHash.Get(), should.Match(datastore.Optional[[]byte, datastore.Indexed]{}.Get()))

				newSecret := &model.SecretBytes{
					Key: model.SecretBytesKey(ctx, trs.TaskRequestKey()),
				}
				err = datastore.Get(ctx, newSecret)
				assert.That(t, err, should.ErrLike(datastore.ErrNoSuchEntity))
				assert.That(t, lt.PopTask("pubsub-go"), should.Equal("2cbe1fa55012fa10"))

				val := globalStore.Get(ctx, metrics.JobsRequested, []any{"spec", "project", "subproject", "pool", "rbe-instance", true})
				assert.Loosely(t, val, should.Equal(1))
			})

			t.Run("found_duplicate_too_old", func(t *ftt.Test) {
				hash, err := hex.DecodeString("d2140bb8e09568358051857a22fb59c7f31ca1170be12ec407e3a3501d738516")
				assert.NoErr(t, err)

				id := "65aba3a3e6b99310"
				reqKey, err := model.TaskIDToRequestKey(ctx, id)
				assert.NoErr(t, err)
				tr := &model.TaskRequest{
					Key: reqKey,
				}
				trs := &model.TaskResultSummary{
					Key: model.TaskResultSummaryKey(ctx, reqKey),
					TaskResultCommon: model.TaskResultCommon{
						State: apipb.TaskState_COMPLETED,
					},
					PropertiesHash: datastore.NewIndexedOptional(hash),
					Created:        now.Add(-10 * time.Hour),
				}
				assert.That(t, datastore.Put(ctx, tr, trs), should.ErrLike(nil))
				inv := &rdbpb.Invocation{
					Name: "invocations/task-example.appspot.com-65aba3a3e6b99311",
				}
				mcf := resultdb.NewMockRecorderClientFactory(nil, inv, nil, "token for 65aba3a3e6b99311")

				c := Creation{
					Request: &model.TaskRequest{
						TaskSlices: []model.TaskSlice{
							{
								Properties: model.TaskProperties{
									Idempotent: true,
								},
							},
						},
						RBEInstance: "rbe-instance",
						Tags: []string{
							"buildername:builder",
						},
					},
					ServerVersion:         "v1",
					Config:                cfg,
					LifecycleTasks:        lt,
					SwarmingProject:       "swarming",
					ResultDBClientFactory: mcf,
				}

				res, err := c.Run(ctx)
				assert.NoErr(t, err)
				trs = res.Result
				assert.That(t, trs.State, should.Equal(apipb.TaskState_PENDING))
				assert.Loosely(t, lt.PopTask("rbe-new"), should.Equal("rbe-instance/swarming-2cbe1fa55012fa10-0-0"))
				// No PubSub notification.
				assert.That(t, lt.PopTask("pubsub-go"), should.Equal(""))
				val := globalStore.Get(ctx, metrics.JobsRequested, []any{"builder", "", "", "", "none", false})
				assert.Loosely(t, val, should.Equal(1))
			})
		})

		t.Run("id_collision", func(t *ftt.Test) {
			ctx := cryptorand.MockForTestWithIOReader(ctx, &oneValue{v: "same_id"})
			key := model.NewTaskRequestKey(ctx)
			tr1 := &model.TaskRequest{
				Key: key,
			}
			assert.That(t, datastore.Put(ctx, tr1), should.ErrLike(nil))

			c := &Creation{
				Request: &model.TaskRequest{
					TaskSlices: []model.TaskSlice{
						{
							Properties: model.TaskProperties{
								Dimensions: model.TaskDimensions{
									"pool": {"pool"},
								},
							},
						},
					},
				},
			}
			_, err := c.Run(ctx)
			assert.That(t, err, should.ErrLike(ErrAlreadyExists))
		})

		t.Run("resultdb_enabled", func(t *ftt.Test) {
			taskRunID := "2cbe1fa55012fa11"
			invocationID := "task-example.appspot.com-2cbe1fa55012fa11"
			realm := "project:realm"
			deadline := testclock.TestRecentTimeUTC.Add(3600 * time.Second)

			prepCreation := func(cfg *cfg.Config, mcf resultdb.RecorderFactory) *Creation {
				return &Creation{
					Request: &model.TaskRequest{
						TaskSlices: []model.TaskSlice{
							{
								Properties: model.TaskProperties{
									Dimensions: model.TaskDimensions{
										"pool": {"pool"},
									},
									ExecutionTimeoutSecs: 2400,
									GracePeriodSecs:      600,
								},
								ExpirationSecs: 600,
							},
						},
						ResultDB: model.ResultDBConfig{
							Enable: true,
						},
						Realm:   realm,
						Created: testclock.TestRecentTimeUTC,
					},
					ResultDBClientFactory: mcf,
					Config:                cfg,
					LifecycleTasks:        lt,
				}
			}

			req := &rdbpb.CreateInvocationRequest{
				InvocationId: invocationID,
				Invocation: &rdbpb.Invocation{
					ProducerResource: fmt.Sprintf("//example.appspot.com/tasks/%s", taskRunID),
					Realm:            realm,
					Deadline:         timestamppb.New(deadline),
				},
			}

			t.Run("resultdb_not_configured", func(t *ftt.Test) {
				mockCfg := &cfgtest.MockedConfigs{
					Settings: &configpb.SettingsCfg{},
				}
				p := cfgtest.MockConfigs(ctx, mockCfg)
				cfg := p.Cached(ctx)
				mcf := resultdb.NewMockRecorderClientFactory(nil, nil, nil, "")
				c := prepCreation(cfg, mcf)
				_, err := c.Run(ctx)
				assert.That(t, err, should.ErrLike("ResultDB integration is not configured"))
				assert.That(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			})

			t.Run("resultdb_configured", func(t *ftt.Test) {
				mockCfg := &cfgtest.MockedConfigs{
					Settings: &configpb.SettingsCfg{
						Resultdb: &configpb.ResultDBSettings{
							Server: "https://rdbhost.example.com",
						},
					},
				}
				p := cfgtest.MockConfigs(ctx, mockCfg)
				cfg := p.Cached(ctx)
				t.Run("failed", func(t *ftt.Test) {
					mcf := resultdb.NewMockRecorderClientFactory(req, nil,
						status.Errorf(codes.PermissionDenied, "boom"), "")
					c := prepCreation(cfg, mcf)
					_, err := c.Run(ctx)
					assert.That(t, err, should.ErrLike("error creating ResultDB invocation"))
				})

				t.Run("already_exists", func(t *ftt.Test) {
					mcf := resultdb.NewMockRecorderClientFactory(req, nil,
						status.Errorf(codes.AlreadyExists, "invocation already exists"), "")
					c := prepCreation(cfg, mcf)
					_, err := c.Run(ctx)
					assert.That(t, err, should.ErrLike(ErrAlreadyExists))
				})

				t.Run("OK", func(t *ftt.Test) {
					token := "token for 2cbe1fa55012fa11"
					inv := &rdbpb.Invocation{
						Name: "invocations/" + invocationID,
					}
					mcf := resultdb.NewMockRecorderClientFactory(req, inv,
						nil, token)
					c := prepCreation(cfg, mcf)
					res, err := c.Run(ctx)
					assert.NoErr(t, err)
					trs := res.Result
					assert.Loosely(t, trs, should.NotBeNil)
					assert.That(t, trs.ResultDBInfo.Hostname, should.Equal("https://rdbhost.example.com"))
					assert.That(t, trs.ResultDBInfo.Invocation, should.Equal(inv.Name))
				})
			})

		})

		t.Run("OK", func(t *ftt.Test) {
			now := testclock.TestRecentTimeUTC
			newTR := &model.TaskRequest{
				TaskSlices: []model.TaskSlice{
					{
						Properties: model.TaskProperties{
							Dimensions: model.TaskDimensions{
								"pool": {"pool"},
							},
						},
					},
				},
				RBEInstance: "rbe-instance",
				PubSubTopic: "pubsub-topic",
			}
			c := &Creation{
				Request:         newTR,
				LifecycleTasks:  lt,
				SwarmingProject: "swarming",
				BuildTask: &model.BuildTask{
					BuildID:          "12345",
					BuildbucketHost:  "bb.example.com",
					LatestTaskStatus: apipb.TaskState_PENDING,
					PubSubTopic:      "topic",
					UpdateID:         now.UnixNano(),
				},
			}
			res, err := c.Run(ctx)
			assert.NoErr(t, err)
			trs := res.Result
			assert.Loosely(t, trs, should.NotBeNil)
			assert.That(
				t, model.RequestKeyToTaskID(trs.TaskRequestKey(), model.AsRequest),
				should.Equal("2cbe1fa55012fa10"))

			updatedTR := res.Request
			updatedTRProto := updatedTR.ToProto()
			assert.That(t, updatedTRProto.TaskId, should.Equal(trs.ToProto().TaskId))
			// To confirm only TaskID is populated during creation.
			newTRProto := newTR.ToProto()
			assert.That(t, newTRProto.TaskId, should.Equal(""))
			newTRProto.TaskId = updatedTRProto.TaskId
			assert.That(t, updatedTRProto, should.Match(newTRProto))
			bt := res.BuildTask
			assert.That(t, bt.BuildID, should.Equal("12345"))
			assert.That(t, bt.Key, should.Match(model.BuildTaskKey(ctx, trs.TaskRequestKey())))

			tr := &model.TaskRequest{
				Key: trs.TaskRequestKey(),
			}
			assert.NoErr(t, datastore.Get(ctx, tr))
			assert.That(t, updatedTRProto, should.Match(tr.ToProto()))

			ttrKey, err := model.TaskRequestToToRunKey(ctx, tr, 0)
			assert.NoErr(t, err)
			ttr := &model.TaskToRun{
				Key: ttrKey,
			}
			assert.NoErr(t, datastore.Get(ctx, ttr))
			assert.That(t, ttr.TaskSliceIndex(), should.Equal(0))
			assert.That(t, lt.PopTask("rbe-new"), should.Equal("rbe-instance/swarming-2cbe1fa55012fa10-0-0"))
		})
	})
}

type oneValue struct {
	v string
}

func (o *oneValue) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = byte(o.v[i])
	}
	return len(p), nil
}
