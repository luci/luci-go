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

	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/gitiles"
	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTestVariantAnalysesServer(t *testing.T) {
	Convey("TestVariantAnalysesServer", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		tvc := testverdicts.FakeReadClient{}
		server := NewTestVariantBranchesServer(&tvc)
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
							Verdicts: []inputbuffer.PositionVerdict{
								{
									CommitPosition:       20,
									IsSimpleExpectedPass: true,
									Hour:                 time.Unix(3600, 0),
								},
							},
						},
						ColdBuffer: inputbuffer.History{
							Verdicts: []inputbuffer.PositionVerdict{
								{
									CommitPosition: 30,
									Hour:           time.Unix(7200, 0),
									Details: inputbuffer.VerdictDetails{
										IsExonerated: true,
										Runs: []inputbuffer.Run{
											{
												Expected: inputbuffer.ResultCounts{
													PassCount: 1,
													FailCount: 2,
												},
												Unexpected: inputbuffer.ResultCounts{
													CrashCount: 3,
													AbortCount: 4,
												},
												IsDuplicate: true,
											},
											{
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
								Hour:               123456,
								UnexpectedVerdicts: 1,
								FlakyVerdicts:      3,
								TotalVerdicts:      12,
							},
							{
								Hour:               123500,
								UnexpectedVerdicts: 3,
								FlakyVerdicts:      7,
								TotalVerdicts:      93,
							},
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
						Verdicts: []*pb.PositionVerdict{
							{
								CommitPosition: 20,
								Hour:           timestamppb.New(time.Unix(3600, 0)),
								Runs: []*pb.PositionVerdict_Run{
									{
										ExpectedPassCount: 1,
									},
								},
							},
						},
					},
					ColdBuffer: &pb.InputBuffer{
						Length: 1,
						Verdicts: []*pb.PositionVerdict{
							{
								CommitPosition: 30,
								Hour:           timestamppb.New(time.Unix(7200, 0)),
								IsExonerated:   true,
								Runs: []*pb.PositionVerdict_Run{
									{
										ExpectedPassCount:    1,
										ExpectedFailCount:    2,
										UnexpectedCrashCount: 3,
										UnexpectedAbortCount: 4,
										IsDuplicate:          true,
									},
									{
										ExpectedCrashCount:   5,
										ExpectedAbortCount:   6,
										UnexpectedPassCount:  7,
										UnexpectedAbortCount: 8,
									},
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
							Verdicts: []inputbuffer.PositionVerdict{
								{
									CommitPosition:       200,
									IsSimpleExpectedPass: true,
									Hour:                 time.Unix(3700, 0),
								},
							},
						},
						ColdBuffer: inputbuffer.History{
							Verdicts: []inputbuffer.PositionVerdict{},
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
							UnexpectedVerdicts: 1,
							TotalVerdicts:      1,
						},
					},
					FinalizedSegments: &cpb.Segments{
						Segments: []*cpb.Segment{
							{
								State:                        cpb.SegmentState_FINALIZED,
								StartHour:                    timestamppb.New(time.Unix(3600, 0)),
								StartPositionLowerBound_99Th: 45,
								StartPositionUpperBound_99Th: 55,
								FinalizedCounts: &cpb.Counts{
									UnexpectedVerdicts: 2,
									TotalVerdicts:      2,
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
							EndHour:                      timestamppb.New(time.Unix(3600, 0)),
							Counts: &pb.Segment_Counts{
								UnexpectedVerdicts: 1,
								FlakyVerdicts:      0,
								TotalVerdicts:      2,
							},
						},
						{
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
				})
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

			bqRef := &testverdicts.Ref{
				Gitiles: &testverdicts.Gitiles{
					Host:    bigquery.NullString{StringVal: "chromium.googlesource.com", Valid: true},
					Project: bigquery.NullString{StringVal: "project", Valid: true},
					Ref:     bigquery.NullString{StringVal: "ref", Valid: true},
				},
			}
			Convey("no test verdicts that is close enough to start_source_position", func() {
				tvc.CommitsWithVerdicts = []*testverdicts.CommitWithVerdicts{
					// Verdict at position 10990.
					// This is the smallest position that is greater than the requested position.
					{
						Position:     10990, // we need 10990 - 1100 + 111 (10001) commits from gitiles.
						CommitHash:   "commithash",
						Ref:          bqRef,
						TestVerdicts: []*testverdicts.TestVerdict{},
					},
					// Verdict at position 1002.
					{
						Position:     1002,
						CommitHash:   "commithash",
						Ref:          bqRef,
						TestVerdicts: []*testverdicts.TestVerdict{},
					},
				}

				res, err := server.QuerySourcePositions(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldErrLike, `cannot find source positions because test verdicts is too sparse`)
				So(err, ShouldHaveGRPCStatus, codes.NotFound)
				So(res, ShouldBeNil)
			})

			Convey("no test verdicts after start_source_position", func() {
				tvc.CommitsWithVerdicts = []*testverdicts.CommitWithVerdicts{
					// Verdict at position 1002.
					{
						Position:     1002,
						CommitHash:   "commithash",
						Ref:          bqRef,
						TestVerdicts: []*testverdicts.TestVerdict{},
					},
				}

				res, err := server.QuerySourcePositions(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldErrLike, `no commit at or after the requested start position`)
				So(err, ShouldHaveGRPCStatus, codes.NotFound)
				So(res, ShouldBeNil)
			})

			Convey("e2e", func() {
				tvc.CommitsWithVerdicts = []*testverdicts.CommitWithVerdicts{
					// Verdict at position 1200.
					// This is the smallest position that is greater than the requested position.
					// We use its commit hash to query gitiles.
					{
						Position:     1200,
						CommitHash:   "commithash",
						Ref:          bqRef,
						TestVerdicts: []*testverdicts.TestVerdict{},
					},
					// Verdict at position 1002.
					// The caller doesn't have access to this verdict, this verdict will be excluded from the response.
					{
						Position:   1002,
						CommitHash: "commithash",
						Ref:        bqRef,
						TestVerdicts: []*testverdicts.TestVerdict{
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
								Changelists: []*testverdicts.Changelist{},
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
						TestVerdicts: []*testverdicts.TestVerdict{
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
								Changelists: []*testverdicts.Changelist{},
								HasAccess:   true,
							},
						},
					},
				}
				makeCommit := func(i int32) *git.Commit {
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
					}
				}
				ctx := gitiles.UseFakeClient(ctx, makeCommit)

				res, err := server.QuerySourcePositions(ctx, req)
				So(err, ShouldBeNil)
				cwvs := []*pb.SourcePosition{}
				for i := req.StartSourcePosition; i > req.StartSourcePosition-int64(req.PageSize); i-- {
					cwv := &pb.SourcePosition{
						Commit:   makeCommit(int32(i - req.StartSourcePosition + int64(req.PageSize))),
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
