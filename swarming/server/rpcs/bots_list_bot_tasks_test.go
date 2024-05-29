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
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListBotTasks(t *testing.T) {
	t.Parallel()

	state := NewMockedRequestState()

	// A bot that should be visible.
	const botID = "visible-bot"
	state.MockBot(botID, "visible-pool")
	state.MockPool("visible-pool", "project:visible-realm")
	state.MockPerm("project:visible-realm", acls.PermPoolsListTasks)

	// A bot the caller has no permissions over.
	state.MockBot("hidden-bot", "hidden-pool")
	state.MockPool("hidden-pool", "project:hidden-realm")

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	putTask := func(botID string, state apipb.TaskState, ts time.Duration) {
		reqKey, err := model.TimestampToRequestKey(ctx, TestTime.Add(ts), 0)
		if err != nil {
			panic(err)
		}
		err = datastore.Put(ctx,
			&model.TaskRunResult{
				TaskResultCommon: model.TaskResultCommon{
					State:         state,
					BotDimensions: map[string][]string{"payload": {fmt.Sprintf("%s-%s", state, ts)}},
					Started:       datastore.NewIndexedNullable(TestTime.Add(ts)),
					Completed:     datastore.NewIndexedNullable(TestTime.Add(ts)),
				},
				Key:   model.TaskRunResultKey(ctx, reqKey),
				BotID: botID,
			},
			&model.PerformanceStats{
				Key:             model.PerformanceStatsKey(ctx, reqKey),
				BotOverheadSecs: 123.0,
			},
		)
		if err != nil {
			panic(err)
		}
	}

	payloads := func(resp *apipb.TaskListResponse) []string {
		var out []string
		for _, t := range resp.Items {
			out = append(out, t.BotDimensions[0].Value[0])
		}
		return out
	}

	call := func(req *apipb.BotTasksRequest) (*apipb.TaskListResponse, error) {
		ctx := MockRequestState(ctx, state)
		return (&BotsServer{}).ListBotTasks(ctx, req)
	}

	paginatedCall := func(req *apipb.BotTasksRequest) (*apipb.TaskListResponse, error) {
		req.Limit = 3
		req.Cursor = ""
		var items []*apipb.TaskResultResponse
		for {
			resp, err := call(req)
			if err != nil {
				return nil, err
			}
			items = append(items, resp.Items...)
			if resp.Cursor == "" {
				break
			}
			req.Cursor = resp.Cursor
		}
		return &apipb.TaskListResponse{Items: items}, nil
	}

	// A mix of completed and killed tasks.
	var now time.Duration
	for i := 0; i < 10; i++ {
		var state apipb.TaskState
		if i%2 == 0 {
			state = apipb.TaskState_COMPLETED
		} else {
			state = apipb.TaskState_KILLED
		}
		putTask(botID, state, now)
		now += 500 * time.Millisecond
		putTask("another-bot", state, now)
		now += 500 * time.Millisecond
	}

	Convey("Bad bot ID", t, func() {
		_, err := call(&apipb.BotTasksRequest{
			BotId: "",
			Limit: 10,
			State: apipb.StateQuery_QUERY_ALL,
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("Limit is checked", t, func() {
		_, err := call(&apipb.BotTasksRequest{
			BotId: "doesnt-matter",
			Limit: -10,
			State: apipb.StateQuery_QUERY_ALL,
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		_, err = call(&apipb.BotTasksRequest{
			BotId: "doesnt-matter",
			Limit: 1001,
			State: apipb.StateQuery_QUERY_ALL,
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("Cursor is checked", t, func() {
		_, err := call(&apipb.BotTasksRequest{
			BotId:  "doesnt-matter",
			Cursor: "!!!!",
			State:  apipb.StateQuery_QUERY_ALL,
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("No permissions", t, func() {
		_, err := call(&apipb.BotTasksRequest{
			BotId: "hidden-bot",
			Limit: 10,
			State: apipb.StateQuery_QUERY_ALL,
		})
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
	})

	Convey("Bot not in a config", t, func() {
		_, err := call(&apipb.BotTasksRequest{
			BotId: "unknown-bot",
			Limit: 10,
			State: apipb.StateQuery_QUERY_ALL,
		})
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
	})

	Convey("Unsupported state filters", t, func() {
		filters := []apipb.StateQuery{
			apipb.StateQuery_QUERY_PENDING,
			apipb.StateQuery_QUERY_PENDING_RUNNING,
			apipb.StateQuery_QUERY_EXPIRED,
			apipb.StateQuery_QUERY_DEDUPED,
			apipb.StateQuery_QUERY_NO_RESOURCE,
			apipb.StateQuery_QUERY_CANCELED,
		}
		for _, f := range filters {
			_, err := call(&apipb.BotTasksRequest{
				BotId: "doesnt-matter",
				Limit: 10,
				State: f,
			})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "filter")
		}
	})

	Convey("Unsupported order", t, func() {
		pairs := []struct {
			sort  apipb.SortQuery
			state apipb.StateQuery
		}{
			{apipb.SortQuery_QUERY_COMPLETED_TS, apipb.StateQuery_QUERY_RUNNING},
			{apipb.SortQuery_QUERY_STARTED_TS, apipb.StateQuery_QUERY_RUNNING},
			{apipb.SortQuery_QUERY_ABANDONED_TS, apipb.StateQuery_QUERY_ALL},
		}
		for _, pair := range pairs {
			_, err := call(&apipb.BotTasksRequest{
				BotId: "doesnt-matter",
				Limit: 10,
				Sort:  pair.sort,
				State: pair.state,
			})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "sort")
		}
	})

	Convey("Order by created_ts", t, func() {
		Convey("Bad time range", func() {
			_, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				State: apipb.StateQuery_QUERY_ALL,
				Start: timestamppb.New(time.Date(1977, time.January, 1, 2, 3, 4, 0, time.UTC)),
			})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid start time")
		})

		Convey("Unpaginated, unfiltered, all states", func() {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				State: apipb.StateQuery_QUERY_ALL,
			})
			So(err, ShouldBeNil)
			So(payloads(resp), ShouldResemble, []string{
				"KILLED-9s",
				"COMPLETED-8s",
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
				"KILLED-1s",
				"COMPLETED-0s",
			})
		})

		Convey("Unpaginated, unfiltered, COMPLETED only", func() {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				State: apipb.StateQuery_QUERY_COMPLETED,
			})
			So(err, ShouldBeNil)
			So(payloads(resp), ShouldResemble, []string{
				"COMPLETED-8s",
				"COMPLETED-6s",
				"COMPLETED-4s",
				"COMPLETED-2s",
				"COMPLETED-0s",
			})
		})

		Convey("Unpaginated, filtered, all states", func() {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				Start: timestamppb.New(TestTime.Add(2 * time.Second)), // inclusive
				End:   timestamppb.New(TestTime.Add(8 * time.Second)), // non-inclusive
				State: apipb.StateQuery_QUERY_ALL,
			})
			So(err, ShouldBeNil)
			So(payloads(resp), ShouldResemble, []string{
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
			})
		})

		Convey("Paginated, filtered, all states", func() {
			resp, err := paginatedCall(&apipb.BotTasksRequest{
				BotId: botID,
				Start: timestamppb.New(TestTime.Add(2 * time.Second)), // inclusive
				End:   timestamppb.New(TestTime.Add(8 * time.Second)), // non-inclusive
				State: apipb.StateQuery_QUERY_ALL,
			})
			So(err, ShouldBeNil)
			So(payloads(resp), ShouldResemble, []string{
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
			})
		})
	})

	Convey("Order by started_ts", t, func() {
		// Note: state filtering is not supported, only StateQuery_QUERY_ALL can be
		// used.

		Convey("Unpaginated, unfiltered", func() {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				Sort:  apipb.SortQuery_QUERY_STARTED_TS,
				State: apipb.StateQuery_QUERY_ALL,
			})
			So(err, ShouldBeNil)
			So(payloads(resp), ShouldResemble, []string{
				"KILLED-9s",
				"COMPLETED-8s",
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
				"KILLED-1s",
				"COMPLETED-0s",
			})
		})

		Convey("Unpaginated, filtered", func() {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				Sort:  apipb.SortQuery_QUERY_STARTED_TS,
				Start: timestamppb.New(TestTime.Add(2 * time.Second)), // inclusive
				End:   timestamppb.New(TestTime.Add(8 * time.Second)), // non-inclusive
				State: apipb.StateQuery_QUERY_ALL,
			})
			So(err, ShouldBeNil)
			So(payloads(resp), ShouldResemble, []string{
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
			})
		})

		Convey("Paginated, filtered", func() {
			resp, err := paginatedCall(&apipb.BotTasksRequest{
				BotId: botID,
				Sort:  apipb.SortQuery_QUERY_STARTED_TS,
				Start: timestamppb.New(TestTime.Add(2 * time.Second)), // inclusive
				End:   timestamppb.New(TestTime.Add(8 * time.Second)), // non-inclusive
				State: apipb.StateQuery_QUERY_ALL,
			})
			So(err, ShouldBeNil)
			So(payloads(resp), ShouldResemble, []string{
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
			})
		})
	})

	Convey("Performance stats included", t, func() {
		countTasksWithPerf := func(tasks []*apipb.TaskResultResponse) int {
			tasksWithPerf := 0
			for _, task := range tasks {
				if task.PerformanceStats != nil {
					So(task.PerformanceStats.BotOverhead, ShouldEqual, float32(123.0))
					tasksWithPerf++
				}
			}
			return tasksWithPerf
		}

		resp, err := call(&apipb.BotTasksRequest{
			BotId:                   botID,
			State:                   apipb.StateQuery_QUERY_ALL,
			IncludePerformanceStats: true,
		})
		So(err, ShouldBeNil)
		// All tasks have performance stats (since they ran on the bot).
		So(countTasksWithPerf(resp.Items), ShouldEqual, len(resp.Items))

		resp, err = call(&apipb.BotTasksRequest{
			BotId:                   botID,
			State:                   apipb.StateQuery_QUERY_ALL,
			IncludePerformanceStats: false,
		})
		So(err, ShouldBeNil)
		// All performance stats are omitted if IncludePerformanceStats is false.
		So(countTasksWithPerf(resp.Items), ShouldEqual, 0)
	})
}
