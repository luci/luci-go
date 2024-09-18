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

package tasks

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var shards int32 = 10

const (
	defaultUpdateID = 5
	staleUpdateID   = 3
	newUpdateID     = 10
)

// fakeFetchTasksResponse mocks the FetchTasks RPC.
func fakeFetchTasksResponse(ctx context.Context, taskReq *pb.FetchTasksRequest, opts ...grpc.CallOption) (*pb.FetchTasksResponse, error) {
	responses := make([]*pb.FetchTasksResponse_Response, 0, len(taskReq.TaskIds))
	for _, tID := range taskReq.TaskIds {
		responseStatus := pb.Status_STARTED
		updateID := newUpdateID
		switch {
		case strings.HasSuffix(tID.Id, "all_fail"):
			return nil, errors.Reason("idk, wanted to fail i guess :/").Err()
		case strings.HasSuffix(tID.Id, "fail_me"):
			responses = append(responses, &pb.FetchTasksResponse_Response{
				Response: &pb.FetchTasksResponse_Response_Error{
					Error: &status.Status{
						Code:    500,
						Message: fmt.Sprintf("could not find task for taskId: %s", tID.Id),
					},
				},
			})
			continue
		case strings.HasSuffix(tID.Id, "ended"):
			responseStatus = pb.Status_SUCCESS
		case strings.HasSuffix(tID.Id, "stale"):
			updateID = staleUpdateID
		case strings.HasSuffix(tID.Id, "unchanged"):
			updateID = defaultUpdateID
		}
		responses = append(responses, &pb.FetchTasksResponse_Response{
			Response: &pb.FetchTasksResponse_Response_Task{
				Task: &pb.Task{
					Id:       tID,
					Status:   responseStatus,
					UpdateId: int64(updateID),
				},
			},
		})
	}
	return &pb.FetchTasksResponse{Responses: responses}, nil
}

func prepEntities(ctx context.Context, bID int64, buildStatus, outputStatus, taskStatus pb.Status, tIDSuffix string, updateTime time.Time) *datastore.Key {
	tID := ""
	if tIDSuffix != "no_task" {
		tID = fmt.Sprintf("task%d%s", bID, tIDSuffix)
	}
	b := &model.Build{
		ID: bID,
		Proto: &pb.Build{
			Id: bID,
			Builder: &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
			Status:     buildStatus,
			UpdateTime: timestamppb.New(updateTime),
			Output: &pb.Build_Output{
				Status: outputStatus,
			},
		},
		Status:              buildStatus,
		BackendTarget:       "swarming",
		BackendSyncInterval: 5 * time.Minute,
		Project:             "project",
	}
	b.GenerateNextBackendSyncTime(ctx, shards)
	bk := datastore.KeyForObj(ctx, b)
	inf := &model.BuildInfra{
		Build: bk,
		Proto: &pb.BuildInfra{
			Backend: &pb.BuildInfra_Backend{
				Task: &pb.Task{
					Status: taskStatus,
					Id: &pb.TaskID{
						Id:     tID,
						Target: "swarming",
					},
					Link:     "a link",
					UpdateId: defaultUpdateID,
				},
			},
		},
	}
	bs := &model.BuildStatus{
		Build:  bk,
		Status: buildStatus,
	}
	bldr := &model.Builder{
		ID:     "builder",
		Parent: model.BucketKey(ctx, "project", "bucket"),
		Config: &pb.BuilderConfig{
			MaxConcurrentBuilds: 2,
		},
	}
	So(datastore.Put(ctx, b, inf, bs, bldr), ShouldBeNil)
	return bk
}

