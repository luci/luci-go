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
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
		}, nil)
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
					RegexContain: "match[1-9]",
				},
			},
			TestIdMatcher: &pb.IDMatcher{
				Matcher: &pb.IDMatcher_HasPrefix{
					HasPrefix: "testidprefix",
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
		Convey("no permission", func() {
			req.Project = "nopermissionproject"
			res, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.artifacts.list in any realm in project \"nopermissionproject\"")
			So(res, ShouldBeNil)
		})

		Convey("invalid request", func() {

			Convey("googler", func() {
				req.StartTime = nil
				res, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
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

				res, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `test_id_matcher: search by prefix is not allowed: insufficient permission to run this query with current filters`)
				So(res, ShouldBeNil)
			})
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
						Snippet:       "before1match1after1",
						Matches: []*pb.ArtifactMatchingContent_Match{{
							StartIndex: 7,
							EndIndex:   13,
						}},
					}},
					MatchingCount: 10,
				}},
				NextPageToken: "next_page_token",
			})
		})
		Convey("query time out", func() {
			mockBQClient.ReadArtifactGroupsFunc = func(ctx context.Context, opts artifacts.ReadArtifactGroupsOpts) (groups []*artifacts.ArtifactGroup, nextPageToken string, err error) {
				return nil, "", artifacts.BQQueryTimeOutErr
			}
			rsp, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
			So(err, ShouldBeRPCInvalidArgument, `query can't finish within the deadline`)
			So(rsp, ShouldBeNil)
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
			TestIdMatcher: &pb.IDMatcher{
				Matcher: &pb.IDMatcher_HasPrefix{
					HasPrefix: "testidprefix",
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
		Convey(`valid`, func() {
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			So(err, ShouldBeNil)
		})

		Convey(`no project`, func() {
			req.Project = ""
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `project: unspecified`)
		})

		Convey(`invalid page size`, func() {
			req.PageSize = -1
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `page_size: negative`)
		})

		Convey(`no search string`, func() {
			req.SearchString = &pb.ArtifactContentMatcher{}
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `search_string: unspecified`)
		})

		Convey(`test id matcher`, func() {
			Convey(`invalid test id prefix`, func() {
				req.TestIdMatcher = &pb.IDMatcher{
					Matcher: &pb.IDMatcher_HasPrefix{
						HasPrefix: "invalid-test-id-\r",
					},
				}
				err := validateQueryTestVariantArtifactGroupsRequest(req, true)
				So(err, ShouldErrLike, `test_id_matcher`)
			})

			Convey(`no test id matcher called by googler`, func() {
				req.TestIdMatcher = nil
				err := validateQueryTestVariantArtifactGroupsRequest(req, true)
				So(err, ShouldBeNil)
			})

			Convey(`no test id matcher called by non-googler`, func() {
				req.TestIdMatcher = nil
				err := validateQueryTestVariantArtifactGroupsRequest(req, false)
				So(err, ShouldErrLike, `test_id_matcher: unspecified`)
			})

			Convey(`test id prefix called by non-googler`, func() {
				req.TestIdMatcher = &pb.IDMatcher{
					Matcher: &pb.IDMatcher_HasPrefix{
						HasPrefix: "test-id-prefix",
					},
				}
				err := validateQueryTestVariantArtifactGroupsRequest(req, false)
				So(err, ShouldErrLike, `test_id_matcher: search by prefix is not allowed: insufficient permission to run this query with current filters`)
			})
		})

		Convey(`invalid artifact id prefix`, func() {
			req.ArtifactIdMatcher = &pb.IDMatcher{
				Matcher: &pb.IDMatcher_HasPrefix{
					HasPrefix: "invalid-artifact-id-\r",
				},
			}
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `artifact_id_matcher`)
		})

		Convey(`no start time`, func() {
			req.StartTime = nil
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			So(err, ShouldErrLike, `start_time: unspecified`)
		})
	})
}

