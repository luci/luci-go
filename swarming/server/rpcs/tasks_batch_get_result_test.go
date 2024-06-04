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

func TestBatchGetResult(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	state, tasks := SetupTestTasks(ctx)

	call := func(req *apipb.BatchGetResultRequest) (*apipb.BatchGetResultResponse, error) {
		return (&TasksServer{}).BatchGetResult(MockRequestState(ctx, state), req)
	}

	respCodes := func(resp *apipb.BatchGetResultResponse) (out []codes.Code) {
		for _, r := range resp.Results {
			if err := r.GetError(); err != nil {
				out = append(out, codes.Code(err.Code))
			} else {
				out = append(out, codes.OK)
			}
		}
		return
	}

	Convey("Requires task_ids", t, func() {
		_, err := call(&apipb.BatchGetResultRequest{})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		So(err, ShouldErrLike, "task_ids is required")
	})

	Convey("Limits task_ids", t, func() {
		_, err := call(&apipb.BatchGetResultRequest{
			TaskIds: make([]string, 301),
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		So(err, ShouldErrLike, "task_ids length should be no more than")
	})

	Convey("Checks task_ids are valid", t, func() {
		_, err := call(&apipb.BatchGetResultRequest{
			TaskIds: []string{tasks["running-0"], "zzz"},
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		So(err, ShouldErrLike, "task_ids: zzz: bad task ID")
	})

	Convey("Checks for dups", t, func() {
		_, err := call(&apipb.BatchGetResultRequest{
			TaskIds: []string{
				tasks["running-0"],
				tasks["running-1"],
				tasks["running-2"],
				tasks["running-0"],
			},
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		So(err, ShouldErrLike, fmt.Sprintf("%s is specified more than once", tasks["running-0"]))
	})

	Convey("All missing", t, func() {
		resp, err := call(&apipb.BatchGetResultRequest{
			TaskIds: []string{
				tasks["missing-0"],
				tasks["missing-1"],
				tasks["missing-2"],
			},
		})
		So(err, ShouldBeNil)
		So(respCodes(resp), ShouldResemble, []codes.Code{
			codes.NotFound,
			codes.NotFound,
			codes.NotFound,
		})
	})

	Convey("All access denied", t, func() {
		resp, err := call(&apipb.BatchGetResultRequest{
			TaskIds: []string{
				tasks["running-1"],
				tasks["pending-1"],
				tasks["failure-1"],
			},
		})
		So(err, ShouldBeNil)
		So(respCodes(resp), ShouldResemble, []codes.Code{
			codes.PermissionDenied,
			codes.PermissionDenied,
			codes.PermissionDenied,
		})
	})

	Convey("Mix of codes", t, func() {
		resp, err := call(&apipb.BatchGetResultRequest{
			TaskIds: []string{
				tasks["running-0"], // visible
				tasks["missing-0"], // non-existent
				tasks["running-1"], // not visible
				tasks["success-0"], // visible
				tasks["missing-1"], // non-existent
				tasks["pending-1"], // not visible
				tasks["failure-0"], // visible
				tasks["missing-2"], // non-existent
				tasks["pending-0"], // visible
			},
			IncludePerformanceStats: true,
		})
		So(err, ShouldBeNil)

		// Fetched correct data.
		type result struct {
			taskID string
			code   codes.Code
			name   string
			perf   bool
		}
		var results []result
		for _, r := range resp.Results {
			if res := r.GetResult(); res != nil {
				So(res.TaskId, ShouldEqual, r.TaskId)
				results = append(results, result{
					taskID: r.TaskId,
					code:   codes.OK,
					name:   res.Name,
					perf:   res.PerformanceStats != nil,
				})
			} else {
				results = append(results, result{
					taskID: r.TaskId,
					code:   codes.Code(r.GetError().Code),
				})
			}
		}
		So(results, ShouldResemble, []result{
			{tasks["running-0"], codes.OK, "running-0", false},
			{tasks["missing-0"], codes.NotFound, "", false},
			{tasks["running-1"], codes.PermissionDenied, "", false},
			{tasks["success-0"], codes.OK, "success-0", true},
			{tasks["missing-1"], codes.NotFound, "", false},
			{tasks["pending-1"], codes.PermissionDenied, "", false},
			{tasks["failure-0"], codes.OK, "failure-0", true},
			{tasks["missing-2"], codes.NotFound, "", false},
			{tasks["pending-0"], codes.OK, "pending-0", false},
		})
	})
}
