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
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/sorbet"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/gitiles"
	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTestVariantBranchesServer(t *testing.T) {
	Convey("TestVariantBranchesServer", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal)

		tvc := testverdicts.FakeReadClient{}
		trc := testresults.FakeReadClient{}
		sorbetClient := sorbet.NewFakeClient()

		server := NewTestVariantBranchesServer(&tvc, &trc, sorbetClient)
		Convey("GetRaw", func() {
			Convey("permission denied", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "anonymous:anonymous",
				})
				req := &pb.GetRawTestVariantBranchRequest{}
				res, err := server.GetRaw(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
				So(res, ShouldBeNil)
			})

			Convey("invalid request", func() {
				ctx = adminContext(ctx)
				req := &pb.GetRawTestVariantBranchRequest{
					Name: "Project/abc/xyz",
				}
				res, err := server.GetRaw(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
				So(res, ShouldBeNil)
			})

			Convey("not found", func() {
				ctx = adminContext(ctx)
				req := &pb.GetRawTestVariantBranchRequest{
					Name: "projects/project/tests/test/variants/abababababababab/refs/abababababababab",
				}
				res, err := server.GetRaw(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.NotFound)
				So(res, ShouldBeNil)
			})

			Convey("invalid ref_hash", func() {
				ctx = adminContext(ctx)
				req := &pb.GetRawTestVariantBranchRequest{
					Name: "projects/project/tests/this//is/a/test/variants/abababababababab/refs/abababababababgh",
				}
				res, err := server.GetRaw(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
				So(res, ShouldBeNil)
			})

			Convey("invalid test id", func() {
				ctx = adminContext(ctx)
				Convey("bad structure", func() {
					ctx = adminContext(ctx)
					req := &pb.GetRawTestVariantBranchRequest{
						Name: "projects/project/tests/a/variants/0123456789abcdef/refs/7265665f68617368/bad/subpath",
					}
					res, err := server.GetRaw(ctx, req)
					So(err, ShouldNotBeNil)
					So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
					So(err, ShouldErrLike, "name must be of format projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}")
					So(res, ShouldBeNil)
				})
				Convey("bad URL escaping", func() {
					req := &pb.GetRawTestVariantBranchRequest{
						Name: "projects/project/tests/abcdef%test/variants/0123456789abcdef/refs/7265665f68617368",
					}
					res, err := server.GetRaw(ctx, req)
					So(err, ShouldNotBeNil)
					So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
					So(err, ShouldErrLike, "malformed test id: invalid URL escape \"%te\"")
					So(res, ShouldBeNil)
				})
				Convey("bad value", func() {
					req := &pb.GetRawTestVariantBranchRequest{
						Name: "projects/project/tests/\u0001atest/variants/0123456789abcdef/refs/7265665f68617368",
					}
					res, err := server.GetRaw(ctx, req)
					So(err, ShouldNotBeNil)
					So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
					So(err, ShouldErrLike, `test id "\x01atest": non-printable rune`)
					So(res, ShouldBeNil)
				})
			})
			Convey("ok", func() {
				ctx = adminContext(ctx)
				// Insert test variant branch to Spanner.
				tvb := &testvariantbranch.Entry{
					IsNew:       true,
					Project:     "project",
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
				So(err, ShouldBeNil)
				testutil.MustApply(ctx, mutation)

				hexStr := "7265665f68617368" // hex string of "ref_hash".
				req := &pb.GetRawTestVariantBranchRequest{
					Name: "projects/project/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
				}
				res, err := server.GetRaw(ctx, req)
				So(err, ShouldBeNil)

				expectedFinalizingSegment, err := anypb.New(tvb.FinalizingSegment)
				So(err, ShouldBeNil)

				expectedFinalizedSegments, err := anypb.New(tvb.FinalizedSegments)
				So(err, ShouldBeNil)

				expectedStatistics, err := anypb.New(tvb.Statistics)
				So(err, ShouldBeNil)

				So(res, ShouldResembleProto, &pb.TestVariantBranchRaw{
					Name:              "projects/project/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
					Project:           "project",
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
				})
			})
		})

		Convey("BatchGet", func() {
			Convey("permission denied", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"luci-analysis-access"},
				})
				req := &pb.BatchGetTestVariantBranchRequest{}

				res, err := server.BatchGet(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
				So(res, ShouldBeNil)
			})

			Convey("invalid request", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"googlers", "luci-analysis-access"},
				})
				Convey("invalid name", func() {
					req := &pb.BatchGetTestVariantBranchRequest{
						Names: []string{"projects/abc/xyz"},
					}

					res, err := server.BatchGet(ctx, req)
					So(err, ShouldNotBeNil)
					So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
					So(res, ShouldBeNil)
				})

				Convey("too many test variant branch requested", func() {
					names := []string{}
					for i := 0; i < 200; i++ {
						names = append(names, "projects/project/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368")
					}
					req := &pb.BatchGetTestVariantBranchRequest{
						Names: names,
					}

					res, err := server.BatchGet(ctx, req)
					So(err, ShouldNotBeNil)
					So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
					So(err, ShouldErrLike, "names: no more than 100 may be queried at a time")
					So(res, ShouldBeNil)
				})
			})

			Convey("e2e", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"googlers", "luci-analysis-access"},
				})
				// Insert test variant branch to Spanner.
				tvb := &testvariantbranch.Entry{
					IsNew:       true,
					Project:     "project",
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
								State:                        cpb.SegmentState_FINALIZED,
								StartPosition:                50,
								StartHour:                    timestamppb.New(time.Unix(3600, 0)),
								StartPositionLowerBound_99Th: 45,
								StartPositionUpperBound_99Th: 55,
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
				So(err, ShouldBeNil)
				testutil.MustApply(ctx, mutation)
				req := &pb.BatchGetTestVariantBranchRequest{
					Names: []string{
						"projects/project/tests/not%2Fexist%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
						"projects/project/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
					},
				}

				res, err := server.BatchGet(ctx, req)
				So(err, ShouldBeNil)
				So(res.TestVariantBranches, ShouldHaveLength, 2)
				So(res.TestVariantBranches[0], ShouldBeNil)
				So(res.TestVariantBranches[1], ShouldResembleProto, &pb.TestVariantBranch{
					Name:        "projects/project/tests/this%2F%2Fis%2Fa%2Ftest/variants/0123456789abcdef/refs/7265665f68617368",
					Project:     "project",
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
							StartHour:                    timestamppb.New(time.Unix(3600, 0)),
							EndPosition:                  200,
							EndHour:                      timestamppb.New(time.Unix(2*3600, 0)),
							Counts: &pb.Segment_Counts{
								UnexpectedVerdicts: 1,
								FlakyVerdicts:      0,
								TotalVerdicts:      3,
							},
						},
						{
							StartPositionLowerBound_99Th: 45,
							StartPositionUpperBound_99Th: 55,
							StartPosition:                50,
							StartHour:                    timestamppb.New(time.Unix(3600, 0)),
							EndHour:                      timestamppb.New(time.Unix(0, 0)),
							Counts: &pb.Segment_Counts{
								UnexpectedVerdicts: 2,
								FlakyVerdicts:      0,
								TotalVerdicts:      2,
							},
						},
					},
				})
			})
		})

		Convey("Query", func() {
			Convey("permission denied", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "anonymous:anonymous",
				})
				req := &pb.QueryTestVariantBranchRequest{}

				res, err := server.Query(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
				So(res, ShouldBeNil)
			})
			Convey("invalid request", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"googlers", "luci-analysis-access"},
				})
				req := &pb.QueryTestVariantBranchRequest{
					Project: "project",
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
				Convey("invalid project", func() {
					req.Project = ""

					_, err := server.Query(ctx, req)
					So(err, ShouldNotBeNil)
					So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
					So(err.Error(), ShouldContainSubstring, "project")
				})
				Convey("invalid test id", func() {
					req.TestId = ""

					_, err := server.Query(ctx, req)
					So(err, ShouldNotBeNil)
					So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
					So(err.Error(), ShouldContainSubstring, "test_id")
				})
				Convey("invalid ref", func() {
					req.Ref.GetGitiles().Host = ""

					_, err := server.Query(ctx, req)
					So(err, ShouldNotBeNil)
					So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
					So(err.Error(), ShouldContainSubstring, "host")
				})
			})
			Convey("e2e", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"googlers", "luci-analysis-access"},
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
				tvb1 := newBuilder().WithProject("project").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var1)).WithRefHash(refHash)
				tvb2 := newBuilder().WithProject("project").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var2)).WithRefHash(refHash)
				tvb3 := newBuilder().WithProject("project").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var3)).WithRefHash(refHash)
				tvb4 := newBuilder().WithProject("project").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var4)).WithRefHash(refHash)
				// Different test id, should not appear in the response.
				tvb5 := newBuilder().WithProject("project").WithTestID("test://is/a/different/test").WithVariantHash(pbutil.VariantHash(var4)).WithRefHash(refHash)
				// Different ref, should not appear in the response.
				tvb6 := newBuilder().WithProject("project").WithTestID("test://is/a/test").WithVariantHash(pbutil.VariantHash(var4)).WithRefHash([]byte("refhash"))

				var hs inputbuffer.HistorySerializer
				tvb1.saveInDB(ctx, hs)
				tvb2.saveInDB(ctx, hs)
				tvb3.saveInDB(ctx, hs)
				tvb4.saveInDB(ctx, hs)
				tvb5.saveInDB(ctx, hs)
				tvb6.saveInDB(ctx, hs)
				req := &pb.QueryTestVariantBranchRequest{
					Project:   "project",
					TestId:    "test://is/a/test",
					Ref:       ref,
					PageSize:  3,
					PageToken: "",
				}

				res, err := server.Query(ctx, req)
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldNotEqual, "")
				So(res.TestVariantBranch, ShouldResembleProto, []*pb.TestVariantBranch{tvb1.buildProto(), tvb3.buildProto(), tvb4.buildProto()})

				// Query next page.
				req.PageToken = res.NextPageToken
				res, err = server.Query(ctx, req)
				So(err, ShouldBeNil)
				So(res.NextPageToken, ShouldEqual, "")
				So(res.TestVariantBranch, ShouldResembleProto, []*pb.TestVariantBranch{tvb2.buildProto()})
			})
		})

		Convey("QuerySourcePositions", func() {
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

			Convey("unauthorised requests are rejected", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"luci-analysis-access"},
				})
				res, err := server.QuerySourcePositions(ctx, req)
				So(err, ShouldErrLike, `caller does not have permission`, `in any realm in project "project"`)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
				So(res, ShouldBeNil)
			})

			Convey("invalid requests are rejected", func() {
				req.PageSize = -1
				res, err := server.QuerySourcePositions(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
				So(res, ShouldBeNil)
			})

			bqRef := &testresults.BQRef{
				Gitiles: &testresults.BQGitiles{
					Host:    bigquery.NullString{StringVal: "chromium.googlesource.com", Valid: true},
					Project: bigquery.NullString{StringVal: "project", Valid: true},
					Ref:     bigquery.NullString{StringVal: "ref", Valid: true},
				},
			}
			Convey("no test verdicts that is close enough to start_source_position", func() {
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
				So(err, ShouldNotBeNil)
				So(err, ShouldErrLike, `cannot find source positions because test verdicts is too sparse`)
				So(err, ShouldHaveGRPCStatus, codes.NotFound)
				So(res, ShouldBeNil)
			})

			Convey("no test verdicts after start_source_position", func() {
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
				So(err, ShouldNotBeNil)
				So(err, ShouldErrLike, `no commit at or after the requested start position`)
				So(err, ShouldHaveGRPCStatus, codes.NotFound)
				So(res, ShouldBeNil)
			})

			Convey("e2e", func() {
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
				So(err, ShouldBeNil)
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
				So(res, ShouldResembleProto, &pb.QuerySourcePositionsResponse{
					SourcePositions: cwvs,
					NextPageToken:   nextPageToken,
				})
			})

		})
		Convey("QuerySourceVerdicts", func() {
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

			Convey("unauthorised requests are rejected", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"luci-analysis-access"},
				})
				res, err := server.QuerySourceVerdicts(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project "project"`)
				So(res, ShouldBeNil)
			})

			Convey("invalid requests are rejected", func() {
				Convey("invalid parent", func() {
					req.Parent = ""
					res, err := server.QuerySourceVerdicts(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, `parent: name must be of format projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}`)
					So(res, ShouldBeNil)
				})
				Convey("invalid start position", func() {
					req.StartSourcePosition = 0
					res, err := server.QuerySourceVerdicts(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, `start_source_position: must be a positive number`)
					So(res, ShouldBeNil)
				})
				Convey("invalid end position", func() {
					req.EndSourcePosition = 0
					res, err := server.QuerySourceVerdicts(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, `end_source_position: must be a positive number`)
					So(res, ShouldBeNil)
				})
				Convey("end position not before start", func() {
					req.StartSourcePosition = 2000
					req.EndSourcePosition = 2000
					res, err := server.QuerySourceVerdicts(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, `end_source_position: must be less than start_source_position`)
					So(res, ShouldBeNil)
				})
				Convey("start to end range too large", func() {
					// More than 1000 positions.
					req.StartSourcePosition = 2000
					req.EndSourcePosition = 999
					res, err := server.QuerySourceVerdicts(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, `end_source_position: must not query more than 1000 source positions from start_source_position`)
					So(res, ShouldBeNil)
				})
			})
			Convey("valid request", func() {
				Convey("no test verdicts", func() {
					res, err := server.QuerySourceVerdicts(ctx, req)
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &pb.QuerySourceVerdictsResponse{})
				})
				Convey("test verdicts", func() {
					refTime := testclock.TestRecentTimeLocal
					tc.Set(refTime)

					err := lowlatency.SetForTesting(ctx, []*lowlatency.TestResult{
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
					So(err, ShouldBeNil)

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
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &pb.QuerySourceVerdictsResponse{
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
					})
				})
			})
		})
		Convey("QueryChangepointAIAnalysis", func() {
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
			tvb1.saveInDB(ctx, hs)

			req := &pb.QueryChangepointAIAnalysisRequest{
				Project:             "testproject",
				TestId:              "mytest",
				VariantHash:         pbutil.VariantHash(var1),
				RefHash:             hex.EncodeToString(refHash),
				StartSourcePosition: 97, // tvb1 has a changepoint at position 100 (99% confidence interval: 95-105)
			}

			Convey("permission denied - not in group luci-analysis-accesss", func() {
				authState.IdentityGroups = removeGroup(authState.IdentityGroups, "luci-analysis-access")

				res, err := server.QueryChangepointAIAnalysis(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, "not a member of luci-analysis-access")
				So(res, ShouldBeNil)
			})
			Convey("permission denied - not in group googlers", func() {
				authState.IdentityGroups = removeGroup(authState.IdentityGroups, "googlers")

				res, err := server.QueryChangepointAIAnalysis(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, "not a member of googlers")
				So(res, ShouldBeNil)
			})
			Convey("permission denied - no permissions to list test results", func() {
				authState.IdentityPermissions = nil

				res, err := server.QueryChangepointAIAnalysis(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project "testproject"`)
				So(res, ShouldBeNil)
			})
			Convey("invalid request", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"googlers", "luci-analysis-access"},
				})
				Convey("invalid project", func() {
					req.Project = ""

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, "project")
				})
				Convey("invalid test id", func() {
					req.TestId = ""

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, "test_id")
				})
				Convey("invalid variant hash", func() {
					req.VariantHash = ""

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, "variant_hash")
				})
				Convey("invalid ref hash", func() {
					req.RefHash = ""

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, "ref_hash")
				})
				Convey("invalid start source position", func() {
					req.StartSourcePosition = 0

					_, err := server.QueryChangepointAIAnalysis(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, "start_source_position")
				})
			})
			Convey("test variant branch not found", func() {
				req.TestId = "not_exists"

				_, err := server.QueryChangepointAIAnalysis(ctx, req)
				So(err, ShouldBeRPCNotFound, "test variant branch not found")
			})
			Convey("changepoint not found", func() {
				// This position is closer to the start of the first segment
				// than it is to the second segment. Just the first segment
				// does not give us enough information to analyse the changepoint,
				// we need the prior segment too.
				req.StartSourcePosition = 11

				_, err := server.QueryChangepointAIAnalysis(ctx, req)
				So(err, ShouldBeRPCNotFound, "test variant branch changepoint not found")
			})
			Convey("valid", func() {
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
				So(err, ShouldBeNil)

				So(rsp.Prompt, ShouldNotBeEmpty)
				So(rsp.Prompt, ShouldContainSubstring, `testproject`)
				So(rsp.Prompt, ShouldContainSubstring, `mytest`)
				So(rsp.Prompt, ShouldContainSubstring, `{"key":"value"}`)
				So(rsp.Prompt, ShouldContainSubstring, "/path/to/test")
				So(rsp.Prompt, ShouldContainSubstring, "[blah.cc(55)] some failure reason")
				So(rsp.Prompt, ShouldContainSubstring, "105")
				So(rsp.Prompt, ShouldContainSubstring, "95")
				So(rsp.AnalysisMarkdown, ShouldEqual, "Test response.")
			})
		})
	})
}

