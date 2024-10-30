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
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

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

func TestQueryTestVariantArtifactGroups(t *testing.T) {
	ftt.Run(`TestQueryTestVariantArtifactGroups`, t, func(t *ftt.Test) {
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
		t.Run("no permission", func(t *ftt.Test) {
			req.Project = "nopermissionproject"
			res, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.artifacts.list in any realm in project \"nopermissionproject\""))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("invalid request", func(t *ftt.Test) {

			t.Run("googler", func(t *ftt.Test) {
				req.StartTime = nil
				res, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`start_time: unspecified`))
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

				res, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`test_id_matcher: search by prefix is not allowed: insufficient permission to run this query with current filters`))
				assert.Loosely(t, res, should.BeNil)
			})
		})

		t.Run("valid request", func(t *ftt.Test) {
			rsp, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp, should.Resemble(&pb.QueryTestVariantArtifactGroupsResponse{
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
			}))
		})
		t.Run("query time out", func(t *ftt.Test) {
			mockBQClient.ReadArtifactGroupsFunc = func(ctx context.Context, opts artifacts.ReadArtifactGroupsOpts) (groups []*artifacts.ArtifactGroup, nextPageToken string, err error) {
				return nil, "", artifacts.BQQueryTimeOutErr
			}
			rsp, err := rdbSvr.QueryTestVariantArtifactGroups(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`query can't finish within the deadline`))
			assert.Loosely(t, rsp, should.BeNil)
		})
	})
}

