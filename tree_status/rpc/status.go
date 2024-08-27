// Copyright 2024 The LUCI Authors.
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

// Package rpc contains the RPC handlers for the tree status service.
package rpc

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/tree_status/internal/perms"
	"go.chromium.org/luci/tree_status/internal/status"
	"go.chromium.org/luci/tree_status/pbutil"
	pb "go.chromium.org/luci/tree_status/proto/v1"
	"go.chromium.org/luci/tree_status/rpc/paginator"
)

type treeStatusServer struct{}

var _ pb.TreeStatusServer = &treeStatusServer{}

// NewTreeStatusServer creates a new server to handle TreeStatus requests.
func NewTreeStatusServer() *pb.DecoratedTreeStatus {
	return &pb.DecoratedTreeStatus{
		Service:  &treeStatusServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

var listPaginator = paginator.Paginator{
	DefaultPageSize: 50,
	MaxPageSize:     1000,
}

// ListStatus lists all status values for a tree in reverse chronological order.
func (*treeStatusServer) ListStatus(ctx context.Context, request *pb.ListStatusRequest) (*pb.ListStatusResponse, error) {
	tree, err := parseStatusParent(request.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}
	ctx = logging.SetField(ctx, "tree_name", tree)
	hasLimitedAccess, msg, err := perms.HasListStatusLimitedPermission(ctx, tree)
	if err != nil {
		return nil, errors.Annotate(err, "checking list status limited permission").Err()
	}
	if !hasLimitedAccess {
		return nil, appstatus.Errorf(codes.PermissionDenied, msg)
	}
	offset, err := listPaginator.Offset(request)
	if err != nil {
		return nil, err
	}
	options := status.ListOptions{
		Offset: offset,
		Limit:  int64(listPaginator.Limit(request.PageSize)),
	}

	// Check if user can access to PII.
	includeUserInResponse, _, err := perms.HasListStatusPermission(ctx, tree)
	if err != nil {
		return nil, errors.Annotate(err, "checking list status permission").Err()
	}

	values, hasNextPage, err := status.List(span.Single(ctx), tree, &options)
	if err != nil {
		return nil, errors.Annotate(err, "listing status values").Err()
	}

	nextPageToken := ""
	if hasNextPage {
		nextPageToken, err = listPaginator.NextPageToken(request, offset+options.Limit)
		if err != nil {
			return nil, err
		}
	}
	response := &pb.ListStatusResponse{
		Status:        []*pb.Status{},
		NextPageToken: nextPageToken,
	}

	for _, value := range values {
		response.Status = append(response.Status, toStatusProto(value, includeUserInResponse))
	}
	return response, nil
}

// toStatusProto converts a status.Status value to a pb.Status proto.
// If includeUser is false, the CreateUser field will be left blank instead of being copied.
func toStatusProto(value *status.Status, includeUser bool) *pb.Status {
	user := ""
	if includeUser {
		user = value.CreateUser
	}
	return &pb.Status{
		Name:         fmt.Sprintf("trees/%s/status/%s", value.TreeName, value.StatusID),
		GeneralState: value.GeneralStatus,
		Message:      value.Message,
		CreateUser:   user,
		CreateTime:   timestamppb.New(value.CreateTime),
	}
}

// GetStatus gets a status for a tree.
// Use the resource alias 'latest' to get just the current status.
func (*treeStatusServer) GetStatus(ctx context.Context, request *pb.GetStatusRequest) (*pb.Status, error) {
	tree, id, err := parseStatusName(request.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "name").Err())
	}
	ctx = logging.SetField(ctx, "tree_name", tree)

	hasLimitedAccess, msg, err := perms.HasGetStatusLimitedPermission(ctx, tree)
	if err != nil {
		return nil, errors.Annotate(err, "checking get status limited permission").Err()
	}
	if !hasLimitedAccess {
		return nil, appstatus.Errorf(codes.PermissionDenied, msg)
	}

	includeUserInResponse, _, err := perms.HasGetStatusPermission(ctx, tree)
	if err != nil {
		return nil, errors.Annotate(err, "checking get status permission").Err()
	}

	if id == "latest" {
		latest, err := status.ReadLatest(span.Single(ctx), tree)
		if errors.Is(err, status.NotExistsErr) {
			return &pb.Status{
				Name:         fmt.Sprintf("trees/%s/status/fallback", tree),
				GeneralState: pb.GeneralState_OPEN,
				Message:      "Tree is open (fallback due to no status updates in past 140 days)",
				CreateUser:   "",
				CreateTime:   timestamppb.New(time.Now()),
			}, nil
		} else if err != nil {
			return nil, errors.Annotate(err, "reading latest status").Err()
		}
		return toStatusProto(latest, includeUserInResponse), nil
	}
	s, err := status.Read(span.Single(ctx), tree, id)
	if errors.Is(err, status.NotExistsErr) {
		return nil, notFoundError(err)
	} else if err != nil {
		return nil, errors.Annotate(err, "reading status").Err()
	}

	return toStatusProto(s, includeUserInResponse), nil
}

