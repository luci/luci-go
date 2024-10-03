// Copyright 2023 The LUCI Authors.
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
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/model"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/sorbet"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/gitiles"
	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTestVariantBranchesServer(t *testing.T) {
	ftt.Run("TestVariantBranchesServer", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal)

		tvc := testverdicts.FakeReadClient{}
		trc := testresults.FakeReadClient{}
		sorbetClient := sorbet.NewFakeClient()

		server := NewTestVariantBranchesServer(&tvc, &trc, sorbetClient)
		t.Run("GetRaw", func(t *ftt.Test) {
			authState := &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{"service-luci-analysis-admins", "luci-analysis-access"},
				IdentityPermissions: []authtest.RealmPermission{
					{
						Realm:      "myproject:@project",
						Permission: perms.PermGetTestVariantBranch,
					},
				},
			}
			ctx = auth.WithState(ctx, authState)

			t.Run("permission denied - no admin", func(t *ftt.Test) {
				authState.IdentityGroups = removeGroup(authState.IdentityGroups, "service-luci-analysis-admins")

				req := &pb.GetRawTestVariantBranchRequest{}
				res, err := server.GetRaw(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("not a member of service-luci-analysis-admins"))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("permission denied - no permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetTestVariantBranch)

				req := &pb.GetRawTestVariantBranchRequest{
					Name: "projects/myproject/tests/test/variants/abababababababab/refs/abababababababab",
				}
				res, err := server.GetRaw(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission analysis.testvariantbranches.get in realm "myproject:@project"`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid request", func(t *ftt.Test) {
				req := &pb.GetRawTestVariantBranchRequest{
					Name: "Project/abc/xyz",
				}
				res, err := server.GetRaw(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name: name must be of format projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}"))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("not found", func(t *ftt.Test) {
				req := &pb.GetRawTestVariantBranchRequest{
					Name: "projects/myproject/tests/test/variants/abababababababab/refs/abababababababab",
				}
				res, err := server.GetRaw(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)())
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid ref_hash", func(t *ftt.Test) {
				req := &pb.GetRawTestVariantBranchRequest{
					Name: "projects/myproject/tests/thisisatest/variants/abababababababab/refs/abababababababgh",
				}
				res, err := server.GetRaw(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name: "))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid test id", func(t *ftt.Test) {
				t.Run("bad structure", func(t *ftt.Test) {
					req := &pb.GetRawTestVariantBranchRequest{
						Name: "projects/myproject/tests/a/variants/0123456789abcdef/refs/7265665f68617368/bad/subpath",
					}
					res, err := server.GetRaw(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name must be of format projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}"))
					assert.Loosely(t, res, should.BeNil)
				})
				t.Run("bad URL escaping", func(t *ftt.Test) {
					req := &pb.GetRawTestVariantBranchRequest{
						Name: "projects/myproject/tests/abcdef%test/variants/0123456789abcdef/refs/7265665f68617368",
					}
					res, err := server.GetRaw(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("malformed test id: invalid URL escape \"%te\""))
					assert.Loosely(t, res, should.BeNil)
				})
				t.Run("bad value", func(t *ftt.Test) {
					req := &pb.GetRawTestVariantBranchRequest{
						Name: "projects/myproject/tests/\u0001atest/variants/0123456789abcdef/refs/7265665f68617368",
					}
					res, err := server.GetRaw(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`test id "\x01atest": non-printable rune`))
					assert.Loosely(t, res, should.BeNil)
				})
			})
			t.Run("ok", func(t *ftt.Test) {
				// Insert test variant branch to Spanner.
				tvb := &testvariantbranch.Entry{
					IsNew:       true,
					Project:     "myproject",
					TestID:      "this//is/a/test",
					VariantHash: "0123456789abcdef",
					RefHash:     []byte("ref_hash"),
					SourceRef: &pb.SourceRef{
						System: &pb.SourceRef_Gitiles{
							Gitiles: &pb.GitilesRef{
								Host:    "host",
								Project: "proj",
								Ref:     "ref",
							},
						},
					},
					Variant: &pb.Variant{
						Def: map[string]string{
							"k": "v",
						},
					},
					InputBuffer: &inputbuffer.Buffer{
						HotBuffer: inputbuffer.History{
							Runs: []inputbuffer.Run{
								{
									CommitPosition: 120,
									Hour:           time.Unix(3600, 0),
									Expected: inputbuffer.ResultCounts{
										PassCount: 1,
									},
								},
							},
						},
						ColdBuffer: inputbuffer.History{
							Runs: []inputbuffer.Run{
								{
									CommitPosition: 130,
									Hour:           time.Unix(2*3600, 0),
									Expected: inputbuffer.ResultCounts{
										PassCount: 1,
										FailCount: 2,
									},
									Unexpected: inputbuffer.ResultCounts{
										CrashCount: 3,
										AbortCount: 4,
									},
								},
								{
									CommitPosition: 131,
									Hour:           time.Unix(3*3600, 0),
									Expected: inputbuffer.ResultCounts{
										CrashCount: 5,
										AbortCount: 6,
									},
									Unexpected: inputbuffer.ResultCounts{
										PassCount:  7,
										AbortCount: 8,
									},
								},
							},
						},
					},
					FinalizingSegment: &cpb.Segment{
						State:                        cpb.SegmentState_FINALIZING,
						HasStartChangepoint:          true,
						StartPosition:                100,
						StartHour:                    timestamppb.New(time.Unix(3600, 0)),
						StartPositionLowerBound_99Th: 95,
						StartPositionUpperBound_99Th: 105,
						FinalizedCounts: &cpb.Counts{
							UnexpectedResults: 1,
							PartialSourceVerdict: &cpb.PartialSourceVerdict{
								CommitPosition:    110,
								LastHour:          timestamppb.New(time.Unix(3600, 0)),
								UnexpectedResults: 1,
							},
						},
					},
					FinalizedSegments: &cpb.Segments{
						Segments: []*cpb.Segment{
							{
								State:                        cpb.SegmentState_FINALIZED,
								StartPosition:                50,
								StartHour:                    timestamppb.New(time.Unix(3600, 0)),
								StartPositionLowerBound_99Th: 45,
								StartPositionUpperBound_99Th: 55,
								FinalizedCounts: &cpb.Counts{
									UnexpectedResults: 2,
								},
							},
						},
					},
					Statistics: &cpb.Statistics{
						HourlyBuckets: []*cpb.Statistics_HourBucket{
							{
								Hour:                     123456,
								UnexpectedSourceVerdicts: 1,
								FlakySourceVerdicts:      3,
								TotalSourceVerdicts:      12,
							},
							{
								Hour:                     123500,
								UnexpectedSourceVerdicts: 3,
								FlakySourceVerdicts:      7,
								TotalSourceVerdicts:      93,
							},
						},
						PartialSourceVerdict: &cpb.PartialSourceVerdict{
							CommitPosition:    99999,
							LastHour:          timestamppb.New(time.Unix(3600, 0)),
							UnexpectedResults: 1,
						},
					},
				}
				var hs inputbuffer.HistorySerializer
				mutation, err := tvb.ToMutation(&hs)
				assert.Loosely(t, err, should.BeNil)
				testutil.MustApply(ctx, t, mutation)

				hexStr := "7265665f68617368" // hex string of "ref_hash".
				req := &pb.GetRawTestVariantBranchRequest{
					Name: "projects/myproject/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
				}
				res, err := server.GetRaw(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				expectedFinalizingSegment, err := anypb.New(tvb.FinalizingSegment)
				assert.Loosely(t, err, should.BeNil)

				expectedFinalizedSegments, err := anypb.New(tvb.FinalizedSegments)
				assert.Loosely(t, err, should.BeNil)

				expectedStatistics, err := anypb.New(tvb.Statistics)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, res, should.Resemble(&pb.TestVariantBranchRaw{
					Name:              "projects/myproject/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
					Project:           "myproject",
					TestId:            "this//is/a/test",
					VariantHash:       "0123456789abcdef",
					RefHash:           hexStr,
					Variant:           tvb.Variant,
					Ref:               tvb.SourceRef,
					FinalizingSegment: expectedFinalizingSegment,
					FinalizedSegments: expectedFinalizedSegments,
					Statistics:        expectedStatistics,
					HotBuffer: &pb.InputBuffer{
						Length: 1,
						Runs: []*pb.InputBuffer_Run{
							{
								CommitPosition: 120,
								Hour:           timestamppb.New(time.Unix(3600, 0)),
								Counts: &pb.InputBuffer_Run_Counts{
									ExpectedPassCount: 1,
								},
							},
						},
					},
					ColdBuffer: &pb.InputBuffer{
						Length: 2,
						Runs: []*pb.InputBuffer_Run{
							{
								CommitPosition: 130,
								Hour:           timestamppb.New(time.Unix(2*3600, 0)),
								Counts: &pb.InputBuffer_Run_Counts{
									ExpectedPassCount:    1,
									ExpectedFailCount:    2,
									UnexpectedCrashCount: 3,
									UnexpectedAbortCount: 4,
								},
							},
							{
								CommitPosition: 131,
								Hour:           timestamppb.New(time.Unix(3*3600, 0)),
								Counts: &pb.InputBuffer_Run_Counts{
									ExpectedCrashCount:   5,
									ExpectedAbortCount:   6,
									UnexpectedPassCount:  7,
									UnexpectedAbortCount: 8,
								},
							},
						},
					},
				}))
			})
		})

		t.Run("BatchGet", func(t *ftt.Test) {
			t.Run("permission denied", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"luci-analysis-access"},
				})
				req := &pb.BatchGetTestVariantBranchRequest{
					Names: []string{
						"projects/testproject/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
					},
				}

				res, err := server.BatchGet(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`does not have permission analysis.testvariantbranches.get in realm "testproject:@project"`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid request", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"luci-analysis-access"},
					IdentityPermissions: []authtest.RealmPermission{{
						Realm:      "myproject:@project",
						Permission: perms.PermGetTestVariantBranch,
					}},
				})
				t.Run("invalid name", func(t *ftt.Test) {
					req := &pb.BatchGetTestVariantBranchRequest{
						Names: []string{"projects/abc/xyz"},
					}

					res, err := server.BatchGet(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name must be of format projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}"))
					assert.Loosely(t, res, should.BeNil)
				})

				t.Run("too many test variant branch requested", func(t *ftt.Test) {
					names := []string{}
					for i := 0; i < 200; i++ {
						names = append(names, "projects/myproject/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368")
					}
					req := &pb.BatchGetTestVariantBranchRequest{
						Names: names,
					}

					res, err := server.BatchGet(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("names: no more than 100 may be queried at a time"))
					assert.Loosely(t, res, should.BeNil)
				})
			})

			t.Run("e2e", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"luci-analysis-access"},
					IdentityPermissions: []authtest.RealmPermission{{
						Realm:      "myproject:@project",
						Permission: perms.PermGetTestVariantBranch,
					}},
				})
				// Insert test variant branch to Spanner.
				tvb := &testvariantbranch.Entry{
					IsNew:       true,
					Project:     "myproject",
					TestID:      "this//is/a/test",
					VariantHash: "0123456789abcdef",
					RefHash:     []byte("ref_hash"),
					SourceRef: &pb.SourceRef{
						System: &pb.SourceRef_Gitiles{
							Gitiles: &pb.GitilesRef{
								Host:    "host",
								Project: "proj",
								Ref:     "ref",
							},
						},
					},
					Variant: &pb.Variant{
						Def: map[string]string{
							"k": "v",
						},
					},
					InputBuffer: &inputbuffer.Buffer{
						HotBuffer: inputbuffer.History{
							Runs: []inputbuffer.Run{
								{
									CommitPosition: 200,
									Hour:           time.Unix(2*3600, 0),
									Expected: inputbuffer.ResultCounts{
										PassCount: 1,
									},
								},
							},
						},
						ColdBuffer: inputbuffer.History{
							Runs: []inputbuffer.Run{},
						},
					},
					FinalizingSegment: &cpb.Segment{
						State:                        cpb.SegmentState_FINALIZING,
						HasStartChangepoint:          true,
						StartPosition:                100,
						StartHour:                    timestamppb.New(time.Unix(3600, 0)),
						StartPositionLowerBound_99Th: 95,
						StartPositionUpperBound_99Th: 105,
						StartPositionDistribution:    model.SimpleDistribution(100, 5).Serialize(),
						FinalizedCounts: &cpb.Counts{
							UnexpectedSourceVerdicts: 1,
							TotalSourceVerdicts:      1,
							PartialSourceVerdict: &cpb.PartialSourceVerdict{
								CommitPosition:  150,
								LastHour:        timestamppb.New(time.Unix(3600, 0)),
								ExpectedResults: 1,
							},
						},
					},
					FinalizedSegments: &cpb.Segments{
						Segments: []*cpb.Segment{
							{
								State:         cpb.SegmentState_FINALIZED,
								StartPosition: 50,
								StartHour:     timestamppb.New(time.Unix(3600, 0)),
								FinalizedCounts: &cpb.Counts{
									UnexpectedSourceVerdicts: 2,
									TotalSourceVerdicts:      2,
								},
							},
						},
					},
				}
				var hs inputbuffer.HistorySerializer
				mutation, err := tvb.ToMutation(&hs)
				assert.Loosely(t, err, should.BeNil)
				testutil.MustApply(ctx, t, mutation)
				req := &pb.BatchGetTestVariantBranchRequest{
					Names: []string{
						"projects/myproject/tests/not%2Fexist%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
						"projects/myproject/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
					},
				}

				res, err := server.BatchGet(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.TestVariantBranches, should.HaveLength(2))
				assert.Loosely(t, res.TestVariantBranches[0], should.BeNil)
				assert.Loosely(t, res.TestVariantBranches[1], should.Resemble(&pb.TestVariantBranch{
					Name:        "projects/myproject/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
					Project:     "myproject",
					TestId:      "this//is/a/test",
					VariantHash: "0123456789abcdef",
					RefHash:     "7265665f68617368",
					Ref: &pb.SourceRef{
						System: &pb.SourceRef_Gitiles{
							Gitiles: &pb.GitilesRef{
								Host:    "host",
								Project: "proj",
								Ref:     "ref",
							},
						},
					},
					Variant: &pb.Variant{
						Def: map[string]string{
							"k": "v",
						},
					},
					Segments: []*pb.Segment{
						{
							HasStartChangepoint:          true,
							StartPosition:                100,
							StartPositionLowerBound_99Th: 95,
							StartPositionUpperBound_99Th: 105,
							StartPositionDistribution: &pb.Segment_PositionDistribution{
								Cdfs: []float64{
									0.0005, 0.005, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25, 0.5,
									0.75, 0.8, 0.85, 0.9, 0.95, 0.975, 0.995, 0.9995,
								},
								SourcePositions: []int64{
									90, 95, 100, 100, 100, 100, 100, 100, 100,
									100, 100, 100, 100, 100, 100, 105, 110,
								},
							},
							StartHour:   timestamppb.New(time.Unix(3600, 0)),
							EndPosition: 200,
							EndHour:     timestamppb.New(time.Unix(2*3600, 0)),
							Counts: &pb.Segment_Counts{
								ExpectedPassedResults: 1,
								TotalResults:          1,

								TotalRuns: 1,

								UnexpectedVerdicts: 1,
								FlakyVerdicts:      0,
								TotalVerdicts:      3,
							},
						},
						{
							StartPosition: 50,
							StartHour:     timestamppb.New(time.Unix(3600, 0)),
							EndHour:       timestamppb.New(time.Unix(0, 0)),
							Counts: &pb.Segment_Counts{
								UnexpectedVerdicts: 2,
								FlakyVerdicts:      0,
								TotalVerdicts:      2,
							},
						},
					},
				}))
			})
		})

		t.Run("Query", func(t *ftt.Test) {
			authState := &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{"luci-analysis-access"},
				IdentityPermissions: []authtest.RealmPermission{{
					Realm:      "myproject:@project",
					Permission: perms.PermListTestVariantBranches,
				}},
			}
			ctx = auth.WithState(ctx, authState)

			t.Run("permission denied", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermListTestVariantBranches)

				req := &pb.QueryTestVariantBranchRequest{
					Project: "myproject",
				}

				res, err := server.Query(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission analysis.testvariantbranches.list in realm "myproject:@project"`))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("invalid request", func(t *ftt.Test) {
				req := &pb.QueryTestVariantBranchRequest{
					Project: "myproject",
					TestId:  "test://is/a/test",
					Ref: &pb.SourceRef{
						System: &pb.SourceRef_Gitiles{
							Gitiles: &pb.GitilesRef{
								Host:    "host",
								Project: "proj",
								Ref:     "ref",
							},
						},
					},
				}
				t.Run("invalid project", func(t *ftt.Test) {
					req.Project = ""

					_, err := server.Query(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("project: unspecified"))
				})
				t.Run("invalid test id", func(t *ftt.Test) {
					req.TestId = ""

					_, err := server.Query(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("test_id: unspecified"))
				})
				t.Run("invalid ref", func(t *ftt.Test) {
					req.Ref.GetGitiles().Host = ""

					_, err := server.Query(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("ref: gitiles: host: unspecified"))
				})
			})
			t.Run("e2e", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"luci-analysis-access"},
					IdentityPermissions: []authtest.RealmPermission{{
						Realm:      "myproject:@project",
						Permission: perms.PermListTestVariantBranches,
					}},
				})
				ref := &pb.SourceRef{
					System: &pb.SourceRef_Gitiles{
						Gitiles: &pb.GitilesRef{
							Host:    "host",
							Project: "proj",
							Ref:     "ref",
						},
					},
				}
				refHash := pbutil.SourceRefHash(ref)
				var1 := pbutil.Variant("key1", "val1", "key2", "val1")
				var2 := pbutil.Variant("key1", "val2", "key2", "val1")
				var3 := pbutil.Variant("key1", "val2", "key2", "val2")
				var4 := pbutil.Variant("key1", "val1", "key2", "val2")
				tvb1 := newBuilder().WithProject("myproject").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var1)).WithRefHash(refHash)
				tvb2 := newBuilder().WithProject("myproject").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var2)).WithRefHash(refHash)
				tvb3 := newBuilder().WithProject("myproject").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var3)).WithRefHash(refHash)
				tvb4 := newBuilder().WithProject("myproject").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var4)).WithRefHash(refHash)
				// Different test id, should not appear in the response.
				tvb5 := newBuilder().WithProject("myproject").WithTestID("test://is/a/different/test").WithVariantHash(pbutil.VariantHash(var4)).WithRefHash(refHash)
				// Different ref, should not appear in the response.
				tvb6 := newBuilder().WithProject("myproject").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var4)).WithRefHash([]byte("refhash"))

				var hs inputbuffer.HistorySerializer
				tvb1.saveInDB(ctx, t, hs)
				tvb2.saveInDB(ctx, t, hs)
				tvb3.saveInDB(ctx, t, hs)
				tvb4.saveInDB(ctx, t, hs)
				tvb5.saveInDB(ctx, t, hs)
				tvb6.saveInDB(ctx, t, hs)
				req := &pb.QueryTestVariantBranchRequest{
					Project:   "myproject",
					TestId:    "test://is/a/test",
					Ref:       ref,
					PageSize:  3,
					PageToken: "",
				}

				res, err := server.Query(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.NotEqual(""))
				assert.Loosely(t, res.TestVariantBranch, should.Resemble([]*pb.TestVariantBranch{tvb1.buildProto(), tvb3.buildProto(), tvb4.buildProto()}))

				// Query next page.
				req.PageToken = res.NextPageToken
				res, err = server.Query(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
				assert.Loosely(t, res.TestVariantBranch, should.Resemble([]*pb.TestVariantBranch{tvb2.buildProto()}))
			})
		})

		t.Run("QuerySourcePositions", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{
						Realm:      "project:realm",
						Permission: rdbperms.PermListTestResults,
					},
					{
						Realm:      "project:realm",
						Permission: rdbperms.PermListTestExonerations,
					},
				},
				IdentityGroups: []string{"luci-analysis-access"},
			})
			var1 := pbutil.Variant("key1", "val1", "key2", "val1")
			ref := &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "project",
						Ref:     "ref",
					},
				},
			}
			refhash := hex.EncodeToString(pbutil.SourceRefHash(ref))
			req := &pb.QuerySourcePositionsRequest{
				Project:             "project",
				TestId:              "testid",
				VariantHash:         pbutil.VariantHash(var1),
				RefHash:             refhash,
				StartSourcePosition: 1100,
				PageToken:           "",
				PageSize:            111,
			}

			t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"luci-analysis-access"},
				})
				res, err := server.QuerySourcePositions(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid requests are rejected", func(t *ftt.Test) {
				req.PageSize = -1
				res, err := server.QuerySourcePositions(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("page_size: negative"))
				assert.Loosely(t, res, should.BeNil)
			})

			bqRef := &testresults.BQRef{
				Gitiles: &testresults.BQGitiles{
					Host:    bigquery.NullString{StringVal: "chromium.googlesource.com", Valid: true},
					Project: bigquery.NullString{StringVal: "project", Valid: true},
					Ref:     bigquery.NullString{StringVal: "ref", Valid: true},
				},
			}
			t.Run("no test verdicts that is close enough to start_source_position", func(t *ftt.Test) {
				trc.CommitsWithVerdicts = []*testresults.CommitWithVerdicts{
					// Verdict at position 10990.
					// This is the smallest position that is greater than the requested position.
					{
						Position:     10990, // we need 10990 - 1100 + 111 (10001) commits from gitiles.
						CommitHash:   "commithash",
						Ref:          bqRef,
						TestVerdicts: []*testresults.BQTestVerdict{},
					},
					// Verdict at position 1002.
					{
						Position:     1002,
						CommitHash:   "commithash",
						Ref:          bqRef,
						TestVerdicts: []*testresults.BQTestVerdict{},
					},
				}

				res, err := server.QuerySourcePositions(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)(`cannot find source positions because test verdicts is too sparse`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("no test verdicts after start_source_position", func(t *ftt.Test) {
				trc.CommitsWithVerdicts = []*testresults.CommitWithVerdicts{
					// Verdict at position 1002.
					{
						Position:     1002,
						CommitHash:   "commithash",
						Ref:          bqRef,
						TestVerdicts: []*testresults.BQTestVerdict{},
					},
				}

				res, err := server.QuerySourcePositions(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)(`no commit at or after the requested start position`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("e2e", func(t *ftt.Test) {
				trc.CommitsWithVerdicts = []*testresults.CommitWithVerdicts{
					// Verdict at position 1200.
					// This is the smallest position that is greater than the requested position.
					// We use its commit hash to query gitiles.
					{
						Position:     1200,
						CommitHash:   "commithash",
						Ref:          bqRef,
						TestVerdicts: []*testresults.BQTestVerdict{},
					},
					// Verdict at position 1002.
					// The caller doesn't have access to this verdict, this verdict will be excluded from the response.
					{
						Position:   1002,
						CommitHash: "commithash",
						Ref:        bqRef,
						TestVerdicts: []*testresults.BQTestVerdict{
							{
								TestID:        "testid",
								VariantHash:   pbutil.VariantHash(var1),
								RefHash:       refhash,
								InvocationID:  "invocation-123",
								Status:        "EXPECTED",
								PartitionTime: time.Unix(1000, 0),
								PassedAvgDurationUsec: bigquery.NullFloat64{
									Float64: 0.001,
									Valid:   true,
								},
								Changelists: []*testresults.BQChangelist{},
								HasAccess:   false,
							},
						},
					},
					// Verdict at position 1001.
					// This is within the queried range, verdict will be included in the response.
					{
						Position:   1001,
						CommitHash: "commithash",
						Ref:        bqRef,
						TestVerdicts: []*testresults.BQTestVerdict{
							{
								TestID:        "testid",
								VariantHash:   pbutil.VariantHash(var1),
								RefHash:       refhash,
								InvocationID:  "invocation-123",
								Status:        "EXPECTED",
								PartitionTime: time.Unix(1000, 0),
								PassedAvgDurationUsec: bigquery.NullFloat64{
									Float64: 0.001,
									Valid:   true,
								},
								Changelists: []*testresults.BQChangelist{},
								HasAccess:   true,
							},
						},
					},
				}
				ctx := gitiles.UseFakeClient(ctx, makeFakeCommit)

				res, err := server.QuerySourcePositions(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				cwvs := []*pb.SourcePosition{}
				for i := req.StartSourcePosition; i > req.StartSourcePosition-int64(req.PageSize); i-- {
					cwv := &pb.SourcePosition{
						Commit:   makeFakeCommit(int32(i - req.StartSourcePosition + int64(req.PageSize))),
						Position: i,
					}
					// Attach verdicts.
					if i == 1001 {
						cwv.Verdicts = []*pb.TestVerdict{{
							TestId:            "testid",
							VariantHash:       pbutil.VariantHash(var1),
							InvocationId:      "invocation-123",
							Status:            pb.TestVerdictStatus_EXPECTED,
							PartitionTime:     timestamppb.New(time.Unix(1000, 0)),
							PassedAvgDuration: durationpb.New(time.Duration(1) * time.Millisecond),
							Changelists:       []*pb.Changelist{},
						}}
					}
					cwvs = append(cwvs, cwv)
				}
				// Query commits 1100 to 990 (111 commits). Next page will start from 989.
				nextPageToken := pagination.Token(fmt.Sprintf("%d", 989))
				assert.Loosely(t, res, should.Resemble(&pb.QuerySourcePositionsResponse{
					SourcePositions: cwvs,
					NextPageToken:   nextPageToken,
				}))
			})

		})
		t.Run("QuerySourceVerdicts", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{
						Realm:      "project:realm",
						Permission: rdbperms.PermListTestResults,
					},
					{
						Realm:      "project:realm",
						Permission: rdbperms.PermListTestExonerations,
					},
				},
				IdentityGroups: []string{"luci-analysis-access"},
			})
			var1 := pbutil.Variant("key1", "val1", "key2", "val1")
			ref := &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "project",
						Ref:     "ref",
					},
				},
			}
			varHash := pbutil.VariantHash(var1)
			refHash := pbutil.SourceRefHash(ref)
			req := &pb.QuerySourceVerdictsRequest{
				Parent:              fmt.Sprintf("projects/project/tests/testid/variants/%s/refs/%s", varHash, hex.EncodeToString(refHash)),
				StartSourcePosition: 2000,
				EndSourcePosition:   1000,
			}

			t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"luci-analysis-access"},
				})
				res, err := server.QuerySourceVerdicts(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project "project"`))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid requests are rejected", func(t *ftt.Test) {
				t.Run("invalid parent", func(t *ftt.Test) {
					req.Parent = ""
					res, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`parent: name must be of format projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}`))
					assert.Loosely(t, res, should.BeNil)
				})
				t.Run("invalid start position", func(t *ftt.Test) {
					req.StartSourcePosition = 0
					res, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`start_source_position: must be a positive number`))
					assert.Loosely(t, res, should.BeNil)
				})
				t.Run("invalid end position", func(t *ftt.Test) {
					req.EndSourcePosition = -1
					res, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`end_source_position: must be a non-negative number`))
					assert.Loosely(t, res, should.BeNil)
				})
				t.Run("end position not before start", func(t *ftt.Test) {
					req.StartSourcePosition = 2000
					req.EndSourcePosition = 2000
					res, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`end_source_position: must be less than start_source_position`))
					assert.Loosely(t, res, should.BeNil)
				})
				t.Run("start to end range too large", func(t *ftt.Test) {
					// More than 1000 positions.
					req.StartSourcePosition = 2000
					req.EndSourcePosition = 999
					res, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`end_source_position: must not query more than 1000 source positions from start_source_position`))
					assert.Loosely(t, res, should.BeNil)
				})
			})
			t.Run("valid request", func(t *ftt.Test) {
				t.Run("no test verdicts", func(t *ftt.Test) {
					res, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Resemble(&pb.QuerySourceVerdictsResponse{}))
				})
				t.Run("end source position is zero", func(t *ftt.Test) {
					req.StartSourcePosition = 1000
					req.EndSourcePosition = 0
					res, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Resemble(&pb.QuerySourceVerdictsResponse{}))
				})
				t.Run("test verdicts", func(t *ftt.Test) {
					refTime := testclock.TestRecentTimeLocal
					tc.Set(refTime)

					err := lowlatency.SetForTesting(ctx, t, []*lowlatency.TestResult{
						{
							Project:     "project",
							TestID:      "testid",
							VariantHash: pbutil.VariantHash(var1),
							Sources: testresults.Sources{
								RefHash:  pbutil.SourceRefHash(ref),
								Position: 2000,
							},
							RootInvocationID: "inv-1",
							InvocationID:     "sub-inv",
							ResultID:         "result-id",
							PartitionTime:    refTime.Add(-1 * time.Hour),
							SubRealm:         "realm",
							IsUnexpected:     true,
							Status:           pb.TestResultStatus_FAIL,
						},
						{
							// This result should not be queried from Spanner,
							// it overlaps with the partition time range queried from
							// BigQuery.
							Project:     "project",
							TestID:      "testid",
							VariantHash: pbutil.VariantHash(var1),
							Sources: testresults.Sources{
								Position: 2000,
								RefHash:  pbutil.SourceRefHash(ref),
							},
							RootInvocationID: "inv-2",
							InvocationID:     "sub-inv",
							ResultID:         "result-id",
							PartitionTime:    refTime.Add(-15 * 24 * time.Hour),
							SubRealm:         "realm",
							IsUnexpected:     true,
							Status:           pb.TestResultStatus_FAIL,
						},
					})
					assert.Loosely(t, err, should.BeNil)

					trc.SourceVerdicts = []testresults.SourceVerdict{
						{
							Position: 2000,
							Verdicts: []testresults.SourceVerdictTestVerdict{
								{
									InvocationID:  "inv-5",
									PartitionTime: refTime.Add(-18 * 24 * time.Hour),
									Status:        "SKIPPED",
									Changelists: []testresults.BQChangelist{
										{
											Host:      bigquery.NullString{StringVal: "host", Valid: true},
											Change:    bigquery.NullInt64{Int64: 1234567, Valid: true},
											Patchset:  bigquery.NullInt64{Int64: 890, Valid: true},
											OwnerKind: bigquery.NullString{StringVal: "AUTOMATION", Valid: true},
										},
									},
								},
								{
									InvocationID:  "inv-6",
									PartitionTime: refTime.Add(-17 * 24 * time.Hour),
									Status:        "EXPECTED",
								},
								{
									InvocationID:  "inv-7",
									PartitionTime: refTime.Add(-16 * 24 * time.Hour),
									Status:        "FLAKY",
								},
								{
									InvocationID:  "inv-8",
									PartitionTime: refTime.Add(-15 * 24 * time.Hour),
									Status:        "UNEXPECTED",
								},
							},
						},
						{
							Position: 1100,
							Verdicts: []testresults.SourceVerdictTestVerdict{
								{
									InvocationID:  "inv-9",
									PartitionTime: refTime.Add(-15 * 24 * time.Hour),
									Status:        "UNEXPECTED",
								},
							},
						},
					}

					res, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Resemble(&pb.QuerySourceVerdictsResponse{
						SourceVerdicts: []*pb.QuerySourceVerdictsResponse_SourceVerdict{
							{
								Position: 2000,
								Status:   pb.QuerySourceVerdictsResponse_FLAKY,
								Verdicts: []*pb.QuerySourceVerdictsResponse_TestVerdict{
									{
										InvocationId:  "inv-5",
										PartitionTime: timestamppb.New(refTime.Add(-18 * 24 * time.Hour)),
										Status:        pb.QuerySourceVerdictsResponse_SKIPPED,
										Changelists: []*pb.Changelist{
											{
												Host:      "host",
												Change:    1234567,
												Patchset:  890,
												OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
											},
										},
									},
									{
										InvocationId:  "inv-6",
										PartitionTime: timestamppb.New(refTime.Add(-17 * 24 * time.Hour)),
										Status:        pb.QuerySourceVerdictsResponse_EXPECTED,
									},
									{
										InvocationId:  "inv-7",
										PartitionTime: timestamppb.New(refTime.Add(-16 * 24 * time.Hour)),
										Status:        pb.QuerySourceVerdictsResponse_FLAKY,
									},
									{
										InvocationId:  "inv-8",
										PartitionTime: timestamppb.New(refTime.Add(-15 * 24 * time.Hour)),
										Status:        pb.QuerySourceVerdictsResponse_UNEXPECTED,
									},
									{
										InvocationId:  "inv-1",
										PartitionTime: timestamppb.New(refTime.Add(-1 * time.Hour)),
										Status:        pb.QuerySourceVerdictsResponse_UNEXPECTED,
									},
								},
							},
							{
								Position: 1100,
								Status:   pb.QuerySourceVerdictsResponse_UNEXPECTED,
								Verdicts: []*pb.QuerySourceVerdictsResponse_TestVerdict{
									{
										InvocationId:  "inv-9",
										PartitionTime: timestamppb.New(refTime.Add(-15 * 24 * time.Hour)),
										Status:        pb.QuerySourceVerdictsResponse_UNEXPECTED,
									},
								},
							},
						},
					}))
				})
			})
		})
		t.Run("QueryChangepointAIAnalysis", func(t *ftt.Test) {
			authState := &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityGroups: []string{
					"googlers",
					"luci-analysis-access",
				},
				IdentityPermissions: []authtest.RealmPermission{
					{
						Realm:      "testproject:realm",
						Permission: rdbperms.PermListTestResults,
					},
					{
						Realm:      "testproject:realm",
						Permission: rdbperms.PermListTestExonerations,
					},
				},
			}
			ctx = auth.WithState(ctx, authState)

			var1 := pbutil.Variant("key1", "val1", "key2", "val1")
			ref := &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "host",
						Project: "project",
						Ref:     "ref",
					},
				},
			}
			refHash := pbutil.SourceRefHash(ref)

			tvb1 := newBuilder().WithProject("testproject").WithTestID("mytest").WithVariantHash(pbutil.VariantHash(var1)).WithRefHash(refHash)
			var hs inputbuffer.HistorySerializer
			tvb1.saveInDB(ctx, t, hs)

			req := &pb.QueryChangepointAIAnalysisRequest{
				Project:             "testproject",
				TestId:              "mytest",
				VariantHash:         pbutil.VariantHash(var1),
				RefHash:             hex.EncodeToString(refHash),
				StartSourcePosition: 97, // tvb1 has a changepoint at position 100 (99% confidence interval: 95-105)
			}

			t.Run("permission denied - not in group luci-analysis-accesss", func(t *ftt.Test) {
				authState.IdentityGroups = removeGroup(authState.IdentityGroups, "luci-analysis-access")

				res, err := server.QueryChangepointAIAnalysis(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("not a member of luci-analysis-access"))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("permission denied - not in group googlers", func(t *ftt.Test) {
				authState.IdentityGroups = removeGroup(authState.IdentityGroups, "googlers")

				res, err := server.QueryChangepointAIAnalysis(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("not a member of googlers"))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("permission denied - no permissions to list test results", func(t *ftt.Test) {
				authState.IdentityPermissions = nil

				res, err := server.QueryChangepointAIAnalysis(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project "testproject"`))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("invalid request", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"googlers", "luci-analysis-access"},
				})
				t.Run("invalid project", func(t *ftt.Test) {
					req.Project = ""

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("project"))
				})
				t.Run("invalid test id", func(t *ftt.Test) {
					req.TestId = ""

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("test_id"))
				})
				t.Run("invalid variant hash", func(t *ftt.Test) {
					req.VariantHash = ""

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("variant_hash"))
				})
				t.Run("invalid ref hash", func(t *ftt.Test) {
					req.RefHash = ""

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("ref_hash"))
				})
				t.Run("invalid start source position", func(t *ftt.Test) {
					req.StartSourcePosition = 0

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("start_source_position"))
				})
			})
			t.Run("test variant branch not found", func(t *ftt.Test) {
				req.TestId = "not_exists"

				_, err := server.QueryChangepointAIAnalysis(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)("test variant branch not found"))
			})
			t.Run("changepoint not found", func(t *ftt.Test) {
				// This position is closer to the start of the first segment
				// than it is to the second segment. Just the first segment
				// does not give us enough information to analyse the changepoint,
				// we need the prior segment too.
				req.StartSourcePosition = 11

				_, err := server.QueryChangepointAIAnalysis(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)("test variant branch changepoint not found"))
			})
			t.Run("valid", func(t *ftt.Test) {
				ctx = gitiles.UseFakeClient(ctx, makeFakeCommit)
				tvc.SourceVerdictAfterPosition = &testverdicts.SourceVerdict{
					Position:   110,
					CommitHash: "0011223344556677889900112233445566778899",
					Variant:    `{"key":"value"}`,
					TestLocation: &testverdicts.TestLocation{
						Repo:     "https://chromium.googlesource.com/chromium/src",
						FileName: "//path/to/test",
					},
					Ref: &testverdicts.BQRef{
						Gitiles: &testverdicts.BQGitiles{
							Host:    bigquery.NullString{StringVal: "myproject.googlesource.com", Valid: true},
							Project: bigquery.NullString{StringVal: "project", Valid: true},
							Ref:     bigquery.NullString{StringVal: "refs/heads/main", Valid: true},
						},
					},
					Results: []testverdicts.TestResult{
						{
							ParentInvocationID:   "some-inv",
							ResultID:             "some-result",
							Expected:             false,
							Status:               "FAIL",
							PrimaryFailureReason: bigquery.NullString{StringVal: "[blah.cc(55)] some failure reason", Valid: true},
						},
					},
				}

				sorbetClient.Response.Candidate = "Test response."

				rsp, err := server.QueryChangepointAIAnalysis(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, rsp.Prompt, should.NotBeEmpty)
				assert.Loosely(t, rsp.Prompt, should.ContainSubstring(`testproject`))
				assert.Loosely(t, rsp.Prompt, should.ContainSubstring(`mytest`))
				assert.Loosely(t, rsp.Prompt, should.ContainSubstring(`{"key":"value"}`))
				assert.Loosely(t, rsp.Prompt, should.ContainSubstring("/path/to/test"))
				assert.Loosely(t, rsp.Prompt, should.ContainSubstring("[blah.cc(55)] some failure reason"))
				assert.Loosely(t, rsp.Prompt, should.ContainSubstring("105"))
				assert.Loosely(t, rsp.Prompt, should.ContainSubstring("95"))
				assert.Loosely(t, rsp.AnalysisMarkdown, should.Equal("Test response."))
			})
		})
	})
}

func TestValidateQuerySourcePositionsRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("validateQuerySourcePositionsRequest", t, func(t *ftt.Test) {
		ref := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "host",
					Project: "project",
					Ref:     "ref",
				},
			},
		}
		refhash := hex.EncodeToString(pbutil.SourceRefHash(ref))
		req := &pb.QuerySourcePositionsRequest{
			Project:             "project",
			TestId:              "testid",
			VariantHash:         pbutil.VariantHash(pbutil.Variant("key1", "val1", "key2", "val1")),
			RefHash:             refhash,
			StartSourcePosition: 110,
			PageToken:           "",
			PageSize:            1,
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no project", func(t *ftt.Test) {
			req.Project = ""
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.ErrLike("project: unspecified"))
		})

		t.Run("invalid project", func(t *ftt.Test) {
			req.Project = "project:realm"
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
		})

		t.Run("no test id", func(t *ftt.Test) {
			req.TestId = ""
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id: unspecified"))
		})

		t.Run("invalid test id", func(t *ftt.Test) {
			req.TestId = "\xFF"
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id: not a valid utf8 string"))
		})

		t.Run("invalid variant hash", func(t *ftt.Test) {
			req.VariantHash = "invalid"
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.ErrLike("variant_hash"))
			assert.Loosely(t, err, should.ErrLike("must match ^[0-9a-f]{16}$"))
		})

		t.Run("invalid ref hash", func(t *ftt.Test) {
			req.RefHash = "invalid"
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.ErrLike("ref_hash:"))
			assert.Loosely(t, err, should.ErrLike("must match ^[0-9a-f]{16}$"))
		})

		t.Run("invalid start commit position", func(t *ftt.Test) {
			req.StartSourcePosition = 0
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.ErrLike("start_source_position: must be a positive number"))
		})

		t.Run("no page size", func(t *ftt.Test) {
			req.PageSize = 0
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("negative page size", func(t *ftt.Test) {
			req.PageSize = -1
			err := validateQuerySourcePositionsRequest(req)
			assert.Loosely(t, err, should.ErrLike("page_size"))
			assert.Loosely(t, err, should.ErrLike("negative"))
		})
	})
}

