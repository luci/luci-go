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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey("Requires task_ids", t, func() {
		_, err := call(nil)
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		So(err, ShouldErrLike, "task_ids is required")
	})

	Convey("Limits task_ids", t, func() {
		_, err := call(make([]string, 1001))
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		So(err, ShouldErrLike, "task_ids length should be no more than")
	})

	Convey("Checks task_ids are valid", t, func() {
		_, err := call([]string{tasks["running-0"], "zzz"})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		So(err, ShouldErrLike, "task_ids: zzz: bad task ID")
	})

	Convey("Checks for dups", t, func() {
		_, err := call([]string{
			tasks["running-0"],
			tasks["running-1"],
			tasks["running-2"],
			tasks["running-0"],
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		So(err, ShouldErrLike, fmt.Sprintf("%s is specified more than once", tasks["running-0"]))
	})

	Convey("Missing", t, func() {
		_, err := call([]string{
			tasks["running-0"],
			tasks["missing-0"],
			tasks["pending-0"],
		})
		So(err, ShouldHaveGRPCStatus, codes.NotFound)
		So(err, ShouldErrLike, fmt.Sprintf("%s: no such task", tasks["missing-0"]))
	})

	Convey("Access denied", t, func() {
		_, err := call([]string{
			tasks["running-0"],
			tasks["running-1"],
			tasks["pending-0"],
		})
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		So(err, ShouldErrLike, fmt.Sprintf("%s: ", tasks["running-1"]), "doesn't have permission")
	})

	Convey("Success", t, func() {
		resp, err := call([]string{
			tasks["running-0"],
			tasks["success-0"],
			tasks["failure-0"],
			tasks["pending-0"],
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, []apipb.TaskState{
			apipb.TaskState_RUNNING,
			apipb.TaskState_COMPLETED,
			apipb.TaskState_COMPLETED,
			apipb.TaskState_PENDING,
		})
	})
}
