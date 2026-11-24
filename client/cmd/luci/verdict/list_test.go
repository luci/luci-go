// Copyright 2026 The LUCI Authors.
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

package verdict

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockListResultDBClient struct {
	pb.ResultDBClient
	getRootInvocation     func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation         func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
	queryTestVerdicts     func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error)
	queryTestVariants     func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error)
	queryTestAggregations func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error)
}

func (m *mockListResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in)
	}
	return &pb.RootInvocation{Name: in.Name}, nil
}

func (m *mockListResultDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in)
	}
	return &pb.Invocation{Name: in.Name}, nil
}

func (m *mockListResultDBClient) QueryTestVerdicts(ctx context.Context, in *pb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*pb.QueryTestVerdictsResponse, error) {
	if m.queryTestVerdicts != nil {
		return m.queryTestVerdicts(ctx, in)
	}
	return &pb.QueryTestVerdictsResponse{}, nil
}

func (m *mockListResultDBClient) QueryTestVariants(ctx context.Context, in *pb.QueryTestVariantsRequest, opts ...grpc.CallOption) (*pb.QueryTestVariantsResponse, error) {
	if m.queryTestVariants != nil {
		return m.queryTestVariants(ctx, in)
	}
	return &pb.QueryTestVariantsResponse{}, nil
}

func (m *mockListResultDBClient) QueryTestAggregations(ctx context.Context, in *pb.QueryTestAggregationsRequest, opts ...grpc.CallOption) (*pb.QueryTestAggregationsResponse, error) {
	if m.queryTestAggregations != nil {
		return m.queryTestAggregations(ctx, in)
	}
	return &pb.QueryTestAggregationsResponse{}, nil
}

func TestListCmd(t *testing.T) {
	t.Parallel()

	ftt.Run(`ListCmd`, t, func(t *ftt.Test) {
		cmd := ListCmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("list [-invocationid <invocation_id>] [<target>]"))
		assert.Loosely(t, cmd.ShortDesc, should.Equal("List test verdicts in an invocation"))

		run := cmd.CommandRun().(*verdictListRun)
		assert.Loosely(t, run.maxVerdicts, should.Equal(100))

		assert.Loosely(t, run.Flags.Parse([]string{"-invocation-id", "inv-1", "-module-name", "ModA", "-testfilter", "AudioFocus", "-n", "25"}), should.BeNil)
		assert.Loosely(t, run.invocationID, should.Equal("inv-1"))
		assert.Loosely(t, run.moduleName, should.Equal("ModA"))
		assert.Loosely(t, run.filter, should.Equal("AudioFocus"))
		assert.Loosely(t, run.maxVerdicts, should.Equal(25))

		run2 := cmd.CommandRun().(*verdictListRun)
		assert.Loosely(t, run2.Flags.Parse([]string{"-test-filter", "Playback", "-limit", "40"}), should.BeNil)
		assert.Loosely(t, run2.filter, should.Equal("Playback"))
		assert.Loosely(t, run2.maxVerdicts, should.Equal(40))
	})
}

func TestParseStatusFilter(t *testing.T) {
	t.Parallel()

	ftt.Run(`parseStatusFilter`, t, func(t *ftt.Test) {
		t.Run(`default`, func(t *ftt.Test) {
			f, err := parseStatusFilter("", false, false, false, false, false, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f.IsDefault(), should.BeTrue)
			assert.Loosely(t, f.failed, should.BeTrue)
			assert.Loosely(t, f.executionErrored, should.BeTrue)
			assert.Loosely(t, f.flaky, should.BeFalse)
			assert.Loosely(t, f.exonerated, should.BeFalse)
			assert.Loosely(t, f.passed, should.BeFalse)
			assert.Loosely(t, f.skipped, should.BeFalse)
			assert.Loosely(t, f.precluded, should.BeFalse)
		})

		t.Run(`include flags`, func(t *ftt.Test) {
			f, err := parseStatusFilter("", true, true, true, false, false, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f.failed, should.BeTrue)
			assert.Loosely(t, f.executionErrored, should.BeTrue)
			assert.Loosely(t, f.exonerated, should.BeTrue)
			assert.Loosely(t, f.flaky, should.BeTrue)
			assert.Loosely(t, f.passed, should.BeTrue)
			assert.Loosely(t, f.skipped, should.BeFalse)
			assert.Loosely(t, f.precluded, should.BeFalse)
		})

		t.Run(`all statuses flag`, func(t *ftt.Test) {
			f, err := parseStatusFilter("", false, false, false, false, false, true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f.All(), should.BeTrue)
		})

		t.Run(`explicit -status`, func(t *ftt.Test) {
			f, err := parseStatusFilter("flaky,passed", false, false, false, false, false, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f.failed, should.BeFalse)
			assert.Loosely(t, f.flaky, should.BeTrue)
			assert.Loosely(t, f.passed, should.BeTrue)
			assert.Loosely(t, f.executionErrored, should.BeFalse)
		})

		t.Run(`explicit -status all`, func(t *ftt.Test) {
			f, err := parseStatusFilter("all", false, false, false, false, false, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f.All(), should.BeTrue)
		})

		t.Run(`invalid -status`, func(t *ftt.Test) {
			_, err := parseStatusFilter("unknown_status", false, false, false, false, false, false)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestQueryRootInvocationListedVerdicts(t *testing.T) {
	t.Parallel()

	ftt.Run(`QueryRootInvocationListedVerdicts`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`default filter queries ui_priority and FULL view with failure reason`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/build-123"))
					assert.Loosely(t, in.OrderBy, should.Equal("ui_priority, test_id_structured"))
					assert.Loosely(t, in.View, should.Equal(pb.TestVerdictView_TEST_VERDICT_VIEW_FULL))
					assert.Loosely(t, in.PageSize, should.Equal(100))
					assert.Loosely(t, in.Predicate, should.NotBeNil)
					assert.Loosely(t, in.Predicate.EffectiveVerdictStatus, should.Match([]pb.VerdictEffectiveStatus{
						pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
						pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXECUTION_ERRORED,
					}))

					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: ":module!junit:pkg.Class#testFail",
								TestIdStructured: &pb.TestIdentifier{
									ModuleName:        "module",
									ModuleScheme:      "junit",
									ModuleVariantHash: "vhash1",
									ModuleVariant: &pb.Variant{
										Def: map[string]string{"os": "Linux"},
									},
								},
								Status: pb.TestVerdict_FAILED,
								Results: []*pb.TestResult{
									{
										StatusV2: pb.TestResult_FAILED,
										FailureReason: &pb.FailureReason{
											PrimaryErrorMessage: "Expected 0 to equal 5\nat Class.java:42",
										},
									},
								},
							},
						},
					}, nil
				},
			}

			filter, _ := parseStatusFilter("", false, false, false, false, false, false)
			verdicts, hasMore, err := QueryRootInvocationListedVerdicts(ctx, client, "build-123", filter, 100)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, len(verdicts), should.Equal(1))
			assert.Loosely(t, verdicts[0].TestID, should.Equal(":module!junit:pkg.Class#testFail"))
			assert.Loosely(t, verdicts[0].Status, should.Equal("FAILED"))
			assert.Loosely(t, verdicts[0].VariantHash, should.Equal("vhash1"))
			assert.Loosely(t, verdicts[0].Variant.Def["os"], should.Equal("Linux"))
			assert.Loosely(t, verdicts[0].FailureReason, should.Equal("Expected 0 to equal 5"))
			assert.Loosely(t, verdicts[0].FailureReasonTruncated, should.BeTrue)
		})

		t.Run(`moduleName and containsFilter populate ContainsTestResultFilter`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					assert.Loosely(t, in.Predicate, should.NotBeNil)
					assert.Loosely(t, in.Predicate.ContainsTestResultFilter, should.Equal(`test_id_structured.module_name = "VtsAidlKeyMintTargetTest" AND (NewKeyGenerationTest)`))
					return &pb.QueryTestVerdictsResponse{}, nil
				},
			}

			filter, _ := parseStatusFilter("", false, false, false, false, false, false)
			filter.moduleName = "VtsAidlKeyMintTargetTest"
			filter.containsFilter = "NewKeyGenerationTest"
			assert.Loosely(t, filter.IsDefault(), should.BeFalse)
			_, _, err := QueryRootInvocationListedVerdicts(ctx, client, "ants-i123", filter, 100)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`exoneration status override`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId:         ":module!junit:pkg.Class#testEx",
								Status:         pb.TestVerdict_FAILED,
								StatusOverride: pb.TestVerdict_EXONERATED,
							},
						},
					}, nil
				},
			}

			filter, _ := parseStatusFilter("", true, false, false, false, false, false)
			verdicts, hasMore, err := QueryRootInvocationListedVerdicts(ctx, client, "build-123", filter, 100)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, len(verdicts), should.Equal(1))
			assert.Loosely(t, verdicts[0].Status, should.Equal("FAILED (EXONERATED)"))
		})

		t.Run(`pagination and maxVerdicts`, func(t *ftt.Test) {
			calls := 0
			client := &mockListResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					calls++
					assert.Loosely(t, in.PageSize, should.Equal(2))
					if in.PageToken == "" {
						return &pb.QueryTestVerdictsResponse{
							TestVerdicts: []*pb.TestVerdict{
								{TestId: "test1", Status: pb.TestVerdict_FAILED},
								{TestId: "test2", Status: pb.TestVerdict_FAILED},
							},
							NextPageToken: "token-page-2",
						}, nil
					}
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{TestId: "test3", Status: pb.TestVerdict_FAILED},
						},
					}, nil
				},
			}

			filter, _ := parseStatusFilter("", false, false, false, false, false, false)
			verdicts, hasMore, err := QueryRootInvocationListedVerdicts(ctx, client, "build-123", filter, 2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(verdicts), should.Equal(2))
			assert.Loosely(t, hasMore, should.BeTrue)
			assert.Loosely(t, calls, should.Equal(1))
		})

		t.Run(`unlimited maxVerdicts uses page size 1000`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					assert.Loosely(t, in.PageSize, should.Equal(1000))
					return &pb.QueryTestVerdictsResponse{}, nil
				},
			}

			filter, _ := parseStatusFilter("", false, false, false, false, false, false)
			verdicts, hasMore, err := QueryRootInvocationListedVerdicts(ctx, client, "build-123", filter, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, len(verdicts), should.Equal(0))
		})
	})
}