type testVariantBranchBuilder struct {
	project     string
	testID      string
	variantHash string
	refHash     []byte
}

func newBuilder() *testVariantBranchBuilder {
	return &testVariantBranchBuilder{}
}

func (b *testVariantBranchBuilder) WithProject(project string) *testVariantBranchBuilder {
	b.project = project
	return b
}

func (b *testVariantBranchBuilder) WithTestID(testID string) *testVariantBranchBuilder {
	b.testID = testID
	return b
}

func (b *testVariantBranchBuilder) WithVariantHash(variantHash string) *testVariantBranchBuilder {
	b.variantHash = variantHash
	return b
}

func (b *testVariantBranchBuilder) WithRefHash(refHash []byte) *testVariantBranchBuilder {
	b.refHash = refHash
	return b
}

func (b *testVariantBranchBuilder) buildProto() *pb.TestVariantBranch {
	refHash := hex.EncodeToString(b.refHash)
	return &pb.TestVariantBranch{
		Name:        testVariantBranchName(b.project, b.testID, b.variantHash, refHash),
		Project:     b.project,
		TestId:      b.testID,
		VariantHash: b.variantHash,
		RefHash:     refHash,
		Ref: &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
				},
			},
		},
		Variant: &pb.Variant{
			Def: map[string]string{
				"k": "v",
			},
		},
		Segments: []*pb.Segment{
			{
				HasStartChangepoint:          true,
				StartPosition:                100,
				StartPositionLowerBound_99Th: 95,
				StartPositionUpperBound_99Th: 105,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				EndPosition:                  200,
				EndHour:                      timestamppb.New(time.Unix(3*3600, 0)),
				Counts: &pb.Segment_Counts{
					ExpectedPassedResults: 1,
					TotalResults:          1,

					TotalRuns: 1,

					UnexpectedVerdicts: 1,
					FlakyVerdicts:      1,
					TotalVerdicts:      2,
				},
			},
			{
				StartPosition:                10,
				StartPositionLowerBound_99Th: 45,
				StartPositionUpperBound_99Th: 55,
				StartHour:                    timestamppb.New(time.Unix(3600, 0)),
				EndHour:                      timestamppb.New(time.Unix(0, 0)),
				Counts: &pb.Segment_Counts{
					UnexpectedResults:        10,
					TotalResults:             20,
					ExpectedPassedResults:    1,
					ExpectedFailedResults:    2,
					ExpectedCrashedResults:   3,
					ExpectedAbortedResults:   4,
					UnexpectedPassedResults:  5,
					UnexpectedFailedResults:  6,
					UnexpectedCrashedResults: 7,
					UnexpectedAbortedResults: 8,

					UnexpectedUnretriedRuns:  1,
					UnexpectedAfterRetryRuns: 2,
					FlakyRuns:                3,
					TotalRuns:                4,

					UnexpectedVerdicts: 2,
					FlakyVerdicts:      0,
					TotalVerdicts:      2,
				},
			},
		},
	}
}

