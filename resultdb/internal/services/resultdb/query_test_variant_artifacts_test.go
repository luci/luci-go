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

	"go.chromium.org/luci/common/errors"
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

func TestQueryTestVariantArtifacts(t *testing.T) {
	Convey("QueryTestVariantArtifacts", t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm1", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:testrealm2", Permission: rdbperms.PermListArtifacts},
			},
		})
		bqClient := artifactstestutil.NewMockBQClient(nil,
			func(ctx context.Context, opts artifacts.ReadTestArtifactsOpts) ([]*artifacts.MatchingArtifact, string, error) {
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
					RegexContain: "f.*oo",
				},
			},
			StartTime: timestamppb.New(time.Date(2024, 5, 5, 0, 0, 0, 0, time.UTC)),
			EndTime:   timestamppb.New(time.Date(2024, 5, 7, 0, 0, 0, 0, time.UTC)),
			PageSize:  10,
		}
		Convey("no permission", func() {
			req.Project = "nopermissionproject"
			res, err := srv.QueryTestVariantArtifacts(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.artifacts.list in any realm in project \"nopermissionproject\"")
			So(res, ShouldBeNil)
		})

		Convey("invalid request", func() {
			req.StartTime = nil
			res, err := srv.QueryTestVariantArtifacts(ctx, req)
			So(err, ShouldBeRPCInvalidArgument, `start_time: unspecified`)
			So(res, ShouldBeNil)
		})

		Convey("Valid request", func() {
			rsp, err := srv.QueryTestVariantArtifacts(ctx, req)
			So(err, ShouldBeNil)
			So(rsp, ShouldResembleProto, &pb.QueryTestVariantArtifactsResponse{
				Artifacts: []*pb.ArtifactMatchingContent{{
					Name:          "invocations/12345678901234567890/tests/test_id/results/1/artifacts/artifact_id",
					PartitionTime: timestamppb.New(time.Date(2024, 5, 6, 5, 58, 57, 490076000, time.UTC)),
					TestStatus:    pb.TestStatus_PASS,
					Match:         "log line 1",
					BeforeMatch:   "log line 0",
					AfterMatch:    "log line 2",
				},
					{
						Name:          "invocations/12345678901234567890/tests/test_id/results/2/artifacts/artifact_id",
						PartitionTime: timestamppb.New(time.Date(2024, 5, 6, 5, 58, 57, 491037000, time.UTC)),
						TestStatus:    pb.TestStatus_STATUS_UNSPECIFIED,
						Match:         "log line 3",
						BeforeMatch:   "log line 2",
						AfterMatch:    "log line 4",
					}},
				NextPageToken: "",
			})
		})

		Convey("BQClient returns error", func() {
			bqClient := artifactstestutil.NewMockBQClient(nil,
				func(ctx context.Context, opts artifacts.ReadTestArtifactsOpts) ([]*artifacts.MatchingArtifact, string, error) {
					return nil, "", errors.New("BQClient error")
				},
			)
			srv := &resultDBServer{
				artifactBQClient: bqClient,
			}
			_, err := srv.QueryTestVariantArtifacts(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "read test artifacts")
		})
	})
}

func TestValidateQueryTestVariantArtifactsRequest(t *testing.T) {
	Convey("ValidateQueryTestVariantArtifactsRequest", t, func() {
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
			StartTime: timestamppb.New(time.Date(2024, 5, 5, 0, 0, 0, 0, time.UTC)),
			EndTime:   timestamppb.New(time.Date(2024, 5, 7, 0, 0, 0, 0, time.UTC)),
			PageSize:  10,
		}
		Convey("Valid request", func() {
			err := validateQueryTestVariantArtifactsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("Invalid request", func() {
			Convey("Invalid project", func() {
				req.Project = "invalid_project"
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "project")
			})

			Convey("Search string unspecified", func() {
				req.SearchString = &pb.ArtifactContentMatcher{}
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "search_string: unspecified")
			})

			Convey("Invalid test_id", func() {
				req.TestId = "invalid-test-id-\r"
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "test_id")
			})

			Convey("Invalid variant hash", func() {
				req.VariantHash = "varianthash"
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "variant_hash")
			})

			Convey("Invalid artifact_id", func() {
				req.ArtifactId = "invalid-artifact-id-\r"
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "artifact_id")
			})

			Convey("Start time unspecified", func() {
				req.StartTime = nil
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "start_time: unspecified")
			})

			Convey("End time unspecified", func() {
				req.EndTime = nil
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "end_time: unspecified")
			})

			Convey("Start time after end time", func() {
				req.StartTime = timestamppb.New(time.Date(2024, 5, 7, 0, 0, 0, 0, time.UTC))
				req.EndTime = timestamppb.New(time.Date(2024, 5, 5, 0, 0, 0, 0, time.UTC))
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "start time must not be later than end time")
			})

			Convey("Time difference greater than 7 days", func() {
				req.StartTime = timestamppb.New(time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC))
				req.EndTime = timestamppb.New(time.Date(2024, 5, 9, 0, 0, 0, 0, time.UTC))
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "difference between start_time and end_time must not be greater than 7 days")
			})

			Convey("Invalid page size", func() {
				req.PageSize = -1
				err := validateQueryTestVariantArtifactsRequest(req)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "page_size: negative")
			})
		})
	})
}