func TestQueryLegacyInvocationListedVerdicts(t *testing.T) {
	t.Parallel()

	ftt.Run(`QueryLegacyInvocationListedVerdicts`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`default filter uses UNEXPECTED_MASK predicate and keeps v1 FLAKY/UNEXPECTEDLY_SKIPPED matching StatusV2`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				queryTestVariants: func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
					assert.Loosely(t, in.Invocations, should.Match([]string{"invocations/build-123"}))
					assert.Loosely(t, in.PageSize, should.Equal(1000))
					assert.Loosely(t, in.Predicate.Status, should.Equal(pb.TestVariantStatus_UNEXPECTED_MASK))
					return &pb.QueryTestVariantsResponse{
						TestVariants: []*pb.TestVariant{
							{
								TestId:      "://chrome/test:browser_tests!gtest::Test1",
								Status:      pb.TestVariantStatus_UNEXPECTED,
								StatusV2:    pb.TestVerdict_FAILED,
								VariantHash: "vhash1",
								Variant: &pb.Variant{
									Def: map[string]string{"builder": "linux-rel"},
								},
							},
							{
								// 1 failed run + 1 expected-skipped retry: v1 FLAKY, StatusV2 FAILED.
								TestId:      "://chrome/test:junit_tests!junit:pkg:Cls#TestSkippedRetry",
								Status:      pb.TestVariantStatus_FLAKY,
								StatusV2:    pb.TestVerdict_FAILED,
								VariantHash: "vhash2",
							},
							{
								// EXECUTION_ERRORED maps to v1 UNEXPECTEDLY_SKIPPED.
								TestId:      "://chrome/test:browser_tests!gtest::TestErrored",
								Status:      pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED,
								StatusV2:    pb.TestVerdict_EXECUTION_ERRORED,
								VariantHash: "vhash3",
							},
							{
								TestId:         "://chrome/test:browser_tests!gtest::TestExonerated",
								Status:         pb.TestVariantStatus_EXONERATED,
								StatusV2:       pb.TestVerdict_FAILED,
								StatusOverride: pb.TestVerdict_EXONERATED,
							},
							{
								TestId:   "://chrome/test:browser_tests!gtest::TestFlaky",
								Status:   pb.TestVariantStatus_FLAKY,
								StatusV2: pb.TestVerdict_FLAKY,
							},
						},
					}, nil
				},
			}

			filter, _ := parseStatusFilter("", false, false, false, false, false, false)
			verdicts, hasMore, hidden, err := queryLegacyInvocationListedVerdicts(ctx, client, "build-123", filter, 100)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, len(verdicts), should.Equal(3))
			assert.Loosely(t, verdicts[0].TestID, should.Equal("://chrome/test:browser_tests!gtest::Test1"))
			assert.Loosely(t, verdicts[0].Status, should.Equal("FAILED"))
			assert.Loosely(t, verdicts[1].TestID, should.Equal("://chrome/test:junit_tests!junit:pkg:Cls#TestSkippedRetry"))
			assert.Loosely(t, verdicts[1].Status, should.Equal("FAILED"))
			assert.Loosely(t, verdicts[2].TestID, should.Equal("://chrome/test:browser_tests!gtest::TestErrored"))
			assert.Loosely(t, verdicts[2].Status, should.Equal("EXECUTION_ERRORED"))
			assert.Loosely(t, hidden.exonerated, should.Equal(1))
			assert.Loosely(t, hidden.flaky, should.Equal(1))
			assert.Loosely(t, hidden.truncated, should.BeFalse)
		})

		t.Run(`caps scanned pages when maxVerdicts > 0 on large non-matching legacy invocations`, func(t *ftt.Test) {
			calls := 0
			client := &mockListResultDBClient{
				queryTestVariants: func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
					calls++
					assert.Loosely(t, in.PageSize, should.Equal(1000))
					return &pb.QueryTestVariantsResponse{
						TestVariants: []*pb.TestVariant{
							{
								TestId:   fmt.Sprintf("test-%d", calls),
								Status:   pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED,
								StatusV2: pb.TestVerdict_EXECUTION_ERRORED,
							},
						},
						NextPageToken: fmt.Sprintf("page-%d", calls+1),
					}, nil
				},
			}

			filter, _ := parseStatusFilter("failed", false, false, false, false, false, false)
			verdicts, hasMore, hidden, err := queryLegacyInvocationListedVerdicts(ctx, client, "build-canceled", filter, 100)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calls, should.Equal(maxLegacyScanPages))
			assert.Loosely(t, len(verdicts), should.Equal(0))
			assert.Loosely(t, hasMore, should.BeTrue)
			assert.Loosely(t, hidden.executionErrored, should.Equal(maxLegacyScanPages))
			assert.Loosely(t, hidden.scanned, should.Equal(maxLegacyScanPages))
			assert.Loosely(t, hidden.truncated, should.BeTrue)
			assert.Loosely(t, hidden.scanCapped, should.BeTrue)
			assert.Loosely(t, hidden.formatNotice(), should.ContainSubstring("10+ execution-errored verdicts not shown"))
			assert.Loosely(t, hidden.formatNotice(), should.ContainSubstring("-status=execution_errored"))
			assert.Loosely(t, hidden.formatNotice(), should.ContainSubstring("stopped after scanning 10 variants, pass -all to scan all"))
		})

		t.Run(`does not cap scanned pages when clientSideFiltering is false`, func(t *ftt.Test) {
			calls := 0
			const totalPages = maxLegacyScanPages + 2
			client := &mockListResultDBClient{
				queryTestVariants: func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
					calls++
					nextToken := ""
					if calls < totalPages {
						nextToken = fmt.Sprintf("page-%d", calls+1)
					}
					return &pb.QueryTestVariantsResponse{
						TestVariants: []*pb.TestVariant{
							{
								TestId:   fmt.Sprintf("test-%d", calls),
								Status:   pb.TestVariantStatus_EXPECTED,
								StatusV2: pb.TestVerdict_PASSED,
							},
						},
						NextPageToken: nextToken,
					}, nil
				},
			}

			filter, _ := parseStatusFilter("", false, false, false, false, false, true)
			verdicts, hasMore, hidden, err := queryLegacyInvocationListedVerdicts(ctx, client, "build-all-statuses", filter, 15)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calls, should.Equal(totalPages))
			assert.Loosely(t, len(verdicts), should.Equal(totalPages))
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, hidden.scanCapped, should.BeFalse)
		})

		t.Run(`v1-only UNEXPECTEDLY_SKIPPED and EXONERATED variants match filtering, display, and hidden counts`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				queryTestVariants: func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
					return &pb.QueryTestVariantsResponse{
						TestVariants: []*pb.TestVariant{
							{
								TestId: "test-v1-errored",
								Status: pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED,
							},
							{
								TestId: "test-v1-exonerated",
								Status: pb.TestVariantStatus_EXONERATED,
							},
							{
								TestId:   "test-v1-exonerated-with-v2",
								Status:   pb.TestVariantStatus_EXONERATED,
								StatusV2: pb.TestVerdict_FAILED,
							},
						},
					}, nil
				},
			}

			defaultFilter, _ := parseStatusFilter("", false, false, false, false, false, false)
			verdicts, _, hidden, err := queryLegacyInvocationListedVerdicts(ctx, client, "build-v1", defaultFilter, 100)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(verdicts), should.Equal(1))
			assert.Loosely(t, verdicts[0].TestID, should.Equal("test-v1-errored"))
			assert.Loosely(t, verdicts[0].Status, should.Equal("EXECUTION_ERRORED"))
			assert.Loosely(t, hidden.exonerated, should.Equal(2))

			exFilter, _ := parseStatusFilter("exonerated", false, false, false, false, false, false)
			exVerdicts, _, exHidden, err := queryLegacyInvocationListedVerdicts(ctx, client, "build-v1", exFilter, 100)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(exVerdicts), should.Equal(2))
			assert.Loosely(t, exVerdicts[0].TestID, should.Equal("test-v1-exonerated"))
			assert.Loosely(t, exVerdicts[0].Status, should.Equal("EXONERATED"))
			assert.Loosely(t, exVerdicts[1].TestID, should.Equal("test-v1-exonerated-with-v2"))
			assert.Loosely(t, exVerdicts[1].Status, should.Equal("FAILED (EXONERATED)"))
			assert.Loosely(t, exHidden.executionErrored, should.Equal(1))
		})

		t.Run(`hiddenVerdictCounts combines failed and execution_errored into single -status flag and scanCapped header uses -all`, func(t *ftt.Test) {
			h := hiddenVerdictCounts{failed: 2, executionErrored: 3, flaky: 1}
			assert.Loosely(t, h.formatNotice(), should.Equal("Note: 2 failed, 3 execution-errored, 1 flaky verdicts not shown (use -status=failed,execution_errored, -flaky or -all-statuses to include).\n"))

			var buf bytes.Buffer
			f, _ := parseStatusFilter("", false, false, false, false, false, false)
			printListedVerdictsWithNoticeAndBudget(&buf, "build-123", []*ListedVerdict{{TestID: "t1", Status: "FAILED"}}, true, true, f, "", 0)
			assert.Loosely(t, buf.String(), should.ContainSubstring("Verdicts (showing 1, use -all to scan all):\n"))
		})

		t.Run(`flaky included uses UNEXPECTED_MASK predicate and client-side filtering`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				queryTestVariants: func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
					assert.Loosely(t, in.Predicate.Status, should.Equal(pb.TestVariantStatus_UNEXPECTED_MASK))
					return &pb.QueryTestVariantsResponse{
						TestVariants: []*pb.TestVariant{
							{
								TestId:   "test1",
								StatusV2: pb.TestVerdict_FAILED,
							},
							{
								TestId:   "test2",
								StatusV2: pb.TestVerdict_FLAKY,
							},
							{
								TestId:         "test3",
								StatusV2:       pb.TestVerdict_FAILED,
								StatusOverride: pb.TestVerdict_EXONERATED,
							},
						},
					}, nil
				},
			}

			// Flaky requested, but not exonerated
			filter, _ := parseStatusFilter("", false, true, false, false, false, false)
			verdicts, hasMore, err := QueryLegacyInvocationListedVerdicts(ctx, client, "build-123", filter, 100)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, len(verdicts), should.Equal(2))
			assert.Loosely(t, verdicts[0].TestID, should.Equal("test1"))
			assert.Loosely(t, verdicts[0].Status, should.Equal("FAILED"))
			assert.Loosely(t, verdicts[1].TestID, should.Equal("test2"))
			assert.Loosely(t, verdicts[1].Status, should.Equal("FLAKY"))
		})
	})
}

