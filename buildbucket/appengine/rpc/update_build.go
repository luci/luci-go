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
	"sort"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var (
	// updatableFieldPaths is a map of updatable field paths to the validation func.
	// TODO(ddoman): implement missing validations.
	updatableFieldPaths = map[string](func(req *pb.UpdateBuildRequest) error){
		"build.output":                noop, // no validation required
		"build.output.properties":     noop, // no validation required
		"build.output.gitiles_commit": noop,
		"build.status":                validateUpdateBuildStatus,
		"build.status_details":        noop, // no validation required
		"build.steps":                 noop,
		"build.summary_markdown":      noop,
		"build.tags":                  validateUpdateBuildTags,
	}

	// a set of build statuses supported by UpdateBuild RPC
	updateBuildStatuses = map[pb.Status]struct{}{
		pb.Status_STARTED: {},
		// kitchen does not actually use SUCCESS. It relies on swarming pubsub
		// handler in Buildbucket because a task may fail after recipe succeeded.
		pb.Status_SUCCESS:       {},
		pb.Status_FAILURE:       {},
		pb.Status_INFRA_FAILURE: {},
	}
)

func noop(req *pb.UpdateBuildRequest) error {
	return nil
}

func validateUpdateBuildStatus(req *pb.UpdateBuildRequest) error {
	if _, ok := updateBuildStatuses[req.Build.Status]; !ok {
		return errors.Reason("invalid status %s for UpdateBuild", req.Build.Status.String()).Err()
	}
	return nil
}

func validateUpdateBuildTags(req *pb.UpdateBuildRequest) error {
	return validateTags(req.Build.Tags, TagAppend)
}

// validateUpdate validates the given request.
func validateUpdate(req *pb.UpdateBuildRequest) error {
	if req.GetBuild().GetId() == 0 {
		return errors.Reason("id is required").Err()
	}
	reqPaths := req.GetUpdateMask().GetPaths()

	// check for unsupported field paths before executing any of the validation funcs.
	var unsupported []string
	for _, p := range reqPaths {
		if _, ok := updatableFieldPaths[p]; !ok {
			unsupported = append(unsupported, p)
		}
	}
	if len(unsupported) > 0 {
		sort.Strings(unsupported)
		return errors.Reason("unsupported path(s) %q", unsupported).Err()
	}

	// run per-path validation funcs.
	for _, p := range reqPaths {
		if err := updatableFieldPaths[p](req); err != nil {
			return err
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
