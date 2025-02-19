// Copyright 2025 The LUCI Authors.
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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

// RunTask implements bbpb.TaskBackendServer.
func (srv *TaskBackend) RunTask(ctx context.Context, req *bbpb.RunTaskRequest) (*bbpb.RunTaskResponse, error) {
	if err := srv.CheckBuildbucket(ctx); err != nil {
		return nil, err
	}

	cfg, err := ingestBackendConfigWithDefaults(req.GetBackendConfig())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad backend config in request: %s", err)
	}

	if err := srv.validateRunTaskRequest(ctx, req, cfg); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad request: %s", err)
	}

	return nil, status.Error(codes.Unimplemented, "not implemented yet")
}

func (srv *TaskBackend) validateRunTaskRequest(ctx context.Context, req *bbpb.RunTaskRequest, cfg *apipb.SwarmingTaskBackendConfig) error {
	var err error
	switch {
	case req.BuildId == "":
		return errors.New("build_id is required")
	case req.PubsubTopic == "":
		return errors.New("pubsub_topic is required")
	case teeErr(srv.validateTarget(req.Target), &err) != nil:
		return errors.Annotate(err, "target").Err()
	case cfg.GetAgentBinaryCipdFilename() == "":
		return errors.New("agent_binary_cipd_filename: required")
	default:
		return nil
	}
}
