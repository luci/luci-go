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
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/buildbucket/appengine/model"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// TODO(crbug/1042991): Move to a common location.
var (
	projRegex    = regexp.MustCompile("^[a-z0-9\\-_]+$")
	bucketRegex  = regexp.MustCompile("^[a-z0-9_\\-.]{1,100}$")
	builderRegex = regexp.MustCompile("^[a-zA-Z0-9_\\-. ]{1,128}$")
)

// GetBuild handles a request to retrieve a build. Implements buildbucketpb.BuildsServer.
func (*Builds) GetBuild(ctx context.Context, req *buildbucketpb.GetBuildRequest) (*buildbucketpb.Build, error) {
	switch {
	case req.GetId() != 0:
		if req.Builder != nil || req.BuildNumber != 0 {
			return nil, status.Errorf(codes.InvalidArgument, "id is mutually exclusive with (builder and build_number)")
		}
	case req.GetBuilder() != nil && req.BuildNumber != 0:
		// TODO(crbug/1042991): Move buildbucketpb.BuilderID validation to a common location.
		switch parts := strings.Split(req.Builder.Bucket, "."); {
		case !projRegex.MatchString(req.Builder.Project):
			return nil, status.Errorf(codes.InvalidArgument, "builder.project must match %q", projRegex.String())
		case !bucketRegex.MatchString(req.Builder.Bucket):
			return nil, status.Errorf(codes.InvalidArgument, "builder.bucket must match %q", bucketRegex.String())
		case !builderRegex.MatchString(req.Builder.Builder):
			return nil, status.Errorf(codes.InvalidArgument, "builder.builder must match %q", builderRegex.String())
		case parts[0] == "luci" && len(parts) > 2:
			return nil, status.Errorf(codes.InvalidArgument, "invalid use of v1 builder.bucket in v2 API (hint: try %q)", parts[2])
		}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "one of id or (builder and build_number) is required")
	}
	// TODO(crbug/1042991): Check that the user can view this build.
	if req.Id > 0 {
		ent := &model.Build{
			ID: req.Id,
		}
		switch err := datastore.Get(ctx, ent); err {
		case nil:
		case datastore.ErrNoSuchEntity:
			return nil, status.Errorf(codes.NotFound, "not found")
		default:
			return nil, errors.Annotate(err, "error fetching build with ID %d", req.Id).Err()
		}
		return &buildbucketpb.Build{
			Builder: &buildbucketpb.BuilderID{
				Project: ent.Project,
				Bucket:  ent.BucketID,
				Builder: ent.BuilderID,
			},
			Id: ent.ID,
		}, nil
	}
	// TODO(crbug/1042991): Implement get by builder/build number.
	return &buildbucketpb.Build{}, nil
}
