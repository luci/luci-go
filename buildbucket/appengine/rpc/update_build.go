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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// updateBuildStatuses is a set of build statuses supported by UpdateBuild RPC.
var updateBuildStatuses = map[pb.Status]struct{}{
	pb.Status_STARTED: {},
	// kitchen does not actually use SUCCESS. It relies on swarming pubsub
	// handler in Buildbucket because a task may fail after recipe succeeded.
	pb.Status_SUCCESS:       {},
	pb.Status_FAILURE:       {},
	pb.Status_INFRA_FAILURE: {},
}

// validateUpdate validates the given request.
func validateUpdate(req *pb.UpdateBuildRequest) error {
	if req.GetBuild().GetId() == 0 {
		return errors.Reason("build.id: required").Err()
	}

	for _, p := range req.UpdateMask.GetPaths() {
		// TODO(1110990): validate gitiles_commit, summary_markdown, and steps
		switch p {
		case "build.output":
		case "build.output.properties":
		case "build.output.gitiles_commit":
			if err := validateCommitWithRef(req.Build.Output.GetGitilesCommit()); err != nil {
				return errors.Annotate(err, "build.output.gitiles_commit").Err()
			}
		case "build.status":
			if _, ok := updateBuildStatuses[req.Build.Status]; !ok {
				return errors.Reason("build.status: invalid status %s for UpdateBuild", req.Build.Status).Err()
			}
		case "build.status_details":
		case "build.steps":
		case "build.summary_markdown":
			if err := validateSummaryMarkdown(req.Build.SummaryMarkdown); err != nil {
				return errors.Annotate(err, "build.summary_markdown").Err()
			}
		case "build.tags":
			if err := validateTags(req.Build.Tags, TagAppend); err != nil {
				return errors.Annotate(err, "build.tags").Err()
			}
		default:
			return errors.Reason("unsupported path %q", p).Err()
		}
	}
	return nil
}

// UpdateBuild handles a request to update a build. Implements pb.UpdateBuild.
func (*Builds) UpdateBuild(ctx context.Context, req *pb.UpdateBuildRequest) (*pb.Build, error) {
	if err := validateUpdate(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	return nil, appstatus.Errorf(codes.Unimplemented, "method not implemented")
}
