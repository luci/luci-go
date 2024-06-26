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

package swarming

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/stringset"

	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskResults(t *testing.T) {
	t.Parallel()

	genTasks := func(start, count int) (out []string) {
		for i := 0; i < count; i++ {
			out = append(out, fmt.Sprintf("task-%d", start+i))
		}
		return
	}

	getTaskIDs := func(res []ResultOrErr) (out []string) {
		for _, r := range res {
			if r.Err != nil {
				out = append(out, "error")
			} else {
				out = append(out, r.Result.TaskId)
			}
		}
		return
	}

	Convey("With mocks", t, func() {
		ctx := context.Background()
		mockedRPC := &mockedTasksClient{}
		impl := &swarmingServiceImpl{tasksClient: mockedRPC}
		taskIDs := genTasks(0, 1100) // 3 calls: 500 + 500 + 100

		Convey("OK", func() {
			res, err := impl.TaskResults(ctx, taskIDs, nil)
			So(err, ShouldBeNil)
			So(getTaskIDs(res), ShouldResemble, taskIDs)

			// No overlaps between calls. Requested all tasks. Note that the order of
			// calls is non-deterministic due to use of goroutines.
			seen := stringset.New(0)
			So(mockedRPC.calls, ShouldHaveLength, 3)
			for _, call := range mockedRPC.calls {
				So(len(call) == 500 || len(call) == 100, ShouldBeTrue)
				for _, taskID := range call {
					So(seen.Add(taskID), ShouldBeTrue)
				}
			}
			So(seen.Len(), ShouldEqual, 1100)
		})

		Convey("RPC error", func() {
			// Make one of RPCs fail. It should abort everything.
			mockedRPC.errs = []error{status.Errorf(codes.PermissionDenied, "boom")}
			res, err := impl.TaskResults(ctx, taskIDs, nil)
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
			So(res, ShouldBeNil)
		})
	})
}

type mockedTasksClient struct {
	swarmingv2.TasksClient

	m     sync.Mutex
	calls [][]string
	errs  []error
}

func (c *mockedTasksClient) BatchGetResult(ctx context.Context, in *swarmingv2.BatchGetResultRequest, opts ...grpc.CallOption) (*swarmingv2.BatchGetResultResponse, error) {
	c.m.Lock()
	c.calls = append(c.calls, in.TaskIds)
	var err error
	if len(c.errs) != 0 {
		err, c.errs = c.errs[0], c.errs[1:]
	}
	c.m.Unlock()

	if err != nil {
		return nil, err
	}

	out := make([]*swarmingv2.BatchGetResultResponse_ResultOrError, len(in.TaskIds))
	for i, taskID := range in.TaskIds {
		out[i] = &swarmingv2.BatchGetResultResponse_ResultOrError{
			TaskId: taskID,
			Outcome: &swarmingv2.BatchGetResultResponse_ResultOrError_Result{
				Result: &swarmingv2.TaskResultResponse{
					TaskId: taskID,
					State:  swarmingv2.TaskState_COMPLETED,
				},
			},
		}
	}

	return &swarmingv2.BatchGetResultResponse{Results: out}, nil
}
