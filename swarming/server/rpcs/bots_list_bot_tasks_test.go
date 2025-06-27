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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/secrets"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

func TestListBotTasks(t *testing.T) {
	t.Parallel()

	state := NewMockedRequestState()

	// A bot that should be visible.
	const botID = "visible-bot"
	state.Configs.MockBot(botID, "visible-pool")
	state.Configs.MockPool("visible-pool", "project:visible-realm")
	state.MockPerm("project:visible-realm", acls.PermPoolsListTasks)

	// A bot the caller has no permissions over.
	state.Configs.MockBot("hidden-bot", "hidden-pool")
	state.Configs.MockPool("hidden-pool", "project:hidden-realm")

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	putTask := func(botID string, state apipb.TaskState, ts time.Duration, old bool) {
		reqKey, err := model.TimestampToRequestKey(ctx, TestTime.Add(ts), 0)
		if err != nil {
			panic(err)
		}
		taskReq := &model.TaskRequest{
			Key:     reqKey,
			Created: TestTime,
			Name:    fmt.Sprintf("%s-%s", state, ts),
			Tags:    []string{"a:b", "c:d"},
			User:    "request-user",
		}
		taskRunRes := &model.TaskRunResult{
			TaskResultCommon: model.TaskResultCommon{
				State:         state,
				BotDimensions: map[string][]string{"payload": {fmt.Sprintf("%s-%s", state, ts)}},
				Started:       datastore.NewIndexedNullable(TestTime.Add(ts)),
				Completed:     datastore.NewIndexedNullable(TestTime.Add(ts)),
			},
			Key:   model.TaskRunResultKey(ctx, reqKey),
			BotID: botID,
		}
		if !old {
			taskRunRes.RequestCreated = taskReq.Created
			taskRunRes.RequestName = taskReq.Name
			taskRunRes.RequestTags = taskReq.Tags
			taskRunRes.RequestUser = taskReq.User
		}
		err = datastore.Put(ctx,
			taskReq,
			taskRunRes,
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
	for i := range 10 {
		var state apipb.TaskState
		var old bool
		if i%2 == 0 {
			state = apipb.TaskState_COMPLETED
			old = false
		} else {
			state = apipb.TaskState_KILLED
			old = true
		}
		putTask(botID, state, now, old)
		now += 500 * time.Millisecond
		putTask("another-bot", state, now, old)
		now += 500 * time.Millisecond
	}

	ftt.Run("Bad bot ID", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotTasksRequest{
			BotId: "",
			Limit: 10,
			State: apipb.StateQuery_QUERY_ALL,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Limit is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotTasksRequest{
			BotId: "doesnt-matter",
			Limit: -10,
			State: apipb.StateQuery_QUERY_ALL,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		_, err = call(&apipb.BotTasksRequest{
			BotId: "doesnt-matter",
			Limit: 1001,
			State: apipb.StateQuery_QUERY_ALL,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Cursor is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotTasksRequest{
			BotId:  "doesnt-matter",
			Cursor: "!!!!",
			State:  apipb.StateQuery_QUERY_ALL,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("No permissions", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotTasksRequest{
			BotId: "hidden-bot",
			Limit: 10,
			State: apipb.StateQuery_QUERY_ALL,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})

	ftt.Run("Bot not in a config", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotTasksRequest{
			BotId: "unknown-bot",
			Limit: 10,
			State: apipb.StateQuery_QUERY_ALL,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})

	ftt.Run("Unsupported state filters", t, func(t *ftt.Test) {
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
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("filter"))
		}
	})

	ftt.Run("Unsupported order", t, func(t *ftt.Test) {
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
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("sort"))
		}
	})

	ftt.Run("Order by created_ts", t, func(t *ftt.Test) {
		t.Run("Bad time range", func(t *ftt.Test) {
			_, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				State: apipb.StateQuery_QUERY_ALL,
				Start: timestamppb.New(time.Date(1977, time.January, 1, 2, 3, 4, 0, time.UTC)),
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid start time"))
		})

		t.Run("Unpaginated, unfiltered, all states", func(t *ftt.Test) {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				State: apipb.StateQuery_QUERY_ALL,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, payloads(resp), should.Match([]string{
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
			}))
		})

		t.Run("Unpaginated, unfiltered, COMPLETED only", func(t *ftt.Test) {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				State: apipb.StateQuery_QUERY_COMPLETED,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, payloads(resp), should.Match([]string{
				"COMPLETED-8s",
				"COMPLETED-6s",
				"COMPLETED-4s",
				"COMPLETED-2s",
				"COMPLETED-0s",
			}))
		})

		t.Run("Unpaginated, filtered, all states", func(t *ftt.Test) {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				Start: timestamppb.New(TestTime.Add(2 * time.Second)), // inclusive
				End:   timestamppb.New(TestTime.Add(8 * time.Second)), // non-inclusive
				State: apipb.StateQuery_QUERY_ALL,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, payloads(resp), should.Match([]string{
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
			}))
		})

		t.Run("Paginated, filtered, all states", func(t *ftt.Test) {
			resp, err := paginatedCall(&apipb.BotTasksRequest{
				BotId: botID,
				Start: timestamppb.New(TestTime.Add(2 * time.Second)), // inclusive
				End:   timestamppb.New(TestTime.Add(8 * time.Second)), // non-inclusive
				State: apipb.StateQuery_QUERY_ALL,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, payloads(resp), should.Match([]string{
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
			}))
		})
	})

	ftt.Run("Order by started_ts", t, func(t *ftt.Test) {
		// Note: state filtering is not supported, only StateQuery_QUERY_ALL can be
		// used.

		t.Run("Unpaginated, unfiltered", func(t *ftt.Test) {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				Sort:  apipb.SortQuery_QUERY_STARTED_TS,
				State: apipb.StateQuery_QUERY_ALL,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, payloads(resp), should.Match([]string{
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
			}))
		})

		t.Run("Unpaginated, filtered", func(t *ftt.Test) {
			resp, err := call(&apipb.BotTasksRequest{
				BotId: botID,
				Sort:  apipb.SortQuery_QUERY_STARTED_TS,
				Start: timestamppb.New(TestTime.Add(2 * time.Second)), // inclusive
				End:   timestamppb.New(TestTime.Add(8 * time.Second)), // non-inclusive
				State: apipb.StateQuery_QUERY_ALL,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, payloads(resp), should.Match([]string{
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
			}))
		})

		t.Run("Paginated, filtered", func(t *ftt.Test) {
			resp, err := paginatedCall(&apipb.BotTasksRequest{
				BotId: botID,
				Sort:  apipb.SortQuery_QUERY_STARTED_TS,
				Start: timestamppb.New(TestTime.Add(2 * time.Second)), // inclusive
				End:   timestamppb.New(TestTime.Add(8 * time.Second)), // non-inclusive
				State: apipb.StateQuery_QUERY_ALL,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, payloads(resp), should.Match([]string{
				"KILLED-7s",
				"COMPLETED-6s",
				"KILLED-5s",
				"COMPLETED-4s",
				"KILLED-3s",
				"COMPLETED-2s",
			}))
		})
	})

	ftt.Run("Performance stats included", t, func(t *ftt.Test) {
		countTasksWithPerf := func(tasks []*apipb.TaskResultResponse) int {
			tasksWithPerf := 0
			for _, task := range tasks {
				if task.PerformanceStats != nil {
					assert.Loosely(t, task.PerformanceStats.BotOverhead, should.Equal(float32(123.0)))
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
		assert.NoErr(t, err)
		// All tasks have performance stats (since they ran on the bot).
		assert.Loosely(t, countTasksWithPerf(resp.Items), should.Equal(len(resp.Items)))

		resp, err = call(&apipb.BotTasksRequest{
			BotId:                   botID,
			State:                   apipb.StateQuery_QUERY_ALL,
			IncludePerformanceStats: false,
		})
		assert.NoErr(t, err)
		// All performance stats are omitted if IncludePerformanceStats is false.
		assert.Loosely(t, countTasksWithPerf(resp.Items), should.BeZero)
	})

	ftt.Run("Fetches TaskRequest fields", t, func(t *ftt.Test) {
		resp, err := call(&apipb.BotTasksRequest{
			BotId: botID,
			State: apipb.StateQuery_QUERY_ALL,
		})
		assert.NoErr(t, err)
		var names []string
		for _, task := range resp.Items {
			names = append(names, task.Name)
			assert.Loosely(t, task.CreatedTs, should.Match(timestamppb.New(TestTime)))
			assert.Loosely(t, task.Tags, should.Match([]string{"a:b", "c:d"}))
			assert.Loosely(t, task.User, should.Equal("request-user"))
		}
		assert.Loosely(t, names, should.Match([]string{
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
		}))
	})
}
