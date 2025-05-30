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
	"encoding/json"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	customerrors "go.chromium.org/luci/auth_service/impl/errors"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/impl/model/graph"
)

// AuthGroupJSON is the underlying data structure returned by the legacy
// method of getting an AuthGroup.
type AuthGroupJSON struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Owners          string   `json:"owners"`
	Members         []string `json:"members"`
	Globs           []string `json:"globs"`
	Nested          []string `json:"nested"`
	CreatedBy       string   `json:"created_by"`
	CreatedTS       int64    `json:"created_ts"`
	ModifiedBy      string   `json:"modified_by"`
	ModifiedTS      int64    `json:"modified_ts"`
	CallerCanModify bool     `json:"caller_can_modify"`
}

type listingPrincipal struct {
	Principal string `json:"principal"`
}

// ListingJSON is the underlying data structure returned by the legacy method of
// getting the expanded listing of an AuthGroup.
type ListingJSON struct {
	Members []listingPrincipal `json:"members"`
	Globs   []listingPrincipal `json:"globs"`
	Nested  []listingPrincipal `json:"nested"`
}

// AuthGroupsProvider is the interface to get all AuthGroup entities.
type AuthGroupsProvider interface {
	GetAllAuthGroups(ctx context.Context, allowStale bool) ([]*model.AuthGroup, error)
	GetGroupsGraph(ctx context.Context, allowStale bool) (*graph.Graph, error)
	RefreshPeriodically(ctx context.Context)
}

// Server implements Groups server.
type Server struct {
	rpcpb.UnimplementedGroupsServer

	authGroupsProvider AuthGroupsProvider
}

func NewServer() *Server {
	return &Server{
		authGroupsProvider: &CachingGroupsProvider{},
	}
}

// WarmUp does the setup for the groups server; it should be called before the
// main serving loop.
func (srv *Server) WarmUp(ctx context.Context) {
	if srv.authGroupsProvider != nil {
		_, err := srv.authGroupsProvider.GetAllAuthGroups(ctx, false)
		if err != nil {
			logging.Errorf(ctx, "error warming up AuthGroups provider")
		}
	}
}

// RefreshPeriodically wraps the groups provider's refresh method.
func (srv *Server) RefreshPeriodically(ctx context.Context) {
	if srv.authGroupsProvider != nil {
		srv.authGroupsProvider.RefreshPeriodically(ctx)
	}
}

// ListGroups implements the corresponding RPC method.
func (srv *Server) ListGroups(ctx context.Context, request *rpcpb.ListGroupsRequest) (*rpcpb.ListGroupsResponse, error) {
	// Get all groups.
	allowStale := !request.GetFresh()
	groups, err := srv.authGroupsProvider.GetAllAuthGroups(ctx, allowStale)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch groups: %s", err)
	}

	var groupList = make([]*rpcpb.AuthGroup, len(groups))
	for idx, entity := range groups {
		g, err := entity.ToProto(ctx, false)
		if err != nil {
			return nil, err
		}
		groupList[idx] = g
	}

	return &rpcpb.ListGroupsResponse{
		Groups: groupList,
	}, nil
}

// GetGroup implements the corresponding RPC method.
func (*Server) GetGroup(ctx context.Context, request *rpcpb.GetGroupRequest) (*rpcpb.AuthGroup, error) {
	switch group, err := model.GetAuthGroup(ctx, request.Name); {
	case err == nil:
		g, err := group.ToProto(ctx, true)
		if err != nil {
			return nil, err
		}
		return g, nil
	case errors.Is(err, customerrors.ErrInvalidName):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no such group %q", request.Name)
	default:
		return nil, status.Errorf(codes.Internal, "failed to fetch group %q: %s", request.Name, err)
	}
}

// CreateGroup implements the corresponding RPC method.
func (srv *Server) CreateGroup(ctx context.Context, request *rpcpb.CreateGroupRequest) (*rpcpb.AuthGroup, error) {
	group := model.AuthGroupFromProto(ctx, request.GetGroup())
	switch createdGroup, err := model.CreateAuthGroup(ctx, group, "Go pRPC API"); {
	case err == nil:
		newGroup, err := createdGroup.ToProto(ctx, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to lookup caller: %s", err)
		}
		return newGroup, nil
	case errors.Is(err, customerrors.ErrAlreadyExists):
		return nil, status.Errorf(codes.AlreadyExists, "group already exists: %s", group.ID)
	case errors.Is(err, customerrors.ErrInvalidName):
		return nil, status.Errorf(codes.InvalidArgument, "invalid group name: %s", group.ID)
	case errors.Is(err, customerrors.ErrInvalidReference):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, customerrors.ErrInvalidIdentity):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, customerrors.ErrInvalidArgument):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	default:
		return nil, status.Errorf(codes.Internal, "failed to create group %q: %s", request.GetGroup().GetName(), err)
	}
}