func TestQueryBuildsToSync(t *testing.T) {
	ctx := context.Background()
	now := testclock.TestRecentTimeUTC
	ctx, _ = testclock.UseTime(ctx, now)
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = memory.UseWithAppID(ctx, "dev~app-id")
	ctx = txndefer.FilterRDS(ctx)
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	t.Parallel()

	Convey("queryBuildsToSync", t, func() {
		put := func(ctx context.Context, project, backend string, bID int64, status pb.Status, updateTime time.Time) {
			b := &model.Build{
				ID: bID,
				Proto: &pb.Build{
					Id: bID,
					Builder: &pb.BuilderID{
						Project: project,
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:     status,
					UpdateTime: timestamppb.New(updateTime),
				},
				Status:              status,
				Project:             project,
				BackendTarget:       backend,
				BackendSyncInterval: 5 * time.Minute,
			}
			b.GenerateNextBackendSyncTime(ctx, shards)
			So(datastore.Put(ctx, b), ShouldBeNil)
		}

		project := "project"
		backend := "swarming"

		// Prepare build entities.
		// Should be included in query results.
		// updated 1 hour ago.
		for i := 1; i <= 5; i++ {
			put(ctx, project, backend, int64(i), pb.Status_STARTED, now.Add(-time.Hour))
		}
		// Should not be included in query results.
		// Just updated.
		put(ctx, project, backend, 6, pb.Status_STARTED, now)
		// Different project.
		put(ctx, "another_project", backend, 7, pb.Status_STARTED, now.Add(-time.Hour))
		// Different backend.
		put(ctx, project, "another_backend", 8, pb.Status_STARTED, now.Add(-time.Hour))
		// Build has completed.
		put(ctx, project, backend, 9, pb.Status_SUCCESS, now.Add(-time.Hour))

		var allBks []*datastore.Key
		err := parallel.RunMulti(ctx, int(shards), func(mr parallel.MultiRunner) error {
			bkC := make(chan []*datastore.Key)
			return mr.RunMulti(func(work chan<- func() error) {
				work <- func() error {
					defer close(bkC)
					return queryBuildsToSync(ctx, mr, backend, project, shards, now, bkC)
				}

				for bks := range bkC {
					bks := bks
					allBks = append(allBks, bks...)
				}
			})
		})
		So(err, ShouldBeNil)
		So(len(allBks), ShouldEqual, 5)
	})
}

func TestSyncBuildsWithBackendTasksOneFetchBatch(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	ctx := context.Background()
	mockBackend := clients.NewMockTaskBackendClient(ctl)
	mockBackend.EXPECT().FetchTasks(gomock.Any(), gomock.Any()).DoAndReturn(fakeFetchTasksResponse).AnyTimes()
	now := testclock.TestRecentTimeUTC
	ctx, _ = testclock.UseTime(ctx, now)
	ctx = context.WithValue(ctx, clients.MockTaskBackendClientKey, mockBackend)
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = memory.UseWithAppID(ctx, "dev~app-id")
	ctx = txndefer.FilterRDS(ctx)
	ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
	ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	getEntities := func(bIDs []int64) []*model.Build {
		var blds []*model.Build
		for _, id := range bIDs {
			blds = append(blds, &model.Build{ID: id})
		}
		So(datastore.Get(ctx, blds), ShouldBeNil)
		return blds
	}

	Convey("syncBuildsWithBackendTasks", t, func() {
		ctx, sch := tq.TestingContext(ctx, nil)
		backendSetting := []*pb.BackendSetting{
			&pb.BackendSetting{
				Target:   "swarming",
				Hostname: "hostname",
			},
		}
		settingsCfg := &pb.SettingsCfg{Backends: backendSetting}

		bc, err := clients.NewBackendClient(ctx, "project", "swarming", settingsCfg)
		So(err, ShouldBeNil)

		sync := func(bks []*datastore.Key) error {
			return parallel.RunMulti(ctx, 5, func(mr parallel.MultiRunner) error {
				return mr.RunMulti(func(work chan<- func() error) {
					work <- func() error {
						return syncBuildsWithBackendTasks(ctx, mr, bc, bks, now, false)
					}
				})
			})
		}

		Convey("nothing to update", func() {
			updateTime := now.Add(-2 * time.Minute)
			bIDs := []int64{3, 4, 5}
			var bks []*datastore.Key
			bks = append(bks, prepEntities(ctx, 3, pb.Status_STARTED, pb.Status_STARTED, pb.Status_STARTED, "", updateTime))
			bks = append(bks, prepEntities(ctx, 4, pb.Status_FAILURE, pb.Status_FAILURE, pb.Status_FAILURE, "", updateTime))
			bks = append(bks, prepEntities(ctx, 5, pb.Status_SCHEDULED, pb.Status_SCHEDULED, pb.Status_SCHEDULED, "no_task", updateTime))
			err := sync(bks)
			So(err, ShouldBeNil)
			So(sch.Tasks(), ShouldBeEmpty)
			blds := getEntities(bIDs)
			for _, b := range blds {
				So(b.Proto.UpdateTime.AsTime(), ShouldEqual, updateTime)
			}
		})

		Convey("ok", func() {
			bIDs := []int64{1, 2}
			var bks []*datastore.Key
			for _, id := range bIDs {
				bks = append(bks, prepEntities(ctx, id, pb.Status_STARTED, pb.Status_STARTED, pb.Status_STARTED, "", now.Add(-time.Hour)))
			}
			err = sync(bks)
			So(err, ShouldBeNil)
			So(sch.Tasks(), ShouldBeEmpty)
			blds := getEntities(bIDs)
			for _, b := range blds {
				So(b.Proto.UpdateTime.AsTime(), ShouldEqual, now)
			}
		})

		Convey("ok end builds", func() {
			bIDs := []int64{3, 4}
			var bks []*datastore.Key
			for _, id := range bIDs {
				bks = append(bks, prepEntities(ctx, id, pb.Status_STARTED, pb.Status_SUCCESS, pb.Status_STARTED, "ended", now.Add(-time.Hour)))
			}

			err := sync(bks)
			So(err, ShouldBeNil)
			// TQ tasks for pubsub-notification *2, and bq-export per build.
			// Resultdb invocation finalization is noop since the builds don't have
			// resultdb invocations.
			So(sch.Tasks(), ShouldHaveLength, 8)
			blds := getEntities(bIDs)
			for _, b := range blds {
				So(b.Proto.UpdateTime.AsTime(), ShouldEqual, now)
				So(b.Status, ShouldEqual, pb.Status_SUCCESS)
			}
		})

		Convey("partially ok", func() {
			preSyncUpdateTime := now.Add(-time.Hour)
			bIDs := []int64{5, 6, 7, 8}
			var bks []*datastore.Key
			// build 5 is ok.
			bks = append(bks, prepEntities(ctx, 5, pb.Status_STARTED, pb.Status_STARTED, pb.Status_STARTED, "", preSyncUpdateTime))
			// failed to get the task for build 6.
			bks = append(bks, prepEntities(ctx, 6, pb.Status_STARTED, pb.Status_STARTED, pb.Status_STARTED, "fail_me", preSyncUpdateTime))
			// task for build 7 is stale.
			bks = append(bks, prepEntities(ctx, 7, pb.Status_STARTED, pb.Status_STARTED, pb.Status_STARTED, "stale", preSyncUpdateTime))
			// task for build 8 is unchanged.
			bks = append(bks, prepEntities(ctx, 8, pb.Status_STARTED, pb.Status_STARTED, pb.Status_STARTED, "unchanged", preSyncUpdateTime))

			blds := getEntities(bIDs)
			nextSyncTimeBeforeSync := blds[3].NextBackendSyncTime

			err := sync(bks)
			So(err, ShouldBeNil)
			So(sch.Tasks(), ShouldBeEmpty)
			blds = getEntities(bIDs)
			// build 5 is updated with new update_id
			So(blds[0].Proto.UpdateTime.AsTime(), ShouldEqual, now)
			// build 6 is not updated due to failing to get the task
			So(blds[1].Proto.UpdateTime.AsTime(), ShouldEqual, preSyncUpdateTime)
			// build 7 has a stale updateID so it is not udpated
			So(blds[2].Proto.UpdateTime.AsTime(), ShouldEqual, preSyncUpdateTime)
			// build 8 is unchanged, but we still update the builds update time
			So(blds[3].Proto.UpdateTime.AsTime(), ShouldEqual, now)
			So(blds[3].NextBackendSyncTime, ShouldBeGreaterThan, nextSyncTimeBeforeSync)

		})

		Convey("all fail", func() {
			preSyncUpdateTime := now.Add(-time.Hour)
			bIDs := []int64{5, 6}
			var bks []*datastore.Key
			bks = append(bks, prepEntities(ctx, 5, pb.Status_STARTED, pb.Status_STARTED, pb.Status_STARTED, "", preSyncUpdateTime))
			bks = append(bks, prepEntities(ctx, 6, pb.Status_STARTED, pb.Status_STARTED, pb.Status_STARTED, "all_fail", preSyncUpdateTime))

			err := sync(bks)
			So(err, ShouldErrLike, "idk, wanted to fail i guess :/")
			So(sch.Tasks(), ShouldBeEmpty)
			blds := getEntities(bIDs)
			for _, b := range blds {
				So(b.Proto.UpdateTime.AsTime(), ShouldEqual, preSyncUpdateTime)
			}
		})
	})
}

func TestSyncBuildsWithBackendTasks(t *testing.T) {
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	ctx := context.Background()
	mockBackend := clients.NewMockTaskBackendClient(ctl)
	mockBackend.EXPECT().FetchTasks(gomock.Any(), gomock.Any()).DoAndReturn(fakeFetchTasksResponse).AnyTimes()
	now := testclock.TestRecentTimeUTC
	ctx, _ = testclock.UseTime(ctx, now)
	ctx = context.WithValue(ctx, clients.MockTaskBackendClientKey, mockBackend)
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = memory.UseWithAppID(ctx, "dev~app-id")
	ctx = txndefer.FilterRDS(ctx)
	ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
	ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	getEntities := func(bIDs []int64) []*model.Build {
		var blds []*model.Build
		for _, id := range bIDs {
			blds = append(blds, &model.Build{ID: id})
		}
		So(datastore.Get(ctx, blds), ShouldBeNil)
		return blds
	}

	Convey("SyncBuildsWithBackendTasks", t, func() {
		ctx, sch := tq.TestingContext(ctx, nil)
		backendSetting := []*pb.BackendSetting{
			{
				Target:   "swarming",
				Hostname: "hostname",
				Mode: &pb.BackendSetting_FullMode_{
					FullMode: &pb.BackendSetting_FullMode{
						BuildSyncSetting: &pb.BackendSetting_BuildSyncSetting{
							Shards: shards,
						},
					},
				},
			},
			{
				Target:   "foo",
				Hostname: "foo_hostname",
				Mode: &pb.BackendSetting_LiteMode_{
					LiteMode: &pb.BackendSetting_LiteMode{},
				},
			},
		}
		settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
		err := config.SetTestSettingsCfg(ctx, settingsCfg)
		So(err, ShouldBeNil)

		Convey("ok - full mode", func() {
			bIDs := []int64{101, 102, 103, 104, 105}
			fetchBatchSize = 1
			for _, id := range bIDs {
				prepEntities(ctx, id, pb.Status_STARTED, pb.Status_SUCCESS, pb.Status_STARTED, "", now.Add(-time.Hour))
			}
			prepEntities(ctx, 106, pb.Status_STARTED, pb.Status_SUCCESS, pb.Status_STARTED, "ended", now.Add(-time.Hour))
			bIDs = append(bIDs, 106)
			err = SyncBuildsWithBackendTasks(ctx, "swarming", "project")
			So(err, ShouldBeNil)
			So(sch.Tasks(), ShouldHaveLength, 4) // 106 completed
			blds := getEntities(bIDs)
			for _, b := range blds {
				So(b.Proto.UpdateTime.AsTime(), ShouldEqual, now)
				if b.ID == int64(106) {
					So(b.Status, ShouldEqual, pb.Status_SUCCESS)
				}
			}
		})

		Convey("no sync - lite mode", func() {
			err = SyncBuildsWithBackendTasks(ctx, "foo", "project")
			So(err, ShouldBeNil)
			So(sch.Tasks(), ShouldHaveLength, 0)
		})

		Convey("backend setting not found", func() {
			err = SyncBuildsWithBackendTasks(ctx, "not_exist", "project")
			So(err, ShouldErrLike, "failed to find backend not_exist from global config")
		})
	})
}
