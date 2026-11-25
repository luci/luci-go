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
			_, err := fetchTestHistory(ctx, nil, "", "", "", nil, 7, defaultTestHistoryLimit, true, fixedNow)
			assert.Loosely(t, err, should.ErrLike("missing required -project flag"))

			_, err = fetchTestHistory(ctx, nil, "projects/turquoise", "", "", nil, 7, defaultTestHistoryLimit, true, fixedNow)
			assert.Loosely(t, err, should.ErrLike("expected a bare ID"))

			_, err = fetchTestHistory(ctx, nil, "turquoise", "", "", nil, 7, defaultTestHistoryLimit, true, fixedNow)
			assert.Loosely(t, err, should.ErrLike("missing required -testid flag"))

			_, err = fetchTestHistory(ctx, nil, "turquoise", "my_test", "", nil, 0, defaultTestHistoryLimit, true, fixedNow)
			assert.Loosely(t, err, should.ErrLike("-days must be positive"))

			_, err = fetchTestHistory(ctx, nil, "turquoise", "my_test", "", []string{"missing_separator"}, 7, defaultTestHistoryLimit, true, fixedNow)
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
				false,
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
				false,
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
	})
}

func TestAndroidSourceVerdictHistory(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 11, 19, 4, 0, 0, 0, time.UTC)

	type androidEnv struct {
		app subcommands.Application
		th  analysispb.TestHistoryClient
		tvb analysispb.TestVariantBranchesClient
		rdb rdbpb.ResultDBClient
	}
	runCmd := func(env androidEnv, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		run := TestHistoryCmd(&base.AuthFlags{}).CommandRun().(*testHistoryRun)
		run.thClient = env.th
		run.tvbClient = env.tvb
		run.rdbClient = env.rdb
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

			// 1. Stopping at -limit=1 on page 0 must NOT report "partial stats (page cap reached)".
			code1, out1, _ := runCmd(androidEnv{app: app, rdb: rdbTarget, th: thPaged, tvb: &mockTestVariantBranchesClient{}}, "-limit", "1", "TR99999999999999999")
			assert.Loosely(t, code1, should.Equal(0))
			assert.Loosely(t, pagesCalled, should.Equal(1))
			assert.Loosely(t, out1, should.ContainSubstring("sampled source verdict counts)"))
			assert.Loosely(t, out1, should.NotContainSubstring("partial stats (page cap reached)"))

			// 2. Exhausting maxSourceVerdictPages (3 pages) with -limit=0 MUST report "partial stats (page cap reached)".
			pagesCalled = 0
			code2, out2, _ := runCmd(androidEnv{app: app, rdb: rdbTarget, th: thPaged, tvb: &mockTestVariantBranchesClient{}}, "-limit", "0", "TR99999999999999999")
			assert.Loosely(t, code2, should.Equal(0))
			assert.Loosely(t, pagesCalled, should.Equal(maxSourceVerdictPages))
			assert.Loosely(t, out2, should.ContainSubstring("sampled source verdict counts, partial stats (page cap reached))"))
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
		})
	})
}