func TestValidateQuerySourcePositionsRequest(t *testing.T) {
	t.Parallel()

	Convey("validateQuerySourcePositionsRequest", t, func() {
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

		Convey("valid", func() {
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("no project", func() {
			req.Project = ""
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldErrLike, "project: unspecified")
		})

		Convey("invalid project", func() {
			req.Project = "project:realm"
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
		})

		Convey("no test id", func() {
			req.TestId = ""
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldErrLike, "test_id: unspecified")
		})

		Convey("invalid test id", func() {
			req.TestId = "\xFF"
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldErrLike, "test_id: not a valid utf8 string")
		})

		Convey("invalid variant hash", func() {
			req.VariantHash = "invalid"
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldErrLike, "variant_hash", "must match ^[0-9a-f]{16}$")
		})

		Convey("invalid ref hash", func() {
			req.RefHash = "invalid"
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldErrLike, "ref_hash:", "must match ^[0-9a-f]{16}$")
		})

		Convey("invalid start commit position", func() {
			req.StartSourcePosition = 0
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldErrLike, "start_source_position: must be a positive number")
		})

		Convey("no page size", func() {
			req.PageSize = 0
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("negative page size", func() {
			req.PageSize = -1
			err := validateQuerySourcePositionsRequest(req)
			So(err, ShouldErrLike, "page_size", "negative")
		})
	})
}

func adminContext(ctx context.Context) context.Context {
	return auth.WithState(ctx, &authtest.FakeState{
		Identity:       "user:admin@example.com",
		IdentityGroups: []string{"service-luci-analysis-admins", "luci-analysis-access"},
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

func (b *testVariantBranchBuilder) saveInDB(ctx context.Context, hs inputbuffer.HistorySerializer) {
	mutation, err := b.buildEntry().ToMutation(&hs)
	So(err, ShouldBeNil)
	testutil.MustApply(ctx, mutation)
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
						UnexpectedSourceVerdicts: 2,
						TotalSourceVerdicts:      2,
					},
				},
			},
		},
	}
}
