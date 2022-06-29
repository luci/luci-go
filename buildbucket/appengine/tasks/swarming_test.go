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
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskDef(t *testing.T) {
	Convey("compute task slice", t, func() {
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
						},
					},
				},
			},
		}
		Convey("only base slice", func() {
			b.Proto.Infra.Swarming = &pb.BuildInfra_Swarming{
				Caches: []*pb.BuildInfra_Swarming_CacheEntry{
					{Name: "shared_builder_cache", Path: "builder"},
				},
				TaskDimensions: []*pb.RequestedDimension{
					{Key: "pool", Value: "Chrome"},
				},
			}
			slices, err := computeTaskSlice(b)
			So(err, ShouldBeNil)
			So(len(slices), ShouldEqual, 1)
			So(slices[0].Properties.Caches, ShouldResemble, []*swarming.SwarmingRpcsCacheEntry{{
				Path: filepath.Join("cache", "builder"),
				Name: "shared_builder_cache",
			}})
			So(slices[0].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{
					Key:   "caches",
					Value: "shared_builder_cache",
				},
				{
					Key:   "pool",
					Value: "Chrome",
				},
			})
		})

		Convey("multiple dimensions and cache fallback", func() {
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
			So(err, ShouldBeNil)
			So(len(slices), ShouldEqual, 4)

			// All slices properties fields have the same value except dimensions.
			for _, tSlice := range slices {
				So(tSlice.Properties.ExecutionTimeoutSecs, ShouldEqual, 4800)
				So(tSlice.Properties.GracePeriodSecs, ShouldEqual, 240)
				So(tSlice.Properties.Caches, ShouldResemble, []*swarming.SwarmingRpcsCacheEntry{
					{Path: filepath.Join("cache", "builder"), Name: "shared_builder_cache"},
					{Path: filepath.Join("cache", "second"), Name: "second_cache"},
				})
				So(tSlice.Properties.Env, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
					{Key: "BUILDBUCKET_EXPERIMENTAL", Value: "FALSE"},
				})
			}

			So(slices[0].ExpirationSecs, ShouldEqual, 60)
			// The dimensions are different. 'a' and 'caches' are injected.
			So(slices[0].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{Key: "a", Value: "1"},
				{Key: "a", Value: "2"},
				{Key: "caches", Value: "second_cache"},
				{Key: "caches", Value: "shared_builder_cache"},
				{Key: "pool", Value: "Chrome"},
			})

			// 120 - 60
			So(slices[1].ExpirationSecs, ShouldEqual, 60)
			// The dimensions are different. 'a' and 'caches' are injected.
			So(slices[1].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{Key: "a", Value: "1"},
				{Key: "a", Value: "2"},
				{Key: "caches", Value: "second_cache"},
				{Key: "pool", Value: "Chrome"},
			})

			// 360 - 120
			So(slices[2].ExpirationSecs, ShouldEqual, 240)
			// 'a' expired, one 'caches' remains.
			So(slices[2].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{Key: "caches", Value: "second_cache"},
				{Key: "pool", Value: "Chrome"},
			})

			// 3600-360
			So(slices[3].ExpirationSecs, ShouldEqual, 3240)
			// # The cold fallback; the last 'caches' expired.
			So(slices[3].Properties.Dimensions, ShouldResemble, []*swarming.SwarmingRpcsStringPair{
				{Key: "pool", Value: "Chrome"},
			})
		})
	})

	Convey("compute bbagent command", t, func() {
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
		Convey("bbagent_getbuild experiment", func() {
			b.Experiments = []string{"luci.buildbucket.bbagent_getbuild"}
			bbagentCmd := computeCommand(b)
			So(bbagentCmd, ShouldResemble, []string{
				"bbagent${EXECUTABLE_SUFFIX}",
				"-host",
				"bbhost.com",
				"-build-id",
				"123",
			})
		})

		Convey("no bbagent_getbuild experiment", func() {
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
			So(bbagentCmd, ShouldResemble, []string{
				"bbagent${EXECUTABLE_SUFFIX}",
				expectedEncoded,
			})
		})
	})

	Convey("compute env_prefixes", t, func() {
		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{},
				},
			},
		}
		Convey("empty swarming cach", func() {
			prefixes := computeEnvPrefixes(b)
			So(prefixes, ShouldResemble, []*swarming.SwarmingRpcsStringListPair{})
		})

		Convey("normal", func() {
			b.Proto.Infra.Swarming.Caches = []*pb.BuildInfra_Swarming_CacheEntry{
				{Path: "vpython", Name: "vpython", EnvVar: "VPYTHON_VIRTUALENV_ROOT"},
				{Path: "abc", Name: "abc", EnvVar: "ABC"},
			}
			prefixes := computeEnvPrefixes(b)
			So(prefixes, ShouldResemble, []*swarming.SwarmingRpcsStringListPair{
				{Key: "ABC", Value: []string{filepath.Join("cache", "abc")}},
				{Key: "VPYTHON_VIRTUALENV_ROOT", Value: []string{filepath.Join("cache", "vpython")}},
			})
		})
	})

	Convey("compute swarming new task req", t, func() {
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
		req.TaskSlices = []*swarming.SwarmingRpcsTaskSlice(nil)
		So(err, ShouldBeNil)
		expected := &swarming.SwarmingRpcsNewTaskRequest{
			RequestUuid:    "203882df-ce4b-5012-b32a-2c1d29c321a7",
			Name:           "bb-123-builder-1",
			Realm:          "project:bucket",
			Tags:           []string{"buildbucket_bucket:bucket", "buildbucket_build_id:123", "buildbucket_hostname:app-id.appspot.com", "buildbucket_template_canary:0", "luci_project:project"},
			Priority:       int64(20),
			PubsubTopic:    "projects/app-id/topics/swarming",
			PubsubUserdata: fmt.Sprintf(pubSubUserDataTemplate, 123, 1444945245000000, "swarm.com"),
			ServiceAccount: "abc",
		}
		So(req, ShouldResemble, expected)
	})
}

