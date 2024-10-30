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

package resultdb

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	artifactstestutil "go.chromium.org/luci/resultdb/internal/artifacts/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestQueryInvocationVariantArtifacts(t *testing.T) {
	ftt.Run("QueryInvocationVariantArtifacts", t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm1", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:testrealm2", Permission: rdbperms.PermListArtifacts},
			},
		})
		bqClient := artifactstestutil.NewMockBQClient(nil,
			func(ctx context.Context, opts artifacts.ReadArtifactsOpts) ([]*artifacts.MatchingArtifact, string, error) {
				return []*artifacts.MatchingArtifact{
					{
						InvocationID:           "12345678901234567890",
						PartitionTime:          time.Date(2024, 8, 6, 5, 58, 57, 490076000, time.UTC),
						Match:                  "log line 1",
						MatchWithContextBefore: "log line 0",
						MatchWithContextAfter:  "log line 2",
					},
					{
						InvocationID:           "12345678901234567891",
						PartitionTime:          time.Date(2024, 8, 6, 5, 58, 57, 491037000, time.UTC),
						Match:                  "log line 3",
						MatchWithContextBefore: "log line 2",
						MatchWithContextAfter:  "log line 4",
					},
				}, "", nil
			},
		)
		srv := pb.DecoratedResultDB{
			Service: &resultDBServer{
				artifactBQClient: bqClient,
			},
			Postlude: internal.CommonPostlude,
		}
		req := &pb.QueryInvocationVariantArtifactsRequest{
			Project:          "testproject",
			ArtifactId:       "artifact_id",
			VariantUnionHash: strings.Repeat("a", 16),
			SearchString: &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "log line [13]",
				},
			},
			StartTime: timestamppb.New(time.Date(2024, 8, 5, 0, 0, 0, 0, time.UTC)),
			EndTime:   timestamppb.New(time.Date(2024, 8, 7, 0, 0, 0, 0, time.UTC)),
			PageSize:  10,
		}
		t.Run("no permission", func(t *ftt.Test) {
			req.Project = "nopermissionproject"
			res, err := srv.QueryInvocationVariantArtifacts(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.artifacts.list in any realm in project \"nopermissionproject\""))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("invalid request", func(t *ftt.Test) {
			req.StartTime = nil
			res, err := srv.QueryInvocationVariantArtifacts(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`start_time: unspecified`))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("Valid request", func(t *ftt.Test) {
			rsp, err := srv.QueryInvocationVariantArtifacts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp, should.Resemble(&pb.QueryInvocationVariantArtifactsResponse{
				Artifacts: []*pb.ArtifactMatchingContent{{
					Name:          "invocations/12345678901234567890/artifacts/artifact_id",
					PartitionTime: timestamppb.New(time.Date(2024, 8, 6, 5, 58, 57, 490076000, time.UTC)),
					Snippet:       "log line 0log line 1log line 2",
					Matches: []*pb.ArtifactMatchingContent_Match{{
						StartIndex: 10,
						EndIndex:   20,
					}},
				},
					{
						Name:          "invocations/12345678901234567891/artifacts/artifact_id",
						PartitionTime: timestamppb.New(time.Date(2024, 8, 6, 5, 58, 57, 491037000, time.UTC)),
						Snippet:       "log line 2log line 3log line 4",
						Matches: []*pb.ArtifactMatchingContent_Match{{
							StartIndex: 10,
							EndIndex:   20,
						}},
					}},
				NextPageToken: "",
			}))
		})

		t.Run("BQClient returns error", func(t *ftt.Test) {
			bqClient := artifactstestutil.NewMockBQClient(nil,
				func(ctx context.Context, opts artifacts.ReadArtifactsOpts) ([]*artifacts.MatchingArtifact, string, error) {
					return nil, "", errors.New("BQClient error")
				},
			)
			srv := &resultDBServer{
				artifactBQClient: bqClient,
			}
			_, err := srv.QueryInvocationVariantArtifacts(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("read artifacts"))
		})
	})
}

func TestValidateQueryInvocationVariantArtifactsRequest(t *testing.T) {
	ftt.Run("ValidateQueryInvocationVariantArtifactsRequest", t, func(t *ftt.Test) {
		req := &pb.QueryInvocationVariantArtifactsRequest{
			Project:          "testproject",
			VariantUnionHash: strings.Repeat("a", 16),
			ArtifactId:       "artifact_id",
			SearchString: &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "foo",
				},
			},
			StartTime: timestamppb.New(time.Date(2024, 8, 5, 0, 0, 0, 0, time.UTC)),
			EndTime:   timestamppb.New(time.Date(2024, 8, 7, 0, 0, 0, 0, time.UTC)),
			PageSize:  10,
		}
		t.Run("Valid request", func(t *ftt.Test) {
			err := validateQueryInvocationVariantArtifactsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Invalid request", func(t *ftt.Test) {
			t.Run("Invalid project", func(t *ftt.Test) {
				req.Project = "invalid_project"
				err := validateQueryInvocationVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("project"))
			})

			t.Run("Search string unspecified", func(t *ftt.Test) {
				req.SearchString = &pb.ArtifactContentMatcher{}
				err := validateQueryInvocationVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("search_string: unspecified"))
			})

			t.Run("Invalid variant union hash", func(t *ftt.Test) {
				req.VariantUnionHash = "varianthash"
				err := validateQueryInvocationVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("variant_union_hash"))
			})

			t.Run("Invalid artifact_id", func(t *ftt.Test) {
				req.ArtifactId = "invalid-artifact-id-\r"
				err := validateQueryInvocationVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("artifact_id"))
			})

			t.Run("Start time unspecified", func(t *ftt.Test) {
				req.StartTime = nil
				err := validateQueryInvocationVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("start_time: unspecified"))
			})

			t.Run("Invalid page size", func(t *ftt.Test) {
				req.PageSize = -1
				err := validateQueryInvocationVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("page_size: negative"))
			})
		})
	})
}
