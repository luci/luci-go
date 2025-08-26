// Copyright 2025 The LUCI Authors.
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

package rpc

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/internal/alertgroups"
)

type alertGroupsServer struct{}

var _ pb.AlertsServer = &alertsServer{}

// NewAlertsServer creates a new server to handle Alerts requests.
func NewAlertGroupsServer() *pb.DecoratedAlertGroups {
	return &pb.DecoratedAlertGroups{
		Prelude:  checkAllowedPrelude,
		Service:  &alertGroupsServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// toAlertGroupProto converts an alertgroups.AlertGroup value to a pb.AlertGroup proto.
func toAlertGroupProto(value *alertgroups.AlertGroup) *pb.AlertGroup {
	return &pb.AlertGroup{
		Name:          fmt.Sprintf("rotations/%s/alertGroups/%s", value.Rotation, value.AlertGroupId),
		DisplayName:   value.DisplayName,
		StatusMessage: value.StatusMessage,
		AlertKeys:     value.AlertKeys,
		Bugs:          value.Bugs,
		UpdateTime:    timestamppb.New(value.UpdateTime),
		UpdatedBy:     value.UpdatedBy,
		Etag:          value.Etag(),
	}
}

// fromAlertGroupProto converts a proto to an AlertGroup.
// Does not fill in rotation or AlertGroupId.
func fromAlertGroupProto(value *pb.AlertGroup) *alertgroups.AlertGroup {
	g := &alertgroups.AlertGroup{
		DisplayName:   value.DisplayName,
		StatusMessage: value.StatusMessage,
		AlertKeys:     value.AlertKeys,
		Bugs:          value.Bugs,
		UpdatedBy:     value.UpdatedBy,
	}
	if g.AlertKeys == nil {
		g.AlertKeys = []string{}
	}
	if g.Bugs == nil {
		g.Bugs = []int64{}
	}
	return g
}

// GetAlertGroup gets an alert group.
func (s *alertGroupsServer) GetAlertGroup(ctx context.Context, req *pb.GetAlertGroupRequest) (*pb.AlertGroup, error) {
	rotationId, alertGroupId, err := alertgroups.ParseAlertGroupName(req.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Fmt("name: %w", err))
	}

	g, err := alertgroups.Read(span.Single(ctx), rotationId, alertGroupId)
	if err != nil {
		return nil, err
	}
	return toAlertGroupProto(g), nil
}

// ListAlertGroups lists alert groups.
func (s *alertGroupsServer) ListAlertGroups(ctx context.Context, req *pb.ListAlertGroupsRequest) (*pb.ListAlertGroupsResponse, error) {
	rotation, err := alertgroups.ParseRotationName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Fmt("parent: %w", err))
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	groups, nextPageToken, err := alertgroups.List(span.Single(ctx), rotation, int(pageSize), req.PageToken)
	if err != nil {
		return nil, err
	}
	response := &pb.ListAlertGroupsResponse{NextPageToken: nextPageToken}
	for _, g := range groups {
		response.AlertGroups = append(response.AlertGroups, toAlertGroupProto(g))
	}
	return response, nil
}

// CreateAlertGroup creates an alert group.
func (s *alertGroupsServer) CreateAlertGroup(ctx context.Context, req *pb.CreateAlertGroupRequest) (*pb.AlertGroup, error) {
	if err := checkWriteAccess(ctx); err != nil {
		return nil, err
	}
	rotationId, err := alertgroups.ParseRotationName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Fmt("parent: %w", err))
	}

	g := fromAlertGroupProto(req.AlertGroup)
	g.Rotation = rotationId
	g.AlertGroupId = req.AlertGroupId
	g.UpdatedBy = auth.CurrentUser(ctx).Email

	m, err := alertgroups.Create(g)
	if err != nil {
		return nil, invalidArgumentError(errors.Fmt("alert_group: %w", err))
	}
	g.UpdateTime, err = span.Apply(ctx, []*spanner.Mutation{m})
	if err != nil {
		return nil, errors.Fmt("apply create alert group to spanner: %w", err)
	}
	return toAlertGroupProto(g), nil
}

