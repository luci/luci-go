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
	"io"
	"os"
	"testing"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockListResultDBClient struct {
	pb.ResultDBClient
	queryTestVerdicts func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error)
	queryTestVariants func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error)
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

func captureStdout(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-outC
}

func TestListCmd(t *testing.T) {
	t.Parallel()

	ftt.Run(`ListCmd`, t, func(t *ftt.Test) {
		cmd := ListCmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("list -invocationid <invocation_id>"))
		assert.Loosely(t, cmd.ShortDesc, should.Equal("List test verdicts in an invocation"))

		run := cmd.CommandRun().(*verdictListRun)
		assert.Loosely(t, run.maxVerdicts, should.Equal(100))
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

		t.Run(`default filter queries ui_priority and BASIC view`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/build-123"))
					assert.Loosely(t, in.OrderBy, should.Equal("ui_priority, test_id_structured"))
					assert.Loosely(t, in.View, should.Equal(pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC))
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

		t.Run(`default filter uses UNEXPECTED predicate`, func(t *ftt.Test) {
			client := &mockListResultDBClient{
				queryTestVariants: func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
					assert.Loosely(t, in.Invocations, should.Match([]string{"invocations/build-123"}))
					assert.Loosely(t, in.PageSize, should.Equal(100))
					assert.Loosely(t, in.Predicate.Status, should.Equal(pb.TestVariantStatus_UNEXPECTED))
					return &pb.QueryTestVariantsResponse{
						TestVariants: []*pb.TestVariant{
							{
								TestId:      "://chrome/test:browser_tests!gtest::Test1",
								StatusV2:    pb.TestVerdict_FAILED,
								VariantHash: "vhash1",
								Variant: &pb.Variant{
									Def: map[string]string{"builder": "linux-rel"},
								},
							},
						},
					}, nil
				},
			}

			filter, _ := parseStatusFilter("", false, false, false, false, false, false)
			verdicts, hasMore, err := QueryLegacyInvocationListedVerdicts(ctx, client, "build-123", filter, 100)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, len(verdicts), should.Equal(1))
			assert.Loosely(t, verdicts[0].TestID, should.Equal("://chrome/test:browser_tests!gtest::Test1"))
			assert.Loosely(t, verdicts[0].Status, should.Equal("FAILED"))
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
			out := captureStdout(func() {
				printListedVerdicts("build-123", nil, false, true)
			})
			assert.Loosely(t, out, should.ContainSubstring("No failed or execution errored verdicts found in invocation \"build-123\""))
			assert.Loosely(t, out, should.ContainSubstring("-all-statuses"))
		})

		t.Run(`with results and variant`, func(t *ftt.Test) {
			verdicts := []*ListedVerdict{
				{
					TestID:      ":no-module-name!junit:pkg.Class#testA",
					Status:      "FAILED",
					VariantHash: "e3b0c442",
				},
				{
					TestID: ":module!junit:pkg.Class#testB",
					Status: "FAILED (EXONERATED)",
					Variant: &pb.Variant{
						Def: map[string]string{"os": "Ubuntu-22.04", "builder": "linux-rel"},
					},
					VariantHash: "vhash1",
					Exonerations: []*pb.TestExoneration{
						{
							ExplanationHtml: "Failed on mainline",
							Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
						},
					},
				},
			}

			out := captureStdout(func() {
				printListedVerdicts("build-123", verdicts, true, false)
			})
			assert.Loosely(t, out, should.ContainSubstring("Verdicts (showing 2, use -max-verdicts or -all to see more):"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED :no-module-name!junit:pkg.Class#testA"))
			assert.Loosely(t, out, should.ContainSubstring("- FAILED (EXONERATED) :module!junit:pkg.Class#testB"))
			assert.Loosely(t, out, should.ContainSubstring("Variant: builder=linux-rel os=Ubuntu-22.04 (hash: vhash1)"))
			assert.Loosely(t, out, should.ContainSubstring("Exoneration: Failed on mainline [OCCURS_ON_MAINLINE]"))
		})
	})
}
