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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
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
func (srv *Server) GetSnapshot(ctx context.Context, request *rpcpb.GetSnapshotRequest) (*rpcpb.Snapshot, error) {
	if request.Revision < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Negative revision numbers are not valid")
	} else if request.Revision == 0 {
		var err error
		if request.Revision, err = getLatestRevision(ctx); err != nil {
			logging.Errorf(ctx, err.Error())
			return nil, err
		}
	}

	switch snapshot, err := model.GetAuthDBSnapshot(ctx, request.Revision, request.SkipBody); {
	case err == nil:
		return snapshot.ToProto(), nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "AuthDB revision %v not found", request.Revision)
	default:
		errStr := "unknown error while calling GetAuthDBSnapshot"
		logging.Errorf(ctx, errStr)
		return nil, status.Error(codes.Internal, errStr)
	}
}

// getLatestRevision is a helper function to get the latest revision number of
// the AuthDB.
func getLatestRevision(ctx context.Context) (int64, error) {
	switch latest, err := model.GetAuthDBSnapshotLatest(ctx); {
	case err == nil:
		return latest.AuthDBRev, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return 0, status.Error(codes.NotFound, "AuthDBSnapshotLatest not found")
	default:
		err = errors.Annotate(err,
			"unknown error while calling GetAuthDBSnapshotLatest").Err()
		return 0, err
	}
}

// HandleLegacyAuthDBServing handles the AuthDBSnapshot serving for legacy
// services. Writes the AuthDBSnapshot JSON to the router.Writer.
func (srv *Server) HandleLegacyAuthDBServing(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	// Log the caller of this legacy function.
	logging.Debugf(c, "HandleLegacyAuthDBServing call by %s", auth.CurrentIdentity(c))

	// Parse the `revID`  param for the requested AuthDB revision.
	var revID int64
	var err error
	revIDStr := ctx.Params.ByName("revID")
	switch revIDStr {
	case "", "latest":
		revID, err = getLatestRevision(c)
		if err != nil {
			return err
		}
	default:
		revID, err = strconv.ParseInt(revIDStr, 10, 64)
		if err != nil {
			return status.Errorf(codes.InvalidArgument,
				"failed to parse revID %q", revIDStr)
		}

		if revID < 0 {
			return status.Errorf(codes.InvalidArgument,
				"negative revision numbers are not valid")
		}
	}

	skipBody := false
	requestURL := r.URL
	if requestURL != nil {
		skipBody = requestURL.Query().Get("skip_body") == "1"
	}

	snapshot, err := model.GetAuthDBSnapshot(c, revID, skipBody)
	if err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return status.Errorf(codes.NotFound,
				"AuthDB revision %d not found", revID)
		}

		return errors.Annotate(err, "error calling GetAuthDBSnapshot").Err()
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err = json.NewEncoder(w).Encode(map[string]SnapshotJSON{
		"snapshot": {
			AuthDBRev:      snapshot.ID,
			AuthDBDeflated: snapshot.AuthDBDeflated,
			AuthDBSha256:   snapshot.AuthDBSha256,
			CreatedTS:      snapshot.CreatedTS.UnixMicro(),
		},
	})
	if err != nil {
		err = errors.Annotate(err, "error encoding JSON").Err()
		return err
	}

	return nil
}

// CheckLegacyMembership serves the legacy REST API GET request to check whether
// a given identity is a member of any of the given groups.
//
// Example query:
//
//	"identity=user:someone@example.com&groups=group-a&groups=group-b"
//
// Example response:
//
//	{
//	   "is_member": true
//	}
func (srv *Server) CheckLegacyMembership(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	// Log the caller of this legacy function.
	logging.Debugf(c, "CheckLegacyMembership call by %s", auth.CurrentIdentity(c))

	params := r.URL.Query()

	// Validate the identity param.
	rawID := params.Get("identity")
	if rawID == "" {
		return status.Error(codes.InvalidArgument,
			"\"identity\" query parameter is required")
	}
	id, err := identity.MakeIdentity(rawID)
	if err != nil {
		return status.Errorf(codes.InvalidArgument,
			"Invalid \"identity\" - %s", err)
	}

	// Get all groups params (there may be multiple).
	rawNames, ok := params["groups"]
	if !ok {
		return status.Error(codes.InvalidArgument,
			"\"groups\" query parameter is required")
	}
	toCheck := stringset.New(len(rawNames))
	for _, name := range rawNames {
		if name != "" {
			toCheck.Add(name)
		}
	}
	if len(toCheck) == 0 {
		return status.Error(codes.InvalidArgument,
			"must specify at least one group to check membership")
	}

	s := auth.GetState(c)
	if s == nil {
		return status.Error(codes.Internal, "no auth state")
	}
	isMember, err := s.DB().IsMember(c, id, toCheck.ToSlice())
	if err != nil {
		return status.Errorf(codes.Internal, "error checking membership for %q",
			id)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err = json.NewEncoder(w).Encode(map[string]bool{
		"is_member": isMember,
	})
	if err != nil {
		return errors.Annotate(err, "error encoding JSON").Err()
	}

	return nil
}
