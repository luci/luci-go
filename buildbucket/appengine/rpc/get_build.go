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
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// TODO(crbug/1042991): Move to a common location.
var (
	projRegex    = regexp.MustCompile(`^[a-z0-9\-_]+$`)
	bucketRegex  = regexp.MustCompile(`^[a-z0-9\-_.]{1,100}$`)
	builderRegex = regexp.MustCompile(`^[a-zA-Z0-9\-.\(\) ]{1,128}$`)
)

// defMask is the default field mask to use for GetBuild requests.
// Initialized by init.
var defMask mask.Mask

func init() {
	var err error
	defMask, err = mask.FromFieldMask(&field_mask.FieldMask{
		Paths: []string{
			"builder",
			"canary",
			"create_time",
			"created_by",
			"critical",
			"end_time",
			"id",
			"input.experimental",
			"input.gerrit_changes",
			"input.gitiles_commit",
			"number",
			"start_time",
			"status",
			"status_details",
			"update_time",
			// TODO(nodir): Add user_duration.
		},
	}, &pb.Build{}, false, false)
	if err != nil {
		panic(err)
	}
}

// TODO(crbug/1042991): Move to a common location.
func getFieldMask(fields *field_mask.FieldMask) (mask.Mask, error) {
	if len(fields.GetPaths()) == 0 {
		return defMask, nil
	}
	return mask.FromFieldMask(fields, &pb.Build{}, false, false)
}

// GetBuild handles a request to retrieve a build. Implements pb.BuildsServer.
func (*Builds) GetBuild(ctx context.Context, req *pb.GetBuildRequest) (*pb.Build, error) {
	switch {
	case req.GetId() != 0:
		if req.Builder != nil || req.BuildNumber != 0 {
			return nil, appstatus.Errorf(codes.InvalidArgument, "id is mutually exclusive with (builder and build_number)")
		}
	case req.GetBuilder() != nil && req.BuildNumber != 0:
		// TODO(crbug/1042991): Move pb.BuilderID validation to a common location.
		switch parts := strings.Split(req.Builder.Bucket, "."); {
		case !projRegex.MatchString(req.Builder.Project):
			return nil, appstatus.Errorf(codes.InvalidArgument, "builder.project must match %q", projRegex.String())
		case !bucketRegex.MatchString(req.Builder.Bucket):
			return nil, appstatus.Errorf(codes.InvalidArgument, "builder.bucket must match %q", bucketRegex.String())
		case !builderRegex.MatchString(req.Builder.Builder):
			return nil, appstatus.Errorf(codes.InvalidArgument, "builder.builder must match %q", builderRegex.String())
		case parts[0] == "luci" && len(parts) > 2:
			return nil, appstatus.Errorf(codes.InvalidArgument, "invalid use of v1 builder.bucket in v2 API (hint: try %q)", parts[2])
		}
	default:
		return nil, appstatus.Errorf(codes.InvalidArgument, "one of id or (builder and build_number) is required")
	}
	m, err := getFieldMask(req.Fields)
	if err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "invalid field mask")
	}
	if req.Id == 0 {
		addr := fmt.Sprintf("luci.%s.%s/%s/%d", req.Builder.Project, req.Builder.Bucket, req.Builder.Builder, req.BuildNumber)
		switch ents, err := model.SearchTagIndex(ctx, "build_address", addr); {
		case model.TagIndexIncomplete.In(err):
			// Shouldn't happen because build address is globally unique (exactly one entry in a complete index).
			return nil, errors.Reason("unexpected incomplete index for build address %q", addr).Err()
		case err != nil:
			return nil, err
		case len(ents) == 0:
			return nil, notFound(ctx)
		case len(ents) == 1:
			req.Id = ents[0].BuildID
		default:
			// Shouldn't happen because build address is globally unique and created before the build.
			return nil, errors.Reason("unexpected number of results for build address %q: %d", addr, len(ents)).Err()
		}
	}
	bld := &model.Build{
		ID: req.Id,
	}
	switch err := datastore.Get(ctx, bld); {
	case err == datastore.ErrNoSuchEntity:
		return nil, notFound(ctx)
	case err != nil:
		return nil, errors.Annotate(err, "error fetching build with ID %d", req.Id).Err()
	}
	bck := &model.Bucket{
		ID:     bld.Proto.Builder.Bucket,
		Parent: datastore.KeyForObj(ctx, &model.Project{ID: bld.Proto.Builder.Project}),
	}
	switch err := datastore.Get(ctx, bck); {
	case err == datastore.ErrNoSuchEntity:
		return nil, notFound(ctx)
	case err != nil:
		return nil, errors.Annotate(err, "error fetching bucket %q", bld.BucketID).Err()
	}
	switch can, err := bck.CanView(ctx); {
	case err != nil:
		return nil, err
	case !can:
		return nil, notFound(ctx)
	}
	return bld.ToProto(ctx, m)
}
