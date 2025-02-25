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

	"cloud.google.com/go/bigquery"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	artifactstestutil "go.chromium.org/luci/resultdb/internal/artifacts/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestQueryTestVariantArtifacts(t *testing.T) {
	ftt.Run("QueryTestVariantArtifacts", t, func(t *ftt.Test) {
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
						ResultID:               "1",
						PartitionTime:          time.Date(2024, 5, 6, 5, 58, 57, 490076000, time.UTC),
						TestStatus:             bigquery.NullString{StringVal: "PASS", Valid: true},
						Match:                  "log line 1",
						MatchWithContextBefore: "log line 0",
						MatchWithContextAfter:  "log line 2",
					},
					{
						InvocationID:           "12345678901234567890",
						ResultID:               "2",
						PartitionTime:          time.Date(2024, 5, 6, 5, 58, 57, 491037000, time.UTC),
						TestStatus:             bigquery.NullString{Valid: false},
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
		req := &pb.QueryTestVariantArtifactsRequest{
			Project:     "testproject",
			TestId:      "test_id",
			ArtifactId:  "artifact_id",
			VariantHash: strings.Repeat("a", 16),
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
			res, err := srv.QueryTestVariantArtifacts(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.artifacts.list in any realm in project \"nopermissionproject\""))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("invalid request", func(t *ftt.Test) {
			req.StartTime = nil
			res, err := srv.QueryTestVariantArtifacts(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`start_time: unspecified`))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("Valid request", func(t *ftt.Test) {
			rsp, err := srv.QueryTestVariantArtifacts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp, should.Match(&pb.QueryTestVariantArtifactsResponse{
				Artifacts: []*pb.ArtifactMatchingContent{{
					Name:          "invocations/12345678901234567890/tests/test_id/results/1/artifacts/artifact_id",
					PartitionTime: timestamppb.New(time.Date(2024, 5, 6, 5, 58, 57, 490076000, time.UTC)),
					TestStatus:    pb.TestStatus_PASS,
					Snippet:       "log line 0log line 1log line 2",
					Matches: []*pb.ArtifactMatchingContent_Match{{
						StartIndex: 10,
						EndIndex:   20,
					}},
				},
					{
						Name:          "invocations/12345678901234567890/tests/test_id/results/2/artifacts/artifact_id",
						PartitionTime: timestamppb.New(time.Date(2024, 5, 6, 5, 58, 57, 491037000, time.UTC)),
						TestStatus:    pb.TestStatus_STATUS_UNSPECIFIED,
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
			_, err := srv.QueryTestVariantArtifacts(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("read test artifacts"))
		})
	})
}

func TestValidateQueryTestVariantArtifactsRequest(t *testing.T) {
	ftt.Run("ValidateQueryTestVariantArtifactsRequest", t, func(t *ftt.Test) {
		req := &pb.QueryTestVariantArtifactsRequest{
			Project:     "testproject",
			TestId:      "test_id",
			VariantHash: strings.Repeat("a", 16),
			ArtifactId:  "artifact_id",
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
			err := validateQueryTestVariantArtifactsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Invalid request", func(t *ftt.Test) {
			t.Run("Invalid project", func(t *ftt.Test) {
				req.Project = "invalid_project"
				err := validateQueryTestVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("project"))
			})

			t.Run("Search string unspecified", func(t *ftt.Test) {
				req.SearchString = &pb.ArtifactContentMatcher{}
				err := validateQueryTestVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("search_string: unspecified"))
			})

			t.Run("Invalid test_id", func(t *ftt.Test) {
				req.TestId = "invalid-test-id-\r"
				err := validateQueryTestVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("test_id"))
			})

			t.Run("Invalid variant hash", func(t *ftt.Test) {
				req.VariantHash = "varianthash"
				err := validateQueryTestVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("variant_hash"))
			})

			t.Run("Invalid artifact_id", func(t *ftt.Test) {
				req.ArtifactId = "invalid-artifact-id-\r"
				err := validateQueryTestVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("artifact_id"))
			})

			t.Run("Start time unspecified", func(t *ftt.Test) {
				req.StartTime = nil
				err := validateQueryTestVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("start_time: unspecified"))
			})

			t.Run("Invalid page size", func(t *ftt.Test) {
				req.PageSize = -1
				err := validateQueryTestVariantArtifactsRequest(req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("page_size: negative"))
			})
		})
	})
}