// UpdateGroup implements the corresponding RPC method.
func (srv *Server) UpdateGroup(ctx context.Context, request *rpcpb.UpdateGroupRequest) (*rpcpb.AuthGroup, error) {
	groupUpdate := model.AuthGroupFromProto(ctx, request.GetGroup())
	switch updatedGroup, err := model.UpdateAuthGroup(ctx, groupUpdate, request.GetUpdateMask(), request.GetGroup().GetEtag(), "Go pRPC API"); {
	case err == nil:
		updatedGroupProto, err := updatedGroup.ToProto(ctx, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to lookup caller: %s", err)
		}
		return updatedGroupProto, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no such group %q", groupUpdate.ID)
	case errors.Is(err, customerrors.ErrPermissionDenied):
		return nil, status.Errorf(codes.PermissionDenied, "%s does not have permission to update group %q: %s", auth.CurrentIdentity(ctx), groupUpdate.ID, err)
	case errors.Is(err, customerrors.ErrConcurrentModification):
		return nil, status.Error(codes.Aborted, err.Error())
	case errors.Is(err, customerrors.ErrInvalidReference):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, customerrors.ErrInvalidArgument):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, customerrors.ErrInvalidIdentity):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, customerrors.ErrCyclicDependency):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	default:
		return nil, status.Errorf(codes.Internal, "failed to update group %q: %s", request.GetGroup().GetName(), err)
	}
}

// DeleteGroup implements the corresponding RPC method.
func (srv *Server) DeleteGroup(ctx context.Context, request *rpcpb.DeleteGroupRequest) (*emptypb.Empty, error) {
	name := request.GetName()
	switch err := model.DeleteAuthGroup(ctx, name, request.GetEtag(), "Go pRPC API"); {
	case err == nil:
		return &emptypb.Empty{}, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no such group %q", name)
	case errors.Is(err, customerrors.ErrPermissionDenied):
		return nil, status.Errorf(codes.PermissionDenied, "%s does not have permission to delete group %q: %s", auth.CurrentIdentity(ctx), name, err)
	case errors.Is(err, customerrors.ErrConcurrentModification):
		return nil, status.Error(codes.Aborted, err.Error())
	case errors.Is(err, customerrors.ErrReferencedEntity):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	// TODO(cjacomet): Handle context cancelled and internal errors in a more general way with logging.
	case errors.Is(err, context.Canceled):
		return nil, status.Error(codes.Canceled, err.Error())
	default:
		return nil, status.Errorf(codes.Internal, "failed to delete group %q: %s", name, err)
	}
}

// GetExpandedGroup implements the corresponding RPC method.
//
// Possible Errors:
//
//	Internal error for datastore access issues.
//	NotFound error wrapping a graph.ErrNoSuchGroup if group is not present in groups graph.
func (srv *Server) GetExpandedGroup(ctx context.Context, request *rpcpb.GetGroupRequest) (*rpcpb.AuthGroup, error) {
	// Get the latest graph of all groups.
	groupsGraph, err := srv.authGroupsProvider.GetGroupsGraph(ctx, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to fetch groups graph: %s", err)
	}

	// Get the recursively expanded group members, globs and nested groups.
	// Members may be redacted based on the caller's group memberships.
	expandedGroup, err := groupsGraph.GetExpandedGroup(ctx, request.Name, false, nil)
	if err != nil {
		if errors.Is(err, graph.ErrNoSuchGroup) {
			return nil, status.Errorf(codes.NotFound,
				"the requested group %q was not found.", request.Name)
		}

		return nil, status.Errorf(codes.Internal,
			"error expanding group %q: %s", request.Name, err)
	}

	return expandedGroup.ToProto(), nil
}

// GetSubgraph implements the corresponding RPC method.
//
// Possible Errors:
//
//	Internal error for datastore access issues.
//	NotFound error wrapping a graph.ErrNoSuchGroup if group is not present in groups graph.
//	InvalidArgument error if the PrincipalKind is unspecified.
//	Annotated error if the subgraph building fails, this may be an InvalidArgument or NotFound error.
func (srv *Server) GetSubgraph(ctx context.Context, request *rpcpb.GetSubgraphRequest) (*rpcpb.Subgraph, error) {
	// Get the latest graph of all groups.
	groupsGraph, err := srv.authGroupsProvider.GetGroupsGraph(ctx, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to fetch groups graph: %s", err)
	}

	principal, err := graph.ConvertPrincipal(request.Principal)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"issue parsing the principal from the request: %s", err)
	}

	subgraph, err := groupsGraph.GetRelevantSubgraph(principal)
	if err != nil {
		if errors.Is(err, graph.ErrNoSuchGroup) {
			return nil, status.Errorf(codes.NotFound,
				"the requested group %q was not found.", principal.Value)
		}

		return nil, status.Errorf(codes.Internal,
			"error getting relevant groups for %q: %s", principal.Value, err)
	}

	subgraphProto := subgraph.ToProto()
	return subgraphProto, nil
}

