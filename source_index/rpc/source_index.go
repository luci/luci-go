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
	"fmt"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching/layered"
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

// canKnowRepoExistsCacheDuration is the cache duration for whether
// the user should be able to know the existence of a Gitiles repository.
//
// We don't return any confidential data in the RPC. Note that commit hashes
// from confidential repositories are not considered confidential (see
// b/352641557). Even if the user's access to a repository was revoked while the
// result is cached, they already knew that the repository existed. As a result,
// this can be cached for a long time.
var canKnowRepoExistsCacheDuration = time.Hour * 24 * 7 * 4
var canKnowRepoExistsCache = layered.RegisterCache(layered.Parameters[bool]{
	GlobalNamespace: "can-know-repository-exists-v1",
	Marshal: func(item bool) ([]byte, error) {
		if item {
			return []byte{0}, nil
		}
		return nil, nil
	},
	Unmarshal: func(blob []byte) (bool, error) {
		return len(blob) > 0, nil
	},
})

// ensureCanKnowRepoExists checks whether the user has access to the repository
// by calling Gitiles with the user's credential.
//
// This is to prevent unauthorized users using this RPC to verify the
// existence of private repositories.
//
// Note that we do not check position_ref here because position_ref does not
// necessarily need to match an actual branch. It can be an arbitrary string
// in the commit message.
//
// Do not check whether the user has access to the commit either because
//  1. commit hashes are not considered to be confidential (see b/352641557);
//     and
//  2. we must check ACL before reading the database otherwise unauthorized
//     users can still verify the existence of a repository by observing the
//     query latency.
func ensureCanKnowRepoExists(ctx context.Context, host, repository string) error {
	client, err := gitilesutil.NewClient(ctx, host, auth.AsCredentialsForwarder)
	if err != nil {
		return errors.Annotate(err, "initialize Gitiles client").Err()
	}
	cacheKey := fmt.Sprintf("user/%q/gitiles/%q/repository/%q", string(auth.CurrentUser(ctx).Identity), host, repository)
	canAccessRepo, err := canKnowRepoExistsCache.GetOrCreate(ctx, cacheKey, func() (v bool, exp time.Duration, err error) {
		_, err = client.GetProject(ctx, &gitilespb.GetProjectRequest{Name: repository})
		if err != nil {
			// Do not cache failed responses. Always return an error.
			return false, 0, err
		}
		return true, canKnowRepoExistsCacheDuration, nil
	})
	if err != nil {
		return appstatus.Attachf(err, grpcutil.Code(err), "cannot access repository https://%s/%s", host, repository)
	}
	if !canAccessRepo {
		// This branch should never been hit.
		return appstatus.Errorf(codes.Internal, "invariant violated: the user must be have access to the repository")
	}
	return nil
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
