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

package analysis

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockTestHistoryClient struct {
	analysispb.TestHistoryClient
	queryStats          func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error)
	query               func(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error)
	queryVariants       func(ctx context.Context, in *analysispb.QueryVariantsRequest, opts ...grpc.CallOption) (*analysispb.QueryVariantsResponse, error)
	queryTests          func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error)
	querySourceVerdicts func(ctx context.Context, in *analysispb.QuerySourceVerdictsV2Request, opts ...grpc.CallOption) (*analysispb.QuerySourceVerdictsV2Response, error)
}

func (m *mockTestHistoryClient) QueryStats(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
	if m.queryStats != nil {
		return m.queryStats(ctx, in, opts...)
	}
	return &analysispb.QueryTestHistoryStatsResponse{}, nil
}

func (m *mockTestHistoryClient) Query(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error) {
	if m.query != nil {
		return m.query(ctx, in, opts...)
	}
	return &analysispb.QueryTestHistoryResponse{}, nil
}

func (m *mockTestHistoryClient) QueryVariants(ctx context.Context, in *analysispb.QueryVariantsRequest, opts ...grpc.CallOption) (*analysispb.QueryVariantsResponse, error) {
	if m.queryVariants != nil {
		return m.queryVariants(ctx, in, opts...)
	}
	return &analysispb.QueryVariantsResponse{}, nil
}

func (m *mockTestHistoryClient) QueryTests(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
	if m.queryTests != nil {
		return m.queryTests(ctx, in, opts...)
	}
	return &analysispb.QueryTestsResponse{}, nil
}

func (m *mockTestHistoryClient) QuerySourceVerdicts(ctx context.Context, in *analysispb.QuerySourceVerdictsV2Request, opts ...grpc.CallOption) (*analysispb.QuerySourceVerdictsV2Response, error) {
	if m.querySourceVerdicts != nil {
		return m.querySourceVerdicts(ctx, in, opts...)
	}
	return &analysispb.QuerySourceVerdictsV2Response{}, nil
}

type mockTestVariantBranchesClient struct {
	analysispb.TestVariantBranchesClient
	query func(ctx context.Context, in *analysispb.QueryTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.QueryTestVariantBranchResponse, error)
}

func (m *mockTestVariantBranchesClient) Query(ctx context.Context, in *analysispb.QueryTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.QueryTestVariantBranchResponse, error) {
	if m.query != nil {
		return m.query(ctx, in, opts...)
	}
	return &analysispb.QueryTestVariantBranchResponse{}, nil
}

type mockAnalysisSchemasClient struct {
	rdbpb.SchemasClient
	getScheme func(ctx context.Context, in *rdbpb.GetSchemeRequest, opts ...grpc.CallOption) (*rdbpb.Scheme, error)
}

func (m *mockAnalysisSchemasClient) GetScheme(ctx context.Context, in *rdbpb.GetSchemeRequest, opts ...grpc.CallOption) (*rdbpb.Scheme, error) {
	if m.getScheme != nil {
		return m.getScheme(ctx, in, opts...)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

type mockAnalysisRDBClient struct {
	rdbpb.ResultDBClient
	getRootInvocation func(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error)
	queryTestVerdicts func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestVerdictsResponse, error)
}

func (m *mockAnalysisRDBClient) GetRootInvocation(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in, opts...)
	}
	return &rdbpb.RootInvocation{}, nil
}

func (m *mockAnalysisRDBClient) QueryTestVerdicts(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestVerdictsResponse, error) {
	if m.queryTestVerdicts != nil {
		return m.queryTestVerdicts(ctx, in, opts...)
	}
	return &rdbpb.QueryTestVerdictsResponse{}, nil
}

func execTestHistoryCmd(thMock *mockTestHistoryClient, fixedNow time.Time, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	cmd := TestHistoryCmd(&base.AuthFlags{})
	run := cmd.CommandRun().(*testHistoryRun)
	run.thClient = thMock
	run.out = &out
	run.errOut = &errOut
	run.now = fixedNow
	return run.Run(nil, args, nil), out.String(), errOut.String()
}

