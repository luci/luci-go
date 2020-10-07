// Copyright 2020 The LUCI Authors.
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

package rpc

import (
	"context"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// validateSchedule validates the given request.
func validateSchedule(req *pb.ScheduleBuildRequest) error {
	var err error
	switch {
	case strings.Contains(req.GetRequestId(), "/"):
		return errors.Reason("request_id cannot contain '/'").Err()
	case req.GetPriority() < 0 || req.GetPriority() > 255:
		return errors.Reason("priority must be in [0, 255]").Err()
	case req.GetBuilder() == nil && req.GetTemplateBuildId() == 0:
		return errors.Reason("builder or template_build_id is required").Err()
	case req.GetBuilder() != nil && teeErr(protoutil.ValidateRequiredBuilderID(req.Builder), &err) != nil:
		return err
	}

	// TODO(crbug/1042991): Validate Exe, Properties, GitilesCommit, Tags, Dimensions, Notify.
	return nil
}

// ScheduleBuild handles a request to schedule a build. Implements pb.BuildsServer.
func (*Builds) ScheduleBuild(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.Build, error) {
	if err := validateSchedule(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	return nil, nil
}
