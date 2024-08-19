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

// AuthGroupsProvider is the interface to get all AuthGroup entities.
type AuthGroupsProvider interface {
	GetAllAuthGroups(ctx context.Context) ([]*model.AuthGroup, error)
	RefreshPeriodically(ctx context.Context)
}

// Server implements Groups server.
type Server struct {
	rpcpb.UnimplementedGroupsServer

	authGroupsProvider AuthGroupsProvider

	// Whether modifications should be done in dry run mode (i.e. skip
	// committing entity changes).
	dryRun bool
}

func NewServer(dryRun bool) *Server {
	return &Server{
		authGroupsProvider: &CachingGroupsProvider{},
		dryRun:             dryRun,
	}
}

// Warmup does the setup for the groups server; it should be called before the
// main serving loop.
func (srv *Server) Warmup(ctx context.Context) {
	if srv.authGroupsProvider != nil {
		_, err := srv.authGroupsProvider.GetAllAuthGroups(ctx)
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
	groups, err := srv.authGroupsProvider.GetAllAuthGroups(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch groups: %s", err)
	}

	var groupList = make([]*rpcpb.AuthGroup, len(groups))
	for idx, entity := range groups {
		g := entity.ToProto(false)
		g.CallerCanModify = canCallerModify(ctx, entity)
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
		g := group.ToProto(true)
		g.CallerCanModify = canCallerModify(ctx, group)
		return g, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no such group %q", request.Name)
	default:
		return nil, status.Errorf(codes.Internal, "failed to fetch group %q: %s", request.Name, err)
	}
}

// CreateGroup implements the corresponding RPC method.
func (srv *Server) CreateGroup(ctx context.Context, request *rpcpb.CreateGroupRequest) (*rpcpb.AuthGroup, error) {
	group := model.AuthGroupFromProto(ctx, request.GetGroup())
	switch createdGroup, err := model.CreateAuthGroup(ctx, group, false, "Go pRPC API", srv.dryRun); {
	case err == nil:
		return createdGroup.ToProto(true), nil
	case errors.Is(err, model.ErrAlreadyExists):
		return nil, status.Errorf(codes.AlreadyExists, "group already exists: %s", group.ID)
	case errors.Is(err, model.ErrInvalidName):
		return nil, status.Errorf(codes.InvalidArgument, "invalid group name: %s", group.ID)
	case errors.Is(err, model.ErrInvalidReference):
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	case errors.Is(err, model.ErrInvalidIdentity):
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	default:
		return nil, status.Errorf(codes.Internal, "failed to create group %q: %s", request.GetGroup().GetName(), err)
	}
}

// UpdateGroup implements the corresponding RPC method.
func (srv *Server) UpdateGroup(ctx context.Context, request *rpcpb.UpdateGroupRequest) (*rpcpb.AuthGroup, error) {
	groupUpdate := model.AuthGroupFromProto(ctx, request.GetGroup())
	switch updatedGroup, err := model.UpdateAuthGroup(ctx, groupUpdate, request.GetUpdateMask(), request.GetGroup().GetEtag(), false, "Go pRPC API", srv.dryRun); {
	case err == nil:
		return updatedGroup.ToProto(true), nil
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
	switch err := model.DeleteAuthGroup(ctx, name, request.GetEtag(), false, "Go pRPC API", srv.dryRun); {
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
	groups, err := srv.authGroupsProvider.GetAllAuthGroups(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"Failed to fetch groups: %s", err)
	}

	// Build graph from groups.
	groupsGraph := graph.NewGraph(groups)
	expandedGroup, err := groupsGraph.GetExpandedGroup(request.Name)
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
	groups, err := srv.authGroupsProvider.GetAllAuthGroups(ctx)
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

func (s *Server) GetLegacyAuthGroup(ctx *router.Context) error {
	c, _, w := ctx.Request.Context(), ctx.Request, ctx.Writer
	name := ctx.Params.ByName("groupName")

	group, err := model.GetAuthGroup(c, name)
	if err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return status.Errorf(codes.NotFound, "no such group %s", name)
		}
		err = errors.Annotate(err, "legacyGetAuthGroup failed").Err()
		logging.Errorf(c, err.Error())
		return status.Errorf(codes.Internal, "failed to fetch group %q", name)
	}

	// Set the Last-Modified header.
	w.Header().Set("Last-Modified", group.ModifiedTS.Format(time.RFC1123Z))

	blob, err := json.Marshal(map[string]AuthGroupJSON{
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
			CallerCanModify: canCallerModify(c, group),
		},
	})
	if err != nil {
		return errors.Annotate(err, "error marshalling JSON").Err()
	}

	if _, err = w.Write(blob); err != nil {
		return errors.Annotate(err, "error writing JSON").Err()
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

// canCallerModify returns whether the current identity can modify the
// given group.
func canCallerModify(ctx context.Context, g *model.AuthGroup) bool {
	if model.IsExternalAuthGroupName(g.ID) {
		return false
	}

	isOwner, err := auth.IsMember(ctx, model.AdminGroup, g.Owners)
	if err != nil {
		logging.Errorf(ctx, "error checking group owner status: %w", err)
		return false
	}
	return isOwner
}
