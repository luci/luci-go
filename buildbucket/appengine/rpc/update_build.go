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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var (
	// updateableFieldPaths is a set of UpdateBuildRequest field paths updatable
	// via UpdateBuild RPC.
	updatableFieldPaths = stringset.NewFromSlice(
		"build.output",
		"build.output.properties",
		"build.output.gitiles_commit",
		"build.status",
		"build.status_details",
		"build.steps",
		"build.summary_markdown",
		"build.tags",
	)
)

// validateUpdate validates the given request.
func validateUpdate(req *pb.UpdateBuildRequest) error {
	// validate the mask
	unsupported := stringset.NewFromSlice(
		req.GetUpdateMask().GetPaths()...).Difference(updatableFieldPaths)
	if len(unsupported) > 0 {
		return errors.Reason("unsupported path(s) %q", unsupported.ToSortedSlice()).Err()
	}

	// validate the build
	switch {
	// TODO(1110990): validate the rest of the message fields
	case req.GetBuild().GetId() == 0:
		return errors.Reason("id is required").Err()
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
