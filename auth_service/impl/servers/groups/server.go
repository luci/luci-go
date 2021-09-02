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

// Package groups contains Groups server implementation.
package groups

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
)

// Server implements Groups server.
type Server struct {
	rpcpb.UnimplementedGroupsServer
}

// ListGroups implements the corresponding RPC method.
func (*Server) ListGroups(ctx context.Context, _ *emptypb.Empty) (*rpcpb.ListGroupsResponse, error) {
	// Get groups from datastore.
	groups, err := model.GetAllAuthGroups(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch groups: %s", err)
	}

	var groupList = make([]*rpcpb.AuthGroup, len(groups))
	for idx, entity := range groups {
		groupList[idx] = entity.ToProto()
	}

	return &rpcpb.ListGroupsResponse{
		Groups: groupList,
	}, nil
}
