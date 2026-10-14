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
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type mockTestHistoryClient struct {
	analysispb.TestHistoryClient
	queryStats    func(ctx context.Context, in *analysispb.QueryTestHistoryStatsRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryStatsResponse, error)
	query         func(ctx context.Context, in *analysispb.QueryTestHistoryRequest, opts ...grpc.CallOption) (*analysispb.QueryTestHistoryResponse, error)
	queryVariants func(ctx context.Context, in *analysispb.QueryVariantsRequest, opts ...grpc.CallOption) (*analysispb.QueryVariantsResponse, error)
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

func TestTestHistorySubcommand(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC)

	ftt.Run("TestHistoryCmd executes cross-build flakiness & verdict triage", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("Command metadata and registration", func(t *ftt.Test) {
			cmd := TestHistoryCmd(&base.AuthFlags{})
			assert.Loosely(t, cmd.UsageLine, should.ContainSubstring("test-history"))
			assert.Loosely(t, cmd.CommandRun(), should.NotBeNil)
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
	})
}
