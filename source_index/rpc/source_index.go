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
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/commit"
	"go.chromium.org/luci/source_index/internal/config"
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

	if err := ensureCanKnowRepoExists(ctx, req.Host, req.Repository); err != nil {
		return nil, err
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
