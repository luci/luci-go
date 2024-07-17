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

func TestQueryTestVariantArtifactGroups(t *testing.T) {
	Convey(`TestQueryTestVariantArtifactGroups`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm1", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:testrealm2", Permission: rdbperms.PermListArtifacts},
			},
		})
		// set up most bqclient
		mockBQClient := artifactstestutil.NewMockBQClient(func(ctx context.Context, opts artifacts.ReadTestArtifactGroupsOpts) (groups []*artifacts.TestArtifactGroup, nextPageToken string, err error) {
			return []*artifacts.TestArtifactGroup{
				{
					TestID:           "test1",
					VariantHash:      "variant1",
					Variant:          bigquery.NullJSON{Valid: true, JSONVal: `{"key1": "value1"}`},
					ArtifactID:       "artifact1",
					MatchingCount:    10,
					MaxPartitionTime: time.Unix(10, 0),
					Artifacts: []*artifacts.MatchingArtifact{
						{
							InvocationID:           "invocation1",
							ResultID:               "result1",
							PartitionTime:          time.Unix(10, 0),
							TestStatus:             bigquery.NullString{Valid: true, StringVal: "PASS"},
							Match:                  "match1",
							MatchWithContextBefore: "before1",
							MatchWithContextAfter:  "after1",
						},
					},
				},
			}, "next_page_token", nil
		})
		rdbSvr := pb.DecoratedResultDB{
			Service: &resultDBServer{
				artifactBQClient: mockBQClient,
			},
			Postlude: internal.CommonPostlude,
		}
		req := &pb.QueryTestVariantArtifactGroupsRequest{
			Project: "testproject",
			SearchString: &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "foo",
				},
			},
			TestIdPrefix:     "testidprefix",
			ArtifactIdPrefix: "artifactidprefix",
			StartTime:        timestamppb.New(time.Unix(10, 0)),
			EndTime:          timestamppb.New(time.Unix(100, 0)),
			PageSize:         1,
			PageToken:        "",
		}
		Convey("no permission", func() {
			req.Project = "nopermissionproject"
			res, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.artifacts.list in any realm in project \"nopermissionproject\"")
			So(res, ShouldBeNil)
		})

		Convey("invalid request", func() {
			req.StartTime = nil
			res, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
			So(err, ShouldBeRPCInvalidArgument, `start_time unspecified`)
			So(res, ShouldBeNil)
		})

		Convey("valid request", func() {
			rsp, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
			So(err, ShouldBeNil)
			So(rsp, ShouldResembleProto, &pb.QueryTestVariantArtifactGroupsResponse{
				Groups: []*pb.QueryTestVariantArtifactGroupsResponse_MatchGroup{{
					TestId:      "test1",
					VariantHash: "variant1",
					Variant:     &pb.Variant{Def: map[string]string{"key1": "value1"}},
					ArtifactId:  "artifact1",
					Artifacts: []*pb.ArtifactMatchingContent{{
						Name:          "invocations/invocation1/tests/test1/results/result1/artifacts/artifact1",
						PartitionTime: timestamppb.New(time.Unix(10, 0)),
						TestStatus:    pb.TestStatus_PASS,
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

func TestValidateQueryTestVariantArtifactGroupsRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateQueryTestVariantArtifactGroupsRequest`, t, func() {
		req := &pb.QueryTestVariantArtifactGroupsRequest{
			Project: "chromium",
			SearchString: &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "foo",
				},
			},
			TestIdPrefix:     "testidprefix",
			ArtifactIdPrefix: "artifactidprefix",
			StartTime:        timestamppb.New(time.Unix(10, 0)),
			EndTime:          timestamppb.New(time.Unix(100, 0)),
			PageSize:         1,
			PageToken:        "",
		}
		Convey(`valid`, func() {
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey(`no project`, func() {
			req.Project = ""
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldErrLike, `project: unspecified`)
		})

		Convey(`invalid page size`, func() {
			req.PageSize = -1
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldErrLike, `page_size: negative`)
		})

		Convey(`no search string`, func() {
			req.SearchString = &pb.ArtifactContentMatcher{}
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldErrLike, `search_string unspecified`)
		})

		Convey(`invalid test id prefix`, func() {
			req.TestIdPrefix = "invalid-test-id-\r"
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldErrLike, `test_id_prefix`)
		})

		Convey(`invalid artifact id prefix`, func() {
			req.ArtifactIdPrefix = "invalid-artifact-id-\r"
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldErrLike, `artifact_id_prefix`)
		})

		Convey(`no start time`, func() {
			req.StartTime = nil
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldErrLike, `start_time unspecified`)
		})

		Convey(`no end time`, func() {
			req.EndTime = nil
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldErrLike, `end_time unspecified`)
		})

		Convey(`start time after end time`, func() {
			req.StartTime = timestamppb.New(time.Unix(100, 0))
			req.EndTime = timestamppb.New(time.Unix(10, 0))
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldErrLike, `start time must not be later than end time`)
		})

		Convey(`time difference greater than 7 days`, func() {
			req.StartTime = timestamppb.New(time.Unix(0, 0))
			req.EndTime = timestamppb.New(time.Unix(7*24*60*60+1, 0))
			err := validateQueryTestVariantArtifactGroupsRequest(req)
			So(err, ShouldErrLike, `difference between start_time and end_time must not be greater than 7 days`)
		})
	})
}

func TestTruncateMatchWithContext(t *testing.T) {
	Convey("truncateMatchWithContext", t, func() {
		Convey("no truncate", func() {
			atf := &artifacts.MatchingArtifact{
				Match:                  "match",
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			match, before, after := truncateMatchWithContext(atf)
			So(match, ShouldEqual, "match")
			So(before, ShouldEqual, "before")
			So(after, ShouldEqual, "after")
		})
		Convey("truncate match", func() {
			atf := &artifacts.MatchingArtifact{
				Match:                  strings.Repeat("a", maxMatchWithContextLength+1),
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			match, before, after := truncateMatchWithContext(atf)
			So(match, ShouldEqual, strings.Repeat("a", maxMatchWithContextLength-3)+"...")
			So(before, ShouldEqual, "")
			So(after, ShouldEqual, "")
		})
		Convey("truncate before and after with no enough remaining bytes", func() {
			atf := &artifacts.MatchingArtifact{
				Match:                  strings.Repeat("a", maxMatchWithContextLength-5),
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			match, before, after := truncateMatchWithContext(atf)
			So(match, ShouldEqual, atf.Match)
			So(before, ShouldEqual, "")
			So(after, ShouldEqual, "")
		})
		Convey("truncate before and after", func() {
			atf := &artifacts.MatchingArtifact{
				Match:                  strings.Repeat("a", maxMatchWithContextLength-9),
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			match, before, after := truncateMatchWithContext(atf)
			So(match, ShouldEqual, atf.Match)
			So(before, ShouldEqual, "...e")
			So(after, ShouldEqual, "a...")
		})
		Convey("doesn't truncate in the middle of a rune", func() {
			// There are 15 remaining bytes to fit in context before and after the match.
			// Each end gets 7 bytes. Each chinese character below is 3 bytes, and the ellipsis takes 3 bytes.
			atf := &artifacts.MatchingArtifact{
				Match:                  strings.Repeat("a", maxMatchWithContextLength-15),
				MatchWithContextBefore: "之前之前之前",
				MatchWithContextAfter:  "之后之后之后",
			}

			match, before, after := truncateMatchWithContext(atf)
			So(match, ShouldEqual, atf.Match)
			// A string of 6 bytes have been returned, when 7 bytes are allowed for each end.
			// Because it doesn't cut from the middle of a chinese character.
			So(before, ShouldEqual, "...前")
			So(after, ShouldEqual, "之...")
		})
	})
}
