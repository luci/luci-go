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

package rpc

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func validateGetBuildStatusRequest(req *pb.GetBuildStatusRequest) error {
	switch {
	case req.GetId() != 0:
		if req.Builder != nil || req.BuildNumber != 0 {
			return errors.New("id is mutually exclusive with (builder + build_number)")
		}
	case req.GetBuilder() != nil && req.BuildNumber != 0:
		if err := protoutil.ValidateRequiredBuilderID(req.Builder); err != nil {
			return errors.Fmt("builder: %w", err)
		}
	default:
		return errors.New("either id or (builder + build_number) is required")
	}
	return nil
}

// GetBuildStatus handles a request to retrieve a build's status. Implements pb.BuildsServer.
func (*Builds) GetBuildStatus(ctx context.Context, req *pb.GetBuildStatusRequest) (*pb.Build, error) {
	if err := validateGetBuildStatusRequest(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	var bs *model.BuildStatus
	var err error
	bldr := req.Builder
	if req.GetId() != 0 {
		bs = &model.BuildStatus{Build: datastore.MakeKey(ctx, model.BuildKind, req.Id)}
		err = datastore.Get(ctx, bs)
	} else {
		// Get BuildStatus by builder + build_number.
		bldAddr := fmt.Sprintf("%s/%s/%s/%d", bldr.Project, bldr.Bucket, bldr.Builder, req.BuildNumber)
		q := datastore.NewQuery(model.BuildStatusKind).Eq("build_address", bldAddr)
		err = datastore.Run(ctx, q, func(bse *model.BuildStatus) error {
			bs = bse
			return nil
		})
	}
	if err != nil && err != datastore.ErrNoSuchEntity {
		return nil, err
	}

	var buildStatus pb.Status
	if bs != nil && bs.Status != pb.Status_STATUS_UNSPECIFIED {
		buildStatus = bs.Status
	} else {
		// BuildStatus not found, fall back to Build.
		bID := req.Id
		if bID == 0 {
			bID, err = getBuildIDByBuildNumber(ctx, req.Builder, req.BuildNumber)
			if err != nil {
				return nil, err
			}
		}
		bld, err := common.GetBuild(ctx, bID)
		if err != nil {
			return nil, err
		}
		buildStatus = bld.Proto.Status
		bldr = bld.Proto.Builder
	}

	// Check user permission on the builder.
	if bldr == nil {
		parts := strings.Split(bs.BuildAddress, "/")
		if len(parts) != 4 {
			return nil, errors.Fmt("failed to parse build_address of build %d", req.Id)
		}
		bldr = &pb.BuilderID{
			Project: parts[0],
			Bucket:  parts[1],
			Builder: parts[2],
		}
	}
	// User needs BuildsGet or BuildsGetLimited permission to call this endpoint.
	_, err = perm.GetFirstAvailablePerm(ctx, bldr, bbperms.BuildsGet, bbperms.BuildsGetLimited)
	if err != nil {
		return nil, err
	}

	return &pb.Build{
		Id:      req.Id,
		Builder: req.Builder,
		Number:  req.BuildNumber,
		Status:  buildStatus,
	}, nil
}
