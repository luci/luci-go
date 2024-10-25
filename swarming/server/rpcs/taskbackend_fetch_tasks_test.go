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

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/swarming/server/model"
)

func TestTaskBackendFetchTasks(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	state, tasks := SetupTestTasks(ctx)

	call := func(taskIDs []string, target string) (*bbpb.FetchTasksResponse, error) {
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
		}).FetchTasks(MockRequestState(ctx, state), &bbpb.FetchTasksRequest{
			TaskIds: ids,
		})
	}

	respCodes := func(resp *bbpb.FetchTasksResponse) (out []codes.Code) {
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, resp.Responses, should.HaveLength(0))
	})

	ftt.Run("Limits task_ids", t, func(t *ftt.Test) {
		_, err := call(make([]string, fetchTasksLimit+1), "swarming://target")
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("the allowed max is"))
	})

	ftt.Run("Checks task_ids are valid", t, func(t *ftt.Test) {
		resp, err := call([]string{tasks["running-0"], "zzz", tasks["pending-0"]}, "swarming://target")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, respCodes(resp), should.Resemble([]codes.Code{codes.OK, codes.InvalidArgument, codes.OK}))
		assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("bad task ID"))
	})

	ftt.Run("Checks target", t, func(t *ftt.Test) {
		resp, err := call([]string{tasks["running-0"], "zzz", tasks["pending-0"]}, "wrong target")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, respCodes(resp), should.Resemble([]codes.Code{codes.InvalidArgument, codes.InvalidArgument, codes.InvalidArgument}))
		assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("wrong buildbucket target"))
	})

	ftt.Run("Success", t, func(t *ftt.Test) {
		expectedTask := func(name, payload string, status bbpb.Status, summary string) *bbpb.Task {
			return &bbpb.Task{
				Status:          status,
				SummaryMarkdown: summary,
				Id: &bbpb.TaskID{
					Id:     tasks[name],
					Target: "swarming://target",
				},
				UpdateId: 100,
				Details: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"bot_dimensions": {
							Kind: &structpb.Value_StructValue{
								StructValue: (model.BotDimensions{"payload": {payload}}).ToStructPB(),
							},
						},
					},
				},
			}
		}

		resp, err := call([]string{tasks["running-0"], tasks["success-0"], tasks["failure-0"], tasks["pending-0"]}, "swarming://target")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, resp.Responses, should.HaveLength(4))

		assert.Loosely(t, resp.Responses[0].GetTask(),
			should.Resemble(
				expectedTask("running-0", "RUNNING-0s", bbpb.Status_STARTED, ""),
			))
		assert.Loosely(t, resp.Responses[1].GetTask(),
			should.Resemble(
				expectedTask("success-0", "COMPLETED-0s", bbpb.Status_SUCCESS, ""),
			))
		assert.Loosely(t, resp.Responses[2].GetTask(),
			should.Resemble(
				expectedTask("failure-0", "COMPLETED-0s", bbpb.Status_FAILURE, "Task completed with failure."),
			))
		assert.Loosely(t, resp.Responses[3].GetTask(),
			should.Resemble(
				expectedTask("pending-0", "PENDING-0s", bbpb.Status_SCHEDULED, ""),
			))
	})

	ftt.Run("Missing task", t, func(t *ftt.Test) {
		resp, err := call([]string{tasks["running-0"], tasks["missing-0"], tasks["pending-0"]}, "swarming://target")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, respCodes(resp), should.Resemble([]codes.Code{codes.OK, codes.NotFound, codes.OK}))
		assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("no such task"))
	})

	ftt.Run("Missing BuildTask", t, func(t *ftt.Test) {
		resp, err := call([]string{tasks["running-0"], tasks["dedup-0"], tasks["pending-0"]}, "swarming://target")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, respCodes(resp), should.Resemble([]codes.Code{codes.OK, codes.NotFound, codes.OK}))
		assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("not a Buildbucket task"))
	})

	ftt.Run("No permission", t, func(t *ftt.Test) {
		resp, err := call([]string{tasks["running-0"], tasks["success-1"], tasks["pending-0"]}, "swarming://target")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, respCodes(resp), should.Resemble([]codes.Code{codes.OK, codes.PermissionDenied, codes.OK}))
		assert.Loosely(t, resp.Responses[1].GetError().Message, should.ContainSubstring("doesn't have permission"))
	})

	ftt.Run("Many kinds of errors at once", t, func(t *ftt.Test) {
		resp, err := call([]string{
			tasks["running-0"], // OK
			"zzz",              // bad task ID
			tasks["success-0"], // OK
			tasks["success-1"], // no access
			tasks["pending-0"], // OK
			tasks["missing-0"], // missing task
			tasks["failure-0"], // OK
			tasks["dedup-0"],   // missing BuildTask
			tasks["expired-0"], // OK
		}, "swarming://target")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, respCodes(resp), should.Resemble([]codes.Code{
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
}