func TestValidateSearchString(t *testing.T) {
	Convey("validateSearchString", t, func() {
		Convey("unspecified", func() {
			err := validateSearchString(nil)
			So(err, ShouldErrLike, `unspecified`)
		})
		Convey("exceed 2048 bytes", func() {
			m := &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_Contain{
					Contain: strings.Repeat("a", 2049),
				},
			}

			err := validateSearchString(m)
			So(err, ShouldErrLike, `longer than 2048 bytes`)
		})
		Convey("invalid regex", func() {
			m := &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "(.*",
				},
			}

			err := validateSearchString(m)
			So(err, ShouldErrLike, `error parsing regexp`)
		})
	})
}

func TestValidateStartEndTime(t *testing.T) {
	ftt.Run(`validateStartEndTime`, t, func(t *ftt.Test) {
		t.Run("no start time", func(t *ftt.Test) {
			err := validateStartEndTime(nil, nil)
			assert.That(t, err, should.ErrLike(`start_time: unspecified`))
		})
		t.Run("start time before cut off time", func(t *ftt.Test) {
			startTime := timestamppb.New(time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC).Add(-time.Millisecond))

			err := validateStartEndTime(startTime, nil)
			assert.That(t, err, should.ErrLike(`start_time: must be after 2024-07-20 00:00:00 +0000 UTC`))
		})
		t.Run(`no end time`, func(t *ftt.Test) {
			startTime := timestamppb.New(time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC))

			err := validateStartEndTime(startTime, nil)
			assert.That(t, err, should.ErrLike(`end_time: unspecified`))
		})
		t.Run(`start time after end time`, func(t *ftt.Test) {
			startTime := timestamppb.New(time.Date(2024, 7, 22, 0, 0, 0, 0, time.UTC))
			endTime := timestamppb.New(time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC))

			err := validateStartEndTime(startTime, endTime)
			assert.That(t, err, should.ErrLike(`start time must not be later than end time`))
		})
		t.Run(`time difference greater than 7 days`, func(t *ftt.Test) {
			startTime := timestamppb.New(time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC))
			endTime := timestamppb.New(time.Date(2024, 7, 27, 0, 0, 0, 1, time.UTC))

			err := validateStartEndTime(startTime, endTime)
			assert.That(t, err, should.ErrLike(`difference between start_time and end_time must not be greater than 7 days`))
		})

		t.Run(`valid`, func(t *ftt.Test) {
			startTime := timestamppb.New(time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC))
			endTime := timestamppb.New(time.Date(2024, 7, 27, 0, 0, 0, 0, time.UTC))

			err := validateStartEndTime(startTime, endTime)
			assert.That(t, err, should.ErrLike(nil))
		})
	})
}

