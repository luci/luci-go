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

	"google.golang.org/grpc/status"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

// ListTaskStates implements the corresponding RPC method.
func (srv *TasksServer) ListTaskStates(ctx context.Context, req *apipb.TaskStatesRequest) (*apipb.TaskStates, error) {
	resp, err := srv.BatchGetResult(ctx, &apipb.BatchGetResultRequest{TaskIds: req.TaskId})
	if err != nil {
		return nil, err
	}
	out := &apipb.TaskStates{States: make([]apipb.TaskState, len(resp.Results))}
	for idx, res := range resp.Results {
		switch v := res.Outcome.(type) {
		case *apipb.BatchGetResultResponse_ResultOrError_Error:
			v.Error.Message = fmt.Sprintf("%s: %s", res.TaskId, v.Error.Message)
			return nil, status.ErrorProto(v.Error)
		case *apipb.BatchGetResultResponse_ResultOrError_Result:
			out.States[idx] = v.Result.State
		default:
			panic("impossible")
		}
	}
	return out, nil
}
