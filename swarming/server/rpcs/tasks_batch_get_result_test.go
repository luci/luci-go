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

	ftt.Run("Requires task_ids", t, func(t *ftt.Test) {
		_, err := call(&apipb.BatchGetResultRequest{})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("task_ids is required"))
	})

	ftt.Run("Limits task_ids", t, func(t *ftt.Test) {
		_, err := call(&apipb.BatchGetResultRequest{
			TaskIds: make([]string, 1001),
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("task_ids length should be no more than"))
	})

	ftt.Run("Checks task_ids are valid", t, func(t *ftt.Test) {
		_, err := call(&apipb.BatchGetResultRequest{
			TaskIds: []string{tasks["running-0"], "zzz"},
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("task_ids: zzz: bad task ID"))
	})

	ftt.Run("Checks for dups", t, func(t *ftt.Test) {
		_, err := call(&apipb.BatchGetResultRequest{
			TaskIds: []string{
				tasks["running-0"],
				tasks["running-1"],
				tasks["running-2"],
				tasks["running-0"],
			},
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("%s is specified more than once", tasks["running-0"])))
	})

	ftt.Run("All missing", t, func(t *ftt.Test) {
		resp, err := call(&apipb.BatchGetResultRequest{
			TaskIds: []string{
				tasks["missing-0"],
				tasks["missing-1"],
				tasks["missing-2"],
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, respCodes(resp), should.Resemble([]codes.Code{
			codes.NotFound,
			codes.NotFound,
			codes.NotFound,
		}))
	})

	ftt.Run("All access denied", t, func(t *ftt.Test) {
		resp, err := call(&apipb.BatchGetResultRequest{
			TaskIds: []string{
				tasks["running-1"],
				tasks["pending-1"],
				tasks["failure-1"],
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, respCodes(resp), should.Resemble([]codes.Code{
			codes.PermissionDenied,
			codes.PermissionDenied,
			codes.PermissionDenied,
		}))
	})

	ftt.Run("Mix of codes", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

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
				assert.Loosely(t, res.TaskId, should.Equal(r.TaskId))
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
		assert.Loosely(t, results, should.Resemble([]result{
			{tasks["running-0"], codes.OK, "running-0", false},
			{tasks["missing-0"], codes.NotFound, "", false},
			{tasks["running-1"], codes.PermissionDenied, "", false},
			{tasks["success-0"], codes.OK, "success-0", true},
			{tasks["missing-1"], codes.NotFound, "", false},
			{tasks["pending-1"], codes.PermissionDenied, "", false},
			{tasks["failure-0"], codes.OK, "failure-0", true},
			{tasks["missing-2"], codes.NotFound, "", false},
			{tasks["pending-0"], codes.OK, "pending-0", false},
		}))
	})
}
