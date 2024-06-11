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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskBackendFetchTasks(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	state, tasks := SetupTestTasks(ctx)

	call := func(taskIDs []string) (*bbpb.FetchTasksResponse, error) {
		var ids []*bbpb.TaskID
		for _, taskID := range taskIDs {
			ids = append(ids, &bbpb.TaskID{
				Target: "ignored",
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

	Convey("No task_ids", t, func() {
		resp, err := call(nil)
		So(err, ShouldBeNil)
		So(resp.Responses, ShouldHaveLength, 0)
	})

	Convey("Limits task_ids", t, func() {
		_, err := call(make([]string, fetchTasksLimit+1))
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		So(err, ShouldErrLike, "the allowed max is")
	})

	Convey("Checks task_ids are valid", t, func() {
		resp, err := call([]string{tasks["running-0"], "zzz", tasks["pending-0"]})
		So(err, ShouldBeNil)
		So(respCodes(resp), ShouldResemble, []codes.Code{codes.OK, codes.InvalidArgument, codes.OK})
		So(resp.Responses[1].GetError().Message, ShouldContainSubstring, "bad task ID")
	})

	Convey("Success", t, func() {
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

		resp, err := call([]string{tasks["running-0"], tasks["success-0"], tasks["failure-0"], tasks["pending-0"]})
		So(err, ShouldBeNil)
		So(resp.Responses, ShouldHaveLength, 4)

		So(resp.Responses[0].GetTask(),
			ShouldResembleProto,
			expectedTask("running-0", "RUNNING-0s", bbpb.Status_STARTED, ""),
		)
		So(resp.Responses[1].GetTask(),
			ShouldResembleProto,
			expectedTask("success-0", "COMPLETED-0s", bbpb.Status_SUCCESS, ""),
		)
		So(resp.Responses[2].GetTask(),
			ShouldResembleProto,
			expectedTask("failure-0", "COMPLETED-0s", bbpb.Status_FAILURE, "Task completed with failure."),
		)
		So(resp.Responses[3].GetTask(),
			ShouldResembleProto,
			expectedTask("pending-0", "PENDING-0s", bbpb.Status_SCHEDULED, ""),
		)
	})

	Convey("Missing task", t, func() {
		resp, err := call([]string{tasks["running-0"], tasks["missing-0"], tasks["pending-0"]})
		So(err, ShouldBeNil)
		So(respCodes(resp), ShouldResemble, []codes.Code{codes.OK, codes.NotFound, codes.OK})
		So(resp.Responses[1].GetError().Message, ShouldContainSubstring, "no such task")
	})

	Convey("Missing BuildTask", t, func() {
		resp, err := call([]string{tasks["running-0"], tasks["dedup-0"], tasks["pending-0"]})
		So(err, ShouldBeNil)
		So(respCodes(resp), ShouldResemble, []codes.Code{codes.OK, codes.NotFound, codes.OK})
		So(resp.Responses[1].GetError().Message, ShouldContainSubstring, "not a Buildbucket task")
	})

	Convey("No permission", t, func() {
		resp, err := call([]string{tasks["running-0"], tasks["success-1"], tasks["pending-0"]})
		So(err, ShouldBeNil)
		So(respCodes(resp), ShouldResemble, []codes.Code{codes.OK, codes.PermissionDenied, codes.OK})
		So(resp.Responses[1].GetError().Message, ShouldContainSubstring, "doesn't have permission")
	})

	Convey("Many kinds of errors at once", t, func() {
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
		})
		So(err, ShouldBeNil)
		So(respCodes(resp), ShouldResemble, []codes.Code{
			codes.OK,
			codes.InvalidArgument,
			codes.OK,
			codes.PermissionDenied,
			codes.OK,
			codes.NotFound,
			codes.OK,
			codes.NotFound,
			codes.OK,
		})
	})
}
