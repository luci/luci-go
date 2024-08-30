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

package rpc

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/commit"
	"go.chromium.org/luci/source_index/internal/config"
	"go.chromium.org/luci/source_index/internal/gitilesutil"
	"go.chromium.org/luci/source_index/internal/spanutil"
	"go.chromium.org/luci/source_index/internal/validationutil"
	pb "go.chromium.org/luci/source_index/proto/v1"
)

// sourceIndexServer implements pb.SourceIndexServer.
type sourceIndexServer struct{}

// NewSourceIndexServer returns a new pb.SourceIndexServer.
func NewSourceIndexServer() pb.SourceIndexServer {
	return &pb.DecoratedSourceIndex{
		Service:  &sourceIndexServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// QueryCommitHash returns commit that matches desired position of commit,
// based on QueryCommitHashRequest parameters
func (si *sourceIndexServer) QueryCommitHash(ctx context.Context, req *pb.QueryCommitHashRequest) (*pb.QueryCommitHashResponse, error) {
	if err := validateQueryCommitHashRequest(req); err != nil {
		return nil, appstatus.Attachf(err, codes.InvalidArgument, "invalid QueryCommitHashRequest %s", err)
	}

	// Check whether the user has access to the repository by calling Gitiles with
	// the user's credential.
	//
	// This is to prevent unauthorized users using this RPC to verify the
	// existence of private repositories.
	//
	// Note that we do not check position_ref here because position_ref does not
	// necessarily need to match an actual branch. It can be an arbitrary string
	// in the commit message.
	//
	// Do not check whether the user has access to the commit either because
	// 1. commit hashes are not considered to be confidential (see b/352641557);
	//    and
	// 2. we must check ACL before reading the database otherwise unauthorized
	//    users can still verify the existence of a repository by observing the
	//    query latency.
	//
	// Future improvements:
	// - Cache the (successful) response for each user to improve RPC latency.
	client, err := gitilesutil.NewClient(ctx, req.Host, auth.AsCredentialsForwarder)
	if err != nil {
		return nil, errors.Annotate(err, "initialize Gitiles client").Err()
	}
	_, err = client.GetProject(ctx, &gitilespb.GetProjectRequest{Name: req.Repository})
	if err != nil {
		return nil, appstatus.Attachf(err, grpcutil.Code(err), "cannot access repository https://%s/%s", req.Host, req.Repository)
	}

	cfg, err := config.Get(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "service-level config").Err()
	}

	// If the commit is missing because the repository is not configured to be
	// indexed, return INVALID_ARGUMENT instead of NOT_FOUND so the error is less
	// confusing to the caller.
	//
	// This is not perfect because a repository may only specify a limited set of
	// branches to be indexed.
	//
	// Note that position_ref is just an arbitrary string in a commit message
	// footer. It does not necessarily match the branch where the commit is
	// ingested from. As such, we should not check whether the position_ref is
	// configured to be indexed.
	if !cfg.ShouldIndexRepo(req.Host, req.Repository) {
		return nil, appstatus.Errorf(codes.InvalidArgument, "repository https://%s/%s is not configured to be indexed", req.Host, req.Repository)
	}

	mappedCommit, err := commit.ReadByPosition(span.Single(ctx), commit.ReadByPositionOpts{
		Host:       req.Host,
		Repository: req.Repository,
		Position: commit.Position{
			Ref:    req.PositionRef,
			Number: req.PositionNumber,
		},
	})
	if err != nil {
		if errors.Is(err, spanutil.ErrNotExists) {
			return nil, appstatus.Attachf(err, codes.NotFound, "commit not found")
		}
		return nil, errors.Annotate(err, "read commit by position").Err()
	}

	return &pb.QueryCommitHashResponse{
		Hash: mappedCommit.Key().CommitHash(),
	}, nil
}

func validateQueryCommitHashRequest(req *pb.QueryCommitHashRequest) error {
	if err := gitiles.ValidateRepoHost(req.Host); err != nil {
		return errors.Annotate(err, "host").Err()
	}

	if err := validationutil.ValidateRepoName(req.Repository); err != nil {
		return errors.Annotate(err, "repository").Err()
	}

	if req.PositionRef == "" {
		return errors.Reason("position_ref must be specified").Err()
	}

	return nil
}
