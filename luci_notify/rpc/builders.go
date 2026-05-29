// Copyright 2026 The LUCI Authors.
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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/buildbucket/bbperms"
	"go.chromium.org/luci/common/data/aip160"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/internal/builders"
)

func init() {
	bbperms.BuildersList.AddFlags(realms.UsedInQueryRealms)
}

type buildersServer struct{}

var _ pb.BuildersServer = &buildersServer{}

// NewBuildersServer creates a new server to handle Builders requests.
func NewBuildersServer() *pb.DecoratedBuilders {
	return &pb.DecoratedBuilders{
		Prelude:  checkAllowedPrelude,
		Service:  &buildersServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// List returns a list of builders and their latest status.
func (*buildersServer) List(ctx context.Context, request *pb.ListBuildersRequest) (*pb.ListBuildersResponse, error) {
	// ACL Check
	allowedRealms, err := auth.QueryRealms(ctx, bbperms.BuildersList, "", nil)
	if err != nil {
		return nil, errors.Fmt("querying realms: %w", err)
	}

	if len(allowedRealms) == 0 {
		return &pb.ListBuildersResponse{}, nil
	}

	var fullRealms []string
	var wildcardProjects []string
	for _, r := range allowedRealms {
		project, realm := realms.Split(r)
		if realm == realms.RootRealm || realm == realms.ProjectRealm {
			wildcardProjects = append(wildcardProjects, project)
		} else {
			fullRealms = append(fullRealms, r)
		}
	}

	filter, err := aip160.ParseFilter(request.Filter)
	if err != nil {
		return nil, invalidArgumentError(errors.Fmt("filter: %w", err))
	}

	opts := builders.ListOptions{
		PageToken:        request.PageToken,
		PageSize:         int(request.PageSize),
		FullRealms:       fullRealms,
		WildcardProjects: wildcardProjects,
		Filter:           filter,
	}

	builderEntities, nextPageToken, err := builders.List(span.Single(ctx), opts)
	if err != nil {
		return nil, errors.Fmt("listing builders: %w", err)
	}

	buildersProto := make([]*pb.BuilderStatus, len(builderEntities))
	for i, b := range builderEntities {
		buildersProto[i] = &pb.BuilderStatus{
			Name:            b.BuilderKey,
			Project:         b.Project,
			Bucket:          b.Bucket,
			Builder:         b.Builder,
			Status:          b.Status,
			UpdateTime:      timestamppb.New(b.UpdateTime),
			BuildId:         b.BuildId,
			OnCallRotations: b.OnCallRotations,
		}
	}

	return &pb.ListBuildersResponse{
		Builders:      buildersProto,
		NextPageToken: nextPageToken,
	}, nil
}