func TestPrintListedVerdicts(t *testing.T) {
	t.Parallel()

	ftt.Run(`printListedVerdicts`, t, func(t *ftt.Test) {
		t.Run(`empty with default filter`, func(t *ftt.Test) {
			var buf bytes.Buffer
			defaultFilter, _ := parseStatusFilter("", false, false, false, false, false, false)
			printListedVerdicts(&buf, "build-123", nil, false, defaultFilter)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("No failed or execution errored verdicts found in invocation \"build-123\""))
			assert.Loosely(t, out, should.ContainSubstring("-all-statuses"))
		})

		t.Run(`with results, failure reason, and variant`, func(t *ftt.Test) {
			verdicts := []*ListedVerdict{
				{
					TestID:                 ":no-module-name!junit:pkg.Class#testA",
					Status:                 "FAILED",
					VariantHash:            "e3b0c442",
					FailureReason:          "keymint_test.cc:154: Expected ErrorCode::OK",
					FailureReasonTruncated: true,
				},
				{
					TestID: ":module!junit:pkg.Class#testB",
					Status: "FAILED (EXONERATED)",
					Variant: &pb.Variant{
						Def: map[string]string{"os": "Ubuntu-22.04", "builder": "linux-rel"},
					},
					VariantHash:   "vhash1",
					FailureReason: "Short single-line error",
					Exonerations: []*pb.TestExoneration{
						{
							ExplanationHtml: "Failed on mainline",
							Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
						},
					},
				},
			}

			var buf bytes.Buffer
			allFilter, _ := parseStatusFilter("all", false, false, false, false, false, true)
			printListedVerdicts(&buf, "build-123", verdicts, true, allFilter)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Verdicts (showing 2, use -max-verdicts or -all to see more):"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED :no-module-name!junit:pkg.Class#testA"))
			assert.Loosely(t, out, should.ContainSubstring("Error (truncated): keymint_test.cc:154: Expected ErrorCode::OK"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED (EXONERATED) :module!junit:pkg.Class#testB"))
			assert.Loosely(t, out, should.ContainSubstring("Variant: builder=linux-rel os=Ubuntu-22.04 (hash: vhash1)"))
			assert.Loosely(t, out, should.ContainSubstring("Error: Short single-line error"))
			assert.Loosely(t, out, should.ContainSubstring("Exoneration: Failed on mainline [OCCURS_ON_MAINLINE]"))
		})
	})
}

