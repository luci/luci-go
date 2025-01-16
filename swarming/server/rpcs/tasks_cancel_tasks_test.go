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
	"sort"
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
	"go.chromium.org/luci/swarming/server/tasks"
)

func TestCancelTasks(t *testing.T) {
	// Note: this test is extremely similar to TestListTaskRequests. They should
	// likely be updated at the same time.

	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	state, taskMap := SetupTestTasks(ctx)

	startTS := timestamppb.New(TestTime)
	endTS := timestamppb.New(TestTime.Add(time.Hour))

	lt := tasks.MockTQTasks()
	callImpl := func(ctx context.Context, req *apipb.TasksCancelRequest) (*apipb.TasksCancelResponse, error) {
		return (&TasksServer{
			// memory.Use(...) datastore fake doesn't support IN queries currently.
			TaskQuerySplitMode: model.SplitCompletely,
			TaskLifecycleTasks: lt,
		}).CancelTasks(ctx, req)
	}
	call := func(req *apipb.TasksCancelRequest) (*apipb.TasksCancelResponse, error) {
		return callImpl(MockRequestState(ctx, state), req)
	}
	callAsAdmin := func(req *apipb.TasksCancelRequest) (*apipb.TasksCancelResponse, error) {
		return callImpl(MockRequestState(ctx, state.SetCaller(AdminFakeCaller)), req)
	}

	ftt.Run("Limit is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.TasksCancelRequest{
			Limit: -10,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		_, err = call(&apipb.TasksCancelRequest{
			Limit: 1001,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Tags are required", t, func(t *ftt.Test) {
		_, err := call(&apipb.TasksCancelRequest{})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("tags are required when cancelling multiple tasks"))
	})

	ftt.Run("ACLs", t, func(t *ftt.Test) {
		t.Run("Cancelling only visible pools: OK", func(t *ftt.Test) {
			_, err := call(&apipb.TasksCancelRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|visible-pool2"},
			})
			assert.NoErr(t, err)
		})

		t.Run("Cancelling visible and invisible pool: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.TasksCancelRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|hidden-pool1"},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("Cancelling visible and invisible pool as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksCancelRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|hidden-pool1"},
			})
			assert.NoErr(t, err)
		})

		t.Run("Cancelling all pools as non-admin: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.TasksCancelRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"k:v"},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("Cancelling all pools as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.TasksCancelRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"k:v"},
			})
			assert.NoErr(t, err)
		})
	})

	ftt.Run("Cancelling", t, func(t *ftt.Test) {
		endRange := endTS

		cancel := func(killRunning bool, tags []string, expected []string) {
			// One single fetch.
			resp, err := callAsAdmin(&apipb.TasksCancelRequest{
				KillRunning: killRunning,
				Start:       startTS,
				End:         endRange,
				Tags:        tags,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Now, should.NotBeNil)
			assert.Loosely(t, resp.Matched, should.Equal(len(expected)))
			assert.Loosely(t, lt.PopTask("cancel-tasks-go"), should.Equal(fmt.Sprintf("%q, purpose: CancelTasks, retry # 0", expected)))

			// With pagination.
			cursor := ""
			var got int32
			for {
				resp, err := callAsAdmin(&apipb.TasksCancelRequest{
					KillRunning: killRunning,
					Start:       startTS,
					End:         endRange,
					Tags:        tags,
					Cursor:      cursor,
					Limit:       2,
				})
				assert.NoErr(t, err)
				cursor = resp.Cursor
				got += resp.Matched
				if cursor == "" {
					break
				}
			}
			assert.Loosely(t, got, should.Equal(len(expected)))
			assert.Loosely(t, lt.PopNTasks("cancel-tasks-go", len(expected)), should.HaveLength(len(expected)/2))
		}

		// A helper to generate expected results. See SetupTestTask.
		expect := func(shards int, states ...string) (out []string) {
			for idx := shards - 1; idx >= 0; idx-- {
				for _, sfx := range states {
					out = append(out, taskMap[fmt.Sprintf("%s-%d", sfx, idx)])
				}
			}
			sort.Strings(out)
			return
		}

		cancel(false, []string{"idx:0|1", "dup:0|1"},
			expect(2, "pending"),
		)
		cancel(true, []string{"idx:0|1", "dup:0|1"},
			expect(2, "running", "pending"),
		)

		// Limited time range (covers only 1 shard instead of 3).
		endRange = timestamppb.New(TestTime.Add(5 * time.Minute))
		cancel(true, []string{"idx:0|1", "dup:0|1"},
			expect(1, "running", "pending"),
		)
	})
}
