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
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	artifactstestutil "go.chromium.org/luci/resultdb/internal/artifacts/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestQueryInvocationVariantArtifactGroups(t *testing.T) {
	ftt.Run(`TestQueryInvocationVariantArtifactGroups`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"googlers"},
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm1", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:testrealm2", Permission: rdbperms.PermListArtifacts},
			},
		})
		// set up most bqclient
		mockBQClient := artifactstestutil.NewMockBQClient(func(ctx context.Context, opts artifacts.ReadArtifactGroupsOpts) (groups []*artifacts.ArtifactGroup, nextPageToken string, err error) {
			return []*artifacts.ArtifactGroup{
				{
					TestID:           "",
					VariantHash:      "variant1",
					Variant:          bigquery.NullJSON{Valid: true, JSONVal: `{"key1": "value1"}`},
					ArtifactID:       "artifact1",
					MatchingCount:    10,
					MaxPartitionTime: time.Unix(10, 0),
					Artifacts: []*artifacts.MatchingArtifact{
						{
							InvocationID:           "invocation1",
							PartitionTime:          time.Unix(10, 0),
							Match:                  "match1",
							MatchWithContextBefore: "before1",
							MatchWithContextAfter:  "after1",
						},
					},
				},
			}, "next_page_token", nil
		}, nil)
		rdbSvr := pb.DecoratedResultDB{
			Service: &resultDBServer{
				artifactBQClient: mockBQClient,
			},
			Postlude: internal.CommonPostlude,
		}
		req := &pb.QueryInvocationVariantArtifactGroupsRequest{
			Project: "testproject",
			SearchString: &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "match[1-9]",
				},
			},
			ArtifactIdMatcher: &pb.IDMatcher{
				Matcher: &pb.IDMatcher_HasPrefix{
					HasPrefix: "artifactidprefix",
				},
			},
			StartTime: timestamppb.New(time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC)),
			EndTime:   timestamppb.New(time.Date(2024, 7, 27, 0, 0, 0, 0, time.UTC)),
			PageSize:  1,
			PageToken: "",
		}
		t.Run("no permission", func(t *ftt.Test) {
			req.Project = "nopermissionproject"
			res, err := rdbSvr.QueryInvocationVariantArtifactGroups(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission resultdb.artifacts.list in any realm in project \"nopermissionproject\""))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("invalid request", func(t *ftt.Test) {

			t.Run("googler", func(t *ftt.Test) {
				req.StartTime = nil
				res, err := rdbSvr.QueryInvocationVariantArtifactGroups(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`start_time: unspecified`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("non-googler", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"other"},
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "testproject:testrealm1", Permission: rdbperms.PermListArtifacts},
						{Realm: "testproject:testrealm2", Permission: rdbperms.PermListArtifacts},
					},
				})

				res, err := rdbSvr.QueryInvocationVariantArtifactGroups(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`artifact_id_matcher: search by prefix is not allowed: insufficient permission to run this query with current filters`))
				assert.Loosely(t, res, should.BeNil)
			})
		})

		t.Run("valid request", func(t *ftt.Test) {
			rsp, err := rdbSvr.QueryInvocationVariantArtifactGroups(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp, should.Resemble(&pb.QueryInvocationVariantArtifactGroupsResponse{
				Groups: []*pb.QueryInvocationVariantArtifactGroupsResponse_MatchGroup{{
					ArtifactId:       "artifact1",
					VariantUnionHash: "variant1",
					VariantUnion:     &pb.Variant{Def: map[string]string{"key1": "value1"}},
					Artifacts: []*pb.ArtifactMatchingContent{{
						Name:          "invocations/invocation1/artifacts/artifact1",
						PartitionTime: timestamppb.New(time.Unix(10, 0)),
						Snippet:       "before1match1after1",
						Matches: []*pb.ArtifactMatchingContent_Match{{
							StartIndex: 7,
							EndIndex:   13,
						}},
					}},
					MatchingCount: 10,
				}},
				NextPageToken: "next_page_token",
			}))
		})
		t.Run("query time out", func(t *ftt.Test) {
			mockBQClient.ReadArtifactGroupsFunc = func(ctx context.Context, opts artifacts.ReadArtifactGroupsOpts) (groups []*artifacts.ArtifactGroup, nextPageToken string, err error) {
				return nil, "", artifacts.BQQueryTimeOutErr
			}
			rsp, err := rdbSvr.QueryInvocationVariantArtifactGroups(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`query can't finish within the deadline`))
			assert.Loosely(t, rsp, should.BeNil)
		})
	})
}

func TestValidateQueryInvocationVariantArtifactGroupsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateQueryInvocationVariantArtifactGroupsRequest`, t, func(t *ftt.Test) {
		req := &pb.QueryInvocationVariantArtifactGroupsRequest{
			Project: "chromium",
			SearchString: &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "foo",
				},
			},
			ArtifactIdMatcher: &pb.IDMatcher{
				Matcher: &pb.IDMatcher_HasPrefix{
					HasPrefix: "artifactidprefix",
				},
			},
			StartTime: timestamppb.New(time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC)),
			EndTime:   timestamppb.New(time.Date(2024, 7, 27, 0, 0, 0, 0, time.UTC)),
			PageSize:  1,
			PageToken: "",
		}
		t.Run(`valid`, func(t *ftt.Test) {
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`no project`, func(t *ftt.Test) {
			req.Project = ""
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`project: unspecified`))
		})

		t.Run(`invalid page size`, func(t *ftt.Test) {
			req.PageSize = -1
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`page_size: negative`))
		})

		t.Run(`no search string`, func(t *ftt.Test) {
			req.SearchString = &pb.ArtifactContentMatcher{}
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`search_string: unspecified`))
		})

		t.Run(`artifact id matcher`, func(t *ftt.Test) {
			t.Run(`invalid artifact id prefix`, func(t *ftt.Test) {
				req.ArtifactIdMatcher = &pb.IDMatcher{
					Matcher: &pb.IDMatcher_HasPrefix{
						HasPrefix: "invalid-artifact-id-\r",
					},
				}
				err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
				assert.Loosely(t, err, should.ErrLike(`artifact_id_matcher`))
			})

			t.Run(`no artifact id matcher called by googler`, func(t *ftt.Test) {
				req.ArtifactIdMatcher = nil
				err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run(`no artifact id matcher called by non-googler`, func(t *ftt.Test) {
				req.ArtifactIdMatcher = nil
				err := validateQueryInvocationVariantArtifactGroupsRequest(req, false)
				assert.Loosely(t, err, should.ErrLike(`artifact_id_matcher: unspecified: insufficient permission to run this query with current filters`))
			})

			t.Run(`artifact id prefix called by non-googler`, func(t *ftt.Test) {
				req.ArtifactIdMatcher = &pb.IDMatcher{
					Matcher: &pb.IDMatcher_HasPrefix{
						HasPrefix: "artifact-prefix",
					},
				}
				err := validateQueryInvocationVariantArtifactGroupsRequest(req, false)
				assert.Loosely(t, err, should.ErrLike(`artifact_id_matcher: search by prefix is not allowed: insufficient permission to run this query with current filters`))
			})
		})

		t.Run(`no start time`, func(t *ftt.Test) {
			req.StartTime = nil
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`start_time: unspecified`))
		})
	})
}
