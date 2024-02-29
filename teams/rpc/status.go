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

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/teams/internal/teams"
	pb "go.chromium.org/luci/teams/proto/v1"
)

type teamsServer struct{}

var _ pb.TeamsServer = &teamsServer{}

// NewTeamsServer creates a new server to handle Teams requests.
func NewTeamsServer() *pb.DecoratedTeams {
	return &pb.DecoratedTeams{
		Prelude:  checkAllowedPrelude,
		Service:  &teamsServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// toTeamProto converts a teams.Team value to a pb.Team proto.
func toTeamProto(value *teams.Team) *pb.Team {
	return &pb.Team{
		Name:       fmt.Sprintf("teams/%s", value.ID),
		CreateTime: timestamppb.New(value.CreateTime),
	}
}

// Get gets a team.
// Use the resource alias 'my' to get just the current user's team.
func (*teamsServer) Get(ctx context.Context, request *pb.GetTeamRequest) (*pb.Team, error) {
	id, err := parseTeamName(request.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "name").Err())
	}

	if id == "my" {
		// TODO: Resolve team for user.
		return nil, appstatus.Errorf(codes.Unimplemented, "resolving team for current user not yet implemented")
	}
	s, err := teams.Read(span.Single(ctx), id)
	if errors.Is(err, teams.NotExistsErr) {
		return nil, notFoundError(err)
	} else if err != nil {
		return nil, errors.Annotate(err, "reading team").Err()
	}

	return toTeamProto(s), nil
}

var teamNameRE = regexp.MustCompile(`^teams/(` + teams.TeamIDExpression + `|my)$`)

// parseTeamName parses a team resource name into its constituent ID
// parts.
func parseTeamName(name string) (id string, err error) {
	if name == "" {
		return "", errors.Reason("must be specified").Err()
	}
	match := teamNameRE.FindStringSubmatch(name)
	if match == nil {
		return "", errors.Reason("expected format: %s", teamNameRE).Err()
	}
	return match[1], nil
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

// notFoundError annotates err as being not found (HTTP 404).
// The error message is shared with the requester as is.
func notFoundError(err error) error {
	return appstatus.Attachf(err, codes.NotFound, "%s", err)
}