// CreateStatus creates a new status update for the tree.
func (*treeStatusServer) CreateStatus(ctx context.Context, request *pb.CreateStatusRequest) (*pb.Status, error) {
	tree, err := parseStatusParent(request.GetParent())
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}
	ctx = logging.SetField(ctx, "tree_name", tree)

	hasWriteAccess, msg, err := perms.HasCreateStatusPermission(ctx, tree)
	if err != nil {
		return nil, errors.Annotate(err, "checking create status permission").Err()
	}
	if !hasWriteAccess {
		return nil, appstatus.Errorf(codes.PermissionDenied, msg)
	}

	id, err := status.GenerateID()
	if err != nil {
		return nil, errors.Annotate(err, "generating status id").Err()
	}

	// Ignore the closing builder name if the status is not closed.
	closingBuilderName := ""
	if request.Status.GeneralState == pb.GeneralState_CLOSED {
		closingBuilderName = request.Status.ClosingBuilderName
	}
	s := &status.Status{
		TreeName:           tree,
		StatusID:           id,
		GeneralStatus:      request.Status.GeneralState,
		Message:            request.Status.Message,
		ClosingBuilderName: closingBuilderName,
	}
	user := auth.CurrentIdentity(ctx).Value()
	m, err := status.Create(s, user)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "create status").Err())
	}
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	if err != nil {
		return nil, errors.Annotate(err, "apply create status to spanner").Err()
	}

	return &pb.Status{
		Name:               fmt.Sprintf("trees/%s/status/%s", tree, id),
		GeneralState:       s.GeneralStatus,
		Message:            s.Message,
		CreateUser:         user,
		CreateTime:         timestamppb.New(ts),
		ClosingBuilderName: closingBuilderName,
	}, nil
}

var statusParentRE = regexp.MustCompile(`^trees/(` + pbutil.TreeNameExpression + `)/status$`)
var statusNameRE = regexp.MustCompile(`^trees/(` + pbutil.TreeNameExpression + `)/status/(` + pbutil.StatusIDExpression + `|latest)$`)

// parseStatusParent parses a status resource parent into its constituent ID
// parts.
func parseStatusParent(parent string) (tree string, err error) {
	if parent == "" {
		return "", errors.Reason("must be specified").Err()
	}
	match := statusParentRE.FindStringSubmatch(parent)
	if match == nil {
		return "", errors.Reason("expected format: %s", statusParentRE).Err()
	}
	return match[1], nil
}

// parseStatusName parses a status resource name into its constituent ID
// parts.
func parseStatusName(name string) (tree string, id string, err error) {
	if name == "" {
		return "", "", errors.Reason("must be specified").Err()
	}
	match := statusNameRE.FindStringSubmatch(name)
	if match == nil {
		return "", "", errors.Reason("expected format: %s", statusNameRE).Err()
	}
	return match[1], match[2], nil
}

// invalidArgumentError annotates err as having an invalid argument.
// The error message is shared with the requester as is.
//
// Note that this differs from FailedPrecondition. It indicates arguments
// that are problematic regardless of the state of the system
// (e.g., a malformed file name).
func invalidArgumentError(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "%s", err)
}

// permissionDeniedError annotates err as being denied (HTTP 403).
// The error message is shared with the requester as is.
func permissionDeniedError(err error) error {
	return appstatus.Attachf(err, codes.PermissionDenied, "%s", err)
}

// notFoundError annotates err as being not found (HTTP 404).
// The error message is shared with the requester as is.
func notFoundError(err error) error {
	return appstatus.Attachf(err, codes.NotFound, "%s", err)
}
