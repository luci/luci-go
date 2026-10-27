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
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockListResultDBClient struct {
	pb.ResultDBClient
	getRootInvocation func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation     func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
	queryTestVerdicts func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error)
	queryTestVariants func(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error)
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

func TestListCmd(t *testing.T) {
	t.Parallel()

	ftt.Run(`ListCmd`, t, func(t *ftt.Test) {
		cmd := ListCmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("list [-invocationid <invocation_id>] [<target>]"))
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
			var buf bytes.Buffer
			printListedVerdicts(&buf, "build-123", nil, false, true)
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
			printListedVerdicts(&buf, "build-123", verdicts, true, false)
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
	})
}