func TestSyncBuild(t *testing.T) {
	t.Parallel()
	Convey("SyncBuild", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		now := testclock.TestRecentTimeUTC
		mockSwarm := clients.NewMockSwarmingClient(ctl)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, &clients.MockSwarmingClientKey, mockSwarm)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
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
		So(datastore.Put(ctx, b), ShouldBeNil)
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
			},
		}
		So(datastore.Put(ctx, inf), ShouldBeNil)

		Convey("build not found", func() {
			err := SyncBuild(ctx, 789, 0)
			So(err, ShouldErrLike, "build 789 or buildInfra not found")
		})

		Convey("build too old", func() {
			So(datastore.Put(ctx, &model.Build{
				ID:         111,
				CreateTime: now.AddDate(0, 0, -3),
				Proto: &pb.Build{
					Builder: &pb.BuilderID{},
				},
			}), ShouldBeNil)

			So(datastore.Put(ctx, &model.BuildInfra{
				ID:    1,
				Build: datastore.KeyForObj(ctx, &model.Build{ID: 111}),
			}), ShouldBeNil)
			err := SyncBuild(ctx, 111, 0)
			So(err, ShouldBeNil)
			// TODO: assert the task isn't pushed back into queue once the enqueueing sync-build is implemented.
		})

		Convey("build ended", func() {
			So(datastore.Put(ctx, &model.Build{
				ID:     111,
				Status: pb.Status_SUCCESS,
				Proto: &pb.Build{
					Builder: &pb.BuilderID{},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildInfra{
				ID:    1,
				Build: datastore.KeyForObj(ctx, &model.Build{ID: 111}),
			}), ShouldBeNil)
			err := SyncBuild(ctx, 111, 0)
			So(err, ShouldBeNil)
			// TODO: assert the task isn't pushed back into queue once the enqueueing sync-build is implemented.
		})

		Convey("create swarming success", func() {
			mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).Return(&swarming.SwarmingRpcsTaskRequestMetadata{
				TaskId: "task123",
			}, nil)
			err := SyncBuild(ctx, 123, 0)
			So(err, ShouldBeNil)
			updatedBuild := &model.Build{ID: 123}
			updatedInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, updatedBuild)}
			So(datastore.Get(ctx, updatedBuild), ShouldBeNil)
			So(datastore.Get(ctx, updatedInfra), ShouldBeNil)
			So(updatedBuild.UpdateToken, ShouldNotBeEmpty)
			So(updatedInfra.Proto.Swarming.TaskId, ShouldEqual, "task123")
		})

		Convey("create swarming http 400 err", func() {
			mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).Return(nil, &googleapi.Error{Code: 400})
			err := SyncBuild(ctx, 123, 0)
			So(err, ShouldErrLike, "failed to create a swarming task")
			So(tq.Fatal.In(err), ShouldBeTrue)
			failedBuild := &model.Build{ID: 123}
			So(datastore.Get(ctx, failedBuild), ShouldBeNil)
			So(failedBuild.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			So(failedBuild.Proto.SummaryMarkdown, ShouldContainSubstring, "failed to create a swarming task: googleapi: got HTTP response code 400")
			So(sch.Tasks(), ShouldHaveLength, 3)
		})

		Convey("create swarming http 500 err", func() {
			mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).Return(nil, &googleapi.Error{Code: 500})
			err := SyncBuild(ctx, 123, 0)
			So(err, ShouldErrLike, "failed to create a swarming task")
			So(transient.Tag.In(err), ShouldBeTrue)
			bld := &model.Build{ID: 123}
			So(datastore.Get(ctx, bld), ShouldBeNil)
			So(bld.Status, ShouldEqual, pb.Status_SCHEDULED)
		})

		Convey("create swarming http 500 err give up", func() {
			ctx1, _ := testclock.UseTime(ctx, now.Add(swarmingCreateTaskGiveUpTimeout))
			mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).Return(nil, &googleapi.Error{Code: 500})
			err := SyncBuild(ctx1, 123, 0)
			So(err, ShouldErrLike, "failed to create a swarming task")
			So(tq.Fatal.In(err), ShouldBeTrue)
			failedBuild := &model.Build{ID: 123}
			So(datastore.Get(ctx, failedBuild), ShouldBeNil)
			So(failedBuild.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			So(failedBuild.Proto.SummaryMarkdown, ShouldContainSubstring, "failed to create a swarming task: googleapi: got HTTP response code 500")
			So(sch.Tasks(), ShouldHaveLength, 3)
		})

		Convey("swarming task creation success but update build fail", func() {
			mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, req *swarming.SwarmingRpcsNewTaskRequest) (*swarming.SwarmingRpcsTaskRequestMetadata, error) {
				// hack to make the build update fail when trying to update build with the new task id.
				inf.Proto.Swarming.TaskId = "old task id"
				So(datastore.Put(ctx, inf), ShouldBeNil)
				return &swarming.SwarmingRpcsTaskRequestMetadata{TaskId: "new task id"}, nil
			})

			err := SyncBuild(ctx, 123, 0)
			So(err, ShouldErrLike, "failed to update build 123: build already has a task old task id")
			currentInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, &model.Build{
				ID: 123,
			})}
			So(datastore.Get(ctx, currentInfra), ShouldBeNil)
			So(currentInfra.Proto.Swarming.TaskId, ShouldEqual, "old task id")
			// One task should be pushed into CancelSwarmingTask queue since update build failed.
			So(sch.Tasks(), ShouldHaveLength, 1)
			So(sch.Tasks().Payloads()[0], ShouldResembleProto, &taskdefs.CancelSwarmingTask{
				Hostname: "swarm",
				TaskId:   "new task id",
				Realm:    "proj:bucket",
			})
		})

		Convey("create swarming - cancel swarming task fail", func() {
			mockSwarm.EXPECT().CreateTask(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, req *swarming.SwarmingRpcsNewTaskRequest) (*swarming.SwarmingRpcsTaskRequestMetadata, error) {
				// hack to make the build update and the equeueing CancelSwarmingTask fail
				inf.Proto.Swarming.TaskId = "old task id"
				So(datastore.Put(ctx, inf), ShouldBeNil)
				return &swarming.SwarmingRpcsTaskRequestMetadata{TaskId: ""}, nil
			})

			err := SyncBuild(ctx, 123, 0)
			So(err, ShouldErrLike, "failed to enqueue swarming task cancellation task for build 123: task_id is required")
		})
	})

}
