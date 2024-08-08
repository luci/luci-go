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
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCountTasks(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	state, _ := SetupTestTasks(ctx)

	startTS := timestamppb.New(TestTime)
	endTS := timestamppb.New(TestTime.Add(time.Hour))

	callImpl := func(ctx context.Context, req *apipb.TasksCountRequest) (*apipb.TasksCount, error) {
		return (&TasksServer{
			// memory.Use(...) datastore fake doesn't support IN queries currently.
			TaskQuerySplitMode: model.SplitCompletely,
		}).CountTasks(ctx, req)
	}
	call := func(req *apipb.TasksCountRequest) (*apipb.TasksCount, error) {
		return callImpl(MockRequestState(ctx, state), req)
	}
	callAsAdmin := func(req *apipb.TasksCountRequest) (*apipb.TasksCount, error) {
		return callImpl(MockRequestState(ctx, state.SetCaller(AdminFakeCaller)), req)
	}

	ftt.Run("Tags filter is checked", t, func(t *ftt.Test) {
		_, err := callAsAdmin(&apipb.TasksCountRequest{
			Start: startTS,
			End:   endTS,
			Tags:  []string{"k:"},
		})
		assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.InvalidArgument))
	})

	ftt.Run("Time range is checked", t, func(t *ftt.Test) {
		t.Run("No start time", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksCountRequest{})
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("start timestamp is required"))
		})

		t.Run("Ancient start time", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: timestamppb.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
			})
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid time range"))
		})

		t.Run("Ancient end time", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: startTS,
				End:   timestamppb.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
			})
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid time range"))
		})

		t.Run("End must be after start", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: endTS,
				End:   startTS,
			})
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid time range"))
		})
	})

	ftt.Run("ACLs", t, func(t *ftt.Test) {
		t.Run("Listing only visible pools: OK", func(t *ftt.Test) {
			_, err := call(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|visible-pool2"},
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Listing visible and invisible pool: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|hidden-pool1"},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
		})

		t.Run("Listing visible and invisible pool as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|hidden-pool1"},
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Listing all pools as non-admin: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
			})
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
		})

		t.Run("Listing all pools as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
			})
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run("Filtering", t, func(t *ftt.Test) {
		endRange := endTS

		count := func(state apipb.StateQuery, tags ...string) int {
			resp, err := callAsAdmin(&apipb.TasksCountRequest{
				State: state,
				Start: startTS,
				End:   endRange,
				Tags:  tags,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Now, should.NotBeNil)
			return int(resp.Count)
		}

		// No filters at all.
		assert.Loosely(t, count(apipb.StateQuery_QUERY_ALL), should.Equal(36))

		// State filters on their own (without tag filtering).
		assert.Loosely(t, count(apipb.StateQuery_QUERY_PENDING), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_RUNNING), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_PENDING_RUNNING), should.Equal(6))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_COMPLETED), should.Equal(9))         // success+failure+dedup
		assert.Loosely(t, count(apipb.StateQuery_QUERY_COMPLETED_SUCCESS), should.Equal(6)) // success+dedup
		assert.Loosely(t, count(apipb.StateQuery_QUERY_COMPLETED_FAILURE), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_EXPIRED), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_TIMED_OUT), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_BOT_DIED), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_CANCELED), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_DEDUPED), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_KILLED), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_NO_RESOURCE), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_CLIENT_ERROR), should.Equal(3))

		// Simple tags filter.
		assert.Loosely(t, count(apipb.StateQuery_QUERY_ALL, "idx:0"), should.Equal(12))
		// AND tags filter.
		assert.Loosely(t, count(apipb.StateQuery_QUERY_ALL, "idx:0", "pfx:pending"), should.Equal(1))
		// OR tags filter.
		assert.Loosely(t, count(apipb.StateQuery_QUERY_ALL, "idx:0|1"), should.Equal(24))
		// OR tags filter with intersecting results.
		assert.Loosely(t, count(apipb.StateQuery_QUERY_ALL, "idx:0|1", "dup:0|1"), should.Equal(24))
		// OR tags filter with no results.
		assert.Loosely(t, count(apipb.StateQuery_QUERY_ALL, "idx:4|5|6"), should.BeZero)

		// Filtering on state + tags (selected non-trivial cases).
		assert.Loosely(t, count(apipb.StateQuery_QUERY_PENDING, "idx:0|1", "dup:0|1"), should.Equal(2))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_PENDING_RUNNING, "idx:0|1", "dup:0|1"), should.Equal(4))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_COMPLETED, "idx:0|1", "dup:0|1"), should.Equal(6))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_COMPLETED_SUCCESS, "idx:0|1", "dup:0|1"), should.Equal(4))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_DEDUPED, "idx:0|1", "dup:0|1"), should.Equal(2))

		// Limited time range (covers only 1 mocked task per category instead of 3).
		endRange = timestamppb.New(TestTime.Add(5 * time.Minute))

		assert.Loosely(t, count(apipb.StateQuery_QUERY_ALL), should.Equal(12))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_ALL, "idx:0"), should.Equal(12))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_ALL, "idx:1"), should.BeZero)
		assert.Loosely(t, count(apipb.StateQuery_QUERY_COMPLETED), should.Equal(3))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_PENDING_RUNNING), should.Equal(2))
		assert.Loosely(t, count(apipb.StateQuery_QUERY_PENDING_RUNNING, "idx:0|1", "dup:0|1"), should.Equal(2))
	})
}
