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

package tasks

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func HandleCancelBackendTask(ctx context.Context, project, target, taskID string) error {
	// Send the cancelation request
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return errors.Annotate(err, "could not get global settings config").Err()
	}
	backendClient, err := clients.NewBackendClient(ctx, project, target, globalCfg)
	if err != nil {
		return tq.Fatal.Apply(errors.Annotate(err, "failed to connect to backend service").Err())
	}
	res, err := backendClient.CancelTasks(ctx, &pb.CancelTasksRequest{
		TaskIds: []*pb.TaskID{
			&pb.TaskID{
				Id:     taskID,
				Target: target,
			},
		},
	})

	errMsg := fmt.Sprintf("error in canceling task %s for target %s", taskID, target)
	if err != nil {
		return errors.Annotate(err, "transient %s", errMsg).Tag(transient.Tag).Err()
	}

	// TODO(b/355013317): require res.Responses after all existing TaskBackend
	// implementations have migrated to use it.
	if len(res.GetResponses()) == 1 && res.Responses[0].GetError() != nil {
		s := res.Responses[0].GetError()
		switch codes.Code(s.Code) {
		case codes.Internal, codes.Unknown, codes.Unavailable, codes.DeadlineExceeded:
			return errors.Reason("transient %s: %s", errMsg, s.Message).Tag(transient.Tag).Err()
		default:
			return errors.Reason("fatal %s: %s", errMsg, s.Message).Tag(tq.Fatal).Err()
		}
	}
	return nil
}