func TestConstructSnippetAndMatches(t *testing.T) {
	Convey("constructSnippetAndMatches", t, func() {
		containSearchMatcher := func(str string) *pb.ArtifactContentMatcher {
			return &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_Contain{
					Contain: str,
				},
			}
		}
		Convey("no truncate", func() {
			atf := &artifacts.MatchingArtifact{
				Match:                  "match",
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			snippet, matches := constructSnippetAndMatches(atf, containSearchMatcher("match"))
			So(snippet, ShouldEqual, "beforematchafter")
			So(matches, ShouldResembleProto, []*pb.ArtifactMatchingContent_Match{{
				StartIndex: 6, EndIndex: 11,
			}})
		})

		Convey("truncate match", func() {
			match := strings.Repeat("a", maxMatchWithContextLength+1)
			atf := &artifacts.MatchingArtifact{
				Match:                  match,
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			snippet, matches := constructSnippetAndMatches(atf, containSearchMatcher(match))
			So(snippet, ShouldEqual, strings.Repeat("a", maxMatchWithContextLength-3)+"...")
			So(matches, ShouldResembleProto, []*pb.ArtifactMatchingContent_Match{{
				StartIndex: 0, EndIndex: maxMatchWithContextLength,
			}})
		})
		Convey("truncate before and after with no enough remaining bytes", func() {
			match := strings.Repeat("a", maxMatchWithContextLength-5)
			atf := &artifacts.MatchingArtifact{
				Match:                  match,
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			snippet, matches := constructSnippetAndMatches(atf, containSearchMatcher(match))
			So(snippet, ShouldEqual, atf.Match)
			So(matches, ShouldResembleProto, []*pb.ArtifactMatchingContent_Match{{
				StartIndex: 0, EndIndex: int32(len(snippet)),
			}})

		})
		Convey("truncate before and after append ellipsis", func() {
			match := strings.Repeat("a", maxMatchWithContextLength-9)
			atf := &artifacts.MatchingArtifact{
				Match:                  match,
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			snippet, matches := constructSnippetAndMatches(atf, containSearchMatcher(match))
			So(snippet, ShouldEqual, fmt.Sprintf("...e%sa...", atf.Match))
			So(matches, ShouldResembleProto, []*pb.ArtifactMatchingContent_Match{{
				StartIndex: 4, EndIndex: int32(len(snippet) - 4),
			}})
		})
		Convey("doesn't truncate in the middle of a rune", func() {
			match := strings.Repeat("a", maxMatchWithContextLength-15)
			// There are 15 remaining bytes to fit in context before and after the match.
			// Each end gets 7 bytes. Each chinese character below is 3 bytes, and the ellipsis takes 3 bytes.
			atf := &artifacts.MatchingArtifact{
				Match:                  match,
				MatchWithContextBefore: "之前之前之前",
				MatchWithContextAfter:  "之后之后之后",
			}

			snippet, matches := constructSnippetAndMatches(atf, containSearchMatcher(match))
			// A string of 6 bytes have been returned, when 7 bytes are allowed for each end.
			// Because it doesn't cut from the middle of a chinese character.
			So(snippet, ShouldEqual, fmt.Sprintf("...前%s之...", atf.Match))
			So(matches, ShouldResembleProto, []*pb.ArtifactMatchingContent_Match{{
				StartIndex: 6, EndIndex: int32(len(snippet) - 6),
			}})
		})
		Convey("find all remaining matches and no truncation", func() {
			atf := &artifacts.MatchingArtifact{
				Match:                  "a",
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "aaa",
			}
			search := &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "a",
				},
			}

			snippet, matches := constructSnippetAndMatches(atf, search)
			So(snippet, ShouldEqual, "beforeaaaa")
			So(matches, ShouldResembleProto, []*pb.ArtifactMatchingContent_Match{
				{StartIndex: 6, EndIndex: 7},
				{StartIndex: 7, EndIndex: 8},
				{StartIndex: 8, EndIndex: 9},
				{StartIndex: 9, EndIndex: 10},
			})
		})

		Convey("find all remaining matches with truncation", func() {
			atf := &artifacts.MatchingArtifact{
				Match:                  "a",
				MatchWithContextBefore: strings.Repeat("b", maxMatchWithContextLength),
				MatchWithContextAfter:  strings.Repeat("a", maxMatchWithContextLength),
			}
			search := &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "a",
				},
			}
			snippet, matches := constructSnippetAndMatches(atf, search)
			// (10240-1)/2 bytes left for each side.
			So(snippet, ShouldEqual, "..."+strings.Repeat("b", 5116)+"a"+strings.Repeat("a", 5116)+"...")
			expectedMatches := []*pb.ArtifactMatchingContent_Match{}
			for i := 5119; i < 5119+1+5116; i++ {
				expectedMatches = append(expectedMatches, &pb.ArtifactMatchingContent_Match{StartIndex: int32(i), EndIndex: int32(i + 1)})
			}
			So(matches, ShouldResembleProto, expectedMatches)
		})

	})
}
