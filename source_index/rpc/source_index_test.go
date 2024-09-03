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
	"math/rand"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/source_index/internal/commit"
	"go.chromium.org/luci/source_index/internal/config"
	"go.chromium.org/luci/source_index/internal/gitilesutil"
	"go.chromium.org/luci/source_index/internal/testutil"
	pb "go.chromium.org/luci/source_index/proto/v1"
)

func TestSourceIndexServer(t *testing.T) {
	ftt.Run(`TestSourceIndexServer`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = memory.Use(ctx)
		err := config.SetTestConfig(ctx, config.TestCfg)
		assert.Loosely(t, err, should.BeNil)

		rng := rand.New(rand.NewSource(7671861294544766002))
		publicCommits := gitilespb.MakeFakeCommits(rng, 5, nil)
		publicCommits[0].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#1}\n"
		publicCommits[1].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#2}\n"
		publicCommits[2].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#3}\n"
		publicCommits[3].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#4}\n"
		publicCommits[4].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#5}\n"

		unindexedCommits := gitilespb.MakeFakeCommits(rng, 5, nil)
		unindexedCommits[0].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#1}\n"
		unindexedCommits[1].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#2}\n"
		unindexedCommits[2].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#3}\n"
		unindexedCommits[3].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#4}\n"
		unindexedCommits[4].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#5}\n"

		confidentialCommits := gitilespb.MakeFakeCommits(rng, 5, nil)
		confidentialCommits[0].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#1}\n"
		confidentialCommits[1].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#2}\n"
		confidentialCommits[2].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#3}\n"
		confidentialCommits[3].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#4}\n"
		confidentialCommits[4].Message = "title\n\nCr-Commit-Position: refs/heads/main@{#5}\n"

		host := "chromium.googlesource.com"
		publicRepository := "chromium/src"
		unindexedRepository := "chromium/unindexed"
		confidentialRepository := "chromium/confidential"

		fakeGitilesClient := &gitilespb.Fake{}
		// Confidential commits are not returned from Gitiles.
		fakeGitilesClient.SetRepository(publicRepository, nil, publicCommits)
		fakeGitilesClient.SetRepository(unindexedRepository, nil, unindexedCommits)
		ctx = gitilesutil.UseFakeClientFactory(ctx, func(
			ctx context.Context,
			requestedHost string,
			kind auth.RPCAuthorityKind,
			opts ...auth.RPCOption,
		) (gitilespb.GitilesClient, error) {
			assert.That(t, requestedHost, should.Equal(host))
			assert.That(t, kind, should.Equal(auth.AsCredentialsForwarder))
			return fakeGitilesClient, nil
		})

		// Unindexed commits are not indexed.
		indexedCommits := make([]commit.Commit, 0, len(publicCommits)+len(confidentialCommits))
		for _, c := range publicCommits {
			gc, err := commit.NewGitCommit(host, publicRepository, c)
			assert.Loosely(t, err, should.BeNil)
			indexedCommits = append(indexedCommits, commit.NewFromGitCommit(gc))
		}
		for _, c := range confidentialCommits {
			gc, err := commit.NewGitCommit(host, confidentialRepository, c)
			assert.Loosely(t, err, should.BeNil)
			indexedCommits = append(indexedCommits, commit.NewFromGitCommit(gc))
		}
		commit.MustSetForTesting(ctx, indexedCommits...)

		err = config.SetTestConfig(ctx, config.TestCfg)
		assert.Loosely(t, err, should.BeNil)

		s := &pb.DecoratedSourceIndex{
			Service:  &sourceIndexServer{},
			Postlude: gRPCifyAndLogPostlude,
		}

		queryRepository := publicRepository
		var queryPosition int64 = 2

		t.Run(`QueryCommitHash`, func(t *ftt.Test) {
			t.Run(`with a matching commit`, func(t *ftt.Test) {
				res, err := s.QueryCommitHash(ctx, &pb.QueryCommitHashRequest{
					Host:           host,
					Repository:     queryRepository,
					PositionRef:    "refs/heads/main",
					PositionNumber: queryPosition,
				})

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res.GetHash(), should.Equal(strings.ToLower(publicCommits[1].Id)))
			})

			t.Run(`with invalid argument`, func(t *ftt.Test) {
				queryRepository = "not-a-repo/"

				res, err := s.QueryCommitHash(ctx, &pb.QueryCommitHashRequest{
					Host:           host,
					Repository:     queryRepository,
					PositionRef:    "refs/heads/main",
					PositionNumber: queryPosition,
				})

				assert.That(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLikeString("invalid QueryCommitHashRequest"))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run(`with unauthorized repository`, func(t *ftt.Test) {
				queryRepository = confidentialRepository

				res, err := s.QueryCommitHash(ctx, &pb.QueryCommitHashRequest{
					Host:           host,
					Repository:     queryRepository,
					PositionRef:    "refs/heads/main",
					PositionNumber: queryPosition,
				})

				assert.That(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.That(t, err, should.ErrLikeString("cannot access repository"))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run(`repository not indexed`, func(t *ftt.Test) {
				queryRepository = unindexedRepository

				res, err := s.QueryCommitHash(ctx, &pb.QueryCommitHashRequest{
					Host:           host,
					Repository:     queryRepository,
					PositionRef:    "refs/heads/main",
					PositionNumber: queryPosition,
				})

				assert.That(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
				assert.That(t, err, should.ErrLikeString("is not configured to be indexed"))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run(`no matching commit`, func(t *ftt.Test) {
				queryPosition = 12

				res, err := s.QueryCommitHash(ctx, &pb.QueryCommitHashRequest{
					Host:           host,
					Repository:     queryRepository,
					PositionRef:    "refs/heads/main",
					PositionNumber: queryPosition,
				})

				assert.That(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.That(t, err, should.ErrLikeString("commit not found"))
				assert.Loosely(t, res, should.BeNil)
			})
		})
	})
}
