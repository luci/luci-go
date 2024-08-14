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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/perms"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestChangepointsServer(t *testing.T) {
	Convey("TestChangepointsServer", t, func() {
		ctx := context.Background()
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-analysis-access"},
		}
		ctx = auth.WithState(ctx, authState)
		client := fakeChangepointClient{}
		server := NewChangepointsServer(&client)

		Convey("Unauthorised requests are rejected", func() {
			// Ensure no access to luci-analysis-access.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of luci-analysis-access.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			req := &pb.QueryChangepointGroupSummariesRequest{
				Project: "chromium",
			}

			res, err := server.QueryChangepointGroupSummaries(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, "not a member of luci-analysis-access")
			So(res, ShouldBeNil)
		})
		Convey("QueryChangepointGroupSummaries", func() {
			authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
				Realm:      "chromium:@project",
				Permission: perms.PermListChangepointGroups,
			})

			Convey("unauthorised requests are rejected", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermListChangepointGroups)

				req := &pb.QueryChangepointGroupSummariesRequest{
					Project: "chromium",
				}

				res, err := server.QueryChangepointGroupSummaries(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `caller does not have permission analysis.changepointgroups.list in realm "chromium:@project"`)
				So(res, ShouldBeNil)
			})
			Convey("invalid requests are rejected - project unspecified", func() {
				// This is the only request validation that occurs prior to permission check
				// comply with https://google.aip.dev/211.
				req := &pb.QueryChangepointGroupSummariesRequest{}

				res, err := server.QueryChangepointGroupSummaries(ctx, req)
				So(err, ShouldBeRPCInvalidArgument, "project: unspecified")
				So(res, ShouldBeNil)
			})
			Convey("invalid requests are rejected - other", func() {
				// Test one type of error detected by validateQueryChangepointGroupSummariesRequest.
				req := &pb.QueryChangepointGroupSummariesRequest{
					Project: "chromium",
					Predicate: &pb.ChangepointPredicate{
						TestIdPrefix: "\u0000",
					},
				}

				res, err := server.QueryChangepointGroupSummaries(ctx, req)
				So(err, ShouldBeRPCInvalidArgument, `predicate: test_id_prefix: non-printable rune '\x00' at byte index 0`)
				So(res, ShouldBeNil)
			})
			Convey("e2e", func() {
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
						VariantHash: "5097aaaaaaaaaaaa",
						Variant: &pb.Variant{
							Def: map[string]string{
								"var":  "abc",
								"varr": "xyx",
							},
						},
						RefHash: "b920ffffffffffff",
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

		Convey("QueryChangepointsInGroup", func() {
			authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
				Realm:      "chromium:@project",
				Permission: perms.PermGetChangepointGroup,
			})

			Convey("unauthorised requests are rejected", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetChangepointGroup)
				req := &pb.QueryChangepointsInGroupRequest{
					Project: "chromium",
				}

				res, err := server.QueryChangepointsInGroup(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `caller does not have permission analysis.changepointgroups.get in realm "chromium:@project"`)
				So(res, ShouldBeNil)
			})
			Convey("invalid requests are rejected - project unspecified", func() {
				// This is the only request validation that occurs prior to permission check
				// comply with https://google.aip.dev/211.
				req := &pb.QueryChangepointsInGroupRequest{}

				res, err := server.QueryChangepointsInGroup(ctx, req)
				So(err, ShouldBeRPCInvalidArgument, "project: unspecified")
				So(res, ShouldBeNil)
			})
			Convey("invalid requests are rejected - other", func() {
				// Test a validation error identified by validateQueryChangepointsInGroupRequest.
				req := &pb.QueryChangepointsInGroupRequest{
					Project:  "chromium",
					GroupKey: &pb.QueryChangepointsInGroupRequest_ChangepointIdentifier{},
				}

				res, err := server.QueryChangepointsInGroup(ctx, req)
				So(err, ShouldBeRPCInvalidArgument, "group_key: test_id: unspecified")
				So(res, ShouldBeNil)
			})

			Convey("e2e", func() {
				// Group1.
				cp1 := makeChangepointRow(1, 2, 4)
				cp2 := makeChangepointRow(2, 2, 3)
				// Group2.
				cp3 := makeChangepointRow(1, 2, 20)
				cp4 := makeChangepointRow(2, 2, 20)
				// Group3.
				cp5 := makeChangepointRow(1, 20, 40)
				cp6 := makeChangepointRow(2, 20, 30)
				client.ReadChangepointsResult = []*changepoints.ChangepointRow{cp1, cp2, cp3, cp4, cp5, cp6}
				req := &pb.QueryChangepointsInGroupRequest{
					Project: "chromium",
					GroupKey: &pb.QueryChangepointsInGroupRequest_ChangepointIdentifier{
						TestId:               "test2",
						VariantHash:          "5097aaaaaaaaaaaa",
						RefHash:              "b920ffffffffffff",
						NominalStartPosition: 20, // Match group 3.
						StartHour:            timestamppb.New(time.Unix(100, 0)),
					},
				}

				Convey("group found", func() {
					Convey("with no predicates", func() {
						res, err := server.QueryChangepointsInGroup(ctx, req)
						So(err, ShouldBeNil)
						So(res.Changepoints, ShouldHaveLength, 2)
						So(res.Changepoints[0].TestId, ShouldEqual, "test1")
						So(res.Changepoints[0].NominalStartPosition, ShouldEqual, cp5.NominalStartPosition)
						So(res.Changepoints[1].TestId, ShouldEqual, "test2")
						So(res.Changepoints[1].NominalStartPosition, ShouldEqual, cp6.NominalStartPosition)
					})

					Convey("with predicates", func() {
						req.Predicate = &pb.ChangepointPredicate{
							TestIdPrefix: "test2",
						}

						res, err := server.QueryChangepointsInGroup(ctx, req)
						So(err, ShouldBeNil)
						So(res.Changepoints, ShouldHaveLength, 1)
						So(res.Changepoints[0].TestId, ShouldEqual, "test2")
						So(res.Changepoints[0].NominalStartPosition, ShouldEqual, cp6.NominalStartPosition)
					})
				})

				Convey("group not found", func() {
					req.GroupKey.NominalStartPosition = 100 // no match.

					res, err := server.QueryChangepointsInGroup(ctx, req)
					So(err, ShouldBeRPCNotFound)
					So(res, ShouldBeNil)
				})
			})
		})
	})
}