func makeFakeCommit(i int32) *git.Commit {
	return &git.Commit{
		Id:      fmt.Sprintf("id %d", i),
		Tree:    "tree",
		Parents: []string{},
		Author: &git.Commit_User{
			Name:  "userX",
			Email: "userx@google.com",
			Time:  timestamppb.New(time.Unix(1000, 0)),
		},
		Committer: &git.Commit_User{
			Name:  "userY",
			Email: "usery@google.com",
			Time:  timestamppb.New(time.Unix(1100, 0)),
		},
		Message: fmt.Sprintf("message %d", i),
		TreeDiff: []*git.Commit_TreeDiff{
			{
				Type:    git.Commit_TreeDiff_DELETE,
				OldPath: "/deleted/path",
				NewPath: "/dev/null",
			},
			{
				Type:    git.Commit_TreeDiff_MODIFY,
				OldPath: "/modified/path",
				NewPath: "/modified/path",
			},
			{
				Type:    git.Commit_TreeDiff_ADD,
				NewPath: "/dev/null",
				OldPath: "/new/path",
			},
		},
	}
}

func (b *testVariantBranchBuilder) saveInDB(ctx context.Context, t testing.TB, hs inputbuffer.HistorySerializer) {
	t.Helper()
	mutation, err := b.buildEntry().ToMutation(&hs)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	testutil.MustApply(ctx, t, mutation)
}

