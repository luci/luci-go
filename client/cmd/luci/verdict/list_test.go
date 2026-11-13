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
	"google.golang.org/protobuf/types/known/structpb"

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

		assert.Loosely(t, run.Flags.Parse([]string{"-invocation-id", "inv-1", "-module-name", "ModA"}), should.BeNil)
		assert.Loosely(t, run.invocationID, should.Equal("inv-1"))
		assert.Loosely(t, run.moduleName, should.Equal("ModA"))
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