// GetLegacyAuthGroup serves the legacy REST API GET request for an AuthGroup.
func (srv *Server) GetLegacyAuthGroup(ctx *router.Context) error {
	c, _, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	// Log the caller of this legacy function.
	logging.Debugf(c, "GetLegacyAuthGroup call by %s", auth.CurrentIdentity(c))

	// Catch-all params match includes the directory index (the leading "/"), so
	// trim it to get the group name.
	name := strings.TrimPrefix(ctx.Params.ByName("groupName"), "/")

	group, err := model.GetAuthGroup(c, name)
	if err != nil {
		if errors.Is(err, customerrors.ErrInvalidName) {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return status.Errorf(codes.NotFound, "no such group %q", name)
		}
		err = errors.Fmt("legacyGetAuthGroup failed: %w", err)
		logging.Errorf(c, err.Error())
		return status.Errorf(codes.Internal, "failed to fetch group %q", name)
	}

	canModify, err := model.CanCallerModify(c, group)
	if err != nil {
		err = errors.Fmt("failed to check if caller can modify: %w", err)
		return status.Error(codes.Internal, err.Error())
	}

	// Define Members, Globs and Nested if they are not set so the fields are
	// marshalled to empty arrays instead of null.
	if group.Members == nil {
		group.Members = make([]string, 0)
	}
	if group.Globs == nil {
		group.Globs = make([]string, 0)
	}
	if group.Nested == nil {
		group.Nested = make([]string, 0)
	}

	// Set the Last-Modified header.
	w.Header().Set("Last-Modified", group.ModifiedTS.Format(time.RFC1123Z))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err = json.NewEncoder(w).Encode(map[string]AuthGroupJSON{
		"group": {
			Name:            group.ID,
			Description:     group.Description,
			Owners:          group.Owners,
			Members:         group.Members,
			Globs:           group.Globs,
			Nested:          group.Nested,
			CreatedBy:       group.CreatedBy,
			CreatedTS:       group.CreatedTS.UnixMicro(),
			ModifiedBy:      group.ModifiedBy,
			ModifiedTS:      group.ModifiedTS.UnixMicro(),
			CallerCanModify: canModify,
		},
	})
	if err != nil {
		return errors.Fmt("error encoding JSON: %w", err)
	}

	return nil
}

// GetLegacyListing serves the legacy REST API GET request for the listing of an
// AuthGroup.
func (srv *Server) GetLegacyListing(ctx *router.Context) error {
	c, _, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	// Log the caller of this legacy function.
	logging.Debugf(c, "GetLegacyListing call by %s", auth.CurrentIdentity(c))

	// Catch-all params match includes the directory index (the leading "/"), so
	// trim it to get the group name.
	name := strings.TrimPrefix(ctx.Params.ByName("groupName"), "/")

	if !auth.IsValidGroupName(name) {
		return status.Error(codes.InvalidArgument, "invalid group name")
	}

	// Get the latest graph of all groups.
	groupsGraph, err := srv.authGroupsProvider.GetGroupsGraph(c, false)
	if err != nil {
		return status.Error(codes.Internal, "failed to fetch graph of all groups")
	}

	// Get the recursively expanded group members, globs and nested groups.
	// Disable the privacy filter to maintain the behavior of this legacy
	// endpoint.
	expandedGroup, err := groupsGraph.GetExpandedGroup(c, name, true, nil)
	if err != nil {
		if errors.Is(err, graph.ErrNoSuchGroup) {
			return status.Errorf(codes.NotFound, "no such group %q", name)
		}

		return status.Errorf(codes.Internal, "failed to expand group %q", name)
	}

	response := ListingJSON{
		Members: make([]listingPrincipal, len(expandedGroup.Members)),
		Globs:   make([]listingPrincipal, len(expandedGroup.Globs)),
		Nested:  make([]listingPrincipal, len(expandedGroup.Nested)),
	}
	for i, member := range expandedGroup.Members.ToSortedSlice() {
		response.Members[i] = listingPrincipal{Principal: member}
	}
	for i, glob := range expandedGroup.Globs.ToSortedSlice() {
		response.Globs[i] = listingPrincipal{Principal: glob}
	}
	for i, nestedGroup := range expandedGroup.Nested.ToSortedSlice() {
		response.Nested[i] = listingPrincipal{Principal: nestedGroup}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err = json.NewEncoder(w).Encode(map[string]ListingJSON{
		"listing": response,
	})
	if err != nil {
		return errors.Fmt("error encoding JSON: %w", err)
	}

	return nil
}