func TestValidateQueryTestVariantArtifactGroupsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateQueryTestVariantArtifactGroupsRequest`, t, func(t *ftt.Test) {
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
		t.Run(`valid`, func(t *ftt.Test) {
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`no project`, func(t *ftt.Test) {
			req.Project = ""
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`project: unspecified`))
		})

		t.Run(`invalid page size`, func(t *ftt.Test) {
			req.PageSize = -1
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`page_size: negative`))
		})

		t.Run(`no search string`, func(t *ftt.Test) {
			req.SearchString = &pb.ArtifactContentMatcher{}
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`search_string: unspecified`))
		})

		t.Run(`test id matcher`, func(t *ftt.Test) {
			t.Run(`invalid test id prefix`, func(t *ftt.Test) {
				req.TestIdMatcher = &pb.IDMatcher{
					Matcher: &pb.IDMatcher_HasPrefix{
						HasPrefix: "invalid-test-id-\r",
					},
				}
				err := validateQueryTestVariantArtifactGroupsRequest(req, true)
				assert.Loosely(t, err, should.ErrLike(`test_id_matcher`))
			})

			t.Run(`no test id matcher called by googler`, func(t *ftt.Test) {
				req.TestIdMatcher = nil
				err := validateQueryTestVariantArtifactGroupsRequest(req, true)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run(`no test id matcher called by non-googler`, func(t *ftt.Test) {
				req.TestIdMatcher = nil
				err := validateQueryTestVariantArtifactGroupsRequest(req, false)
				assert.Loosely(t, err, should.ErrLike(`test_id_matcher: unspecified`))
			})

			t.Run(`test id prefix called by non-googler`, func(t *ftt.Test) {
				req.TestIdMatcher = &pb.IDMatcher{
					Matcher: &pb.IDMatcher_HasPrefix{
						HasPrefix: "test-id-prefix",
					},
				}
				err := validateQueryTestVariantArtifactGroupsRequest(req, false)
				assert.Loosely(t, err, should.ErrLike(`test_id_matcher: search by prefix is not allowed: insufficient permission to run this query with current filters`))
			})
		})

		t.Run(`invalid artifact id prefix`, func(t *ftt.Test) {
			req.ArtifactIdMatcher = &pb.IDMatcher{
				Matcher: &pb.IDMatcher_HasPrefix{
					HasPrefix: "invalid-artifact-id-\r",
				},
			}
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`artifact_id_matcher`))
		})

		t.Run(`no start time`, func(t *ftt.Test) {
			req.StartTime = nil
			err := validateQueryTestVariantArtifactGroupsRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`start_time: unspecified`))
		})
	})
}

func TestValidateSearchString(t *testing.T) {
	ftt.Run("validateSearchString", t, func(t *ftt.Test) {
		t.Run("unspecified", func(t *ftt.Test) {
			err := validateSearchString(nil)
			assert.Loosely(t, err, should.ErrLike(`unspecified`))
		})
		t.Run("exceed 2048 bytes", func(t *ftt.Test) {
			m := &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_Contain{
					Contain: strings.Repeat("a", 2049),
				},
			}

			err := validateSearchString(m)
			assert.Loosely(t, err, should.ErrLike(`longer than 2048 bytes`))
		})
		t.Run("invalid regex", func(t *ftt.Test) {
			m := &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "(.*",
				},
			}

			err := validateSearchString(m)
			assert.Loosely(t, err, should.ErrLike(`error parsing regexp`))
		})
		t.Run("capture group is not allowed", func(t *ftt.Test) {
			m := &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "xx([0-9]*)",
				},
			}

			err := validateSearchString(m)
			assert.Loosely(t, err, should.ErrLike(`capture group is not allowed`))
		})
		t.Run("non-capture group is allowed", func(t *ftt.Test) {
			m := &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_RegexContain{
					RegexContain: "xx(?:[0-9]*)",
				},
			}

			err := validateSearchString(m)
			assert.Loosely(t, err, should.BeNil)
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
	ftt.Run("constructSnippetAndMatches", t, func(t *ftt.Test) {
		containSearchMatcher := func(str string) *pb.ArtifactContentMatcher {
			return &pb.ArtifactContentMatcher{
				Matcher: &pb.ArtifactContentMatcher_Contain{
					Contain: str,
				},
			}
		}
		t.Run("no truncate", func(t *ftt.Test) {
			atf := &artifacts.MatchingArtifact{
				Match:                  "match",
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			snippet, matches := constructSnippetAndMatches(atf, containSearchMatcher("match"))
			assert.Loosely(t, snippet, should.Equal("beforematchafter"))
			assert.Loosely(t, matches, should.Resemble([]*pb.ArtifactMatchingContent_Match{{
				StartIndex: 6, EndIndex: 11,
			}}))
		})

		t.Run("truncate match", func(t *ftt.Test) {
			match := strings.Repeat("a", maxMatchWithContextLength+1)
			atf := &artifacts.MatchingArtifact{
				Match:                  match,
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			snippet, matches := constructSnippetAndMatches(atf, containSearchMatcher(match))
			assert.Loosely(t, snippet, should.Equal(strings.Repeat("a", maxMatchWithContextLength-3)+"..."))
			assert.Loosely(t, matches, should.Resemble([]*pb.ArtifactMatchingContent_Match{{
				StartIndex: 0, EndIndex: maxMatchWithContextLength,
			}}))
		})
		t.Run("truncate before and after with no enough remaining bytes", func(t *ftt.Test) {
			match := strings.Repeat("a", maxMatchWithContextLength-5)
			atf := &artifacts.MatchingArtifact{
				Match:                  match,
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			snippet, matches := constructSnippetAndMatches(atf, containSearchMatcher(match))
			assert.Loosely(t, snippet, should.Equal(atf.Match))
			assert.Loosely(t, matches, should.Resemble([]*pb.ArtifactMatchingContent_Match{{
				StartIndex: 0, EndIndex: int32(len(snippet)),
			}}))

		})
		t.Run("truncate before and after append ellipsis", func(t *ftt.Test) {
			match := strings.Repeat("a", maxMatchWithContextLength-9)
			atf := &artifacts.MatchingArtifact{
				Match:                  match,
				MatchWithContextBefore: "before",
				MatchWithContextAfter:  "after",
			}

			snippet, matches := constructSnippetAndMatches(atf, containSearchMatcher(match))
			assert.Loosely(t, snippet, should.Equal(fmt.Sprintf("...e%sa...", atf.Match)))
			assert.Loosely(t, matches, should.Resemble([]*pb.ArtifactMatchingContent_Match{{
				StartIndex: 4, EndIndex: int32(len(snippet) - 4),
			}}))
		})
		t.Run("doesn't truncate in the middle of a rune", func(t *ftt.Test) {
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
			assert.Loosely(t, snippet, should.Equal(fmt.Sprintf("...前%s之...", atf.Match)))
			assert.Loosely(t, matches, should.Resemble([]*pb.ArtifactMatchingContent_Match{{
				StartIndex: 6, EndIndex: int32(len(snippet) - 6),
			}}))
		})
		t.Run("find all remaining matches and no truncation", func(t *ftt.Test) {
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
			assert.Loosely(t, snippet, should.Equal("beforeaaaa"))
			assert.Loosely(t, matches, should.Resemble([]*pb.ArtifactMatchingContent_Match{
				{StartIndex: 6, EndIndex: 7},
				{StartIndex: 7, EndIndex: 8},
				{StartIndex: 8, EndIndex: 9},
				{StartIndex: 9, EndIndex: 10},
			}))
		})

		t.Run("find all remaining matches with truncation", func(t *ftt.Test) {
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
			assert.Loosely(t, snippet, should.Equal("..."+strings.Repeat("b", 5116)+"a"+strings.Repeat("a", 5116)+"..."))
			expectedMatches := []*pb.ArtifactMatchingContent_Match{}
			for i := 5119; i < 5119+1+5116; i++ {
				expectedMatches = append(expectedMatches, &pb.ArtifactMatchingContent_Match{StartIndex: int32(i), EndIndex: int32(i + 1)})
			}
			assert.Loosely(t, matches, should.Resemble(expectedMatches))
		})

	})
}
