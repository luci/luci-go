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

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestChangepointsServer(t *testing.T) {
	ftt.Run("TestChangepointsServer", t, func(t *ftt.Test) {
		ctx := context.Background()
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-analysis-access"},
		}
		ctx = auth.WithState(ctx, authState)
		client := fakeChangepointClient{}
		server := NewChangepointsServer(&client)

		t.Run("Unauthorised requests are rejected", func(t *ftt.Test) {
			// Ensure no access to luci-analysis-access.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of luci-analysis-access.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			req := &pb.QueryChangepointGroupSummariesRequestLegacy{
				Project: "chromium",
			}

			res, err := server.QueryChangepointGroupSummaries(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("not a member of luci-analysis-access"))
			assert.Loosely(t, res, should.BeNil)
		})
		t.Run("QueryChangepointGroupSummaries", func(t *ftt.Test) {
			authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
				Realm:      "chromium:@project",
				Permission: perms.PermListChangepointGroups,
			})

			t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermListChangepointGroups)

				req := &pb.QueryChangepointGroupSummariesRequestLegacy{
					Project: "chromium",
				}

				res, err := server.QueryChangepointGroupSummaries(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission analysis.changepointgroups.list in realm "chromium:@project"`))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("invalid requests are rejected - project unspecified", func(t *ftt.Test) {
				// This is the only request validation that occurs prior to permission check
				// comply with https://google.aip.dev/211.
				req := &pb.QueryChangepointGroupSummariesRequestLegacy{}

				res, err := server.QueryChangepointGroupSummaries(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("project: unspecified"))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("invalid requests are rejected - other", func(t *ftt.Test) {
				// Test one type of error detected by validateQueryChangepointGroupSummariesRequest.
				req := &pb.QueryChangepointGroupSummariesRequestLegacy{
					Project: "chromium",
					Predicate: &pb.ChangepointPredicateLegacy{
						TestIdPrefix: "\u0000",
					},
				}

				res, err := server.QueryChangepointGroupSummaries(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`predicate: test_id_prefix: non-printable rune '\x00' at byte index 0`))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("e2e", func(t *ftt.Test) {
				cp1 := makeChangepointDetailRow(1, 2, 4)
				cp2 := makeChangepointDetailRow(2, 2, 3)
				client.ReadChangepointsResult = []*changepoints.ChangepointDetailRow{cp1, cp2}
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
				t.Run("with no predicates", func(t *ftt.Test) {
					req := &pb.QueryChangepointGroupSummariesRequestLegacy{Project: "chromium"}

					res, err := server.QueryChangepointGroupSummaries(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					stats.Count = 2
					stats.UnexpectedVerdictRateBefore.Average = 0.3
					stats.UnexpectedVerdictRateBefore.Buckets.CountAbove_5LessThan_95Percent = 2
					stats.UnexpectedVerdictRateAfter.Average = 0.99
					stats.UnexpectedVerdictRateAfter.Buckets.CountAbove_95Percent = 2
					stats.UnexpectedVerdictRateCurrent.Average = 0
					stats.UnexpectedVerdictRateCurrent.Buckets.CountLess_5Percent = 2
					stats.UnexpectedVerdictRateChange.CountIncreased_50To_100Percent = 2
					changepointGroupSummary.Statistics = stats
					assert.Loosely(t, res, should.Resemble(&pb.QueryChangepointGroupSummariesResponseLegacy{
						GroupSummaries: []*pb.ChangepointGroupSummary{changepointGroupSummary},
					}))
				})
				t.Run("with predicates", func(t *ftt.Test) {
					t.Run("test id prefix predicate", func(t *ftt.Test) {
						req := &pb.QueryChangepointGroupSummariesRequestLegacy{
							Project: "chromium",
							Predicate: &pb.ChangepointPredicateLegacy{
								TestIdPrefix: "test2",
							}}

						res, err := server.QueryChangepointGroupSummaries(ctx, req)
						assert.Loosely(t, err, should.BeNil)
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
						assert.Loosely(t, res, should.Resemble(&pb.QueryChangepointGroupSummariesResponseLegacy{
							GroupSummaries: []*pb.ChangepointGroupSummary{changepointGroupSummary},
						}))
					})
					t.Run("failure rate change predicate", func(t *ftt.Test) {
						req := &pb.QueryChangepointGroupSummariesRequestLegacy{
							Project: "chromium",
							Predicate: &pb.ChangepointPredicateLegacy{
								UnexpectedVerdictRateChangeRange: &pb.NumericRange{
									LowerBound: 0.7,
									UpperBound: 1,
								},
							}}

						res, err := server.QueryChangepointGroupSummaries(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, res, should.Resemble(&pb.QueryChangepointGroupSummariesResponseLegacy{}))
					})
				})
			})
		})

		t.Run("QueryGroupSummaries", func(t *ftt.Test) {
			authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
				Realm:      "chromium:@project",
				Permission: perms.PermListChangepointGroups,
			})

			t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermListChangepointGroups)
				req := &pb.QueryChangepointGroupSummariesRequest{
					Project: "chromium",
				}

				res, err := server.QueryGroupSummaries(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission analysis.changepointgroups.list in realm "chromium:@project"`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid requests are rejected - project unspecified", func(t *ftt.Test) {
				// This is the only request validation that occurs prior to permission check
				// comply with https://google.aip.dev/211.
				req := &pb.QueryChangepointGroupSummariesRequest{}

				res, err := server.QueryGroupSummaries(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("project: unspecified"))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid requests are rejected - other", func(t *ftt.Test) {
				// Test one type of error detected by validateQueryChangepointGroupSummariesRequest.
				req := &pb.QueryChangepointGroupSummariesRequest{
					Project: "chromium",
					Predicate: &pb.ChangepointPredicate{
						TestIdContain: "\u0000",
					},
				}

				res, err := server.QueryGroupSummaries(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`predicate: test_id_prefix: non-printable rune '\x00' at byte index 0`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("e2e", func(t *ftt.Test) {
				client.ReadChangepointGroupSummariesResult = []*changepoints.GroupSummary{
					{
						CanonicalChangepoint: *makeChangepointRow("test1"),
						Total:                2,
						UnexpectedSourceVerdictRateBefore: changepoints.RateDistribution{
							Mean:                    0.3,
							Less5Percent:            2,
							Above5LessThan95Percent: 0,
							Above95Percent:          0,
						},
						UnexpectedSourceVerdictRateAfter: changepoints.RateDistribution{
							Mean:                    0.99,
							Less5Percent:            0,
							Above5LessThan95Percent: 0,
							Above95Percent:          2,
						},
						UnexpectedSourceVerdictRateCurrent: changepoints.RateDistribution{
							Mean:                    0,
							Less5Percent:            2,
							Above5LessThan95Percent: 0,
							Above95Percent:          0,
						},
						UnexpectedSourveVerdictRateChange: changepoints.RateChangeDistribution{
							Increase0to20percent:   0,
							Increase20to50percent:  0,
							Increase50to100percent: 2,
						},
					},
				}
				req := &pb.QueryChangepointGroupSummariesRequest{Project: "chromium"}

				res, err := server.QueryGroupSummaries(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&pb.QueryChangepointGroupSummariesResponse{
					GroupSummaries: []*pb.ChangepointGroupSummary{
						{
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
								StartHour:                         timestamppb.New(time.Unix(1000, 0)),
								StartPositionLowerBound_99Th:      2,
								StartPositionUpperBound_99Th:      4,
								NominalStartPosition:              3,
								PreviousSegmentNominalEndPosition: 1,
							},
							Statistics: &pb.ChangepointGroupStatistics{
								Count: 2,
								UnexpectedVerdictRateBefore: &pb.ChangepointGroupStatistics_RateDistribution{
									Average: 0.3,
									Buckets: &pb.ChangepointGroupStatistics_RateDistribution_RateBuckets{
										CountLess_5Percent:             2,
										CountAbove_5LessThan_95Percent: 0,
										CountAbove_95Percent:           0,
									},
								},
								UnexpectedVerdictRateAfter: &pb.ChangepointGroupStatistics_RateDistribution{
									Average: 0.99,
									Buckets: &pb.ChangepointGroupStatistics_RateDistribution_RateBuckets{
										CountLess_5Percent:             0,
										CountAbove_5LessThan_95Percent: 0,
										CountAbove_95Percent:           2,
									},
								},
								UnexpectedVerdictRateCurrent: &pb.ChangepointGroupStatistics_RateDistribution{
									Average: 0,
									Buckets: &pb.ChangepointGroupStatistics_RateDistribution_RateBuckets{
										CountLess_5Percent:             2,
										CountAbove_5LessThan_95Percent: 0,
										CountAbove_95Percent:           0,
									},
								},
								UnexpectedVerdictRateChange: &pb.ChangepointGroupStatistics_RateChangeBuckets{
									CountIncreased_0To_20Percent:   0,
									CountIncreased_20To_50Percent:  0,
									CountIncreased_50To_100Percent: 2,
								},
							},
						},
					},
					NextPageToken: "next-page",
				}))
			})

		})

		t.Run("QueryChangepointsInGroup", func(t *ftt.Test) {
			authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
				Realm:      "chromium:@project",
				Permission: perms.PermGetChangepointGroup,
			})

			t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetChangepointGroup)
				req := &pb.QueryChangepointsInGroupRequest{
					Project: "chromium",
				}

				res, err := server.QueryChangepointsInGroup(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission analysis.changepointgroups.get in realm "chromium:@project"`))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("invalid requests are rejected - project unspecified", func(t *ftt.Test) {
				// This is the only request validation that occurs prior to permission check
				// comply with https://google.aip.dev/211.
				req := &pb.QueryChangepointsInGroupRequest{}

				res, err := server.QueryChangepointsInGroup(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("project: unspecified"))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("invalid requests are rejected - other", func(t *ftt.Test) {
				// Test a validation error identified by validateQueryChangepointsInGroupRequest.
				req := &pb.QueryChangepointsInGroupRequest{
					Project:  "chromium",
					GroupKey: &pb.QueryChangepointsInGroupRequest_ChangepointIdentifier{},
				}

				res, err := server.QueryChangepointsInGroup(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("group_key: test_id: unspecified"))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("e2e", func(t *ftt.Test) {
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

				t.Run("group found", func(t *ftt.Test) {
					client.ReadChangepointsInGroupResult = []*changepoints.ChangepointRow{
						makeChangepointRow("test1"),
					}

					res, err := server.QueryChangepointsInGroup(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Resemble(&pb.QueryChangepointsInGroupResponse{
						Changepoints: []*pb.Changepoint{{
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
							StartHour:                         timestamppb.New(time.Unix(1000, 0)),
							StartPositionLowerBound_99Th:      2,
							StartPositionUpperBound_99Th:      4,
							NominalStartPosition:              3,
							PreviousSegmentNominalEndPosition: 1,
						}},
					}))

				})

				t.Run("group not found", func(t *ftt.Test) {
					client.ReadChangepointsInGroupResult = []*changepoints.ChangepointRow{}

					res, err := server.QueryChangepointsInGroup(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)())
					assert.Loosely(t, res, should.BeNil)
				})
			})
		})
	})
}

func TestValidateRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("validateQueryChangepointGroupSummariesRequest", t, func(t *ftt.Test) {
		req := &pb.QueryChangepointGroupSummariesRequestLegacy{
			Project: "chromium",
			Predicate: &pb.ChangepointPredicateLegacy{
				TestIdPrefix: "test",
				UnexpectedVerdictRateChangeRange: &pb.NumericRange{
					LowerBound: 0,
					UpperBound: 1,
				},
			},
			BeginOfWeek: timestamppb.New(time.Date(2024, 8, 25, 0, 0, 0, 0, time.UTC)),
		}
		t.Run("valid", func(t *ftt.Test) {
			err := validateQueryChangepointGroupSummariesRequestLegacy(req)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("invalid predicate", func(t *ftt.Test) {
			req.Predicate.TestIdPrefix = "\xFF"
			err := validateQueryChangepointGroupSummariesRequestLegacy(req)
			assert.Loosely(t, err, should.ErrLike("test_id_prefix: not a valid utf8 string"))
		})
		t.Run("invalid begin_of_week", func(t *ftt.Test) {
			req.BeginOfWeek = timestamppb.New(time.Date(2024, 8, 25, 1, 0, 0, 0, time.UTC))
			err := validateQueryChangepointGroupSummariesRequestLegacy(req)
			assert.Loosely(t, err, should.ErrLike("begin_of_week: must be Sunday midnight"))
		})
		t.Run("begin_of_week at different time zone", func(t *ftt.Test) {
			req.BeginOfWeek = timestamppb.New(time.Date(2024, 8, 25, 0, 0, 0, 0, time.FixedZone("10sec", 10)))
			err := validateQueryChangepointGroupSummariesRequestLegacy(req)
			assert.Loosely(t, err, should.ErrLike("begin_of_week: must be Sunday midnight"))
		})
		t.Run("no begin_of_week", func(t *ftt.Test) {
			req.BeginOfWeek = nil
			err := validateQueryChangepointGroupSummariesRequestLegacy(req)
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run("validateQueryChangepointsInGroupRequest", t, func(t *ftt.Test) {
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
		t.Run("valid", func(t *ftt.Test) {
			err := validateQueryChangepointsInGroupRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("no group key", func(t *ftt.Test) {
			req.GroupKey = nil
			err := validateQueryChangepointsInGroupRequest(req)
			assert.Loosely(t, err, should.ErrLike("group_key: unspecified"))
		})
		t.Run("invalid group key", func(t *ftt.Test) {
			req.GroupKey.TestId = "\xFF"
			err := validateQueryChangepointsInGroupRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id: not a valid utf8 string"))
		})
	})

	ftt.Run("validateQueryChangepointGroupSummariesRequest", t, func(t *ftt.Test) {
		req := &pb.QueryChangepointGroupSummariesRequest{
			Project: "chromium",
			Predicate: &pb.ChangepointPredicate{
				TestIdContain: "test",
			},
		}
		t.Run("valid", func(t *ftt.Test) {
			err := validateQueryChangepointGroupSummariesRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("invalid page size ", func(t *ftt.Test) {
			req.PageSize = -1
			err := validateQueryChangepointGroupSummariesRequest(req)
			assert.Loosely(t, err, should.ErrLike("page_size"))
		})
		t.Run("invalid predicate", func(t *ftt.Test) {
			req.Predicate.TestIdContain = "\xFF"
			err := validateQueryChangepointGroupSummariesRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id_prefix: not a valid utf8 string"))
		})
	})

	ftt.Run("validateChangepointPredicate", t, func(t *ftt.Test) {
		t.Run("invalid test prefix", func(t *ftt.Test) {
			predicate := &pb.ChangepointPredicateLegacy{
				TestIdPrefix: "\xFF",
			}
			err := validateChangepointPredicateLegacy(predicate)
			assert.Loosely(t, err, should.ErrLike("test_id_prefix: not a valid utf8 string"))
		})
		t.Run("invalid lower bound", func(t *ftt.Test) {
			predicate := &pb.ChangepointPredicateLegacy{
				UnexpectedVerdictRateChangeRange: &pb.NumericRange{
					LowerBound: 2,
				},
			}
			err := validateChangepointPredicateLegacy(predicate)
			assert.Loosely(t, err, should.ErrLike("unexpected_verdict_rate_change_range_range: lower_bound: should between 0 and 1"))
		})
		t.Run("invalid upper bound", func(t *ftt.Test) {
			predicate := &pb.ChangepointPredicateLegacy{
				UnexpectedVerdictRateChangeRange: &pb.NumericRange{
					UpperBound: 2,
				},
			}
			err := validateChangepointPredicateLegacy(predicate)
			assert.Loosely(t, err, should.ErrLike("unexpected_verdict_rate_change_range_range: upper_bound:  should between 0 and 1"))
		})
		t.Run("upper bound smaller than lower bound", func(t *ftt.Test) {
			predicate := &pb.ChangepointPredicateLegacy{
				UnexpectedVerdictRateChangeRange: &pb.NumericRange{
					UpperBound: 0.1,
					LowerBound: 0.2,
				},
			}
			err := validateChangepointPredicateLegacy(predicate)
			assert.Loosely(t, err, should.ErrLike("unexpected_verdict_rate_change_range_range: upper_bound must greater or equal to lower_bound"))
		})
	})
}

func makeChangepointDetailRow(TestIDNum, lowerBound, upperBound int64) *changepoints.ChangepointDetailRow {
	return &changepoints.ChangepointDetailRow{
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
		RefHash:                            "b920ffffffffffff",
		UnexpectedSourceVerdictRateCurrent: 0,
		UnexpectedSourceVerdictRateAfter:   0.99,
		UnexpectedSourceVerdictRateBefore:  0.3,
		StartHour:                          time.Unix(1000, 0),
		LowerBound99th:                     lowerBound,
		UpperBound99th:                     upperBound,
		NominalStartPosition:               (lowerBound + upperBound) / 2,
	}
}

func makeChangepointRow(testID string) *changepoints.ChangepointRow {
	return &changepoints.ChangepointRow{
		Project:     "chromium",
		TestID:      testID,
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
		RefHash:                    "b920ffffffffffff",
		StartHour:                  time.Unix(1000, 0),
		LowerBound99th:             2,
		UpperBound99th:             4,
		NominalStartPosition:       3,
		PreviousNominalEndPosition: 1,
	}
}

type fakeChangepointClient struct {
	ReadChangepointsResult              []*changepoints.ChangepointDetailRow
	ReadChangepointGroupSummariesResult []*changepoints.GroupSummary
	ReadChangepointsInGroupResult       []*changepoints.ChangepointRow
}

func (f *fakeChangepointClient) ReadChangepoints(ctx context.Context, project string, week time.Time) ([]*changepoints.ChangepointDetailRow, error) {
	return f.ReadChangepointsResult, nil
}

func (f *fakeChangepointClient) ReadChangepointGroupSummaries(ctx context.Context, opts changepoints.ReadChangepointGroupSummariesOptions) ([]*changepoints.GroupSummary, string, error) {
	return f.ReadChangepointGroupSummariesResult, "next-page", nil
}

func (f *fakeChangepointClient) ReadChangepointsInGroup(ctx context.Context, opts changepoints.ReadChangepointsInGroupOptions) (changepoints []*changepoints.ChangepointRow, err error) {
	return f.ReadChangepointsInGroupResult, nil
}
