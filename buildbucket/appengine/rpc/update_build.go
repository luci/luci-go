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
	// valPaths is a list of UpdateBuildRequest field paths updatable via UpdateBuild RPC.
	valPaths = []string{
		"build.output",
		"build.output.properties",
		"build.output.gitiles_commit",
		"build.status",
		"build.status_details",
		"build.steps",
		"build.summary_markdown",
		"build.tags",
	}
)

func init() {
	sort.Strings(valPaths)
}

// validateUpdate validates the given request.
func validateUpdate(req *pb.UpdateBuildRequest) error {
	// validate the mask
	var unsupported []string
	var reqPaths []string
	if req.GetUpdateMask() != nil {
		reqPaths = req.UpdateMask.Paths
		sort.Strings(req.UpdateMask.Paths)
	}
	for i, j := 0, 0; i < len(reqPaths); i++ {
		for ; j < len(valPaths) && reqPaths[i] > valPaths[j]; j++ {
		}
		if j == len(valPaths) || reqPaths[i] != valPaths[j] {
			unsupported = append(unsupported, reqPaths[i])
		}
	}

	if len(unsupported) > 0 {
		return errors.Reason("unsupported path(s) %q", unsupported).Err()
	}

	// validate the build
	switch {
	case req.GetBuild().GetId() == 0:
		return errors.Reason("id is required").Err()
		// TODO(1110990): validate the rest of the message fields
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
