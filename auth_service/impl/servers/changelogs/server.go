// Copyright 2022 The LUCI Authors.
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

// Package changelogs contains ChangeLogs server implementation.
package changelogs

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/pagination"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
)

// Server implements ChangeLogs server.
type Server struct {
	rpcpb.UnimplementedChangeLogsServer
}

// ListChangeLogs implements the corresponding RPC method.
func (*Server) ListChangeLogs(ctx context.Context, req *rpcpb.ListChangeLogsRequest) (*rpcpb.ListChangeLogsResponse, error) {
	changes, nextPageToken, err := model.GetAllAuthDBChange(ctx, req.GetTarget(), req.GetAuthDbRev(), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidTarget):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, pagination.ErrInvalidPageToken):
			return nil, status.Errorf(codes.InvalidArgument, "Invalid page token")
		default:
			return nil, status.Errorf(codes.Internal, "Failed to fetch change logs: %s", err)
		}
	}

	changeList := make([]*rpcpb.AuthDBChange, len(changes))
	for idx, entity := range changes {
		changeList[idx] = entity.ToProto()
	}

	return &rpcpb.ListChangeLogsResponse{
		Changes:       changeList,
		NextPageToken: nextPageToken,
	}, nil
}