func TestVerdictListRunAutoDetect(t *testing.T) {
	t.Parallel()

	ftt.Run(`verdictListRun auto-detection`, t, func(t *ftt.Test) {
		t.Run(`auto-detects legacy build when GetRootInvocation returns NotFound`, func(t *ftt.Test) {
			legacyQueried := false
			client := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
				getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
					return &pb.Invocation{Name: in.Name}, nil
				},
				queryTestVariants: func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
					legacyQueried = true
					return &pb.QueryTestVariantsResponse{}, nil
				},
			}
			var buf bytes.Buffer
			run := &verdictListRun{
				client:       client,
				out:          &buf,
				invocationID: "8697111068122866129",
				maxVerdicts:  10,
			}
			rc := run.Run(nil, nil, nil)
			assert.Loosely(t, rc, should.Equal(0))
			assert.Loosely(t, legacyQueried, should.BeTrue)
		})

		t.Run(`fails immediately on PermissionDenied without falling back to legacy`, func(t *ftt.Test) {
			legacyQueried := false
			client := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.PermissionDenied, "caller does not have permission")
				},
				queryTestVariants: func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
					legacyQueried = true
					return &pb.QueryTestVariantsResponse{}, nil
				},
			}
			var buf bytes.Buffer
			run := &verdictListRun{
				client:       client,
				out:          &buf,
				invocationID: "build-123",
				maxVerdicts:  10,
			}
			rc := run.Run(nil, nil, nil)
			assert.Loosely(t, rc, should.Equal(1))
			assert.Loosely(t, legacyQueried, should.BeFalse)
		})

		t.Run(`accepts positional URL and prints derived IDs`, func(t *ftt.Test) {
			var gotParent string
			client := &mockListResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					gotParent = in.Parent
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: ":ModA!junit:pkg:Cls#testOne",
								Status: pb.TestVerdict_FAILED,
							},
						},
					}, nil
				},
			}
			var buf bytes.Buffer
			cmd := ListCmd(nil)
			run := cmd.CommandRun().(*verdictListRun)
			run.client = client
			run.out = &buf
			rc := run.Run(nil, []string{"https://ci.chromium.org/ui/p/chromium/builders/try/linux-rel/b8765432109876543210"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := buf.String()
			assert.Loosely(t, gotParent, should.Equal("rootInvocations/build-8765432109876543210"))
			assert.Loosely(t, out, should.ContainSubstring("Build ID:          8765432109876543210"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     build-8765432109876543210"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED :ModA!junit:pkg:Cls#testOne"))
		})

		t.Run(`prints RootInvocation metadata by default for root invocations`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:  in.Name,
						State: pb.RootInvocation_FAILED,
						PrimaryBuild: &pb.BuildDescriptor{
							Definition: &pb.BuildDescriptor_AndroidBuild{
								AndroidBuild: &pb.AndroidBuildDescriptor{
									Branch:      "git_main",
									BuildTarget: "cf_x86_64_phone-trunk_staging-userdebug",
									BuildId:     "16133789",
								},
							},
						},
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{TestId: ":mod!junit:pkg.Class#testA", Status: pb.TestVerdict_FAILED},
						},
					}, nil
				},
			}
			var buf bytes.Buffer
			run := &verdictListRun{
				client:       client,
				out:          &buf,
				invocationID: "ants-i123",
				maxVerdicts:  10,
			}
			assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Primary Build: branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=16133789\n"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED :mod!junit:pkg.Class#testA\n"))
		})

		t.Run(`surfaces Sponge2 and Fusion2 links, module counts, and actionable hints on 0-verdict root invocation`, func(t *ftt.Test) {
			props, _ := structpb.NewStruct(map[string]any{"runner": "TRADEFED"})
			var gotAggReq *pb.QueryTestAggregationsRequest
			client := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:              in.Name,
						State:             pb.RootInvocation_SUCCEEDED,
						FinalizationState: pb.RootInvocation_FINALIZED,
						SummaryMarkdown:   "http://sponge2/3920feb1-7663-4085-b120-47a767784c3c",
						Tags: []*pb.StringPair{
							{Key: "result_link", Value: "http://sponge2/3920feb1-7663-4085-b120-47a767784c3c"},
							{Key: "sponge_invocation_id", Value: "19cd1c2a-4869-45ec-98a5-17651f29d248"},
							{Key: "test_uri", Value: "http://fusion2/19cd1c2a-4869-45ec-98a5-17651f29d248"},
							{Key: "runner_url", Value: "https://mobileharness-fe.corp.google.com/sessiondetailview/3920feb1-7663-4085-b120-47a767784c3c"},
							{Key: "failure_status", Value: "presubmit_error"},
							{Key: "presubmit_failure_status", Value: "ERROR"},
						},
						Properties: props,
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{}, nil
				},
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					gotAggReq = in
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{Skipped: 1},
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
							},
						},
					}, nil
				},
			}

			var buf bytes.Buffer
			run := &verdictListRun{
				client:       client,
				out:          &buf,
				invocationID: "ants-i90900010658207079",
				maxVerdicts:  100,
			}
			assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(0))
			assert.Loosely(t, gotAggReq, should.NotBeNil)
			assert.Loosely(t, gotAggReq.Parent, should.Equal("rootInvocations/ants-i90900010658207079"))
			assert.Loosely(t, gotAggReq.PageSize, should.Equal(1))
			assert.Loosely(t, gotAggReq.Predicate.GetAggregationLevel(), should.Equal(pb.AggregationLevel_INVOCATION))

			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Inv State:     SUCCEEDED (FINALIZED)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Runner:        TRADEFED\n"))
			assert.Loosely(t, out, should.ContainSubstring("Result Link:   http://sponge2/3920feb1-7663-4085-b120-47a767784c3c\n"))
			assert.Loosely(t, out, should.ContainSubstring("Sponge Invocation ID: 19cd1c2a-4869-45ec-98a5-17651f29d248\n"))
			assert.Loosely(t, out, should.ContainSubstring("Test URI:      http://fusion2/19cd1c2a-4869-45ec-98a5-17651f29d248\n"))
			assert.Loosely(t, out, should.ContainSubstring("Runner URL:    https://mobileharness-fe.corp.google.com/sessiondetailview/3920feb1-7663-4085-b120-47a767784c3c\n"))
			assert.Loosely(t, out, should.ContainSubstring("Failure Policy: failure_status=presubmit_error presubmit_failure_status=ERROR\n"))
			assert.Loosely(t, out, should.ContainSubstring("Module Statuses: [1 skipped] (1 total)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Test Verdicts:   0 tests ran\n"))
			assert.Loosely(t, out, should.NotContainSubstring("Summary:"))
			assert.Loosely(t, out, should.ContainSubstring(`No test verdicts were uploaded to ResultDB for invocation "ants-i90900010658207079" (0 tests ran).`))
			assert.Loosely(t, out, should.ContainSubstring("Note: Inv State: SUCCEEDED with 0 ResultDB verdicts only means the runner exited without a harness crash (it does not mean tests passed). Failure Policy reflects static test config, not a pass or failure verdict. Do not fall back to ants_cli when luci succeeds with 0 tests ran (ants_cli also reports 0 test results)."))
			assert.Loosely(t, out, should.NotContainSubstring("-all-statuses"))
			assert.Loosely(t, out, should.ContainSubstring("luci work-unit get -invocationid=ants-i90900010658207079 -workunitid=root"))
			assert.Loosely(t, out, should.ContainSubstring("luci artifact list -invocationid=ants-i90900010658207079"))
			assert.Loosely(t, out, should.ContainSubstring("Inspect inner Sponge2 test targets/logs first (pass bare UUID --invocation_id=19cd1c2a-4869-45ec-98a5-17651f29d248 via sponge skill; http://fusion2/19cd1c2a-4869-45ec-98a5-17651f29d248)"))
			assert.Loosely(t, out, should.ContainSubstring("Inspect outer session in Sponge2 if inner run is absent (pass bare UUID --invocation_id=3920feb1-7663-4085-b120-47a767784c3c; http://sponge2/3920feb1-7663-4085-b120-47a767784c3c)"))
		})

		t.Run(`prints distinct SummaryMarkdown on 0-verdict root invocation without external links`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:              in.Name,
						State:             pb.RootInvocation_FAILED,
						FinalizationState: pb.RootInvocation_FINALIZED,
						SummaryMarkdown:   "TargetSetupError: failed to flash device",
					}, nil
				},
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{Failed: 1},
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
							},
						},
					}, nil
				},
			}

			var buf bytes.Buffer
			run := &verdictListRun{
				client:       client,
				out:          &buf,
				invocationID: "ants-i555",
				maxVerdicts:  100,
			}
			assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Module Statuses: [1 failed, 0 succeeded] (1 total)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Test Verdicts:   0 tests ran\n"))
			assert.Loosely(t, out, should.ContainSubstring("Summary:\n  TargetSetupError: failed to flash device\n"))
			assert.Loosely(t, out, should.ContainSubstring(`No test verdicts were uploaded to ResultDB for invocation "ants-i555" (0 tests ran).`))
			assert.Loosely(t, out, should.NotContainSubstring("Inspect external test results"))
		})

		t.Run(`includes inline verdict counts alongside -all-statuses hint on all-passed root invocation`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:              in.Name,
						State:             pb.RootInvocation_SUCCEEDED,
						FinalizationState: pb.RootInvocation_FINALIZED,
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{}, nil
				},
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{Succeeded: 2},
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Passed: 42},
							},
						},
					}, nil
				},
			}

			var buf bytes.Buffer
			run := &verdictListRun{
				client:       client,
				out:          &buf,
				invocationID: "ants-i456",
				maxVerdicts:  100,
			}
			assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Module Statuses: [2 succeeded] (2 total)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Test Verdicts:   [42 passed] (42 total)\n"))
			assert.Loosely(t, out, should.ContainSubstring(`No failed or execution errored verdicts found in invocation "ants-i456" [42 passed] (use -all-statuses or status flags to see other verdicts)`))

			// Partial status flags (-flaky) on all-passed invocation still show inline counts and -all-statuses hint,
			// and suppress generic ANDROID_TRADEFED_TEST_HAS_FAIL_SUBTEST wrapper summaries when verdicts exist.
			origGetRoot := client.getRootInvocation
			client.getRootInvocation = func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
				ri, err := origGetRoot(ctx, in)
				if ri != nil {
					ri.SummaryMarkdown = "name: ANDROID_TRADEFED_TEST_HAS_FAIL_SUBTEST\nstack trace: MobileHarnessException"
				}
				return ri, err
			}
			buf.Reset()
			runFlaky := &verdictListRun{
				client:       client,
				out:          &buf,
				invocationID: "ants-i456",
				includeFlaky: true,
				maxVerdicts:  100,
			}
			assert.Loosely(t, runFlaky.Run(nil, nil, nil), should.Equal(0))
			outFlaky := buf.String()
			assert.Loosely(t, outFlaky, should.NotContainSubstring("ANDROID_TRADEFED_TEST_HAS_FAIL_SUBTEST"))
			assert.Loosely(t, outFlaky, should.ContainSubstring(`No matching verdicts found in invocation "ants-i456" [42 passed] (use -all-statuses or status flags to see other verdicts)`))
		})

		t.Run(`avoids contradictory invocation counts and suggests module get when -modulename matches 0 failed verdicts`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:              in.Name,
						State:             pb.RootInvocation_FAILED,
						FinalizationState: pb.RootInvocation_FINALIZED,
						SummaryMarkdown:   "OtherModule failed",
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{}, nil
				},
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{Failed: 1, Succeeded: 1},
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Failed: 2, Passed: 40},
							},
						},
					}, nil
				},
			}

			var buf bytes.Buffer
			run := &verdictListRun{
				client:       client,
				out:          &buf,
				invocationID: "ants-i789",
				moduleName:   "PassingModule",
				maxVerdicts:  100,
			}
			assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()
			assert.Loosely(t, out, should.NotContainSubstring("Test Verdicts:"))
			assert.Loosely(t, out, should.NotContainSubstring("OtherModule failed"))
			assert.Loosely(t, out, should.ContainSubstring(`No matching verdicts found in invocation "ants-i789" (use -all-statuses or status flags to see other verdicts)`))
			assert.Loosely(t, out, should.ContainSubstring("Hint: inspect module state with: luci module get -invocationid=ants-i789 -modulename=PassingModule"))

			// Preserves -all-statuses and module get hints even when QueryTestAggregations fails (rootAgg == nil).
			client.queryTestAggregations = func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
				return nil, status.Error(codes.Unavailable, "transient error")
			}
			buf.Reset()
			assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(0))
			assert.Loosely(t, buf.String(), should.ContainSubstring(`No matching verdicts found in invocation "ants-i789" (use -all-statuses or status flags to see other verdicts)`))
			assert.Loosely(t, buf.String(), should.ContainSubstring("Hint: inspect module state with: luci module get -invocationid=ants-i789 -modulename=PassingModule"))

			// Includes module get next-step bullet when -modulename is passed on a 0-verdict invocation.
			client.queryTestAggregations = func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
				return &pb.QueryTestAggregationsResponse{
					Aggregations: []*pb.TestAggregation{
						{
							ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{Failed: 1},
							TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
						},
					},
				}, nil
			}
			buf.Reset()
			runZeroMod := &verdictListRun{
				client:       client,
				out:          &buf,
				invocationID: "ants-i789",
				moduleName:   "BrokenSetupModule",
				maxVerdicts:  100,
			}
			assert.Loosely(t, runZeroMod.Run(nil, nil, nil), should.Equal(0))
			assert.Loosely(t, buf.String(), should.ContainSubstring("Inspect module state and setup errors: luci module get -invocationid=ants-i789 -modulename=BrokenSetupModule"))
		})
	})
}

