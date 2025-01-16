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
	"go.chromium.org/luci/swarming/server/model"
)

func TestListTaskRequests(t *testing.T) {
	// Note: this test is extremely similar to TestListTasks. They should likely
	// be updated at the same time.

	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	state, _ := SetupTestTasks(ctx)

	startTS := timestamppb.New(TestTime)
	endTS := timestamppb.New(TestTime.Add(time.Hour))

	callImpl := func(ctx context.Context, req *apipb.TasksRequest) (*apipb.TaskRequestsResponse, error) {
		return (&TasksServer{
			// memory.Use(...) datastore fake doesn't support IN queries currently.
			TaskQuerySplitMode: model.SplitCompletely,
		}).ListTaskRequests(ctx, req)
	}
	call := func(req *apipb.TasksRequest) (*apipb.TaskRequestsResponse, error) {
		return callImpl(MockRequestState(ctx, state), req)
	}
	callAsAdmin := func(req *apipb.TasksRequest) (*apipb.TaskRequestsResponse, error) {
		return callImpl(MockRequestState(ctx, state.SetCaller(AdminFakeCaller)), req)
	}

	ftt.Run("Limit is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.TasksRequest{
			Limit: -10,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		_, err = call(&apipb.TasksRequest{
			Limit: 1001,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Cursor is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.TasksRequest{
			Cursor: "!!!!",
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Tags filter is checked", t, func(t *ftt.Test) {
		_, err := callAsAdmin(&apipb.TasksRequest{
			Start: startTS,
			End:   endTS,
			Tags:  []string{"k:"},
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Time range is checked", t, func(t *ftt.Test) {
		t.Run("Ancient start time", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksRequest{
				Start: timestamppb.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid time range"))
		})

		t.Run("Ancient end time", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksRequest{
				Start: startTS,
				End:   timestamppb.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid time range"))
		})

		t.Run("End must be after start", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksRequest{
				Start: endTS,
				End:   startTS,
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid time range"))
		})
	})

	ftt.Run("ACLs", t, func(t *ftt.Test) {
		t.Run("Listing only visible pools: OK", func(t *ftt.Test) {
			_, err := call(&apipb.TasksRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|visible-pool2"},
			})
			assert.NoErr(t, err)
		})

		t.Run("Listing visible and invisible pool: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.TasksRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|hidden-pool1"},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("Listing visible and invisible pool as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|hidden-pool1"},
			})
			assert.NoErr(t, err)
		})

		t.Run("Listing all pools as non-admin: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.TasksRequest{
				Start: startTS,
				End:   endTS,
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("Listing all pools as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksRequest{
				Start: startTS,
				End:   endTS,
			})
			assert.NoErr(t, err)
		})
	})

	ftt.Run("Filtering", t, func(t *ftt.Test) {
		endRange := endTS

		checkQuery := func(state apipb.StateQuery, tags []string, expected []string) {
			// One single fetch.
			resp, err := callAsAdmin(&apipb.TasksRequest{
				State: state,
				Start: startTS,
				End:   endRange,
				Tags:  tags,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Now, should.NotBeNil)
			var got []string
			for _, t := range resp.Items {
				got = append(got, t.Name)
			}
			assert.Loosely(t, got, should.Resemble(expected))

			// With pagination.
			cursor := ""
			got = nil
			for {
				resp, err := callAsAdmin(&apipb.TasksRequest{
					State:  state,
					Start:  startTS,
					End:    endRange,
					Tags:   tags,
					Cursor: cursor,
					Limit:  2,
				})
				assert.NoErr(t, err)
				for _, t := range resp.Items {
					got = append(got, t.Name)
				}
				cursor = resp.Cursor
				if cursor == "" {
					break
				}
			}
			assert.Loosely(t, got, should.Resemble(expected))
		}

		// A helper to generate expected results. See SetupTestTask.
		expect := func(shards int, states ...string) (out []string) {
			if states == nil {
				// Note the order matches the task submission order in SetupTestTask.
				states = []string{
					"clienterror",
					"noresource",
					"killed",
					"canceled",
					"botdead",
					"timeout",
					"expired",
					"dedup",
					"failure",
					"success",
					"running",
					"pending",
				}
			}
			for idx := shards - 1; idx >= 0; idx-- {
				for _, sfx := range states {
					out = append(out, fmt.Sprintf("%s-%d", sfx, idx))
				}
			}
			return
		}

		// No filters at all. Returns all 36 tasks (most recent first).
		checkQuery(apipb.StateQuery_QUERY_ALL, nil,
			expect(3),
		)

		// State filters on their own (without tag filtering).
		checkQuery(apipb.StateQuery_QUERY_PENDING, nil,
			expect(3, "pending"),
		)
		checkQuery(apipb.StateQuery_QUERY_RUNNING, nil,
			expect(3, "running"),
		)
		checkQuery(apipb.StateQuery_QUERY_PENDING_RUNNING, nil,
			expect(3, "running", "pending"),
		)
		checkQuery(apipb.StateQuery_QUERY_COMPLETED, nil,
			expect(3, "dedup", "failure", "success"),
		)
		checkQuery(apipb.StateQuery_QUERY_COMPLETED_SUCCESS, nil,
			expect(3, "dedup", "success"),
		)
		checkQuery(apipb.StateQuery_QUERY_COMPLETED_FAILURE, nil,
			expect(3, "failure"),
		)
		checkQuery(apipb.StateQuery_QUERY_EXPIRED, nil,
			expect(3, "expired"),
		)
		checkQuery(apipb.StateQuery_QUERY_TIMED_OUT, nil,
			expect(3, "timeout"),
		)
		checkQuery(apipb.StateQuery_QUERY_BOT_DIED, nil,
			expect(3, "botdead"),
		)
		checkQuery(apipb.StateQuery_QUERY_CANCELED, nil,
			expect(3, "canceled"),
		)
		checkQuery(apipb.StateQuery_QUERY_DEDUPED, nil,
			expect(3, "dedup"),
		)
		checkQuery(apipb.StateQuery_QUERY_KILLED, nil,
			expect(3, "killed"),
		)
		checkQuery(apipb.StateQuery_QUERY_NO_RESOURCE, nil,
			expect(3, "noresource"),
		)
		checkQuery(apipb.StateQuery_QUERY_CLIENT_ERROR, nil,
			expect(3, "clienterror"),
		)

		// Simple tags filter.
		checkQuery(apipb.StateQuery_QUERY_ALL, []string{"idx:0"},
			expect(1),
		)
		// AND tags filter.
		checkQuery(apipb.StateQuery_QUERY_ALL, []string{"idx:0", "pfx:pending"},
			expect(1, "pending"),
		)
		// OR tags filter.
		checkQuery(apipb.StateQuery_QUERY_ALL, []string{"idx:0|1"},
			expect(2),
		)
		// OR tags filter with intersecting results.
		checkQuery(apipb.StateQuery_QUERY_ALL, []string{"idx:0|1", "dup:0|1"},
			expect(2),
		)
		// OR tags filter with no results.
		checkQuery(apipb.StateQuery_QUERY_ALL, []string{"idx:4|5|6"},
			nil,
		)

		// Filtering on state + tags (selected non-trivial cases).
		checkQuery(apipb.StateQuery_QUERY_PENDING, []string{"idx:0|1", "dup:0|1"},
			expect(2, "pending"),
		)
		checkQuery(apipb.StateQuery_QUERY_PENDING_RUNNING, []string{"idx:0|1", "dup:0|1"},
			expect(2, "running", "pending"),
		)
		checkQuery(apipb.StateQuery_QUERY_COMPLETED, []string{"idx:0|1", "dup:0|1"},
			expect(2, "dedup", "failure", "success"),
		)
		checkQuery(apipb.StateQuery_QUERY_COMPLETED_SUCCESS, []string{"idx:0|1", "dup:0|1"},
			expect(2, "dedup", "success"),
		)
		checkQuery(apipb.StateQuery_QUERY_DEDUPED, []string{"idx:0|1", "dup:0|1"},
			expect(2, "dedup"),
		)

		// Limited time range (covers only 1 shard instead of 3).
		endRange = timestamppb.New(TestTime.Add(5 * time.Minute))

		checkQuery(apipb.StateQuery_QUERY_ALL, nil,
			expect(1),
		)
		checkQuery(apipb.StateQuery_QUERY_ALL, []string{"idx:0"},
			expect(1),
		)
		checkQuery(apipb.StateQuery_QUERY_ALL, []string{"idx:1"},
			nil,
		)
		checkQuery(apipb.StateQuery_QUERY_COMPLETED, nil,
			expect(1, "dedup", "failure", "success"),
		)
		checkQuery(apipb.StateQuery_QUERY_PENDING_RUNNING, nil,
			expect(1, "running", "pending"),
		)
		checkQuery(apipb.StateQuery_QUERY_PENDING_RUNNING, []string{"idx:0|1", "dup:0|1"},
			expect(1, "running", "pending"),
		)
	})
}