func TestTestHistorySubcommand(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC)

	ftt.Run("TestHistoryCmd executes cross-build flakiness & verdict triage", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("Command metadata and registration", func(t *ftt.Test) {
			cmd := TestHistoryCmd(&base.AuthFlags{})
			assert.Loosely(t, cmd.UsageLine, should.ContainSubstring("test-history"))
			assert.Loosely(t, cmd.CommandRun(), should.NotBeNil)

			run := cmd.CommandRun().(*testHistoryRun)
			assert.Loosely(t, run.Flags.Parse([]string{
				"-test-id", "t1",
				"-module-name", "m1",
				"-module-scheme", "junit",
				"-sub-realm", "ci",
			}), should.BeNil)
			assert.Loosely(t, run.testID, should.Equal("t1"))
			assert.Loosely(t, run.moduleName, should.Equal("m1"))
			assert.Loosely(t, run.moduleScheme, should.Equal("junit"))
			assert.Loosely(t, run.subRealm, should.Equal("ci"))

			run2 := TestHistoryCmd(&base.AuthFlags{}).CommandRun().(*testHistoryRun)
			assert.Loosely(t, run2.Flags.Parse([]string{"-modulescheme", "gtest"}), should.BeNil)
			assert.Loosely(t, run2.moduleScheme, should.Equal("gtest"))

			code, _, errStr := execTestHistoryCmd(nil, fixedNow, "-project", "chromium", "-test-id=", "-test-substring", "Suite#Case")
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, errStr, should.ContainSubstring("specify either -test-substring or -testid, not both"))
		})

		t.Run("Validates required flags and structured test ID resolution", func(t *ftt.Test) {
			_, err := fetchTestHistory(ctx, nil, "", "", "", nil, 7, defaultTestHistoryLimit, fixedNow)
			assert.Loosely(t, err, should.ErrLike("missing required -project flag"))

			_, err = fetchTestHistory(ctx, nil, "projects/turquoise", "", "", nil, 7, defaultTestHistoryLimit, fixedNow)
			assert.Loosely(t, err, should.ErrLike("expected a bare ID"))

			_, err = fetchTestHistory(ctx, nil, "turquoise", "", "", nil, 7, defaultTestHistoryLimit, fixedNow)
			assert.Loosely(t, err, should.ErrLike("missing required -testid flag"))

			_, err = fetchTestHistory(ctx, nil, "turquoise", "my_test", "", nil, 0, defaultTestHistoryLimit, fixedNow)
			assert.Loosely(t, err, should.ErrLike("-days must be positive"))

			_, err = fetchTestHistory(ctx, nil, "turquoise", "my_test", "", []string{"missing_separator"}, 7, defaultTestHistoryLimit, fixedNow)
			assert.Loosely(t, err, should.ErrLike("invalid -variant filter"))

			// Structured test ID resolution
			encoded, err := resolveTestHistoryTestID("", "//chrome/test:browser_tests", "gtest", "", "WebAppBrowserTest", "BrowserDisplayNotInstallable/All.0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, encoded, should.Equal(`://chrome/test\:browser_tests!gtest::WebAppBrowserTest#BrowserDisplayNotInstallable/All.0`))

			_, err = resolveTestHistoryTestID("flat_id", "//mod", "gtest", "", "", "case")
			assert.Loosely(t, err, should.ErrLike("not both"))

			_, err = resolveTestHistoryTestID("", "//mod", "", "", "", "case")
			assert.Loosely(t, err, should.ErrLike("requires at least -modulename, -scheme, and -case"))
		})

		t.Run("Aggregates paginated QueryStats without double-counting exonerations, tracks recency, and formats output", func(t *ftt.Test) {
			var capturedStatsPredicate *analysispb.TestVerdictPredicate
			thMock := &mockTestHistoryClient{
				queryVariants: func(ctx context.Context, in *analysispb.QueryVariantsRequest, opts ...grpc.CallOption) (*analysispb.QueryVariantsResponse, error) {
					assert.Loosely(t, in.Project, should.Equal("turquoise"))
					assert.Loosely(t, in.SubRealm, should.Equal("ci"))
					return &analysispb.QueryVariantsResponse{
						Variants: []*analysispb.QueryVariantsResponse_VariantInfo{
							{
								VariantHash: "hash-hw-vim3",
								Variant:     &analysispb.Variant{Def: map[string]string{"board": "vim3", "builder": "core.vim3-debug"}},
							},
							{
								VariantHash: "hash-emu-qemu",
								Variant:     &analysispb.Variant{Def: map[string]string{"board": "arm64", "builder": "core.arm64-debug"}},
							},
						},
					}, nil
				},
				queryStats: func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
					capturedStatsPredicate = in.Predicate
					if in.PageToken == "" {
						return &analysispb.QueryTestHistoryStatsResponse{
							Groups: []*analysispb.QueryTestHistoryStatsResponse_Group{
								{
									PartitionTime: timestamppb.New(fixedNow.Add(-24 * time.Hour)),
									VariantHash:   "hash-hw-vim3",
									VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
										Passed:           10,
										Flaky:            3,
										Failed:           5,
										ExecutionErrored: 1,
										FailedExonerated: 2,
									},
								},
								{
									PartitionTime: timestamppb.New(fixedNow.Add(-24 * time.Hour)),
									VariantHash:   "hash-emu-qemu",
									VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
										Passed: 40,
										Flaky:  1,
										Failed: 0,
									},
								},
							},
							NextPageToken: "page2",
						}, nil
					}
					return &analysispb.QueryTestHistoryStatsResponse{
						Groups: []*analysispb.QueryTestHistoryStatsResponse_Group{
							{
								PartitionTime: timestamppb.New(fixedNow.Add(-48 * time.Hour)),
								VariantHash:   "hash-hw-vim3",
								// Also cover deprecated v1 fallback counts in second partition bucket
								ExpectedCount:   5,
								FlakyCount:      1,
								UnexpectedCount: 2,
								ExoneratedCount: 1,
							},
						},
					}, nil
				},
				query: func(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error) {
					return &analysispb.QueryTestHistoryResponse{
						Verdicts: []*analysispb.TestVerdict{
							{
								TestId:        in.TestId,
								VariantHash:   "hash-emu-qemu",
								InvocationId:  "build-8888888888888888881",
								StatusV2:      analysispb.TestVerdict_PASSED,
								PartitionTime: timestamppb.New(fixedNow.Add(-1 * time.Hour)),
							},
							{
								TestId:         in.TestId,
								VariantHash:    "hash-hw-vim3",
								InvocationId:   "invocations/build-8888888888888888882",
								StatusV2:       analysispb.TestVerdict_FAILED,
								StatusOverride: analysispb.TestVerdict_EXONERATED,
								PartitionTime:  timestamppb.New(fixedNow.Add(-2 * time.Hour)),
							},
							{
								TestId:        in.TestId,
								VariantHash:   "hash-hw-vim3",
								InvocationId:  "build-8888888888888888883",
								StatusV2:      analysispb.TestVerdict_FLAKY,
								PartitionTime: timestamppb.New(fixedNow.Add(-4 * time.Hour)),
							},
						},
					}, nil
				},
			}

			report, err := fetchTestHistory(
				ctx,
				thMock,
				"turquoise",
				"fuchsia-pkg://fuchsia.com/netstack#meta/test.cm",
				"turquoise:ci", // verifies "<project>:" prefix stripping
				[]string{"board:vim3"},
				120, // verifies clamping to 90d max TTL
				defaultTestHistoryLimit,
				fixedNow,
			)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, report.Days, should.Equal(90))
			assert.Loosely(t, report.SubRealm, should.Equal("ci"))
			assert.Loosely(t, report.AllVerdictsPassed, should.BeFalse)
			assert.Loosely(t, report.NewerPassingCount, should.Equal(1))
			assert.Loosely(t, capturedStatsPredicate.GetSubRealm(), should.Equal("ci"))
			assert.Loosely(t, capturedStatsPredicate.GetVariantPredicate().GetContains().GetDef()["board"], should.Equal("vim3"))

			// Variant stats ordered by non-passing count DESC: hash-hw-vim3 (5 un-exonerated failed + 3 exonerated + 4 flaky + 1 execution_errored = 13 non-passing, 15 passed -> total 28)
			assert.Loosely(t, len(report.VariantStats), should.Equal(2))
			assert.Loosely(t, report.VariantStats[0].VariantHash, should.Equal("hash-hw-vim3"))
			assert.Loosely(t, report.VariantStats[0].Counts.Passed, should.Equal(int32(15)))
			assert.Loosely(t, report.VariantStats[0].Counts.Flaky, should.Equal(int32(4)))
			assert.Loosely(t, report.VariantStats[0].Counts.Failed, should.Equal(int32(5)))
			assert.Loosely(t, report.VariantStats[0].Counts.ExecutionErrored, should.Equal(int32(1)))
			assert.Loosely(t, report.VariantStats[0].Counts.Exonerated, should.Equal(int32(3)))
			assert.Loosely(t, report.VariantStats[0].Total, should.Equal(int32(28)))
			assert.Loosely(t, report.VariantStats[0].LatestNonPassingDate, should.Equal("2026-10-08"))

			// Verdicts timeline keeps non-passing verdicts and normalizes invocation prefix
			assert.Loosely(t, len(report.Verdicts), should.Equal(2))
			assert.Loosely(t, report.Verdicts[0].InvocationID, should.Equal("build-8888888888888888882"))
			assert.Loosely(t, report.Verdicts[0].Status, should.Equal("FAILED"))
			assert.Loosely(t, report.Verdicts[0].Exonerated, should.BeTrue)
			assert.Loosely(t, report.Verdicts[1].Status, should.Equal("FLAKY"))

			var textBuf bytes.Buffer
			assert.Loosely(t, printTestHistoryReport(ctx, &textBuf, nil, report), should.BeNil)
			out := textBuf.String()
			assert.Loosely(t, out, should.ContainSubstring("Project:   turquoise\nSubRealm:  ci\nTest ID:   fuchsia-pkg://fuchsia.com/netstack#meta/test.cm\nWindow:    last 90d\n"))
			assert.Loosely(t, out, should.ContainSubstring("- [5 failed, 1 execution_errored, 4 flaky, 3 exonerated, 15 passed] board=vim3 builder=core.vim3-debug (hash: hash-hw-vim3, last non-passing: 2026-10-08)"))
			assert.Loosely(t, out, should.ContainSubstring("Recent Non-Passing Verdicts (showing 2; 1 newer verdicts passed since latest non-passing):"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED (EXONERATED) build-8888888888888888882 [2026-10-09T00:00:00Z]"))
			assert.Loosely(t, out, should.ContainSubstring("luci verdict get -invocationid build-8888888888888888882 -testid \"fuchsia-pkg://fuchsia.com/netstack#meta/test.cm\" -varianthash hash-hw-vim3"))
		})

		t.Run("Renders structured test IDs and falls back to recent passing verdicts when all runs passed", func(t *ftt.Test) {
			thMock := &mockTestHistoryClient{
				queryVariants: func(ctx context.Context, in *analysispb.QueryVariantsRequest, opts ...grpc.CallOption) (*analysispb.QueryVariantsResponse, error) {
					return nil, status.Error(codes.PermissionDenied, "variants restricted")
				},
				queryStats: func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
					return &analysispb.QueryTestHistoryStatsResponse{
						Groups: []*analysispb.QueryTestHistoryStatsResponse_Group{
							{
								VariantHash:   "hash-pass-only",
								VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Passed: 12},
							},
						},
					}, nil
				},
				query: func(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error) {
					return &analysispb.QueryTestHistoryResponse{
						Verdicts: []*analysispb.TestVerdict{
							{
								TestId:        in.TestId,
								VariantHash:   "hash-pass-only",
								InvocationId:  "ants-i999",
								StatusV2:      analysispb.TestVerdict_PASSED,
								PartitionTime: timestamppb.New(fixedNow.Add(-30 * time.Minute)),
							},
						},
					}, nil
				},
			}

			report, err := fetchTestHistory(
				ctx,
				thMock,
				"android",
				":Module!junit:pkg:Class#testMethod",
				"",
				nil,
				7,
				defaultTestHistoryLimit,
				fixedNow,
			)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, report.AllVerdictsPassed, should.BeTrue)
			assert.Loosely(t, len(report.Verdicts), should.Equal(1))

			var textBuf bytes.Buffer
			assert.Loosely(t, printTestHistoryReport(ctx, &textBuf, nil, report), should.BeNil)
			out := textBuf.String()
			assert.Loosely(t, out, should.ContainSubstring("Test ID:\n  Module:    Module (scheme: junit)\n  Coarse:    pkg\n  Fine:      Class\n  Case:      testMethod\n"))
			assert.Loosely(t, out, should.ContainSubstring("Recent Passing Verdicts (no failing/flaky verdicts in window; showing 1):"))
			assert.Loosely(t, out, should.ContainSubstring("luci verdict get -invocationid ants-i999"))
		})

		t.Run("Executes testHistoryRun.Run with positional Milo test history URL", func(t *ftt.Test) {
			thMock := &mockTestHistoryClient{
				queryStats: func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
					assert.Loosely(t, in.Project, should.Equal("turquoise"))
					assert.Loosely(t, in.TestId, should.Equal("fuchsia-pkg://fuchsia.com/netstack#meta/test.cm"))
					return &analysispb.QueryTestHistoryStatsResponse{
						Groups: []*analysispb.QueryTestHistoryStatsResponse_Group{
							{
								VariantHash:   "hash-1",
								VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Failed: 1},
							},
						},
					}, nil
				},
				query: func(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error) {
					return &analysispb.QueryTestHistoryResponse{
						Verdicts: []*analysispb.TestVerdict{
							{
								TestId:       in.TestId,
								VariantHash:  "hash-1",
								InvocationId: "build-8888888888888888882",
								StatusV2:     analysispb.TestVerdict_FAILED,
							},
						},
					}, nil
				},
			}

			var out bytes.Buffer
			cmd := TestHistoryCmd(&base.AuthFlags{})
			run := cmd.CommandRun().(*testHistoryRun)
			run.thClient = thMock
			run.out = &out
			miloURL := "https://ci.chromium.org/ui/test/turquoise/fuchsia-pkg%3A%2F%2Ffuchsia.com%2Fnetstack%23meta%2Ftest.cm"
			assert.Loosely(t, run.Flags.Parse([]string{miloURL, "-days", "14"}), should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
			assert.Loosely(t, out.String(), should.ContainSubstring("Project:           turquoise"))
			assert.Loosely(t, out.String(), should.ContainSubstring("Test ID:           fuchsia-pkg://fuchsia.com/netstack#meta/test.cm"))
			assert.Loosely(t, out.String(), should.ContainSubstring("Window:    last 14d"))
		})

		t.Run("many_variants_truncation_and_presubmit_labels", func(t *ftt.Test) {
			var variantInfos []*analysispb.QueryVariantsResponse_VariantInfo
			var statGroups []*analysispb.QueryTestHistoryStatsResponse_Group

			// 1: presubmit failing builder (CL verdicts only)
			variantInfos = append(variantInfos, &analysispb.QueryVariantsResponse_VariantInfo{
				VariantHash: "hash-try-win",
				Variant:     &analysispb.Variant{Def: map[string]string{"builder": "win-rel", "os": "Windows-10", "test_suite": "browser_tests"}},
			})
			statGroups = append(statGroups, &analysispb.QueryTestHistoryStatsResponse_Group{
				PartitionTime: timestamppb.New(fixedNow.Add(-6 * time.Hour)),
				VariantHash:   "hash-try-win",
				VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Failed: 60, Passed: 400},
			})

			// 2: shared presubmit+submitted builder (both CL and non-CL verdicts on same hash)
			variantInfos = append(variantInfos, &analysispb.QueryVariantsResponse_VariantInfo{
				VariantHash: "hash-shared-linux",
				Variant:     &analysispb.Variant{Def: map[string]string{"os": "Ubuntu-22.04", "test_suite": "browser_tests"}},
			})
			statGroups = append(statGroups, &analysispb.QueryTestHistoryStatsResponse_Group{
				PartitionTime: timestamppb.New(fixedNow.Add(-12 * time.Hour)),
				VariantHash:   "hash-shared-linux",
				VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Failed: 3, Passed: 100},
			})

			// 3..11: 9 skipped-only CI builders + 12: 1 passing-only presubmit builder (CL verdict only in passing verdicts)
			for i := 0; i < 9; i++ {
				h := fmt.Sprintf("hash-skip-top-%02d", i)
				variantInfos = append(variantInfos, &analysispb.QueryVariantsResponse_VariantInfo{
					VariantHash: h,
					Variant:     &analysispb.Variant{Def: map[string]string{"bucket": "ci", "builder": fmt.Sprintf("android-x64-%02d", i), "test_suite": "browser_tests"}},
				})
				statGroups = append(statGroups, &analysispb.QueryTestHistoryStatsResponse_Group{
					VariantHash:   h,
					VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Skipped: int32(300 - i)},
				})
			}
			variantInfos = append(variantInfos, &analysispb.QueryVariantsResponse_VariantInfo{
				VariantHash: "hash-try-mac-pass",
				Variant:     &analysispb.Variant{Def: map[string]string{"builder": "mac-rel", "os": "Mac-26", "test_suite": "browser_tests"}},
			})
			statGroups = append(statGroups, &analysispb.QueryTestHistoryStatsResponse_Group{
				VariantHash:   "hash-try-mac-pass",
				VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Passed: 200},
			})

			// 13..16: 4 hidden variants (2 skipped-only, 1 passing-only, 1 passing+skipped)
			variantInfos = append(variantInfos,
				&analysispb.QueryVariantsResponse_VariantInfo{
					VariantHash: "hash-hid-pass",
					Variant:     &analysispb.Variant{Def: map[string]string{"bucket": "ci", "builder": "linux-pass"}},
				},
				&analysispb.QueryVariantsResponse_VariantInfo{
					VariantHash: "hash-hid-mixed",
					Variant:     &analysispb.Variant{Def: map[string]string{"bucket": "ci", "builder": "linux-mixed"}},
				},
				&analysispb.QueryVariantsResponse_VariantInfo{
					VariantHash: "hash-hid-skip1",
					Variant:     &analysispb.Variant{Def: map[string]string{"bucket": "ci", "builder": "x86-skip-1"}},
				},
				&analysispb.QueryVariantsResponse_VariantInfo{
					VariantHash: "hash-hid-skip2",
					Variant:     &analysispb.Variant{Def: map[string]string{"builder": "x86-try-skip-2"}},
				},
			)
			statGroups = append(statGroups,
				&analysispb.QueryTestHistoryStatsResponse_Group{
					VariantHash:   "hash-hid-pass",
					VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Passed: 50},
				},
				&analysispb.QueryTestHistoryStatsResponse_Group{
					VariantHash:   "hash-hid-mixed",
					VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Passed: 20, Skipped: 10},
				},
				&analysispb.QueryTestHistoryStatsResponse_Group{
					VariantHash:   "hash-hid-skip1",
					VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Skipped: 15},
				},
				&analysispb.QueryTestHistoryStatsResponse_Group{
					VariantHash:   "hash-hid-skip2",
					VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Skipped: 10},
				},
			)

			thMock := &mockTestHistoryClient{
				queryVariants: func(ctx context.Context, in *analysispb.QueryVariantsRequest, opts ...grpc.CallOption) (*analysispb.QueryVariantsResponse, error) {
					return &analysispb.QueryVariantsResponse{Variants: variantInfos}, nil
				},
				queryStats: func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
					return &analysispb.QueryTestHistoryStatsResponse{Groups: statGroups}, nil
				},
				query: func(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error) {
					return &analysispb.QueryTestHistoryResponse{
						Verdicts: []*analysispb.TestVerdict{
							{
								TestId:        in.TestId,
								VariantHash:   "hash-try-mac-pass",
								InvocationId:  "build-9000",
								StatusV2:      analysispb.TestVerdict_PASSED,
								PartitionTime: timestamppb.New(fixedNow.Add(-10 * time.Minute)),
								Changelists:   []*analysispb.Changelist{{Host: "chromium-review.googlesource.com", Change: 124, Patchset: 1}},
							},
							{
								TestId:        in.TestId,
								VariantHash:   "hash-shared-linux",
								InvocationId:  "build-9001",
								StatusV2:      analysispb.TestVerdict_PASSED,
								PartitionTime: timestamppb.New(fixedNow.Add(-20 * time.Minute)),
								Changelists:   []*analysispb.Changelist{{Host: "chromium-review.googlesource.com", Change: 125, Patchset: 1}},
							},
							{
								TestId:        in.TestId,
								VariantHash:   "hash-try-win",
								InvocationId:  "build-9002",
								StatusV2:      analysispb.TestVerdict_FAILED,
								PartitionTime: timestamppb.New(fixedNow.Add(-30 * time.Minute)),
								Changelists:   []*analysispb.Changelist{{Host: "chromium-review.googlesource.com", Change: 123, Patchset: 1}},
							},
							{
								TestId:        in.TestId,
								VariantHash:   "hash-shared-linux",
								InvocationId:  "build-9003",
								StatusV2:      analysispb.TestVerdict_FAILED,
								PartitionTime: timestamppb.New(fixedNow.Add(-40 * time.Minute)),
							},
							{
								TestId:        in.TestId,
								VariantHash:   "hash-try-win",
								InvocationId:  "build-9004",
								StatusV2:      analysispb.TestVerdict_FAILED,
								PartitionTime: timestamppb.New(fixedNow.Add(-50 * time.Minute)),
								Changelists:   []*analysispb.Changelist{{Host: "chromium-review.googlesource.com", Change: 126, Patchset: 1}},
							},
							{
								TestId:        in.TestId,
								VariantHash:   "hash-try-win",
								InvocationId:  "build-9005",
								StatusV2:      analysispb.TestVerdict_FAILED,
								PartitionTime: timestamppb.New(fixedNow.Add(-60 * time.Minute)),
								Changelists:   []*analysispb.Changelist{{Host: "chromium-review.googlesource.com", Change: 127, Patchset: 1}},
							},
						},
						NextPageToken: "more-pages",
					}, nil
				},
			}

			code, out, errStr := execTestHistoryCmd(thMock, fixedNow, "-project", "chromium", "-testid", `://chrome/test\:browser_tests!gtest::OmniboxTest#Composebox`)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, errStr, should.BeEmpty)

			// Default -limit=3 is preserved even with 16 variants (>12), and omitted non-passing count (63 - 3 = 60) is shown.
			assert.Loosely(t, out, should.ContainSubstring("Variants (16 total, 2 with non-passing verdicts, 11 skipped-only)"))
			assert.Loosely(t, out, should.ContainSubstring("builder=win-rel os=Windows-10 test_suite=browser_tests (presubmit, hash: hash-try-win, last non-passing: 2026-10-08)"))
			// Shared hash mixing CL and submitted verdicts gets (presubmit+submitted, ...).
			assert.Loosely(t, out, should.ContainSubstring("os=Ubuntu-22.04 test_suite=browser_tests (presubmit+submitted, hash: hash-shared-linux, last non-passing: 2026-10-08)"))
			// Passing-only presubmit builder in row 12 gets (presubmit) from page-wide changelist scan even though passing verdicts are omitted from report.Verdicts.
			assert.Loosely(t, out, should.ContainSubstring("[200 passed] builder=mac-rel os=Mac-26 test_suite=browser_tests (presubmit, hash: hash-try-mac-pass)"))
			assert.Loosely(t, out, should.ContainSubstring("... +4 more variants (1 passing-only, 1 passing+skipped, 2 skipped-only; pass -variant <k:v> or -subrealm <realm> to narrow)"))
			assert.Loosely(t, out, should.ContainSubstring("Recent Non-Passing Verdicts (showing 3 of 63, 60 omitted; 2 newer verdicts passed since latest non-passing; pass -limit 0 for all):"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED (presubmit CL) build-9002 [2026-10-09T01:30:00Z]\n    Variant: builder=win-rel os=Windows-10 test_suite=browser_tests (hash: hash-try-win)"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED build-9003 [2026-10-09T01:20:00Z]\n    Variant: os=Ubuntu-22.04 test_suite=browser_tests (hash: hash-shared-linux)"))
			assert.Loosely(t, out, should.NotContainSubstring("build-9005"))

			// Unset or empty -variant filters for ANY key fail fast with a generic -subrealm hint instead of silently matching 0 rows.
			for _, badFilter := range []string{"bucket:(unset)", "board:(unset)", "os:"} {
				badCode, _, badErr := execTestHistoryCmd(thMock, fixedNow, "-project", "turquoise", "-testid", "t1", "-variant", badFilter)
				assert.Loosely(t, badCode, should.Equal(1))
				assert.Loosely(t, badErr, should.ContainSubstring("pass -subrealm <realm>"))
			}

			// Unit-check edge cases in helper functions.
			omitted, plus := computeOmittedNonPassing(3, 3, true, reportCountsWithFailed(3), false)
			assert.Loosely(t, omitted, should.Equal(0))
			assert.Loosely(t, plus, should.BeFalse)

			// When QueryStats lags behind Query (statsNonPassing < observedNonPassing <= shownCount) and hasMorePages is true, keep "+".
			omitted, plus = computeOmittedNonPassing(3, 3, true, reportCountsWithFailed(2), false)
			assert.Loosely(t, omitted, should.Equal(0))
			assert.Loosely(t, plus, should.BeTrue)

			// When !hasMorePages, observedNonPassing is exact even if 24h-bucketed stats are higher.
			omitted, plus = computeOmittedNonPassing(3, 5, false, reportCountsWithFailed(10), false)
			assert.Loosely(t, omitted, should.Equal(2))
			assert.Loosely(t, plus, should.BeFalse)

			// When hasMorePages is true (e.g. older passing pages) and untruncated stats match observedNonPassing, no "+".
			omitted, plus = computeOmittedNonPassing(3, 5, true, reportCountsWithFailed(5), false)
			assert.Loosely(t, omitted, should.Equal(2))
			assert.Loosely(t, plus, should.BeFalse)

			// When stats are truncated and exceed observedNonPassing, omitted count is a lower bound ("+").
			omitted, plus = computeOmittedNonPassing(3, 5, true, reportCountsWithFailed(10), true)
			assert.Loosely(t, omitted, should.Equal(7))
			assert.Loosely(t, plus, should.BeTrue)

			// formatHiddenVariantsBreakdown handles non-passing, nil Counts, and zero-count variants safely.
			breakdown := formatHiddenVariantsBreakdown([]variantStatsSummary{
				{Counts: &rdbpb.TestAggregation_VerdictCounts{Failed: 2}},
				{Counts: nil},
				{Counts: &rdbpb.TestAggregation_VerdictCounts{}},
				{Counts: &rdbpb.TestAggregation_VerdictCounts{Passed: 4}},
			})
			assert.Loosely(t, breakdown, should.Equal("1 with non-passing, 1 passing-only"))

			// Positive -limit exhausting maxTestHistoryPages (10 pages) with < limit non-passing verdicts
			// sets VerdictsPageCapped = true instead of suggesting -limit 0.
			cappedMock := &mockTestHistoryClient{
				queryStats: func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
					return &analysispb.QueryTestHistoryStatsResponse{
						Groups: []*analysispb.QueryTestHistoryStatsResponse_Group{
							{
								VariantHash:   "h1",
								VerdictCounts: &analysispb.QueryTestHistoryStatsResponse_Group_VerdictCounts{Failed: 5, Passed: 100},
							},
						},
					}, nil
				},
				query: func(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error) {
					st := analysispb.TestVerdict_PASSED
					if in.PageToken == "" {
						st = analysispb.TestVerdict_FAILED
					}
					return &analysispb.QueryTestHistoryResponse{
						Verdicts: []*analysispb.TestVerdict{
							{
								TestId:        in.TestId,
								VariantHash:   "h1",
								InvocationId:  "build-cap",
								StatusV2:      st,
								PartitionTime: timestamppb.New(fixedNow.Add(-1 * time.Hour)),
							},
						},
						NextPageToken: "next",
					}, nil
				},
			}
			capCode, capOut, _ := execTestHistoryCmd(cappedMock, fixedNow, "-project", "chromium", "-testid", "t_cap")
			assert.Loosely(t, capCode, should.Equal(0))
			assert.Loosely(t, capOut, should.ContainSubstring("Recent Non-Passing Verdicts (showing 1 of 5, 4 omitted; page cap reached; pass -variant <k:v> or smaller -days to narrow):"))
		})
	})
}

