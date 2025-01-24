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
func (srv *Server) ListGroups(ctx context.Context, _ *emptypb.Empty) (*rpcpb.ListGroupsResponse, error) {
	// Get all groups.
	groups, err := srv.authGroupsProvider.GetAllAuthGroups(ctx, true)
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
	case errors.Is(err, model.ErrInvalidName):
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
	case errors.Is(err, model.ErrAlreadyExists):
		return nil, status.Errorf(codes.AlreadyExists, "group already exists: %s", group.ID)
	case errors.Is(err, model.ErrInvalidName):
		return nil, status.Errorf(codes.InvalidArgument, "invalid group name: %s", group.ID)
	case errors.Is(err, model.ErrInvalidReference):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, model.ErrInvalidIdentity):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, model.ErrInvalidArgument):
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
	case errors.Is(err, model.ErrPermissionDenied):
		return nil, status.Errorf(codes.PermissionDenied, "%s does not have permission to update group %q: %s", auth.CurrentIdentity(ctx), groupUpdate.ID, err)
	case errors.Is(err, model.ErrConcurrentModification):
		return nil, status.Error(codes.Aborted, err.Error())
	case errors.Is(err, model.ErrInvalidReference):
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	case errors.Is(err, model.ErrInvalidArgument):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, model.ErrInvalidIdentity):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, model.ErrCyclicDependency):
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
	case errors.Is(err, model.ErrPermissionDenied):
		return nil, status.Errorf(codes.PermissionDenied, "%s does not have permission to delete group %q: %s", auth.CurrentIdentity(ctx), name, err)
	case errors.Is(err, model.ErrConcurrentModification):
		return nil, status.Error(codes.Aborted, err.Error())
	case errors.Is(err, model.ErrReferencedEntity):
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
	// Get all groups.
	groups, err := srv.authGroupsProvider.GetAllAuthGroups(ctx, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"Failed to fetch groups: %s", err)
	}

	// Build graph from groups.
	groupsGraph := graph.NewGraph(groups)

	// Get the recursively expanded group members, globs and nested groups.
	// Members may be redacted based on the caller's group memberships.
	expandedGroup, err := groupsGraph.GetExpandedGroup(ctx, request.Name, false)
	if err != nil {
		if errors.Is(err, graph.ErrNoSuchGroup) {
			return nil, status.Errorf(codes.NotFound,
				"The requested group \"%s\" was not found.", request.Name)
		}

		return nil, status.Errorf(codes.Internal,
			"Error expanding group: %s", err)
	}

	return expandedGroup, nil
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
	// Get all groups.
	groups, err := srv.authGroupsProvider.GetAllAuthGroups(ctx, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"Failed to fetch groups: %s", err)
	}

	// Build graph from groups.
	groupsGraph := graph.NewGraph(groups)

	principal, err := convertPrincipal(request.Principal)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"Issue parsing the principal from the request: %s", err)
	}

	subgraph, err := groupsGraph.GetRelevantSubgraph(principal)
	if err != nil {
		if errors.Is(err, graph.ErrNoSuchGroup) {
			return nil, status.Errorf(codes.NotFound,
				"The requested group \"%s\" was not found.", principal.Value)
		}

		return nil, status.Errorf(codes.Internal,
			"Error getting relevant groups: %s", err)
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
		if errors.Is(err, model.ErrInvalidName) {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return status.Errorf(codes.NotFound, "no such group %s", name)
		}
		err = errors.Annotate(err, "legacyGetAuthGroup failed").Err()
		logging.Errorf(c, err.Error())
		return status.Errorf(codes.Internal, "failed to fetch group %q", name)
	}

	canModify, err := model.CanCallerModify(c, group)
	if err != nil {
		err = errors.Annotate(err, "failed to check if caller can modify").Err()
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
		return errors.Annotate(err, "error encoding JSON").Err()
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

	// Get all groups.
	groups, err := srv.authGroupsProvider.GetAllAuthGroups(c, false)
	if err != nil {
		return status.Error(codes.Internal, "failed to fetch all groups")
	}

	// Build graph from groups.
	groupsGraph := graph.NewGraph(groups)

	// Get the recursively expanded group members, globs and nested groups.
	// Disable the privacy filter to maintain the behavior of this legacy
	// endpoint.
	expandedGroup, err := groupsGraph.GetExpandedGroup(c, name, true)
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
	for i, member := range expandedGroup.Members {
		response.Members[i] = listingPrincipal{Principal: member}
	}
	for i, glob := range expandedGroup.Globs {
		response.Globs[i] = listingPrincipal{Principal: glob}
	}
	for i, nestedGroup := range expandedGroup.Nested {
		response.Nested[i] = listingPrincipal{Principal: nestedGroup}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err = json.NewEncoder(w).Encode(map[string]ListingJSON{
		"listing": response,
	})
	if err != nil {
		return errors.Annotate(err, "error encoding JSON").Err()
	}

	return nil
}

// convertPrincipal handles the conversion of rpcpb.Principal -> graph.NodeKey
func convertPrincipal(p *rpcpb.Principal) (graph.NodeKey, error) {
	switch p.Kind {
	case rpcpb.PrincipalKind_GLOB:
		return graph.NodeKey{Kind: graph.Glob, Value: p.Name}, nil
	case rpcpb.PrincipalKind_IDENTITY:
		return graph.NodeKey{Kind: graph.Identity, Value: p.Name}, nil
	case rpcpb.PrincipalKind_GROUP:
		return graph.NodeKey{Kind: graph.Group, Value: p.Name}, nil
	default:
		return graph.NodeKey{}, status.Errorf(codes.InvalidArgument, "invalid principal kind")
	}
}
