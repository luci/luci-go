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
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/changepoints"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestChangepointsServer(t *testing.T) {
	Convey("TestChangepointsServer", t, func() {
		ctx := context.Background()
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-analysis-access"},
		})
		client := fakeChangepointClient{}
		server := NewChangepointsServer(&client)
		Convey("QueryChangepointGroupSummaries", func() {
			Convey("unauthorised requests are rejected", func() {
				req := &pb.QueryChangepointGroupSummariesRequest{
					Project: "chromium",
				}

				res, err := server.QueryChangepointGroupSummaries(ctx, req)
				So(err, ShouldErrLike, `not a member of googlers`)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
				So(res, ShouldBeNil)
			})
			Convey("invalid requests are rejected", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@google.com",
					IdentityGroups: []string{"googlers", "luci-analysis-access"},
				})
				req := &pb.QueryChangepointGroupSummariesRequest{}

				res, err := server.QueryChangepointGroupSummaries(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
				So(res, ShouldBeNil)
			})
			Convey("e2e", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@google.com",
					IdentityGroups: []string{"googlers", "luci-analysis-access"},
				})
				// Group 1.
				cp1 := makeChangepointRow(1, 2, 4)
				cp2 := makeChangepointRow(2, 2, 3)
				client.ReadChangepointsResult = []*changepoints.ChangepointRow{cp1, cp2}
				stats := &pb.ChangepointGroupStatistics{
					UnexpectedVerdictRateBefore: &pb.ChangepointGroupStatistics_RateDistribution{
						Buckets: &pb.ChangepointGroupStatistics_RateDistribution_RateBuckets{},
					},
					UnexpectedVerdictRateAfter: &pb.ChangepointGroupStatistics_RateDistribution{
						Buckets: &pb.ChangepointGroupStatistics_RateDistribution_RateBuckets{},
					},
					UnexpectedVerdictRateCurrent: &pb.ChangepointGroupStatistics_RateDistribution{
						Buckets: &pb.ChangepointGroupStatistics_RateDistribution_RateBuckets{},
					},
					UnexpectedVerdictRateChange: &pb.ChangepointGroupStatistics_RateChangeBuckets{},
				}
				changepointGroupSummary := &pb.ChangepointGroupSummary{
					CanonicalChangepoint: &pb.Changepoint{
						Project:     "chromium",
						TestId:      "test1",
						VariantHash: "varianthash",
						RefHash:     "refhash",
						Ref: &pb.SourceRef{
							System: &pb.SourceRef_Gitiles{
								Gitiles: &pb.GitilesRef{
									Host:    "host",
									Project: "project",
									Ref:     "ref",
								},
							},
						},
						StartHour:                    timestamppb.New(time.Unix(1000, 0)),
						StartPositionLowerBound_99Th: cp1.LowerBound99th,
						StartPositionUpperBound_99Th: cp1.UpperBound99th,
						NominalStartPosition:         cp1.NominalStartPosition,
					},
					Statistics: stats,
				}
				Convey("with no predicates", func() {
					req := &pb.QueryChangepointGroupSummariesRequest{Project: "chromium"}

					res, err := server.QueryChangepointGroupSummaries(ctx, req)
					So(err, ShouldBeNil)
					stats.Count = 2
					stats.UnexpectedVerdictRateBefore.Average = 0.3
					stats.UnexpectedVerdictRateBefore.Buckets.CountAbove_5LessThan_95Percent = 2
					stats.UnexpectedVerdictRateAfter.Average = 0.99
					stats.UnexpectedVerdictRateAfter.Buckets.CountAbove_95Percent = 2
					stats.UnexpectedVerdictRateCurrent.Average = 0
					stats.UnexpectedVerdictRateCurrent.Buckets.CountLess_5Percent = 2
					stats.UnexpectedVerdictRateChange.CountIncreased_50To_100Percent = 2
					changepointGroupSummary.Statistics = stats
					So(res, ShouldResembleProto, &pb.QueryChangepointGroupSummariesResponse{
						GroupSummaries: []*pb.ChangepointGroupSummary{changepointGroupSummary},
					})
				})
				Convey("with predicates", func() {
					Convey("test id prefix predicate", func() {
						req := &pb.QueryChangepointGroupSummariesRequest{
							Project: "chromium",
							Predicate: &pb.ChangepointPredicate{
								TestIdPrefix: "test2",
							}}

						res, err := server.QueryChangepointGroupSummaries(ctx, req)
						So(err, ShouldBeNil)
						stats.Count = 1
						stats.UnexpectedVerdictRateBefore.Average = 0.3
						stats.UnexpectedVerdictRateBefore.Buckets.CountAbove_5LessThan_95Percent = 1
						stats.UnexpectedVerdictRateAfter.Average = 0.99
						stats.UnexpectedVerdictRateAfter.Buckets.CountAbove_95Percent = 1
						stats.UnexpectedVerdictRateCurrent.Average = 0
						stats.UnexpectedVerdictRateCurrent.Buckets.CountLess_5Percent = 1
						stats.UnexpectedVerdictRateChange.CountIncreased_50To_100Percent = 1
						changepointGroupSummary.Statistics = stats
						changepointGroupSummary.CanonicalChangepoint.TestId = "test2"
						changepointGroupSummary.CanonicalChangepoint.NominalStartPosition = 2
						changepointGroupSummary.CanonicalChangepoint.StartPositionUpperBound_99Th = 3
						So(res, ShouldResembleProto, &pb.QueryChangepointGroupSummariesResponse{
							GroupSummaries: []*pb.ChangepointGroupSummary{changepointGroupSummary},
						})
					})
					Convey("failure rate change predicate", func() {
						req := &pb.QueryChangepointGroupSummariesRequest{
							Project: "chromium",
							Predicate: &pb.ChangepointPredicate{
								UnexpectedVerdictRateChangeRange: &pb.NumericRange{
									LowerBound: 0.7,
									UpperBound: 1,
								},
							}}

						res, err := server.QueryChangepointGroupSummaries(ctx, req)
						So(err, ShouldBeNil)
						So(res, ShouldResembleProto, &pb.QueryChangepointGroupSummariesResponse{})
					})
				})
			})
		})
	})
}

