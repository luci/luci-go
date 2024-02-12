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
	"encoding/json"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
)

// Server implements AuthDB server.
type Server struct {
	rpcpb.UnimplementedAuthDBServer
}

type SnapshotJSON struct {
	AuthDBRev      int64  `json:"auth_db_rev"`
	AuthDBDeflated []byte `json:"deflated_body,omitempty"`
	AuthDBSha256   string `json:"sha256"`
	CreatedTS      int64  `json:"created_ts"`
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

	switch snapshot, err := model.GetAuthDBSnapshot(ctx, request.Revision, request.SkipBody, false); {
	case err == nil:
		return snapshot.ToProto(), nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "AuthDB revision %v not found", request.Revision)
	default:
		errStr := "unknown error while calling GetAuthDBSnapshot"
		logging.Errorf(ctx, errStr)
		return nil, status.Errorf(codes.Internal, errStr)
	}
}

// getLatestRevision is a helper function to set the latest revision number for the GetSnapshotRequest.
func getLatestRevision(ctx context.Context, request *rpcpb.GetSnapshotRequest) (int64, error) {
	switch latest, err := model.GetAuthDBSnapshotLatest(ctx, false); {
	case err == nil:
		return latest.AuthDBRev, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return 0, status.Errorf(codes.NotFound, "AuthDBSnapshotLatest not found")
	default:
		errStr := "unknown error while calling GetAuthDBSnapshotLatest"
		logging.Errorf(ctx, errStr)
		return 0, status.Errorf(codes.Internal, errStr)
	}
}

// HandleLegacyAuthDBServing handles the AuthDBSnapshot serving for legacy
// services. Writes the AuthDBSnapshot JSON to the router.Writer. gRPC Error is returned
// and adapted to HTTP format if operation is unsuccesful.
func (s *Server) HandleLegacyAuthDBServing(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer
	var snap *rpcpb.Snapshot
	var err error

	skipBody := r.URL.Query().Get("skip_body") == "1"

	if revIDStr := ctx.Params.ByName("revID"); revIDStr != "latest" {
		revID, err := strconv.ParseInt(revIDStr, 10, 64)
		switch {
		case err != nil:
			errors.Log(c, errors.Annotate(err, "issue while parsing revID %s", revIDStr).Err())
			return status.Errorf(codes.InvalidArgument, "unable to parse revID: %s", revIDStr)
		case revID < 0:
			return status.Errorf(codes.InvalidArgument, "Negative revision numbers are not valid")
		default:
			snap, err = s.GetSnapshot(c, &rpcpb.GetSnapshotRequest{
				Revision: revID,
				SkipBody: skipBody,
			})
			if err != nil {
				return errors.Annotate(err, "Error while getting snapshot %d", revID).Err()
			}
		}
	} else {
		snap, err = s.GetSnapshot(c, &rpcpb.GetSnapshotRequest{
			Revision: 0,
			SkipBody: skipBody,
		})
		if err != nil {
			return errors.Annotate(err, "Error while getting latest snapshot").Err()
		}
	}

	unixMicro := snap.CreatedTs.AsTime().UnixNano() / 1000

	blob, err := json.Marshal(map[string]any{
		"snapshot": SnapshotJSON{
			AuthDBRev:      snap.AuthDbRev,
			AuthDBDeflated: snap.AuthDbDeflated,
			AuthDBSha256:   snap.AuthDbSha256,
			CreatedTS:      unixMicro,
		},
	})

	if err != nil {
		return errors.Annotate(err, "Error while marshaling JSON").Err()
	}

	if _, err = w.Write(blob); err != nil {
		return errors.Annotate(err, "Error while writing json").Err()
	}

	return nil
}

// HandleV2AuthDBServing handles the V2AuthDBSnapshot serving for
// validation. Writes the V2AuthDBSnapshot JSON to the router.Writer.
// gRPC Error is returned and adapted to HTTP format if operation is
// unsuccessful.
//
// TODO: Remove this once we have fully rolled out Auth Service v2
// (b/321019030).
func (s *Server) HandleV2AuthDBServing(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	var revID int64
	if revIDStr := ctx.Params.ByName("revID"); revIDStr != "latest" {
		revID, err := strconv.ParseInt(revIDStr, 10, 64)
		if err != nil {
			errors.Log(c, errors.Annotate(err, "issue while parsing revID %s", revIDStr).Err())
			return status.Errorf(codes.InvalidArgument, "unable to parse revID: %s", revIDStr)
		}
		if revID < 0 {
			return status.Errorf(codes.InvalidArgument, "Negative revision numbers are not valid")
		}
	} else {
		switch latest, err := model.GetAuthDBSnapshotLatest(c, true); {
		case err == nil:
			revID = latest.AuthDBRev
		case errors.Is(err, datastore.ErrNoSuchEntity):
			return status.Errorf(codes.NotFound, "V2AuthDBSnapshotLatest not found")
		default:
			errStr := "unknown error while getting V2AuthDBSnapshotLatest"
			logging.Errorf(c, errStr)
			return status.Errorf(codes.Internal, errStr)
		}
	}

	skipBody := r.URL.Query().Get("skip_body") == "1"

	snapshot, err := model.GetAuthDBSnapshot(c, revID, skipBody, true)
	if err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return status.Errorf(codes.NotFound, "V2 AuthDB revision %v not found", revID)
		}
		// Unexpected error.
		errStr := "unknown error while getting V2AuthDBSnapshot"
		logging.Errorf(c, errStr)
		return status.Errorf(codes.Internal, errStr)
	}

	snap := snapshot.ToProto()
	unixMicro := snap.CreatedTs.AsTime().UnixNano() / 1000
	blob, err := json.Marshal(map[string]any{
		"snapshot": SnapshotJSON{
			AuthDBRev:      snap.AuthDbRev,
			AuthDBDeflated: snap.AuthDbDeflated,
			AuthDBSha256:   snap.AuthDbSha256,
			CreatedTS:      unixMicro,
		},
	})
	if err != nil {
		return errors.Annotate(err, "Error while marshalling JSON").Err()
	}

	if _, err = w.Write(blob); err != nil {
		return errors.Annotate(err, "Error while writing json").Err()
	}

	return nil
}
