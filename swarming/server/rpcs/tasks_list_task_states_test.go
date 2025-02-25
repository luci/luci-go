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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestListTaskStates(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	state, tasks := SetupTestTasks(ctx)

	call := func(taskIDs []string) ([]apipb.TaskState, error) {
		resp, err := (&TasksServer{}).ListTaskStates(MockRequestState(ctx, state), &apipb.TaskStatesRequest{
			TaskId: taskIDs,
		})
		if err != nil {
			return nil, err
		}
		return resp.States, nil
	}

	ftt.Run("Requires task_ids", t, func(t *ftt.Test) {
		_, err := call(nil)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("task_ids is required"))
	})

	ftt.Run("Limits task_ids", t, func(t *ftt.Test) {
		_, err := call(make([]string, 1001))
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("task_ids length should be no more than"))
	})

	ftt.Run("Checks task_ids are valid", t, func(t *ftt.Test) {
		_, err := call([]string{tasks["running-0"], "zzz"})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("task_ids: zzz: bad task ID"))
	})

	ftt.Run("Checks for dups", t, func(t *ftt.Test) {
		_, err := call([]string{
			tasks["running-0"],
			tasks["running-1"],
			tasks["running-2"],
			tasks["running-0"],
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("%s is specified more than once", tasks["running-0"])))
	})

	ftt.Run("Missing", t, func(t *ftt.Test) {
		_, err := call([]string{
			tasks["running-0"],
			tasks["missing-0"],
			tasks["pending-0"],
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
		assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("%s: no such task", tasks["missing-0"])))
	})

	ftt.Run("Access denied", t, func(t *ftt.Test) {
		_, err := call([]string{
			tasks["running-0"],
			tasks["running-1"],
			tasks["pending-0"],
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("%s: ", tasks["running-1"])))
		assert.Loosely(t, err, should.ErrLike("doesn't have permission"))
	})

	ftt.Run("Success", t, func(t *ftt.Test) {
		resp, err := call([]string{
			tasks["running-0"],
			tasks["success-0"],
			tasks["failure-0"],
			tasks["pending-0"],
		})
		assert.NoErr(t, err)
		assert.Loosely(t, resp, should.Match([]apipb.TaskState{
			apipb.TaskState_RUNNING,
			apipb.TaskState_COMPLETED,
			apipb.TaskState_COMPLETED,
			apipb.TaskState_PENDING,
		}))
	})
}