func (b *testVariantBranchBuilder) buildEntry() *testvariantbranch.Entry {
	return &testvariantbranch.Entry{
		IsNew:       true,
		Project:     b.project,
		TestID:      b.testID,
		VariantHash: b.variantHash,
		RefHash:     b.refHash,
		SourceRef: &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
				},
			},
		},
		Variant: &pb.Variant{
			Def: map[string]string{
				"k": "v",
			},
		},
		InputBuffer: &inputbuffer.Buffer{
			HotBuffer: inputbuffer.History{
				Runs: []inputbuffer.Run{
					{
						CommitPosition: 200,
						Hour:           time.Unix(3*3600, 0),
						Expected: inputbuffer.ResultCounts{
							PassCount: 1,
						},
					},
				},
			},
			ColdBuffer: inputbuffer.History{
				Runs: []inputbuffer.Run{},
			},
		},
		FinalizingSegment: &cpb.Segment{
			State:                        cpb.SegmentState_FINALIZING,
			HasStartChangepoint:          true,
			StartPosition:                100,
			StartHour:                    timestamppb.New(time.Unix(3600, 0)),
			StartPositionLowerBound_99Th: 95,
			StartPositionUpperBound_99Th: 105,
			FinalizedCounts: &cpb.Counts{
				UnexpectedSourceVerdicts: 1,
				TotalSourceVerdicts:      1,
				PartialSourceVerdict: &cpb.PartialSourceVerdict{
					CommitPosition:    200,
					LastHour:          timestamppb.New(time.Unix(7200, 0)),
					UnexpectedResults: 1,
				},
			},
		},
		FinalizedSegments: &cpb.Segments{
			Segments: []*cpb.Segment{
				{
					State:                        cpb.SegmentState_FINALIZED,
					StartPosition:                10,
					StartHour:                    timestamppb.New(time.Unix(3600, 0)),
					StartPositionLowerBound_99Th: 45,
					StartPositionUpperBound_99Th: 55,
					FinalizedCounts: &cpb.Counts{
						UnexpectedResults:        10,
						TotalResults:             20,
						ExpectedPassedResults:    1,
						ExpectedFailedResults:    2,
						ExpectedCrashedResults:   3,
						ExpectedAbortedResults:   4,
						UnexpectedPassedResults:  5,
						UnexpectedFailedResults:  6,
						UnexpectedCrashedResults: 7,
						UnexpectedAbortedResults: 8,

						UnexpectedUnretriedRuns:  1,
						UnexpectedAfterRetryRuns: 2,
						FlakyRuns:                3,
						TotalRuns:                4,

						UnexpectedSourceVerdicts: 2,
						TotalSourceVerdicts:      2,
					},
				},
			},
		},
	}
}
