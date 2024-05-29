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

package rpcs

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetRequest(t *testing.T) {
	t.Parallel()

	state := NewMockedRequestState()
	state.MockPerm("project:visible-realm", acls.PermTasksGet)

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	visibleReq, _ := model.TimestampToRequestKey(ctx, TestTime, 1)
	hiddenReq, _ := model.TimestampToRequestKey(ctx, TestTime, 2)
	missingReq, _ := model.TimestampToRequestKey(ctx, TestTime, 3)

	_ = datastore.Put(ctx,
		&model.TaskRequest{
			Key:   visibleReq,
			Realm: "project:visible-realm",
			TaskSlices: []model.TaskSlice{
				{
					Properties: model.TaskProperties{
						Dimensions: model.TaskDimensions{
							"pool": {"some-pool"},
						},
					},
				},
			},
		},
		&model.TaskRequest{
			Key:   hiddenReq,
			Realm: "project:hidden-realm",
			TaskSlices: []model.TaskSlice{
				{
					Properties: model.TaskProperties{
						Dimensions: model.TaskDimensions{
							"pool": {"some-pool"},
						},
					},
				},
			},
		},
	)

	call := func(taskID string) (*apipb.TaskRequestResponse, error) {
		ctx := MockRequestState(ctx, state)
		return (&TasksServer{}).GetRequest(ctx, &apipb.TaskIdRequest{
			TaskId: taskID,
		})
	}

	Convey("Bad task ID", t, func() {
		_, err := call("")
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("Missing task", t, func() {
		// Note: existence of a task is not a secret (task IDs are predictable).
		_, err := call(model.RequestKeyToTaskID(missingReq, model.AsRequest))
		So(err, ShouldHaveGRPCStatus, codes.NotFound)
	})

	Convey("No permissions", t, func() {
		_, err := call(model.RequestKeyToTaskID(hiddenReq, model.AsRequest))
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
	})

	Convey("OK", t, func() {
		resp, err := call(model.RequestKeyToTaskID(visibleReq, model.AsRequest))
		So(err, ShouldBeNil)
		So(resp.TaskId, ShouldEqual, model.RequestKeyToTaskID(visibleReq, model.AsRequest))
	})

	Convey("OK via ID of RunResult", t, func() {
		resp, err := call(model.RequestKeyToTaskID(visibleReq, model.AsRunResult))
		So(err, ShouldBeNil)
		So(resp.TaskId, ShouldEqual, model.RequestKeyToTaskID(visibleReq, model.AsRequest))
	})
}