func TestValidateRequest(t *testing.T) {
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
		Convey("invalid predicate", func() {
			req.Predicate.TestIdPrefix = "\xFF"
			err := validateQueryChangepointGroupSummariesRequest(req)
			So(err, ShouldErrLike, "test_id_prefix: not a valid utf8 string")
		})
	})

	Convey("validateQueryChangepointsInGroupRequest", t, func() {
		req := &pb.QueryChangepointsInGroupRequest{
			Project: "chromium",
			GroupKey: &pb.QueryChangepointsInGroupRequest_ChangepointIdentifier{
				TestId:               "testid",
				VariantHash:          "5097aaaaaaaaaaaa",
				RefHash:              "b920ffffffffffff",
				NominalStartPosition: 1,
				StartHour:            timestamppb.New(time.Unix(1000, 0)),
			},
			Predicate: &pb.ChangepointPredicate{},
		}
		Convey("valid", func() {
			err := validateQueryChangepointsInGroupRequest(req)
			So(err, ShouldBeNil)
		})
		Convey("no group key", func() {
			req.GroupKey = nil
			err := validateQueryChangepointsInGroupRequest(req)
			So(err, ShouldErrLike, "group_key: unspecified")
		})
		Convey("invalid group key", func() {
			req.GroupKey.TestId = "\xFF"
			err := validateQueryChangepointsInGroupRequest(req)
			So(err, ShouldErrLike, "test_id: not a valid utf8 string")
		})
	})

	Convey("validateChangepointPredicate", t, func() {
		Convey("invalid test prefix", func() {
			predicate := &pb.ChangepointPredicate{
				TestIdPrefix: "\xFF",
			}
			err := validateChangepointPredicate(predicate)
			So(err, ShouldErrLike, "test_id_prefix: not a valid utf8 string")
		})
		Convey("invalid lower bound", func() {
			predicate := &pb.ChangepointPredicate{
				UnexpectedVerdictRateChangeRange: &pb.NumericRange{
					LowerBound: 2,
				},
			}
			err := validateChangepointPredicate(predicate)
			So(err, ShouldErrLike, "unexpected_verdict_rate_change_range_range: lower_bound: should between 0 and 1")
		})
		Convey("invalid upper bound", func() {
			predicate := &pb.ChangepointPredicate{
				UnexpectedVerdictRateChangeRange: &pb.NumericRange{
					UpperBound: 2,
				},
			}
			err := validateChangepointPredicate(predicate)
			So(err, ShouldErrLike, "unexpected_verdict_rate_change_range_range: upper_bound:  should between 0 and 1")
		})
		Convey("upper bound smaller than lower bound", func() {
			predicate := &pb.ChangepointPredicate{
				UnexpectedVerdictRateChangeRange: &pb.NumericRange{
					UpperBound: 0.1,
					LowerBound: 0.2,
				},
			}
			err := validateChangepointPredicate(predicate)
			So(err, ShouldErrLike, "unexpected_verdict_rate_change_range_range: upper_bound must greater or equal to lower_bound")
		})
	})
}

func makeChangepointRow(TestIDNum, lowerBound, upperBound int64) *changepoints.ChangepointRow {
	return &changepoints.ChangepointRow{
		Project:     "chromium",
		TestIDNum:   TestIDNum,
		TestID:      fmt.Sprintf("test%d", TestIDNum),
		VariantHash: "5097aaaaaaaaaaaa",
		Variant: bigquery.NullJSON{
			JSONVal: "{\"var\":\"abc\",\"varr\":\"xyx\"}",
			Valid:   true,
		},
		Ref: &changepoints.Ref{
			Gitiles: &changepoints.Gitiles{
				Host:    bigquery.NullString{Valid: true, StringVal: "host"},
				Project: bigquery.NullString{Valid: true, StringVal: "project"},
				Ref:     bigquery.NullString{Valid: true, StringVal: "ref"},
			},
		},
		RefHash:                      "b920ffffffffffff",
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

func (f *fakeChangepointClient) ReadChangepoints(ctx context.Context, project string, week time.Time) ([]*changepoints.ChangepointRow, error) {
	return f.ReadChangepointsResult, nil
}
