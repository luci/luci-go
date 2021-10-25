// Copyright 2021 The LUCI Authors.
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

package backend

import (
	"context"
	"strconv"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var queryRecentBuildsPageSize = PageSizeLimiter{
	Max:     100,
	Default: 25,
}

// QueryRecentBuilds implements milopb.MiloInternal service
func (s *MiloInternalService) QueryRecentBuilds(ctx context.Context, req *milopb.QueryRecentBuildsRequest) (_ *milopb.QueryRecentBuildsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()

	err = validatesQueryRecentBuildsRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	allowed, err := common.IsAllowed(ctx, req.GetBuilder().GetProject())
	if err != nil {
		return nil, err
	}
	if !allowed {
		if auth.CurrentIdentity(ctx) == identity.AnonymousIdentity {
			return nil, appstatus.Error(codes.Unauthenticated, "not logged in")
		}
		return nil, appstatus.Error(codes.PermissionDenied, "no access to the project")
	}

	cur, err := decodeCursor(ctx, req.PageToken)
	if err != nil {
		return nil, appstatus.Error(codes.InvalidArgument, "invalid page token")
	}

	pageSize := int(queryRecentBuildsPageSize.Adjust(req.PageSize))

	legacyBuilderID := common.LegacyBuilderIDString(req.Builder)
	q := datastore.NewQuery("BuildSummary").
		Eq("BuilderID", legacyBuilderID).
		Order("-Created").
		Start(cur)

	recentBuilds := make([]*buildbucketpb.Build, 0, pageSize)
	nextPageToken := ""
	err = datastore.Run(ctx, q, func(b *model.BuildSummary, getCursor datastore.CursorCB) error {
		if !b.Summary.Status.Terminal() {
			return nil
		}

		var buildID int64 = 0
		_, buildNum, err := common.ParseLegacyBuildID(b.BuildID)
		if err != nil {
			// If the BuildID is not the legacy build ID, trying parsing it as
			// the new build ID.
			buildID, err = strconv.ParseInt(b.BuildID, 10, 64)
			if err != nil {
				return err
			}
		}

		recentBuilds = append(recentBuilds, &buildbucketpb.Build{
			Id:         buildID,
			Number:     buildNum,
			Builder:    req.Builder,
			Status:     b.Summary.Status.ToBuildbucket(),
			CreateTime: timestamppb.New(b.Created),
		})

		if len(recentBuilds) == pageSize {
			cursor, err := getCursor()
			if err != nil {
				return err
			}
			nextPageToken = cursor.String()

			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &milopb.QueryRecentBuildsResponse{
		Builds:        recentBuilds,
		NextPageToken: nextPageToken,
	}, nil
}

func validatesQueryRecentBuildsRequest(req *milopb.QueryRecentBuildsRequest) error {
	switch {
	case req.PageSize < 0:
		return errors.Reason("page_size can not be negative").Err()
	case req.Builder == nil || req.Builder.Project == "" || req.Builder.Bucket == "" || req.Builder.Builder == "":
		return errors.Reason("builder_id is required").Err()
	default:
		return nil
	}
}