func TestAndroidSourceVerdictHistory(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 11, 19, 4, 0, 0, 0, time.UTC)

	type androidEnv struct {
		app     subcommands.Application
		th      analysispb.TestHistoryClient
		tvb     analysispb.TestVariantBranchesClient
		rdb     rdbpb.ResultDBClient
		schemas rdbpb.SchemasClient
	}
	runCmd := func(env androidEnv, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		run := TestHistoryCmd(&base.AuthFlags{}).CommandRun().(*testHistoryRun)
		run.thClient = env.th
		run.tvbClient = env.tvb
		run.rdbClient = env.rdb
		run.schemasClient = env.schemas
		run.out = &out
		run.errOut = &errOut
		run.now = fixedNow
		code := run.Run(env.app, args, nil)
		return code, out.String(), errOut.String()
	}

	ftt.Run("android_source_verdict_history", t, func(t *ftt.Test) {
		const androidTestID = ":CtsVoiceInteractionTestCases!junit:android.voiceinteraction.cts:DirectActionsTest#testPerformDirectAction"

		t.Run("falls_back_to_test_variant_branches_and_source_verdicts", func(t *ftt.Test) {
			const hashX86 = "a1b2c3d4e5f60718"
			const hashArm64 = "b2c3d4e5f6071829"
			var capturedBranch string
			tvbMock := &mockTestVariantBranchesClient{
				query: func(ctx context.Context, in *analysispb.QueryTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.QueryTestVariantBranchResponse, error) {
					capturedBranch = in.GetRef().GetAndroidBuild().GetBranch()
					assert.Loosely(t, in.GetRef().GetAndroidBuild().GetDataRealm(), should.Equal("prod"))
					return &analysispb.QueryTestVariantBranchResponse{
						TestVariantBranch: []*analysispb.TestVariantBranch{
							{
								VariantHash: hashX86,
								Variant:     &analysispb.Variant{Def: map[string]string{"module_abi": "x86_64"}},
								Segments: []*analysispb.Segment{
									{
										StartHour: timestamppb.New(fixedNow.Add(-20 * 24 * time.Hour)),
										EndHour:   timestamppb.New(fixedNow.Add(-2 * time.Hour)),
										Counts: &analysispb.Segment_Counts{
											TotalVerdicts:      120,
											FlakyVerdicts:      8,
											UnexpectedVerdicts: 2,
										},
									},
									{
										// Older segment outside 7d window should be excluded.
										StartHour: timestamppb.New(fixedNow.Add(-60 * 24 * time.Hour)),
										EndHour:   timestamppb.New(fixedNow.Add(-40 * 24 * time.Hour)),
										Counts: &analysispb.Segment_Counts{
											TotalVerdicts:      999,
											UnexpectedVerdicts: 50,
										},
									},
								},
							},
							{
								VariantHash: hashArm64,
								Variant:     &analysispb.Variant{Def: map[string]string{"module_abi": "arm64-v8a"}},
								Segments: []*analysispb.Segment{
									{
										StartHour: timestamppb.New(fixedNow.Add(-5 * 24 * time.Hour)),
										EndHour:   timestamppb.New(fixedNow.Add(-1 * time.Hour)),
										Counts: &analysispb.Segment_Counts{
											TotalVerdicts: 80,
										},
									},
								},
							},
						},
					}, nil
				},
			}
			thMock := &mockTestHistoryClient{
				querySourceVerdicts: func(ctx context.Context, in *analysispb.QuerySourceVerdictsV2Request, opts ...grpc.CallOption) (*analysispb.QuerySourceVerdictsV2Response, error) {
					switch in.GetTestIdFlat().GetVariantHash() {
					case hashX86:
						return &analysispb.QuerySourceVerdictsV2Response{
							SourceVerdicts: []*analysispb.SourceVerdict{
								{
									Position: 12900100,
									InvocationVerdicts: []*analysispb.SourceVerdict_InvocationTestVerdict{
										{
											RootInvocation: "rootInvocations/ants-i100000000000000001",
											PartitionTime:  timestamppb.New(fixedNow.Add(-30 * time.Minute)),
											Status:         analysispb.TestVerdict_PASSED,
										},
									},
								},
								{
									Position: 12900050,
									InvocationVerdicts: []*analysispb.SourceVerdict_InvocationTestVerdict{
										{
											RootInvocation: "rootInvocations/ants-i100000000000000002",
											PartitionTime:  timestamppb.New(fixedNow.Add(-6 * time.Hour)),
											Status:         analysispb.TestVerdict_FLAKY,
										},
										{
											// Out-of-window verdict (>7d old) should be skipped.
											RootInvocation: "rootInvocations/ants-i100000000000000099",
											PartitionTime:  timestamppb.New(fixedNow.Add(-15 * 24 * time.Hour)),
											Status:         analysispb.TestVerdict_FAILED,
										},
									},
								},
							},
						}, nil
					case hashArm64:
						// Unsubmitted presubmit CL failure on a 100%-passing postsubmit
						// variant must be annotated (presubmit CL) and must not stamp
						// last non-passing onto the [80 passed] postsubmit stats row.
						return &analysispb.QuerySourceVerdictsV2Response{
							SourceVerdicts: []*analysispb.SourceVerdict{
								{
									Position: 12900090,
									InvocationVerdicts: []*analysispb.SourceVerdict_InvocationTestVerdict{
										{
											RootInvocation: "rootInvocations/ants-i100000000000000055",
											PartitionTime:  timestamppb.New(fixedNow.Add(-2 * time.Hour)),
											Status:         analysispb.TestVerdict_FAILED,
											Changelists: []*analysispb.Changelist{
												{Host: "android-review.googlesource.com", Change: 12345},
											},
										},
									},
								},
							},
						}, nil
					default:
						return &analysispb.QuerySourceVerdictsV2Response{}, nil
					}
				},
			}

			code, stdout, stderr := runCmd(androidEnv{th: thMock, tvb: tvbMock}, "-project", "android", "-testid", androidTestID)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, stderr, should.BeEmpty)
			assert.Loosely(t, capturedBranch, should.Equal("git_main"))
			assert.Loosely(t, stdout, should.ContainSubstring("Project:   android\nBranch:    git_main\n"))
			assert.Loosely(t, stdout, should.ContainSubstring("Variants (2 total, 1 with non-passing verdicts, branch segment counts; active segments start before window) [2 failed, 8 flaky, 190 passed]:"))
			assert.Loosely(t, stdout, should.ContainSubstring("- [2 failed, 8 flaky, 110 passed] module_abi=x86_64 (hash: a1b2c3d4e5f60718, last non-passing: 2026-11-18)"))
			assert.Loosely(t, stdout, should.ContainSubstring("- [80 passed] module_abi=arm64-v8a (hash: b2c3d4e5f6071829)\n"))
			assert.Loosely(t, stdout, should.ContainSubstring("Recent Non-Passing Verdicts (showing 1; 1 newer verdicts passed since latest non-passing):"))
			assert.Loosely(t, stdout, should.ContainSubstring("- FLAKY ants-i100000000000000002 [2026-11-18T22:00:00Z]"))
			assert.Loosely(t, stdout, should.NotContainSubstring("ants-i100000000000000099"))
			assert.Loosely(t, stdout, should.ContainSubstring(`luci verdict get -invocationid ants-i100000000000000002 -testid "`+androidTestID+`" -varianthash a1b2c3d4e5f60718`))

			// Verify -variant filter narrows Android TestVariantBranches, surfaces
			// (presubmit CL) annotation, and never stamps last non-passing on [80 passed].
			code, stdout, stderr = runCmd(androidEnv{th: thMock, tvb: tvbMock}, "-project", "android", "-testid", androidTestID, "-variant", "module_abi:arm64-v8a")
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, stderr, should.BeEmpty)
			assert.Loosely(t, stdout, should.ContainSubstring("Variants (1 total, 0 with non-passing verdicts, branch segment counts) [80 passed]:"))
			assert.Loosely(t, stdout, should.ContainSubstring("- [80 passed] module_abi=arm64-v8a (hash: b2c3d4e5f6071829)\n"))
			assert.Loosely(t, stdout, should.ContainSubstring("- FAILED (presubmit CL) ants-i100000000000000055 [2026-11-19T02:00:00Z]"))
		})

		t.Run("extracts_project_and_branch_from_positional_ati_url_including_presubmit_only", func(t *ftt.Test) {
			antsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"testResultId": "TR67230470437261916",
					"invocationId": "I46700010657835987",
					"workUnitId": "WU75000271333595955",
					"testStatus": "FAIL",
					"testIdentifier": {
						"testClass": "com.android.devicehealthchecks.SystemCheck",
						"method": "system_tombstone"
					}
				}`))
			}))
			defer antsSrv.Close()

			app := &cli.Application{
				Name: "luci",
				Context: func(ctx context.Context) context.Context {
					return base.WithAntsHTTPClient(t.Context(), antsSrv.Client(), antsSrv.URL)
				},
			}

			const tombstoneID = ":no-module-name!junit:com.android.devicehealthchecks:SystemCheck#system_tombstone"
			rdbMock := &mockAnalysisRDBClient{
				getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
					return &rdbpb.RootInvocation{
						Name:  in.GetName(),
						Realm: "android:presubmit",
						Sources: &rdbpb.Sources{
							BaseSources: &rdbpb.Sources_SubmittedAndroidBuild{
								SubmittedAndroidBuild: &rdbpb.SubmittedAndroidBuild{
									DataRealm: "prod",
									Branch:    "git_trunk-release",
									BuildId:   12888888,
								},
							},
						},
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestVerdictsResponse, error) {
					return &rdbpb.QueryTestVerdictsResponse{
						TestVerdicts: []*rdbpb.TestVerdict{
							{
								TestId: tombstoneID,
								Results: []*rdbpb.TestResult{
									{
										Name:        "rootInvocations/ants-i46700010657835987/workUnits/ants-wu75000271333595955/tests/t1/results/r1",
										ResultId:    "r1",
										VariantHash: "e3b0c44298fc1c14",
										StatusV2:    rdbpb.TestResult_FAILED,
									},
								},
							},
						},
					}, nil
				},
			}

			var mu sync.Mutex
			var queriedBranch string
			var queriedHashes []string
			// Case 1: Presubmit-only test where TestVariantBranches has 0 rows; falls back
			// to extracted VariantHash and labels stats as "sampled source verdict counts".
			thMock := &mockTestHistoryClient{
				querySourceVerdicts: func(ctx context.Context, in *analysispb.QuerySourceVerdictsV2Request, opts ...grpc.CallOption) (*analysispb.QuerySourceVerdictsV2Response, error) {
					vh := in.GetTestIdFlat().GetVariantHash()
					mu.Lock()
					queriedBranch = in.GetSourceRef().GetAndroidBuild().GetBranch()
					queriedHashes = append(queriedHashes, vh)
					mu.Unlock()
					if vh == "e3b0c44298fc1c14" {
						return &analysispb.QuerySourceVerdictsV2Response{
							SourceVerdicts: []*analysispb.SourceVerdict{
								{
									Position: 12888888,
									InvocationVerdicts: []*analysispb.SourceVerdict_InvocationTestVerdict{
										{
											RootInvocation: "rootInvocations/ants-i46700010657835987",
											PartitionTime:  timestamppb.New(fixedNow.Add(-1 * time.Hour)),
											Status:         analysispb.TestVerdict_FAILED,
										},
									},
								},
							},
						}, nil
					}
					return &analysispb.QuerySourceVerdictsV2Response{}, nil
				},
			}

			atiURL := "https://android-build.corp.google.com/test_investigate/test/TR67230470437261916/"
			code, stdout, stderr := runCmd(androidEnv{app: app, rdb: rdbMock, th: thMock, tvb: &mockTestVariantBranchesClient{}}, "-days", "30", atiURL)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, stderr, should.BeEmpty)
			assert.Loosely(t, queriedBranch, should.Equal("git_trunk-release"))
			assert.Loosely(t, queriedHashes, should.Match([]string{"e3b0c44298fc1c14"}))
			assert.Loosely(t, stdout, should.ContainSubstring("Project:           android\nBranch:            git_trunk-release\n"))
			assert.Loosely(t, stdout, should.ContainSubstring("Project:   android\nBranch:    git_trunk-release\n"))
			assert.Loosely(t, stdout, should.ContainSubstring("sampled source verdict counts"))
			assert.Loosely(t, stdout, should.ContainSubstring("- FAILED ants-i46700010657835987"))

			// Case 2: Even when TestVariantBranches has >= 4 healthy postsubmit variants,
			// positional ATI fallbackVariantHash ("e3b0c44298fc1c14") is still queried!
			tvbManyHealthy := &mockTestVariantBranchesClient{
				query: func(ctx context.Context, in *analysispb.QueryTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.QueryTestVariantBranchResponse, error) {
					var branches []*analysispb.TestVariantBranch
					for _, h := range []string{"0000000000000001", "0000000000000002", "0000000000000003", "0000000000000004", "0000000000000005"} {
						branches = append(branches, &analysispb.TestVariantBranch{
							VariantHash: h,
							Segments: []*analysispb.Segment{
								{
									StartHour: timestamppb.New(fixedNow.Add(-2 * 24 * time.Hour)),
									EndHour:   timestamppb.New(fixedNow.Add(-1 * time.Hour)),
									Counts:    &analysispb.Segment_Counts{TotalVerdicts: 50},
								},
							},
						})
					}
					return &analysispb.QueryTestVariantBranchResponse{TestVariantBranch: branches}, nil
				},
			}
			code2, stdout2, stderr2 := runCmd(androidEnv{app: app, rdb: rdbMock, th: thMock, tvb: tvbManyHealthy}, "-days", "30", atiURL)
			assert.Loosely(t, code2, should.Equal(0))
			assert.Loosely(t, stderr2, should.BeEmpty)
			assert.Loosely(t, stdout2, should.ContainSubstring("- FAILED ants-i46700010657835987"))

			// Case 3: Target-extracted non-default branch ("git_trunk-release") is preserved in
			// Next step re-run commands (and omits conflicting -subrealm) when disambiguating.
			invURL := "https://android-build.corp.google.com/test_investigate/invocation/I46700010657835987/"
			thDisambig := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					return &analysispb.QueryTestsResponse{
						TestIds: []string{tombstoneID + "_A", tombstoneID + "_B"},
					}, nil
				},
			}
			code3, stdout3, stderr3 := runCmd(androidEnv{app: app, rdb: rdbMock, th: thDisambig, tvb: &mockTestVariantBranchesClient{}}, "-subrealm", "presubmit", "-test-substring", "system_tombstone", invURL)
			assert.Loosely(t, code3, should.Equal(0))
			assert.Loosely(t, stderr3, should.BeEmpty)
			assert.Loosely(t, stdout3, should.ContainSubstring("-branch git_trunk-release"))
			assert.Loosely(t, stdout3, should.NotContainSubstring("-subrealm presubmit"))
		})

		t.Run("supports_explicit_branch_multipage_pagination_and_dirty_sources", func(t *ftt.Test) {
			const varHash = "0123456789abcdef"
			var seenBranch, seenDataRealm string
			rdbStaging := &mockAnalysisRDBClient{
				getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
					return &rdbpb.RootInvocation{
						Name:  in.GetName(),
						Realm: "android:ci",
						Sources: &rdbpb.Sources{
							BaseSources: &rdbpb.Sources_SubmittedAndroidBuild{
								SubmittedAndroidBuild: &rdbpb.SubmittedAndroidBuild{
									DataRealm: "staging",
									Branch:    "git_main",
									BuildId:   12900000,
								},
							},
						},
					}, nil
				},
			}
			tvbMock := &mockTestVariantBranchesClient{
				query: func(ctx context.Context, in *analysispb.QueryTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.QueryTestVariantBranchResponse, error) {
					seenBranch = in.GetRef().GetAndroidBuild().GetBranch()
					seenDataRealm = in.GetRef().GetAndroidBuild().GetDataRealm()
					return &analysispb.QueryTestVariantBranchResponse{
						TestVariantBranch: []*analysispb.TestVariantBranch{
							{
								VariantHash: varHash,
								Variant:     &analysispb.Variant{Def: map[string]string{"module_abi": "x86_64"}},
								Segments: []*analysispb.Segment{
									{
										StartHour: timestamppb.New(fixedNow.Add(-3 * 24 * time.Hour)),
										EndHour:   timestamppb.New(fixedNow.Add(-1 * time.Hour)),
										Counts: &analysispb.Segment_Counts{
											TotalVerdicts:      10,
											UnexpectedVerdicts: 1,
										},
									},
								},
							},
						},
					}, nil
				},
			}
			// Page 0 has >= limit (2) passing verdicts; Page 1 has a clean FAILED verdict and
			// a dirty-sources FAILED verdict. Variant has nonPassingCount() > 0, so pagination
			// must continue to Page 1 instead of stopping at Page 0.
			thMock := &mockTestHistoryClient{
				querySourceVerdicts: func(ctx context.Context, in *analysispb.QuerySourceVerdictsV2Request, opts ...grpc.CallOption) (*analysispb.QuerySourceVerdictsV2Response, error) {
					if in.GetPageToken() == "" {
						return &analysispb.QuerySourceVerdictsV2Response{
							NextPageToken: "page-1",
							SourceVerdicts: []*analysispb.SourceVerdict{
								{
									Position: 12900010,
									InvocationVerdicts: []*analysispb.SourceVerdict_InvocationTestVerdict{
										{
											RootInvocation: "rootInvocations/ants-i200000000000000001",
											PartitionTime:  timestamppb.New(fixedNow.Add(-10 * time.Minute)),
											Status:         analysispb.TestVerdict_PASSED,
										},
										{
											RootInvocation: "rootInvocations/ants-i200000000000000002",
											PartitionTime:  timestamppb.New(fixedNow.Add(-20 * time.Minute)),
											Status:         analysispb.TestVerdict_PASSED,
										},
									},
								},
							},
						}, nil
					}
					return &analysispb.QuerySourceVerdictsV2Response{
						SourceVerdicts: []*analysispb.SourceVerdict{
							{
								Position: 12900005,
								InvocationVerdicts: []*analysispb.SourceVerdict_InvocationTestVerdict{
									{
										RootInvocation: "rootInvocations/ants-i200000000000000003",
										PartitionTime:  timestamppb.New(fixedNow.Add(-2 * time.Hour)),
										Status:         analysispb.TestVerdict_FAILED,
										IsSourcesDirty: true,
									},
									{
										RootInvocation: "rootInvocations/ants-i200000000000000004",
										PartitionTime:  timestamppb.New(fixedNow.Add(-26 * time.Hour)),
										Status:         analysispb.TestVerdict_FAILED,
									},
								},
							},
						},
					}, nil
				},
			}

			// Explicit -branch overrides target-extracted branch ("git_main" -> "git_udc-release")
			// while still preserving target-extracted non-"prod" DataRealm ("staging").
			code, stdout, stderr := runCmd(
				androidEnv{rdb: rdbStaging, th: thMock, tvb: tvbMock},
				"-branch", "git_udc-release",
				"-testid", androidTestID,
				"-limit", "2",
				"ants-i46700010657835987",
			)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, stderr, should.BeEmpty)
			assert.Loosely(t, seenBranch, should.Equal("git_udc-release"))
			assert.Loosely(t, seenDataRealm, should.Equal("staging"))
			assert.Loosely(t, stdout, should.ContainSubstring("Branch:            git_udc-release\n"))
			assert.Loosely(t, stdout, should.ContainSubstring("Project:   android\nBranch:    git_udc-release\n"))
			assert.Loosely(t, stdout, should.ContainSubstring("(hash: 0123456789abcdef, last non-passing: 2026-11-18)"))
			assert.Loosely(t, stdout, should.ContainSubstring("- FAILED (dirty sources) ants-i200000000000000003"))
			assert.Loosely(t, stdout, should.ContainSubstring("- FAILED ants-i200000000000000004"))
		})

		t.Run("sampled_source_verdict_truncation_distinguishes_limit_from_page_cap", func(t *ftt.Test) {
			const varHash = "fedcba9876543210"
			rdbTarget := &mockAnalysisRDBClient{
				getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
					return &rdbpb.RootInvocation{
						Name:  in.GetName(),
						Realm: "android:presubmit",
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestVerdictsResponse, error) {
					return &rdbpb.QueryTestVerdictsResponse{
						TestVerdicts: []*rdbpb.TestVerdict{
							{
								TestId: androidTestID,
								Results: []*rdbpb.TestResult{
									{
										Name:        "rootInvocations/ants-i46700010657835987/workUnits/ants-wu75000271333595955/tests/t1/results/r1",
										ResultId:    "r1",
										VariantHash: varHash,
										StatusV2:    rdbpb.TestResult_FAILED,
									},
								},
							},
						},
					}, nil
				},
			}
			antsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"testResultId": "TR99999999999999999",
					"invocationId": "I46700010657835987",
					"workUnitId": "WU75000271333595955",
					"testStatus": "FAIL",
					"testIdentifier": {"module": "CtsVoiceInteractionTestCases", "testClass": "android.voiceinteraction.cts.DirectActionsTest", "method": "testPerformDirectAction"}
				}`))
			}))
			defer antsSrv.Close()
			app := &cli.Application{
				Name: "luci",
				Context: func(ctx context.Context) context.Context {
					return base.WithAntsHTTPClient(t.Context(), antsSrv.Client(), antsSrv.URL)
				},
			}

			pagesCalled := 0
			thPaged := &mockTestHistoryClient{
				querySourceVerdicts: func(ctx context.Context, in *analysispb.QuerySourceVerdictsV2Request, opts ...grpc.CallOption) (*analysispb.QuerySourceVerdictsV2Response, error) {
					pagesCalled++
					return &analysispb.QuerySourceVerdictsV2Response{
						NextPageToken: "more",
						SourceVerdicts: []*analysispb.SourceVerdict{
							{
								Position: int64(100 - pagesCalled),
								InvocationVerdicts: []*analysispb.SourceVerdict_InvocationTestVerdict{
									{
										RootInvocation: "rootInvocations/ants-i300",
										PartitionTime:  timestamppb.New(fixedNow.Add(-time.Duration(pagesCalled) * time.Hour)),
										Status:         analysispb.TestVerdict_FAILED,
									},
								},
							},
						},
					}, nil
				},
			}

			// 1. Stopping at -limit=1 on page 0 must NOT report "partial stats (page cap reached)",
			// while still reporting "more omitted; pass -limit 0 for all" since NextPageToken != "".
			code1, out1, _ := runCmd(androidEnv{app: app, rdb: rdbTarget, th: thPaged, tvb: &mockTestVariantBranchesClient{}}, "-limit", "1", "TR99999999999999999")
			assert.Loosely(t, code1, should.Equal(0))
			assert.Loosely(t, pagesCalled, should.Equal(1))
			assert.Loosely(t, out1, should.ContainSubstring("sampled source verdict counts)"))
			assert.Loosely(t, out1, should.NotContainSubstring("partial stats (page cap reached)"))
			assert.Loosely(t, out1, should.ContainSubstring("Recent Non-Passing Verdicts (showing 1, more omitted; pass -limit 0 for all):"))

			// 2. Exhausting maxSourceVerdictPages (3 pages) with -limit=0 (or positive -limit=5 > 3 found)
			// MUST report "partial stats (page cap reached)" and the page-cap notice rather than "-limit 0".
			pagesCalled = 0
			code2, out2, _ := runCmd(androidEnv{app: app, rdb: rdbTarget, th: thPaged, tvb: &mockTestVariantBranchesClient{}}, "-limit", "0", "TR99999999999999999")
			assert.Loosely(t, code2, should.Equal(0))
			assert.Loosely(t, pagesCalled, should.Equal(maxSourceVerdictPages))
			assert.Loosely(t, out2, should.ContainSubstring("sampled source verdict counts, partial stats (page cap reached))"))
			assert.Loosely(t, out2, should.ContainSubstring("Recent Non-Passing Verdicts (showing 3, more omitted; page cap reached; pass -variant <k:v> or smaller -days to narrow):"))

			pagesCalled = 0
			code3, out3, _ := runCmd(androidEnv{app: app, rdb: rdbTarget, th: thPaged, tvb: &mockTestVariantBranchesClient{}}, "-limit", "5", "TR99999999999999999")
			assert.Loosely(t, code3, should.Equal(0))
			assert.Loosely(t, pagesCalled, should.Equal(maxSourceVerdictPages))
			assert.Loosely(t, out3, should.ContainSubstring("Recent Non-Passing Verdicts (showing 3, more omitted; page cap reached; pass -variant <k:v> or smaller -days to narrow):"))
		})

		t.Run("unregistered_scheme_tradefed_emits_scheme_hint_and_preserves_valid_suggestions", func(t *ftt.Test) {
			const deadCandidate = ":binderRpcTest!junit:binderRpcTest32:BinderARpcNdk#Dead"
			const realGTestJUnitID = ":binderRpcTest!junit:binderRpcTest64:BinderARpcNdk#ARpcAccessorNewDelete"
			schemasMock := &mockAnalysisSchemasClient{
				getScheme: func(ctx context.Context, in *rdbpb.GetSchemeRequest, opts ...grpc.CallOption) (*rdbpb.Scheme, error) {
					return nil, status.Error(codes.NotFound, `scheme with ID "tradefed" not found`)
				},
			}
			tvbMock := &mockTestVariantBranchesClient{
				query: func(ctx context.Context, in *analysispb.QueryTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.QueryTestVariantBranchResponse, error) {
					if in.GetTestId() == realGTestJUnitID {
						return &analysispb.QueryTestVariantBranchResponse{
							TestVariantBranch: []*analysispb.TestVariantBranch{{VariantHash: "0123456789abcdef"}},
						}, nil
					}
					return &analysispb.QueryTestVariantBranchResponse{}, nil
				},
			}
			thMock := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					return &analysispb.QueryTestsResponse{TestIds: []string{deadCandidate, realGTestJUnitID}}, nil
				},
			}

			code, stdout, stderr := runCmd(
				androidEnv{th: thMock, tvb: tvbMock, schemas: schemasMock},
				"-project", "android",
				"-modulename", "libkoru_get_svgelem_size_gtest",
				"-scheme", "tradefed",
				"-case", "binderRpcTest",
			)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, stderr, should.BeEmpty)
			assert.Loosely(t, stdout, should.ContainSubstring(`Hint: ResultDB has no "tradefed" scheme; Android TradeFed modules use -scheme junit, flat, mobly, or deqp`))
			assert.Loosely(t, stdout, should.NotContainSubstring("ants_cli"))
			assert.Loosely(t, stdout, should.ContainSubstring("Did you mean one of these test IDs?"))
			assert.Loosely(t, stdout, should.NotContainSubstring(deadCandidate))
			assert.Loosely(t, stdout, should.ContainSubstring(realGTestJUnitID))
		})

		t.Run("zero_row_android_suppresses_dead_end_did_you_mean_and_prints_contextual_hints", func(t *ftt.Test) {
			const unindexedCandidate = `:CtsVoiceInteractionTestCases[foldable\:1\:HALF_OPENED]!junit:android.voiceinteraction.cts:DirectActionsTest#testPerformDirectAction`
			const indexedSibling = `:CtsVoiceInteractionTestCases!junit:android.voiceinteraction.cts:DirectActionsTest#testPerformDirectActionHelper`
			queryTestsCalls := 0
			thMock := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					queryTestsCalls++
					return &analysispb.QueryTestsResponse{TestIds: []string{unindexedCandidate}}, nil
				},
			}
			schemasMock := &mockAnalysisSchemasClient{
				getScheme: func(ctx context.Context, in *rdbpb.GetSchemeRequest, opts ...grpc.CallOption) (*rdbpb.Scheme, error) {
					return &rdbpb.Scheme{Id: "junit", HumanReadableName: "JUnit"}, nil
				},
			}

			// 1. 0 branch rows anywhere -> suppresses dead-end suggestion and prints ants_cli hint;
			// surviving candidate with registered scheme is probed once (no duplicate post-pass RPCs).
			var mu sync.Mutex
			probeCalls := make(map[string]int)
			tvbProbeTracker := &mockTestVariantBranchesClient{
				query: func(ctx context.Context, in *analysispb.QueryTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.QueryTestVariantBranchResponse, error) {
					mu.Lock()
					probeCalls[in.GetTestId()]++
					mu.Unlock()
					if in.GetTestId() == indexedSibling {
						return &analysispb.QueryTestVariantBranchResponse{
							TestVariantBranch: []*analysispb.TestVariantBranch{{VariantHash: "1111111111111111"}},
						}, nil
					}
					return &analysispb.QueryTestVariantBranchResponse{}, nil
				},
			}
			code, stdout, _ := runCmd(androidEnv{th: thMock, tvb: tvbProbeTracker, schemas: schemasMock}, "-project", "android", "-testid", androidTestID)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, probeCalls[unindexedCandidate], should.Equal(1))
			assert.Loosely(t, stdout, should.NotContainSubstring("Did you mean"))
			assert.Loosely(t, stdout, should.ContainSubstring(`Hint: LUCI Analysis only indexes Android runs anchored to a submitted build (sources.submittedAndroidBuild) on branch "git_main".`))
			assert.Loosely(t, stdout, should.ContainSubstring("retry with -branch <branch>"))
			assert.Loosely(t, stdout, should.ContainSubstring("ants_cli test-history --test_identifier_id=<ID>"))

			thSurviving := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					return &analysispb.QueryTestsResponse{TestIds: []string{indexedSibling}}, nil
				},
			}
			codeSurv, stdoutSurv, _ := runCmd(androidEnv{th: thSurviving, tvb: tvbProbeTracker, schemas: schemasMock}, "-project", "android", "-testid", androidTestID)
			assert.Loosely(t, codeSurv, should.Equal(0))
			assert.Loosely(t, probeCalls[indexedSibling], should.Equal(1))
			assert.Loosely(t, stdoutSurv, should.ContainSubstring("Did you mean one of these test IDs?"))
			assert.Loosely(t, stdoutSurv, should.ContainSubstring(indexedSibling))

			// 2. Branch rows exist, but active segments ended > 7d ago -> skips leaf suggestion
			// search, preserves -days 90 hint even if an indexed sibling exists (and even when
			// -test-substring populates Other matching test IDs).
			tvbOlderAndSibling := &mockTestVariantBranchesClient{
				query: func(ctx context.Context, in *analysispb.QueryTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.QueryTestVariantBranchResponse, error) {
					switch in.GetTestId() {
					case androidTestID:
						return &analysispb.QueryTestVariantBranchResponse{
							TestVariantBranch: []*analysispb.TestVariantBranch{
								{
									VariantHash: "0123456789abcdef",
									Variant:     &analysispb.Variant{Def: map[string]string{"module_abi": "x86_64"}},
									Segments: []*analysispb.Segment{
										{
											StartHour: timestamppb.New(fixedNow.Add(-40 * 24 * time.Hour)),
											EndHour:   timestamppb.New(fixedNow.Add(-20 * 24 * time.Hour)),
											Counts:    &analysispb.Segment_Counts{TotalVerdicts: 25},
										},
									},
								},
							},
						}, nil
					case indexedSibling:
						return &analysispb.QueryTestVariantBranchResponse{
							TestVariantBranch: []*analysispb.TestVariantBranch{{VariantHash: "1111111111111111"}},
						}, nil
					default:
						return &analysispb.QueryTestVariantBranchResponse{}, nil
					}
				},
			}
			queryTestsCalls = 0
			thWithSibling := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					queryTestsCalls++
					return &analysispb.QueryTestsResponse{
						TestIds: []string{
							androidTestID,
							indexedSibling,
						},
					}, nil
				},
			}
			code2, stdout2, _ := runCmd(androidEnv{th: thWithSibling, tvb: tvbOlderAndSibling, schemas: schemasMock}, "-project", "android", "-testid", androidTestID)
			assert.Loosely(t, code2, should.Equal(0))
			assert.Loosely(t, queryTestsCalls, should.Equal(0))
			assert.Loosely(t, stdout2, should.ContainSubstring(`Hint: Test exists on branch "git_main", but active segments ended more than 7 days ago; retry with -days 90.`))
			assert.Loosely(t, stdout2, should.NotContainSubstring("ants_cli"))

			code2Sub, stdout2Sub, _ := runCmd(androidEnv{th: thWithSibling, tvb: tvbOlderAndSibling, schemas: schemasMock}, "-project", "android", "-test-substring", "DirectActionsTest#testPerformDirectAction")
			assert.Loosely(t, code2Sub, should.Equal(0))
			assert.Loosely(t, stdout2Sub, should.ContainSubstring(`Hint: Test exists on branch "git_main", but active segments ended more than 7 days ago; retry with -days 90.`))
			assert.Loosely(t, stdout2Sub, should.ContainSubstring("Other matching test IDs:"))
			assert.Loosely(t, stdout2Sub, should.ContainSubstring(indexedSibling))
			assert.Loosely(t, stdout2Sub, should.NotContainSubstring("ants_cli"))

			// 3. Branch rows exist, but -variant filter excluded them -> reports -variant mismatch.
			code3, stdout3, _ := runCmd(androidEnv{th: thMock, tvb: tvbOlderAndSibling, schemas: schemasMock}, "-project", "android", "-testid", androidTestID, "-variant", "module_abi:riscv64")
			assert.Loosely(t, code3, should.Equal(0))
			assert.Loosely(t, stdout3, should.ContainSubstring(`Hint: Test exists on branch "git_main", but no variants matched the -variant filter.`))
			assert.Loosely(t, stdout3, should.NotContainSubstring("ants_cli"))

			// 4. -test-substring with 0 QueryTests matches and 0 leaf suggestions omits useless "-branch <branch>" retry hint,
			// whereas when leaf suggestions existed in QueryTests and were filtered out on git_main, "-branch <branch>" IS shown.
			code4Empty, stdout4Empty, _ := runCmd(androidEnv{th: &mockTestHistoryClient{}, tvb: &mockTestVariantBranchesClient{}, schemas: schemasMock}, "-project", "android", "-test-substring", "CompletelyUnknownTest#missing")
			assert.Loosely(t, code4Empty, should.Equal(0))
			assert.Loosely(t, stdout4Empty, should.NotContainSubstring("retry with -branch <branch>"))
			assert.Loosely(t, stdout4Empty, should.ContainSubstring("ants_cli test-history"))

			thLeafOnly := &mockTestHistoryClient{
				queryTests: func(ctx context.Context, in *analysispb.QueryTestsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestsResponse, error) {
					if in.GetTestIdSubstring() == "testPerformDirectAction" {
						return &analysispb.QueryTestsResponse{TestIds: []string{unindexedCandidate}}, nil
					}
					return &analysispb.QueryTestsResponse{}, nil
				},
			}
			code4Filtered, stdout4Filtered, _ := runCmd(androidEnv{th: thLeafOnly, tvb: &mockTestVariantBranchesClient{}, schemas: schemasMock}, "-project", "android", "-test-substring", "WrongClass#testPerformDirectAction")
			assert.Loosely(t, code4Filtered, should.Equal(0))
			assert.Loosely(t, stdout4Filtered, should.ContainSubstring("retry with -branch <branch>"))

			// 5. Non-Android project with misplaced "-branch git_main" emits Gitiles hint (not ants_cli),
			// and Android realm with empty branch omits 'on branch ""'.
			code5Gitiles, stdout5Gitiles, _ := runCmd(androidEnv{th: &mockTestHistoryClient{}, tvb: &mockTestVariantBranchesClient{}, schemas: schemasMock}, "-project", "chromium", "-branch", "git_main", "-testid", androidTestID)
			assert.Loosely(t, code5Gitiles, should.Equal(0))
			assert.Loosely(t, stdout5Gitiles, should.ContainSubstring("Hint: -branch queries Android Build source refs (e.g. -project android -branch git_main); omit -branch for Gitiles projects."))
			assert.Loosely(t, stdout5Gitiles, should.NotContainSubstring("ants_cli"))

			code5Arsp, stdout5Arsp, _ := runCmd(androidEnv{th: &mockTestHistoryClient{}, tvb: &mockTestVariantBranchesClient{}, schemas: schemasMock}, "-project", "arsp", "-testid", androidTestID)
			assert.Loosely(t, code5Arsp, should.Equal(0))
			assert.Loosely(t, stdout5Arsp, should.ContainSubstring("Hint: LUCI Analysis only indexes Android runs anchored to a submitted build (sources.submittedAndroidBuild).\n"))
			assert.Loosely(t, stdout5Arsp, should.NotContainSubstring(`on branch ""`))
		})

		t.Run("validates_branch_flag_and_subrealm_conflict", func(t *ftt.Test) {
			code1, _, err1 := runCmd(androidEnv{}, "-project", "android", "-branch", "   ", "-testid", androidTestID)
			assert.Loosely(t, code1, should.Equal(1))
			assert.Loosely(t, err1, should.ContainSubstring("flag -branch cannot be empty"))

			code2, _, err2 := runCmd(androidEnv{}, "-project", "chromium", "-branch", "refs/heads/main", "-testid", androidTestID)
			assert.Loosely(t, code2, should.Equal(1))
			assert.Loosely(t, err2, should.ContainSubstring("expected an Android Build branch name (e.g. git_main), not a Gitiles ref"))

			code3, _, err3 := runCmd(androidEnv{}, "-project", "android", "-branch", "git_main", "-subrealm", "presubmit", "-testid", androidTestID)
			assert.Loosely(t, code3, should.Equal(1))
			assert.Loosely(t, err3, should.ContainSubstring("-subrealm cannot be combined with -branch"))

			code4, _, err4 := runCmd(androidEnv{}, "-project", "android", "-branch", "git_main", "-variant", "module_param:(unset)", "-testid", androidTestID)
			assert.Loosely(t, code4, should.Equal(1))
			assert.Loosely(t, err4, should.ContainSubstring("cannot match unset variant keys via -variant"))
			assert.Loosely(t, err4, should.NotContainSubstring("-subrealm"))
		})

		t.Run("source_verdict_mode_caps_unqueried_failing_variants_and_suppresses_subrealm_hints", func(t *ftt.Test) {
			// 5 failing postsubmit variants (> maxSourceVerdictVariants=4) plus a presubmit CL verdict on hash 1.
			var tvbs []*analysispb.TestVariantBranch
			for i := 0; i < 5; i++ {
				h := fmt.Sprintf("%016x", i+1)
				def := map[string]string{"module_abi": fmt.Sprintf("abi-%d", i)}
				tvbs = append(tvbs, &analysispb.TestVariantBranch{
					VariantHash: h,
					Variant:     &analysispb.Variant{Def: def},
					Segments: []*analysispb.Segment{
						{
							StartHour: timestamppb.New(fixedNow.Add(-24 * time.Hour)),
							EndHour:   timestamppb.New(fixedNow.Add(-1 * time.Hour)),
							Counts:    &analysispb.Segment_Counts{UnexpectedVerdicts: 2, TotalVerdicts: 10},
						},
					},
				})
			}
			tvb5 := &mockTestVariantBranchesClient{
				query: func(ctx context.Context, in *analysispb.QueryTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.QueryTestVariantBranchResponse, error) {
					return &analysispb.QueryTestVariantBranchResponse{TestVariantBranch: tvbs}, nil
				},
			}
			th5 := &mockTestHistoryClient{
				querySourceVerdicts: func(ctx context.Context, in *analysispb.QuerySourceVerdictsV2Request, opts ...grpc.CallOption) (*analysispb.QuerySourceVerdictsV2Response, error) {
					if in.GetTestIdFlat().GetVariantHash() == fmt.Sprintf("%016x", 1) {
						return &analysispb.QuerySourceVerdictsV2Response{
							SourceVerdicts: []*analysispb.SourceVerdict{
								{
									Position: 100,
									InvocationVerdicts: []*analysispb.SourceVerdict_InvocationTestVerdict{
										{
											RootInvocation: "rootInvocations/ants-i501",
											PartitionTime:  timestamppb.New(fixedNow.Add(-1 * time.Hour)),
											Status:         analysispb.TestVerdict_FAILED,
											Changelists:    []*analysispb.Changelist{{Host: "android-review.googlesource.com", Change: 1}},
										},
									},
								},
							},
						}, nil
					}
					return &analysispb.QuerySourceVerdictsV2Response{}, nil
				},
			}

			code, stdout, stderr := runCmd(androidEnv{th: th5, tvb: tvb5}, "-project", "android", "-branch", "git_main", "-testid", androidTestID)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, stderr, should.BeEmpty)
			// Postsubmit Variants row on hash 1 has no presubmit tag in SourceVerdictMode, while the CL verdict itself shows (presubmit CL).
			assert.Loosely(t, stdout, should.ContainSubstring("module_abi=abi-0 (hash: 0000000000000001)"))
			assert.Loosely(t, stdout, should.ContainSubstring("- FAILED (presubmit CL) ants-i501"))
			assert.Loosely(t, stdout, should.NotContainSubstring("-subrealm"))
			// 5th failing variant was excluded by maxSourceVerdictVariants=4, so header notes "more omitted; page cap reached".
			assert.Loosely(t, stdout, should.ContainSubstring("Recent Non-Passing Verdicts (showing 1, more omitted; page cap reached; pass -variant <k:v> or smaller -days to narrow):"))
		})

		t.Run("permission_denied_enriches_android_and_unmigrated_realm_errors", func(t *ftt.Test) {
			thDenied := &mockTestHistoryClient{
				queryStats: func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error) {
					return nil, status.Error(codes.PermissionDenied, `caller does not have permission in realm "`+in.GetProject()+`:@project"`)
				},
			}

			code1, _, errAndroid := execTestHistoryCmd(thDenied, fixedNow, "-project", "android", "-testid", androidTestID)
			assert.Loosely(t, code1, should.Equal(1))
			assert.Loosely(t, errAndroid, should.ContainSubstring("mdb/android-build-access"))

			code2, _, errArsp := execTestHistoryCmd(thDenied, fixedNow, "-project", "arsp", "-testid", androidTestID)
			assert.Loosely(t, code2, should.Equal(1))
			assert.Loosely(t, errArsp, should.ContainSubstring(`project "arsp" is not accessible in LUCI Analysis`))
			assert.Loosely(t, errArsp, should.ContainSubstring("ants_cli test-history --test_identifier_id=<ID>"))
		})
	})
}

func reportCountsWithFailed(n int32) *rdbpb.TestAggregation_VerdictCounts {
	return &rdbpb.TestAggregation_VerdictCounts{Failed: n}
}