func TestVerdictListDefaultByteCap(t *testing.T) {
	t.Parallel()

	ftt.Run(`verdictListRun enforces format.DefaultMaxOutputBytes by default and bypasses on -all or explicit -max-verdicts`, t, func(t *ftt.Test) {
		testVerdicts := make([]*pb.TestVerdict, 0, 100)
		for i := range 100 {
			testID := fmt.Sprintf("://\\:CtsMediaAudioTestCases!cts:android.media.audio.cts:AudioFocusParameterizedTest#testAudioFocusRequest_gainTransientMayDuck_case_%03d", i)
			testVerdicts = append(testVerdicts, &pb.TestVerdict{
				TestId: testID,
				Status: pb.TestVerdict_FAILED,
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:        "//:CtsMediaAudioTestCases",
					ModuleScheme:      "cts",
					ModuleVariantHash: "b7de9035241e76cc",
					ModuleVariant: &pb.Variant{
						Def: map[string]string{
							"abi":          "arm64-v8a",
							"build_target": "cf_x86_64_phone-trunk_staging-userdebug",
						},
					},
				},
				Results: []*pb.TestResult{
					{
						StatusV2: pb.TestResult_FAILED,
						FailureReason: &pb.FailureReason{
							PrimaryErrorMessage: "java.lang.AssertionError: expected AUDIOFOCUS_REQUEST_GRANTED (1) but was AUDIOFOCUS_REQUEST_FAILED (0) during playback transition",
						},
					},
				},
			})
		}

		client := &mockListResultDBClient{
			getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
				return &pb.RootInvocation{
					Name:              in.Name,
					State:             pb.RootInvocation_FAILED,
					FinalizationState: pb.RootInvocation_FINALIZED,
					PrimaryBuild: &pb.BuildDescriptor{
						Definition: &pb.BuildDescriptor_AndroidBuild{
							AndroidBuild: &pb.AndroidBuildDescriptor{
								Branch:      "git_main",
								BuildTarget: "cf_x86_64_phone-trunk_staging-userdebug",
								BuildId:     "16133789",
							},
						},
					},
				}, nil
			},
			queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
				return &pb.QueryTestVerdictsResponse{
					TestVerdicts:  testVerdicts,
					NextPageToken: "more-verdicts-token",
				}, nil
			},
		}

		t.Run(`default invocation stays strictly within 9.5 KB and preserves whole entries`, func(t *ftt.Test) {
			var buf bytes.Buffer
			cmd := ListCmd(nil)
			run := cmd.CommandRun().(*verdictListRun)
			run.client = client
			run.out = &buf

			rc := run.Run(nil, []string{"rootInvocations/ants-i999"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := buf.String()

			assert.Loosely(t, len(out) <= format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("within 9.5 KB output limit; pass -all, -max-verdicts=<N>, or -filter=<SUBSTR> to see more"))
			assert.Loosely(t, out, should.ContainSubstring("more verdicts omitted to stay within 9.5 KB limit; run 'luci verdict list -invocationid=ants-i999 -all' (or pass -modulename / -filter) to view more"))
			assert.Loosely(t, out, should.ContainSubstring("case_000"))
			assert.Loosely(t, out, should.NotContainSubstring("case_099"))
		})

		t.Run(`-all bypasses the 9.5 KB cap`, func(t *ftt.Test) {
			plainClient := &mockListResultDBClient{
				getRootInvocation: client.getRootInvocation,
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{TestVerdicts: testVerdicts}, nil
				},
			}
			var buf bytes.Buffer
			cmd := ListCmd(nil)
			run := cmd.CommandRun().(*verdictListRun)
			run.client = plainClient
			run.out = &buf

			rc := run.Run(nil, []string{"-invocationid", "ants-i999", "-all"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := buf.String()

			assert.Loosely(t, len(out) > format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("case_000"))
			assert.Loosely(t, out, should.ContainSubstring("case_099"))
			assert.Loosely(t, out, should.NotContainSubstring("within 9.5 KB output limit"))
		})

		t.Run(`explicit -max-verdicts=100 bypasses the 9.5 KB cap`, func(t *ftt.Test) {
			var buf bytes.Buffer
			cmd := ListCmd(nil)
			run := cmd.CommandRun().(*verdictListRun)
			run.client = client
			run.out = &buf

			rc := run.Run(nil, []string{"-invocationid", "ants-i999", "-max-verdicts", "100"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := buf.String()

			assert.Loosely(t, len(out) > format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("case_099"))
			assert.Loosely(t, out, should.NotContainSubstring("within 9.5 KB output limit"))
		})

		t.Run(`explicit -limit=100 alias bypasses the 9.5 KB cap`, func(t *ftt.Test) {
			var buf bytes.Buffer
			cmd := ListCmd(nil)
			run := cmd.CommandRun().(*verdictListRun)
			run.client = client
			run.out = &buf

			rc := run.Run(nil, []string{"-invocationid", "ants-i999", "-limit", "100"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := buf.String()

			assert.Loosely(t, len(out) > format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("case_099"))
			assert.Loosely(t, out, should.NotContainSubstring("within 9.5 KB output limit"))
		})

		t.Run(`explicit -n=100 alias bypasses the 9.5 KB cap`, func(t *ftt.Test) {
			var buf bytes.Buffer
			cmd := ListCmd(nil)
			run := cmd.CommandRun().(*verdictListRun)
			run.client = client
			run.out = &buf

			rc := run.Run(nil, []string{"-invocationid", "ants-i999", "-n", "100"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := buf.String()

			assert.Loosely(t, len(out) > format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("case_099"))
			assert.Loosely(t, out, should.NotContainSubstring("within 9.5 KB output limit"))
		})

		t.Run(`caps 0-verdict root invocation with oversized SummaryMarkdown and allows -all bypass`, func(t *ftt.Test) {
			oversizedSummary := "TOP_ERROR_LINE\n" + strings.Repeat("diagnostic harness detail line 1234567890\n", 320) + "BOTTOM_TRACE_LINE"
			zeroClient := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:              in.Name,
						State:             pb.RootInvocation_SUCCEEDED,
						FinalizationState: pb.RootInvocation_FINALIZED,
						SummaryMarkdown:   oversizedSummary,
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{}, nil
				},
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{Skipped: 1},
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
							},
						},
					}, nil
				},
			}

			var cappedBuf bytes.Buffer
			cappedCmd := ListCmd(nil)
			cappedRun := cappedCmd.CommandRun().(*verdictListRun)
			cappedRun.client = zeroClient
			cappedRun.out = &cappedBuf
			assert.Loosely(t, cappedRun.Run(nil, []string{"-invocationid", "ants-i999"}, nil), should.Equal(0))
			cappedOut := cappedBuf.String()
			assert.Loosely(t, len(cappedOut) <= format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, utf8.ValidString(cappedOut), should.BeTrue)
			assert.Loosely(t, cappedOut, should.ContainSubstring("TOP_ERROR_LINE"))
			assert.Loosely(t, cappedOut, should.ContainSubstring("0 tests ran"))
			assert.Loosely(t, cappedOut, should.ContainSubstring("Next steps:"))
			assert.Loosely(t, cappedOut, should.ContainSubstring("bytes truncated; pass -all to view full output"))

			var allBuf bytes.Buffer
			allCmd := ListCmd(nil)
			allRun := allCmd.CommandRun().(*verdictListRun)
			allRun.client = zeroClient
			allRun.out = &allBuf
			assert.Loosely(t, allRun.Run(nil, []string{"-invocationid", "ants-i999", "-all"}, nil), should.Equal(0))
			allOut := allBuf.String()
			assert.Loosely(t, len(allOut) > format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, allOut, should.ContainSubstring("BOTTOM_TRACE_LINE"))
			assert.Loosely(t, allOut, should.NotContainSubstring("bytes truncated"))
		})

		t.Run(`caps oversized RootInvocation header metadata while preserving an actionable Verdicts section`, func(t *ftt.Test) {
			extraBuilds := make([]*pb.BuildDescriptor, 0, 200)
			for i := range 200 {
				extraBuilds = append(extraBuilds, &pb.BuildDescriptor{
					Definition: &pb.BuildDescriptor_AndroidBuild{
						AndroidBuild: &pb.AndroidBuildDescriptor{
							Branch:      "git_main",
							BuildTarget: fmt.Sprintf("extra_build_target_with_long_name_%03d_userdebug", i),
							BuildId:     fmt.Sprintf("16133%03d", i),
						},
					},
				})
			}
			defProps := make(map[string]string, 100)
			for i := range 100 {
				defProps[fmt.Sprintf("prop_%03d", i)] = strings.Repeat("v", 80)
			}
			heavyHeaderClient := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:              in.Name,
						State:             pb.RootInvocation_FAILED,
						FinalizationState: pb.RootInvocation_FINALIZED,
						ExtraBuilds:       extraBuilds,
						Definition: &pb.RootInvocationDefinition{
							System:     "atp",
							Name:       "suite-name",
							Properties: &pb.RootInvocationDefinition_Properties{Def: defProps},
						},
					}, nil
				},
				queryTestVerdicts: client.queryTestVerdicts,
			}

			var buf bytes.Buffer
			cmd := ListCmd(nil)
			run := cmd.CommandRun().(*verdictListRun)
			run.client = heavyHeaderClient
			run.out = &buf
			assert.Loosely(t, run.Run(nil, []string{"-invocationid", "ants-i999"}, nil), should.Equal(0))
			out := buf.String()

			assert.Loosely(t, len(out) <= format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, utf8.ValidString(out), should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("bytes truncated; pass -all to view full output"))
			assert.Loosely(t, out, should.ContainSubstring("case_000"))
			assert.Loosely(t, out, should.ContainSubstring("Verdicts ("))
			assert.Loosely(t, out, should.ContainSubstring("run 'luci verdict list -invocationid=ants-i999 -all'"))
		})

		t.Run(`preserves active status and scope flags in suggested -all command`, func(t *ftt.Test) {
			runArgs := func(args ...string) string {
				var buf bytes.Buffer
				cmd := ListCmd(nil)
				run := cmd.CommandRun().(*verdictListRun)
				run.client = client
				run.out = &buf
				assert.Loosely(t, run.Run(nil, args, nil), should.Equal(0))
				out := buf.String()
				assert.Loosely(t, len(out) <= format.DefaultMaxOutputBytes, should.BeTrue)
				return out
			}

			outAllStatuses := runArgs("-invocationid", "ants-i999", "-all-statuses")
			assert.Loosely(t, outAllStatuses, should.ContainSubstring("run 'luci verdict list -invocationid=ants-i999 -all-statuses -all'"))

			outPassed := runArgs("-invocationid", "ants-i999", "-passed")
			assert.Loosely(t, outPassed, should.ContainSubstring("run 'luci verdict list -invocationid=ants-i999 -passed -all'"))

			outStatusOnly := runArgs("-invocationid", "ants-i999", "-status", "FAILED,FLAKY")
			assert.Loosely(t, outStatusOnly, should.ContainSubstring("run 'luci verdict list -invocationid=ants-i999 -status=failed,flaky -all'"))

			outScoped := runArgs("-invocationid", "ants-i999", "-modulename", "//:CtsMediaAudioTestCases", "-filter", `test_id:"AudioFocus case"`)
			assert.Loosely(t, outScoped, should.ContainSubstring(`run 'luci verdict list -invocationid=ants-i999 -modulename=//:CtsMediaAudioTestCases -filter='test_id:"AudioFocus case"' -all'`))
		})

		t.Run(`does not truncate ~8 KB RootInvocation header when 1 short verdict fits within 9.5 KB`, func(t *ftt.Test) {
			defProps := make(map[string]string, 80)
			for i := range 80 {
				defProps[fmt.Sprintf("prop_%03d", i)] = strings.Repeat("p", 82)
			}
			eightKBHeaderClient := &mockListResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:              in.Name,
						State:             pb.RootInvocation_FAILED,
						FinalizationState: pb.RootInvocation_FINALIZED,
						Definition: &pb.RootInvocationDefinition{
							System:     "atp",
							Name:       "suite-eight-kb",
							Properties: &pb.RootInvocationDefinition_Properties{Def: defProps},
						},
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: ":CtsShortModule!junit:pkg.Cls#testSingleFailure",
								Status: pb.TestVerdict_FAILED,
								Results: []*pb.TestResult{
									{
										StatusV2: pb.TestResult_FAILED,
										FailureReason: &pb.FailureReason{
											PrimaryErrorMessage: "AssertionError: expected true",
										},
									},
								},
							},
						},
					}, nil
				},
			}

			var buf bytes.Buffer
			cmd := ListCmd(nil)
			run := cmd.CommandRun().(*verdictListRun)
			run.client = eightKBHeaderClient
			run.out = &buf
			assert.Loosely(t, run.Run(nil, []string{"-invocationid", "ants-i999"}, nil), should.Equal(0))
			out := buf.String()

			assert.Loosely(t, len(out) > 7500 && len(out) <= format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, out, should.NotContainSubstring("bytes truncated"))
			assert.Loosely(t, out, should.NotContainSubstring("omitted"))
			assert.Loosely(t, out, should.ContainSubstring("Verdicts (1):\n"))
			assert.Loosely(t, out, should.ContainSubstring("testSingleFailure"))
		})

		t.Run(`surfaces hidden exonerated/flaky footer and ACTIVE/FINALIZING notices on legacy and root invocations`, func(t *ftt.Test) {
			t.Run(`legacy ACTIVE invocation with hidden exonerated verdicts and 0 failures`, func(t *ftt.Test) {
				legacyClient := &mockListResultDBClient{
					getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
						return nil, status.Error(codes.NotFound, "not found")
					},
					getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
						return &pb.Invocation{
							Name:  in.Name,
							State: pb.Invocation_ACTIVE,
						}, nil
					},
					queryTestVariants: func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
						return &pb.QueryTestVariantsResponse{
							TestVariants: []*pb.TestVariant{
								{
									TestId:         "test-ex-1",
									Status:         pb.TestVariantStatus_EXONERATED,
									StatusV2:       pb.TestVerdict_FAILED,
									StatusOverride: pb.TestVerdict_EXONERATED,
								},
								{
									TestId:         "test-ex-2",
									Status:         pb.TestVariantStatus_EXONERATED,
									StatusV2:       pb.TestVerdict_FAILED,
									StatusOverride: pb.TestVerdict_EXONERATED,
								},
							},
						}, nil
					},
				}

				var buf bytes.Buffer
				cmd := ListCmd(nil)
				run := cmd.CommandRun().(*verdictListRun)
				run.client = legacyClient
				run.out = &buf
				assert.Loosely(t, run.Run(nil, []string{"-invocationid", "build-8664637210134224081"}, nil), should.Equal(0))
				out := buf.String()
				assert.Loosely(t, out, should.ContainSubstring(`Note: Invocation "build-8664637210134224081" is still ACTIVE (results may be incomplete; test execution is still in progress).`))
				assert.Loosely(t, out, should.ContainSubstring(`No failed or execution errored verdicts found in invocation "build-8664637210134224081"`))
				assert.Loosely(t, out, should.ContainSubstring(`Note: 2 exonerated verdicts not shown (use -exonerated or -all-statuses to include).`))
			})

			t.Run(`root FINALIZING invocation with failures and hidden exonerated/flaky counts`, func(t *ftt.Test) {
				rootClient := &mockListResultDBClient{
					getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
						return &pb.RootInvocation{
							Name:              in.Name,
							State:             pb.RootInvocation_RUNNING,
							FinalizationState: pb.RootInvocation_FINALIZING,
						}, nil
					},
					queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
						return &pb.QueryTestVerdictsResponse{
							TestVerdicts: []*pb.TestVerdict{
								{TestId: ":mod!junit:pkg:Cls#testFail", Status: pb.TestVerdict_FAILED},
							},
						}, nil
					},
					queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
						return &pb.QueryTestAggregationsResponse{
							Aggregations: []*pb.TestAggregation{
								{
									TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
										Failed:     1,
										Flaky:      3,
										Exonerated: 5,
										Passed:     100,
									},
								},
							},
						}, nil
					},
				}

				var buf bytes.Buffer
				cmd := ListCmd(nil)
				run := cmd.CommandRun().(*verdictListRun)
				run.client = rootClient
				run.out = &buf
				assert.Loosely(t, run.Run(nil, []string{"-invocationid", "ants-i999"}, nil), should.Equal(0))
				out := buf.String()
				assert.Loosely(t, out, should.ContainSubstring(`Note: Invocation "ants-i999" is FINALIZING (results may be incomplete until finalized).`))
				assert.Loosely(t, out, should.ContainSubstring(`- FAILED :mod!junit:pkg:Cls#testFail`))
				assert.Loosely(t, out, should.ContainSubstring(`Note: 3 flaky, 5 exonerated verdicts not shown (use -flaky, -exonerated or -all-statuses to include).`))
			})

			t.Run(`does not issue duplicate QueryTestAggregations RPC when concurrent fetch returns nil on 0-verdict root invocation`, func(t *ftt.Test) {
				aggCalls := 0
				rootClient := &mockListResultDBClient{
					getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
						return &pb.RootInvocation{
							Name:              in.Name,
							State:             pb.RootInvocation_FAILED,
							FinalizationState: pb.RootInvocation_FINALIZED,
						}, nil
					},
					queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
						return &pb.QueryTestVerdictsResponse{}, nil
					},
					queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
						aggCalls++
						return nil, status.Error(codes.Unavailable, "transient error")
					},
				}

				var buf bytes.Buffer
				cmd := ListCmd(nil)
				run := cmd.CommandRun().(*verdictListRun)
				run.client = rootClient
				run.out = &buf
				assert.Loosely(t, run.Run(nil, []string{"-invocationid", "ants-i999"}, nil), should.Equal(0))
				assert.Loosely(t, aggCalls, should.Equal(1))
			})
		})
	})
}
