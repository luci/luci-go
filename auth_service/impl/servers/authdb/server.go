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

// Package authdb contains methods to work with authdb.
package authdb

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
)

// Server implements AuthDB server.
type Server struct {
	rpcpb.UnimplementedAuthDBServer
}

// GetSnapshot implements the corresponding RPC method.
func (*Server) GetSnapshot(ctx context.Context, request *rpcpb.GetSnapshotRequest) (*rpcpb.Snapshot, error) {
	if request.Revision < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Negative revision numbers are not valid")
	} else if request.Revision == 0 {
		var err error
		if request.Revision, err = getLatestRevision(ctx, request); err != nil {
			logging.Errorf(ctx, err.Error())
			return nil, err
		}
	}

	switch snapshot, err := model.GetAuthDBSnapshot(ctx, request.Revision, request.SkipBody); {
	case err == nil:
		return snapshot.ToProto(), nil
	case err == datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "AuthDB revision %v not found", request.Revision)
	default:
		errStr := "unknown error while calling GetAuthDBSnapshot"
		logging.Errorf(ctx, errStr)
		return nil, status.Errorf(codes.Internal, errStr)
	}
}

// getLatestRevision is a helper function to set the latest revision number for the GetSnapshotRequest.
func getLatestRevision(ctx context.Context, request *rpcpb.GetSnapshotRequest) (int64, error) {
	switch latest, err := model.GetAuthDBSnapshotLatest(ctx); {
	case err == nil:
		return latest.AuthDBRev, nil
	case err == datastore.ErrNoSuchEntity:
		return 0, status.Errorf(codes.NotFound, "AuthDBSnapshotLatest not found")
	default:
		errStr := "unknown error while calling GetAuthDBSnapshotLatest"
		logging.Errorf(ctx, errStr)
		return 0, status.Errorf(codes.Internal, errStr)
	}
}
