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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueryInvocationVariantArtifactGroups(t *testing.T) {
	Convey(`TestQueryInvocationVariantArtifactGroups`, t, func() {
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
					RegexContain: "foo",
				},
			},
			ArtifactIdMatcher: &pb.IDMatcher{
				Matcher: &pb.IDMatcher_HasPrefix{
					HasPrefix: "artifactidprefix",
				},
			},
			StartTime: timestamppb.New(time.Unix(10, 0)),
			EndTime:   timestamppb.New(time.Unix(100, 0)),
			PageSize:  1,
			PageToken: "",
		}
		Convey("no permission", func() {
			req.Project = "nopermissionproject"
			res, err := rdbSvr.QueryInvocationVariantArtifactGroups(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.artifacts.list in any realm in project \"nopermissionproject\"")
			So(res, ShouldBeNil)
		})

		Convey("invalid request", func() {

			Convey("googler", func() {
				req.StartTime = nil
				res, err := rdbSvr.QueryInvocationVariantArtifactGroups(ctx, req)
				So(err, ShouldBeRPCInvalidArgument, `start_time: unspecified`)
				So(res, ShouldBeNil)
			})

			Convey("non-googler", func() {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"other"},
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "testproject:testrealm1", Permission: rdbperms.PermListArtifacts},
						{Realm: "testproject:testrealm2", Permission: rdbperms.PermListArtifacts},
					},
				})

				res, err := rdbSvr.QueryInvocationVariantArtifactGroups(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `artifact_id_matcher: search by prefix is not allowed: insufficient permission to run this query with current filters`)
				So(res, ShouldBeNil)
			})
		})

		Convey("valid request", func() {
			rsp, err := rdbSvr.QueryInvocationVariantArtifactGroups(ctx, req)
			So(err, ShouldBeNil)
			So(rsp, ShouldResembleProto, &pb.QueryInvocationVariantArtifactGroupsResponse{
				Groups: []*pb.QueryInvocationVariantArtifactGroupsResponse_MatchGroup{{
					ArtifactId:       "artifact1",
					VariantUnionHash: "variant1",
					VariantUnion:     &pb.Variant{Def: map[string]string{"key1": "value1"}},
					Artifacts: []*pb.ArtifactMatchingContent{{
						Name:          "invocations/invocation1/artifacts/artifact1",
						PartitionTime: timestamppb.New(time.Unix(10, 0)),
						Match:         "match1",
						BeforeMatch:   "before1",
						AfterMatch:    "after1",
					}},
					MatchingCount: 10,
				}},
				NextPageToken: "next_page_token",
			})
		})

	})
}

func TestValidateQueryInvocationVariantArtifactGroupsRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateQueryInvocationVariantArtifactGroupsRequest`, t, func() {
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
			StartTime: timestamppb.New(time.Unix(10, 0)),
			EndTime:   timestamppb.New(time.Unix(100, 0)),
			PageSize:  1,
			PageToken: "",
		}
		Convey(`valid`, func() {
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			So(err, ShouldBeNil)
		})

		Convey(`no project`, func() {
			req.Project = ""
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `project: unspecified`)
		})

		Convey(`invalid page size`, func() {
			req.PageSize = -1
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `page_size: negative`)
		})

		Convey(`no search string`, func() {
			req.SearchString = &pb.ArtifactContentMatcher{}
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `search_string: unspecified`)
		})

		Convey(`artifact id matcher`, func() {
			Convey(`invalid artifact id prefix`, func() {
				req.ArtifactIdMatcher = &pb.IDMatcher{
					Matcher: &pb.IDMatcher_HasPrefix{
						HasPrefix: "invalid-artifact-id-\r",
					},
				}
				err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
				So(err, ShouldErrLike, `artifact_id_matcher`)
			})

			Convey(`no artifact id matcher called by googler`, func() {
				req.ArtifactIdMatcher = nil
				err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
				So(err, ShouldBeNil)
			})

			Convey(`no artifact id matcher called by non-googler`, func() {
				req.ArtifactIdMatcher = nil
				err := validateQueryInvocationVariantArtifactGroupsRequest(req, false)
				So(err, ShouldErrLike, `artifact_id_matcher: unspecified: insufficient permission to run this query with current filters`)
			})

			Convey(`artifact id prefix called by non-googler`, func() {
				req.ArtifactIdMatcher = &pb.IDMatcher{
					Matcher: &pb.IDMatcher_HasPrefix{
						HasPrefix: "artifact-prefix",
					},
				}
				err := validateQueryInvocationVariantArtifactGroupsRequest(req, false)
				So(err, ShouldErrLike, `artifact_id_matcher: search by prefix is not allowed: insufficient permission to run this query with current filters`)
			})
		})

		Convey(`no start time`, func() {
			req.StartTime = nil
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `start_time: unspecified`)
		})

		Convey(`no end time`, func() {
			req.EndTime = nil
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `end_time: unspecified`)
		})

		Convey(`start time after end time`, func() {
			req.StartTime = timestamppb.New(time.Unix(100, 0))
			req.EndTime = timestamppb.New(time.Unix(10, 0))
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `start time must not be later than end time`)
		})

		Convey(`time difference greater than 7 days`, func() {
			req.StartTime = timestamppb.New(time.Unix(0, 0))
			req.EndTime = timestamppb.New(time.Unix(7*24*60*60+1, 0))
			err := validateQueryInvocationVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `difference between start_time and end_time must not be greater than 7 days`)
		})
	})
}