// UpdateAlertGroup updates an alert group.
func (s *alertGroupsServer) UpdateAlertGroup(ctx context.Context, req *pb.UpdateAlertGroupRequest) (*pb.AlertGroup, error) {
	if err := checkWriteAccess(ctx); err != nil {
		return nil, err
	}
	if req.AlertGroup == nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "alert_group is required")
	}
	rotationId, alertGroupId, err := alertgroups.ParseAlertGroupName(req.AlertGroup.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Fmt("name: %w", err))
	}
	g := fromAlertGroupProto(req.AlertGroup)
	g.Rotation = rotationId
	g.AlertGroupId = alertGroupId
	g.UpdatedBy = auth.CurrentUser(ctx).Email

	// Normalize and validate the update mask.
	if req.UpdateMask == nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "update_mask is required")
	}
	req.UpdateMask.Normalize()
	if !req.UpdateMask.IsValid(req.AlertGroup) {
		return nil, appstatus.Errorf(codes.InvalidArgument, "invalid update_mask")
	}
	// Ensure the user is not trying to update output-only fields.
	for _, path := range req.UpdateMask.GetPaths() {
		switch path {
		case "display_name", "status_message", "alert_keys", "bugs":
			// These fields are mutable.
		default:
			return nil, appstatus.Errorf(codes.InvalidArgument, "cannot update read-only field %q", path)
		}
	}
	commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		existing, err := alertgroups.Read(ctx, rotationId, alertGroupId)
		if err != nil {
			return err
		}
		if req.AlertGroup.Etag != "" && req.AlertGroup.Etag != existing.Etag() {
			return appstatus.Errorf(codes.InvalidArgument, "etag mismatch")
		}

		// Apply the update mask.
		for _, path := range req.UpdateMask.GetPaths() {
			switch path {
			case "display_name":
				existing.DisplayName = g.DisplayName
			case "status_message":
				existing.StatusMessage = g.StatusMessage
			case "alert_keys":
				existing.AlertKeys = g.AlertKeys
			case "bugs":
				existing.Bugs = g.Bugs
			}
		}
		existing.UpdatedBy = g.UpdatedBy

		m, err := alertgroups.Update(existing)
		if err != nil {
			return invalidArgumentError(errors.Fmt("alert_group: %w", err))
		}
		g = existing
		span.BufferWrite(ctx, m)
		return nil
	})
	if err != nil {
		return nil, errors.Fmt("apply update alert group to spanner: %w", err)
	}
	g.UpdateTime = commitTime
	return toAlertGroupProto(g), nil
}

// DeleteAlertGroup deletes an alert group.
func (s *alertGroupsServer) DeleteAlertGroup(ctx context.Context, req *pb.DeleteAlertGroupRequest) (*emptypb.Empty, error) {
	if err := checkWriteAccess(ctx); err != nil {
		return nil, err
	}
	rotationId, alertGroupId, err := alertgroups.ParseAlertGroupName(req.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Fmt("name: %w", err))
	}
	m := alertgroups.Delete(ctx, rotationId, alertGroupId)
	if _, err := span.Apply(ctx, []*spanner.Mutation{m}); err != nil {
		return nil, errors.Fmt("apply delete alert group to spanner: %w", err)
	}
	return &emptypb.Empty{}, nil
}

func checkWriteAccess(ctx context.Context) error {
	hasWriteAccess, err := auth.IsMember(ctx, luciNotifyWriteAccessGroup)
	if err != nil {
		return errors.Fmt("checking write group membership: %w", err)
	}
	// TODO: configurable ACLs based on the rotation.
	if !hasWriteAccess {
		if auth.CurrentIdentity(ctx).Kind() == identity.Anonymous {
			return permissionDeniedError(errors.New("please log in before updating alerts"))
		}
		return permissionDeniedError(errors.New("you do not have permission to update alerts"))
	}
	return nil
}
