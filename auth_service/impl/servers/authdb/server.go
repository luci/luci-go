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
	"go.chromium.org/luci/auth_service/impl/model/graph"
	"go.chromium.org/luci/auth_service/impl/util/indexset"
)

// PermissionsProvider is the interface to get all permissions entities.
type PermissionsProvider interface {
	Get(ctx context.Context) (*PermissionsSnapshot, error)
	RefreshPeriodically(ctx context.Context)
}

// Server implements AuthDB server.
type Server struct {
	rpcpb.UnimplementedAuthDBServer
	permissionsProvider PermissionsProvider
}

func NewServer() *Server {
	return &Server{
		permissionsProvider: &CachingPermissionsProvider{},
	}
}

type SnapshotJSON struct {
	AuthDBRev      int64  `json:"auth_db_rev"`
	AuthDBDeflated []byte `json:"deflated_body,omitempty"`
	AuthDBSha256   string `json:"sha256"`
	CreatedTS      int64  `json:"created_ts"`
}

// WarmUp does the setup for the permissions server; it should be called before the
// main serving loop.
func (srv *Server) WarmUp(ctx context.Context) {
	if srv.permissionsProvider != nil {
		_, err := srv.permissionsProvider.Get(ctx)
		if err != nil {
			logging.Errorf(ctx, "error warming up Permissions provider")
		}
	}
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

// RefreshPeriodically wraps the groups provider's refresh method.
func (srv *Server) RefreshPeriodically(ctx context.Context) {
	if srv.permissionsProvider != nil {
		srv.permissionsProvider.RefreshPeriodically(ctx)
	}
}

// GetPrincipalPermissions implements the corresponding RPC method.
func (srv *Server) GetPrincipalPermissions(ctx context.Context, request *rpcpb.GetPrincipalPermissionsRequest) (*rpcpb.PrincipalPermissions, error) {
	if request.Principal == nil || request.Principal.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "principal is required")
	}

	principalNode, err := graph.ConvertPrincipal(request.Principal)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	snap, err := srv.permissionsProvider.Get(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch permissions snapshot: %s", err)
	}

	// Get the subgraph of groups that include this principal.
	subgraph, err := snap.groupsGraph.GetRelevantSubgraph(principalNode)
	if err != nil {
		if errors.Is(err, graph.ErrNoSuchGroup) {
			return nil, status.Errorf(codes.NotFound,
				"The requested group \"%s\" was not found.", principalNode.Value)
		}

		return nil, status.Errorf(codes.Internal,
			"Error getting relevant groups: %s", err)
	}

	// The first node is always the principal that was used as the root of the
	// subgraph.
	principal := subgraph.Nodes[0].ToPermissionKey()
	// Collate the permissions for each node that includes this principal.
	allRealms := stringset.New(0)
	permissionsByRealm := make(map[string]indexset.Set)
	for _, node := range subgraph.Nodes {
		permKey := node.ToPermissionKey()
		if permKey == "" {
			continue
		}
		nodePerms, ok := snap.permissionsMap[permKey]
		if !ok {
			continue
		}
		for _, realmPerms := range nodePerms {
			realm := realmPerms.Name
			allRealms.Add(realm)
			if _, ok := permissionsByRealm[realm]; !ok {
				permissionsByRealm[realm] = indexset.New(0)
			}
			permissionsByRealm[realm].AddAll(realmPerms.Permissions)
		}
	}

	realmPermissions := make([]*rpcpb.RealmPermissions, allRealms.Len())
	for i, realm := range allRealms.ToSortedSlice() {
		// Map the permission indices to their names. We can sort the indices first
		// to guarantee the permission names are in lexicographical order.
		permIndices := permissionsByRealm[realm].ToSortedSlice()
		permNames := make([]string, len(permIndices))
		for j, permIndex := range permIndices {
			permNames[j] = snap.permissionNames[permIndex]
		}

		realmPermissions[i] = &rpcpb.RealmPermissions{
			Name:        realm,
			Permissions: permNames,
		}
	}

	return &rpcpb.PrincipalPermissions{
		Name:             principal,
		RealmPermissions: realmPermissions,
	}, nil
}
