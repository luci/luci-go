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

	"google.golang.org/grpc/codes"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

func TestTaskBackendCancelTasks(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	state, taskEnts := SetupTestTasks(ctx)

	lt := tasks.MockTQTasks()
	call := func(taskIDs []string, target string) (*bbpb.CancelTasksResponse, error) {
		var ids []*bbpb.TaskID
		for _, taskID := range taskIDs {
			ids = append(ids, &bbpb.TaskID{
				Target: target,
				Id:     taskID,
			})
		}
		return (&TaskBackend{
			BuildbucketTarget:       "swarming://target",
			BuildbucketAccount:      "ignored-in-the-test",
			DisableBuildbucketCheck: true,
			TasksServer:             &TasksServer{TaskLifecycleTasks: lt},
		}).CancelTasks(MockRequestState(ctx, state), &bbpb.CancelTasksRequest{
			TaskIds: ids,
		})
	}

	respCodes := func(resp *bbpb.CancelTasksResponse) (out []codes.Code) {
		for _, r := range resp.Responses {
			if err := r.GetError(); err != nil {
				out = append(out, codes.Code(err.Code))
			} else {
				out = append(out, codes.OK)
			}
		}
		return
	}

	ftt.Run("No task_ids", t, func(t *ftt.Test) {
		resp, err := call(nil, "swarming://target")
		assert.NoErr(t, err)
		assert.Loosely(t, resp.Responses, should.HaveLength(0))
	})

	ftt.Run("Limits task_ids", t, func(t *ftt.Test) {
		_, err := call(make([]string, cancelTasksLimit+1), "swarming://target")
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("the allowed max is"))
	})

	ftt.Run("Checks task_ids are valid", t, func(t *ftt.Test) {
		resp, err := call([]string{taskEnts["running-0"], "zzz", taskEnts["pending-0"]}, "swarming://target")
		assert.NoErr(t, err)
		assert.Loosely(t, respCodes(resp), should.Match([]codes.Code{codes.OK, codes.InvalidArgument, codes.OK}))
		assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("bad task ID"))
	})

	ftt.Run("Checks target", t, func(t *ftt.Test) {
		resp, err := call([]string{taskEnts["running-0"], "zzz", taskEnts["pending-0"]}, "wrong target")
		assert.NoErr(t, err)
		assert.Loosely(t, respCodes(resp), should.Match([]codes.Code{codes.InvalidArgument, codes.InvalidArgument, codes.InvalidArgument}))
		assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("wrong buildbucket target"))
	})

	ftt.Run("Cancel one task", t, func(t *ftt.Test) {
		t.Run("pending", func(t *ftt.Test) {
			taskID := taskEnts["pending-0"]
			reqKey, _ := model.TaskIDToRequestKey(ctx, taskID)
			trs := &model.TaskRunResult{
				Key: model.TaskResultSummaryKey(ctx, reqKey),
			}
			_ = datastore.Get(ctx, trs)
			before := *trs // keep a copy to resume its original state after the test.
			resp, err := call([]string{taskEnts["pending-0"]}, "swarming://target")
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Responses, should.HaveLength(1))
			assert.Loosely(t, resp.Responses[0].GetTask().GetStatus(), should.Equal(bbpb.Status_CANCELED))
			assert.Loosely(t, lt.PopTask("rbe-cancel"), should.Equal("/reservation"))
			_ = datastore.Put(ctx, &before)
		})

		t.Run("running", func(t *ftt.Test) {
			taskID := taskEnts["running-0"]
			resp, err := call([]string{taskID}, "swarming://target")
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Responses, should.HaveLength(1))
			assert.Loosely(t, resp.Responses[0].GetTask().GetStatus(), should.Equal(bbpb.Status_STARTED))
			reqKey, _ := model.TaskIDToRequestKey(ctx, taskID)
			trr := &model.TaskRunResult{
				Key: model.TaskRunResultKey(ctx, reqKey),
			}
			assert.NoErr(t, datastore.Get(ctx, trr))
			assert.Loosely(t, trr.Killing, should.BeTrue)
			assert.Loosely(t, lt.PopTask("cancel-children-tasks-go"), should.Equal(taskID))
		})

		t.Run("ended", func(t *ftt.Test) {
			resp, err := call([]string{taskEnts["success-0"]}, "swarming://target")
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Responses, should.HaveLength(1))
			assert.Loosely(t, resp.Responses[0].GetTask().GetStatus(), should.Equal(bbpb.Status_SUCCESS))
		})
	})

	ftt.Run("Cancel multiple tasks", t, func(t *ftt.Test) {
		tqMsg := func(taskIDs []string) string {
			sort.Strings(taskIDs)
			return fmt.Sprintf("%q, purpose: TaskBackend.CancelTasks, retry # 0", taskIDs)
		}

		t.Run("success", func(t *ftt.Test) {
			resp, err := call([]string{taskEnts["pending-0"], taskEnts["running-0"], taskEnts["failure-0"]}, "swarming://target")
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Responses, should.HaveLength(3))
			assert.Loosely(t, resp.Responses[0].GetTask().GetStatus(), should.Equal(bbpb.Status_SCHEDULED))
			assert.Loosely(t, resp.Responses[1].GetTask().GetStatus(), should.Equal(bbpb.Status_STARTED))
			assert.Loosely(t, resp.Responses[2].GetTask().GetStatus(), should.Equal(bbpb.Status_FAILURE))
			assert.Loosely(t, lt.PopTask("cancel-tasks-go"), should.Equal(tqMsg([]string{taskEnts["pending-0"], taskEnts["running-0"]})))
		})

		// Below tests are the same as the ones in TestTaskBackendFetchTasks.
		t.Run("Missing task", func(t *ftt.Test) {
			resp, err := call([]string{taskEnts["running-0"], taskEnts["missing-0"], taskEnts["pending-0"]}, "swarming://target")
			assert.NoErr(t, err)
			assert.Loosely(t, respCodes(resp), should.Match([]codes.Code{codes.OK, codes.NotFound, codes.OK}))
			assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("no such task"))
			assert.Loosely(t, lt.PopTask("cancel-tasks-go"), should.Equal(tqMsg([]string{taskEnts["pending-0"], taskEnts["running-0"]})))
		})

		t.Run("Missing BuildTask", func(t *ftt.Test) {
			resp, err := call([]string{taskEnts["running-0"], taskEnts["dedup-0"], taskEnts["pending-0"]}, "swarming://target")
			assert.NoErr(t, err)
			assert.Loosely(t, respCodes(resp), should.Match([]codes.Code{codes.OK, codes.NotFound, codes.OK}))
			assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("not a Buildbucket task"))
		})

		t.Run("No permission", func(t *ftt.Test) {
			resp, err := call([]string{taskEnts["running-0"], taskEnts["success-1"], taskEnts["pending-0"]}, "swarming://target")
			assert.NoErr(t, err)
			assert.Loosely(t, respCodes(resp), should.Match([]codes.Code{codes.OK, codes.PermissionDenied, codes.OK}))
			assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("doesn't have permission"))
		})

		t.Run("Many kinds of errors at once", func(t *ftt.Test) {
			resp, err := call([]string{
				taskEnts["running-0"], // OK
				"zzz",                 // bad task ID
				taskEnts["success-0"], // OK
				taskEnts["success-1"], // no access
				taskEnts["pending-0"], // OK
				taskEnts["missing-0"], // missing task
				taskEnts["failure-0"], // OK
				taskEnts["dedup-0"],   // missing BuildTask
				taskEnts["expired-0"], // OK
			}, "swarming://target")
			assert.NoErr(t, err)
			assert.Loosely(t, respCodes(resp), should.Match([]codes.Code{
				codes.OK,
				codes.InvalidArgument,
				codes.OK,
				codes.PermissionDenied,
				codes.OK,
				codes.NotFound,
				codes.OK,
				codes.NotFound,
				codes.OK,
			}))
		})

	})
}
