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

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// BatchGetResult implements the corresponding RPC method.
func (*TasksServer) BatchGetResult(ctx context.Context, req *apipb.BatchGetResultRequest) (*apipb.BatchGetResultResponse, error) {
	if len(req.TaskIds) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "task_ids is required")
	}
	if len(req.TaskIds) > 300 {
		return nil, status.Errorf(codes.InvalidArgument, "task_ids length should be no more than 300")
	}

	// Prepare TaskResultSummary entities to fetch.
	results := make([]*model.TaskResultSummary, len(req.TaskIds))
	seenIDs := make(map[int64]struct{}, len(req.TaskIds))
	for idx, id := range req.TaskIds {
		key, err := model.TaskIDToRequestKey(ctx, id)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "task_ids: %s: %s", id, err)
		}
		if _, seen := seenIDs[key.IntID()]; seen {
			return nil, status.Errorf(codes.InvalidArgument, "task_ids: %s is specified more than once", id)
		}
		seenIDs[key.IntID()] = struct{}{}
		results[idx] = &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, key)}
	}

	// Helpers for placing the result or an error into the output.
	out := &apipb.BatchGetResultResponse{
		Results: make([]*apipb.BatchGetResultResponse_ResultOrError, len(results)),
	}
	reportErr := func(idx int, code codes.Code, msg string) {
		if out.Results[idx] != nil {
			panic(fmt.Sprintf("result at slot %d is reported twice", idx))
		}
		out.Results[idx] = &apipb.BatchGetResultResponse_ResultOrError{
			TaskId: req.TaskIds[idx],
			Outcome: &apipb.BatchGetResultResponse_ResultOrError_Error{
				Error: &statuspb.Status{
					Code:    int32(code),
					Message: msg,
				},
			},
		}
	}
	reportRes := func(idx int, res *apipb.TaskResultResponse) {
		if out.Results[idx] != nil {
			panic(fmt.Sprintf("result at slot %d is reported twice", idx))
		}
		out.Results[idx] = &apipb.BatchGetResultResponse_ResultOrError{
			TaskId: req.TaskIds[idx],
			Outcome: &apipb.BatchGetResultResponse_ResultOrError_Result{
				Result: res,
			},
		}
	}

	// Fetch all results at once. This happens before checking ACLs because we
	// need to fetch entities to know what ACLs to check. Note that existence of
	// a task is not a secret (task IDs are predictable).
	err := datastore.Get(ctx, results)

	// Get a MultiError with individual fetch errors.
	var merr errors.MultiError
	if err != nil {
		// `err` is not a MultiError if the datastore e.g. timed out. Fail the
		// overall request in this case.
		if !errors.As(err, &merr) {
			logging.Errorf(ctx, "Error fetching many TaskResultSummary: %s", err)
			return nil, status.Errorf(codes.Internal, "datastore error fetching tasks")
		}
	}

	var stats *model.PerformanceStatsFetcher
	var fetching []*apipb.TaskResultResponse
	if req.IncludePerformanceStats {
		stats = model.NewPerformanceStatsFetcher(ctx)
		defer stats.Close()
	}

	// Filter out missing and "invisible" tasks. Start fetching perf stats.
	checker := State(ctx).ACL
	for taskIdx, taskRes := range results {
		var err error
		if len(merr) != 0 {
			err = merr[taskIdx]
		}

		if err != nil {
			if errors.Is(err, datastore.ErrNoSuchEntity) {
				reportErr(taskIdx, codes.NotFound, "no such task")
			} else {
				logging.Errorf(ctx,
					"Error fetching TaskResultSummary %s: %s",
					model.RequestKeyToTaskID(taskRes.Key, model.AsRequest), err,
				)
				reportErr(taskIdx, codes.Internal, "datastore error fetching the task")
			}
			continue
		}

		aclRes := checker.CheckTaskPerm(ctx, taskRes, acls.PermTasksGet)
		if !aclRes.Permitted {
			status := status.Convert(aclRes.ToGrpcErr())
			reportErr(taskIdx, status.Code(), status.Message())
		} else {
			taskRes := results[taskIdx].ToProto()
			if stats != nil {
				stats.Fetch(ctx, taskRes, results[taskIdx])
				fetching = append(fetching, taskRes)
			}
			reportRes(taskIdx, taskRes)
		}
	}

	// Finish fetching perf stats. This updates protos in `out` in-place.
	if stats != nil {
		if err := stats.Finish(fetching); err != nil {
			logging.Errorf(ctx, "Error fetching PerformanceStats: %s", err)
			return nil, status.Errorf(codes.Internal, "error fetching performance stats")
		}
	}

	return out, nil
}