func TestGroupChangepoints(t *testing.T) {
	t.Parallel()
	Convey("TestGroupChangepoints", t, func() {
		// Group 1 - test id num gap less than the TestIDGroupingThreshold in the same group.
		cp1 := makeChangepointRow(4, 100, 200)
		cp2 := makeChangepointRow(30, 160, 261) // 41 commits, 40.6% overlap with cp1
		cp3 := makeChangepointRow(90, 100, 200)
		cp4 := makeChangepointRow(1, 100, 300) // large regression range.
		// Group 2 - same test id group, but different regression range with group 1.
		cp5 := makeChangepointRow(2, 161, 263) // 40 commits. 39.6% overlap wtih cp 1.
		cp6 := makeChangepointRow(3, 161, 264)
		// Group 4 - different id group
		cp7 := makeChangepointRow(1000, 100, 200)
		cp8 := makeChangepointRow(1001, 100, 200)
		// Group 5 - same test variant can't be in the same group more than once.
		cp9 := makeChangepointRow(1001, 120, 230)

		groups := changepoints.GroupChangepoints([]*changepoints.ChangepointRow{cp1, cp2, cp3, cp4, cp5, cp6, cp7, cp8, cp9})
		So(groups, ShouldResemble, [][]*changepoints.ChangepointRow{
			{cp1, cp3, cp2, cp4},
			{cp5, cp6},
			{cp7, cp8},
			{cp9},
		})
	})
}

func TestValidateQueryChangepointGroupSummariesRequest(t *testing.T) {
	t.Parallel()
	Convey("validateQueryChangepointGroupSummariesRequest", t, func() {
		req := &pb.QueryChangepointGroupSummariesRequest{
			Project: "chromium",
			Predicate: &pb.ChangepointPredicate{
				TestIdPrefix: "test",
				UnexpectedVerdictRateChangeRange: &pb.NumericRange{
					LowerBound: 0,
					UpperBound: 1,
				},
			},
		}
		Convey("valid", func() {
			err := validateQueryChangepointGroupSummariesRequest(req)
			So(err, ShouldBeNil)
		})
		Convey("no project", func() {
			req.Project = ""
			err := validateQueryChangepointGroupSummariesRequest(req)
			So(err, ShouldErrLike, "project: unspecified")
		})
		Convey("invalid test prefix", func() {
			req.Predicate.TestIdPrefix = "\xFF"
			err := validateQueryChangepointGroupSummariesRequest(req)
			So(err, ShouldErrLike, "test_id_prefix: not a valid utf8 string")
		})
		Convey("invalid lower bound", func() {
			req.Predicate.UnexpectedVerdictRateChangeRange.LowerBound = 2
			err := validateQueryChangepointGroupSummariesRequest(req)
			So(err, ShouldErrLike, "unexpected_verdict_rate_change_range_range: lower_bound: should between 0 and 1")
		})
		Convey("invalid upper bound", func() {
			req.Predicate.UnexpectedVerdictRateChangeRange.UpperBound = 2
			err := validateQueryChangepointGroupSummariesRequest(req)
			So(err, ShouldErrLike, "unexpected_verdict_rate_change_range_range: upper_bound:  should between 0 and 1")
		})
		Convey("upper bound smaller than lower bound", func() {
			req.Predicate.UnexpectedVerdictRateChangeRange.UpperBound = 0.1
			req.Predicate.UnexpectedVerdictRateChangeRange.LowerBound = 0.2
			err := validateQueryChangepointGroupSummariesRequest(req)
			So(err, ShouldErrLike, "unexpected_verdict_rate_change_range_range: upper_bound must greater or equal to lower_bound")
		})
	})
}

func makeChangepointRow(TestIDNum, lowerBound, upperBound int64) *changepoints.ChangepointRow {
	return &changepoints.ChangepointRow{
		Project:     "chromium",
		TestIDNum:   TestIDNum,
		TestID:      fmt.Sprintf("test%d", TestIDNum),
		VariantHash: "varianthash",
		Ref: &changepoints.Ref{
			Gitiles: &changepoints.Gitiles{
				Host:    bigquery.NullString{Valid: true, StringVal: "host"},
				Project: bigquery.NullString{Valid: true, StringVal: "project"},
				Ref:     bigquery.NullString{Valid: true, StringVal: "ref"},
			},
		},
		RefHash:                      "refhash",
		UnexpectedVerdictRateCurrent: 0,
		UnexpectedVerdictRateAfter:   0.99,
		UnexpectedVerdictRateBefore:  0.3,
		StartHour:                    time.Unix(1000, 0),
		LowerBound99th:               lowerBound,
		UpperBound99th:               upperBound,
		NominalStartPosition:         (lowerBound + upperBound) / 2,
	}
}

type fakeChangepointClient struct {
	ReadChangepointsResult []*changepoints.ChangepointRow
}

func (f *fakeChangepointClient) ReadChangepoints(ctx context.Context, project string) ([]*changepoints.ChangepointRow, error) {
	return f.ReadChangepointsResult, nil
}
