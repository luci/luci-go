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
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockAggResultDBClient struct {
	pb.ResultDBClient
	getRootInvocation     func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	queryTestAggregations func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error)
	queryTestVerdicts     func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error)
	queryWorkUnits        func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error)
	listArtifacts         func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error)
}

func (m *mockAggResultDBClient) QueryTestAggregations(ctx context.Context, in *pb.QueryTestAggregationsRequest, opts ...grpc.CallOption) (*pb.QueryTestAggregationsResponse, error) {
	if m.queryTestAggregations != nil {
		return m.queryTestAggregations(ctx, in)
	}
	return &pb.QueryTestAggregationsResponse{}, nil
}

func (m *mockAggResultDBClient) QueryTestVerdicts(ctx context.Context, in *pb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*pb.QueryTestVerdictsResponse, error) {
	if m.queryTestVerdicts != nil {
		return m.queryTestVerdicts(ctx, in)
	}
	return &pb.QueryTestVerdictsResponse{}, nil
}

func (m *mockAggResultDBClient) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
	return &pb.WorkUnit{Name: in.Name}, nil
}

func (m *mockAggResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in)
	}
	return &pb.RootInvocation{Name: in.Name}, nil
}

func (m *mockAggResultDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	return &pb.Invocation{Name: in.Name}, nil
}

func (m *mockAggResultDBClient) QueryWorkUnits(ctx context.Context, in *pb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*pb.QueryWorkUnitsResponse, error) {
	if m.queryWorkUnits != nil {
		return m.queryWorkUnits(ctx, in)
	}
	return &pb.QueryWorkUnitsResponse{}, nil
}

func (m *mockAggResultDBClient) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
	if m.listArtifacts != nil {
		return m.listArtifacts(ctx, in)
	}
	return &pb.ListArtifactsResponse{}, nil
}

func (m *mockAggResultDBClient) QueryArtifacts(ctx context.Context, in *pb.QueryArtifactsRequest, opts ...grpc.CallOption) (*pb.QueryArtifactsResponse, error) {
	return &pb.QueryArtifactsResponse{}, nil
}

type mockSchemasClient struct {
	pb.SchemasClient
}

func (m *mockSchemasClient) GetScheme(ctx context.Context, in *pb.GetSchemeRequest, opts ...grpc.CallOption) (*pb.Scheme, error) {
	return &pb.Scheme{
		Name:              in.Name,
		HumanReadableName: "JUnit",
		Coarse:            &pb.Scheme_Level{HumanReadableName: "Package"},
		Fine:              &pb.Scheme_Level{HumanReadableName: "Class"},
		Case:              &pb.Scheme_Level{HumanReadableName: "Method"},
	}, nil
}

func TestPrintVerdictSummaryWithAggregations(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchHierarchyAggregations and printVerdictSummary`, t, func(t *ftt.Test) {
		ctx := context.Background()
		client := &mockAggResultDBClient{
			getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
				return &pb.RootInvocation{
					Name:  in.Name,
					State: pb.RootInvocation_FAILED,
					Realm: "google:android-test-results",
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
			queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
				assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i85300010634749400"))
				switch in.Predicate.AggregationLevel {
				case pb.AggregationLevel_INVOCATION:
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Failed: 312, Passed: 4498}},
						},
					}, nil
				case pb.AggregationLevel_MODULE:
					assert.Loosely(t, in.Predicate.TestPrefixFilter.Id.ModuleName, should.Equal("VtsAidlKeyMintTargetTest"))
					assert.Loosely(t, in.Predicate.TestPrefixFilter.Id.ModuleVariantHash, should.Equal("vhash1"))
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Failed: 245, Passed: 218, Skipped: 7}},
						},
					}, nil
				case pb.AggregationLevel_COARSE:
					assert.Loosely(t, in.Predicate.TestPrefixFilter.Id.CoarseName, should.Equal("no-package-name"))
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Failed: 245, Passed: 218, Skipped: 7}},
						},
					}, nil
				case pb.AggregationLevel_FINE:
					assert.Loosely(t, in.Predicate.TestPrefixFilter.Id.FineName, should.Equal("PerInstance/NewKeyGenerationTest"))
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Failed: 42, Passed: 36}},
						},
					}, nil
				default:
					return &pb.QueryTestAggregationsResponse{}, nil
				}
			},
		}

		vg := &VerdictGroup{
			Key: VerdictKey{TestID: ":VtsAidlKeyMintTargetTest!junit:no-package-name:PerInstance/NewKeyGenerationTest#TripleDesInvalidSize/0_strongbox", VariantHash: "vhash1"},
			Results: []*pb.TestResult{
				{
					Name:        "rootInvocations/ants-i85300010634749400/workUnits/wu1/tests/t1/results/r1",
					TestId:      ":VtsAidlKeyMintTargetTest!junit:no-package-name:PerInstance/NewKeyGenerationTest#TripleDesInvalidSize/0_strongbox",
					ResultId:    "r1",
					VariantHash: "vhash1",
					StatusV2:    pb.TestResult_FAILED,
					TestIdStructured: &pb.TestIdentifier{
						ModuleName:        "VtsAidlKeyMintTargetTest",
						ModuleScheme:      "junit",
						ModuleVariantHash: "vhash1",
						CoarseName:        "no-package-name",
						FineName:          "PerInstance/NewKeyGenerationTest",
						CaseName:          "TripleDesInvalidSize/0_strongbox",
					},
				},
			},
		}

		var buf bytes.Buffer
		printVerdictSummary(&buf, ctx, &mockSchemasClient{}, client, nil, "ants-i85300010634749400", vg, false, false)
		out := buf.String()

		assert.Loosely(t, out, should.ContainSubstring("Invocation:   ants-i85300010634749400 [312 failed, 4498 passed]"))
		assert.Loosely(t, out, should.ContainSubstring("Module (JUnit): VtsAidlKeyMintTargetTest [245 failed, 218 passed, 7 skipped]"))
		assert.Loosely(t, out, should.ContainSubstring("Package: no-package-name [245 failed, 218 passed, 7 skipped]"))
		assert.Loosely(t, out, should.ContainSubstring("Class: PerInstance/NewKeyGenerationTest [42 failed, 36 passed]"))
		assert.Loosely(t, out, should.ContainSubstring("Method: TripleDesInvalidSize/0_strongbox"))
		assert.Loosely(t, out, should.NotContainSubstring("Primary Build:"))
		assert.Loosely(t, out, should.NotContainSubstring("Realm:"))

		var metaBuf bytes.Buffer
		printVerdictSummary(&metaBuf, ctx, &mockSchemasClient{}, client, nil, "ants-i85300010634749400", vg, true, false)
		metaOut := metaBuf.String()
		assert.Loosely(t, metaOut, should.ContainSubstring("Primary Build: branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=16133789\n"))
		assert.Loosely(t, metaOut, should.ContainSubstring("Realm:        google:android-test-results\n"))

		t.Run(`returns nil for legacy invocations`, func(t *ftt.Test) {
			aggs := FetchHierarchyAggregations(ctx, client, "build-123", vg.Results[0], true)
			assert.Loosely(t, aggs, should.BeNil)
		})
	})
}

func TestParseVerdictName(t *testing.T) {
	t.Parallel()

	ftt.Run(`ParseVerdictName`, t, func(t *ftt.Test) {
		t.Run(`structured UI URL with domain and query params`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8676886509240051393/modules/%2F%2Fchrome%3Achrome_private_code_test/schemes/single/variants/b7de9035241e76cc/cases/*fixture?artifact=summary_node"
			invName, variantHash, testIDRegexp, matchFunc, err := ParseVerdictName(url)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invName, should.Equal("invocations/build-8676886509240051393"))
			assert.Loosely(t, variantHash, should.Equal("b7de9035241e76cc"))
			assert.Loosely(t, testIDRegexp, should.Equal(`.*\*fixture`))

			tr := &pb.TestResult{
				VariantHash: "b7de9035241e76cc",
				TestIdStructured: &pb.TestIdentifier{
					ModuleName: "//chrome:chrome_private_code_test",
					CaseName:   "*fixture",
				},
			}
			assert.Loosely(t, matchFunc(tr), should.BeTrue)

			trWrongVariant := &pb.TestResult{
				VariantHash: "otherhash",
				TestIdStructured: &pb.TestIdentifier{
					ModuleName: "//chrome:chrome_private_code_test",
					CaseName:   "*fixture",
				},
			}
			assert.Loosely(t, matchFunc(trWrongVariant), should.BeFalse)
		})

		t.Run(`flat format`, func(t *ftt.Test) {
			name := "invocations/build-123/tests/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case/variants/hash123"
			invName, variantHash, testIDRegexp, matchFunc, err := ParseVerdictName(name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invName, should.Equal("invocations/build-123"))
			assert.Loosely(t, variantHash, should.Equal("hash123"))
			assert.Loosely(t, testIDRegexp, should.Equal(`ninja://chrome/test:browser_tests/MyTest\.Case`))

			tr := &pb.TestResult{
				VariantHash: "hash123",
				TestId:      "ninja://chrome/test:browser_tests/MyTest.Case",
			}
			assert.Loosely(t, matchFunc(tr), should.BeTrue)
		})

		t.Run(`missing variants`, func(t *ftt.Test) {
			_, _, _, _, err := ParseVerdictName("invocations/build-123/tests/mytest")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestCmdVerdict(t *testing.T) {
	t.Parallel()

	ftt.Run(`Cmd`, t, func(t *ftt.Test) {
		cmd := Cmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("verdict <subcommand>"))

		getCmd := GetCmd(nil)
		assert.Loosely(t, getCmd, should.NotBeNil)
		assert.Loosely(t, getCmd.UsageLine, should.Equal("get [-invocationid <invocation_id> -testid <test_id>] [-varianthash <hash>] [<target>]"))

		t.Run(`flag aliases -invocation-id, -test-id, and -variant-hash`, func(t *ftt.Test) {
			r := GetCmd(nil).CommandRun().(*verdictGetRun)
			err := r.Flags.Parse([]string{
				"-invocation-id", "inv-1",
				"-test-id", "test-1",
				"-variant-hash", "vhash-1",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r.invocationID, should.Equal("inv-1"))
			assert.Loosely(t, r.testID, should.Equal("test-1"))
			assert.Loosely(t, r.variantHash, should.Equal("vhash-1"))
		})

		t.Run(`verdictGetRun accepts positional verdict URL and prints derived IDs`, func(t *ftt.Test) {
			client := &mockAggResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: ":ModA!junit:pkg:Cls#testOne",
								TestIdStructured: &pb.TestIdentifier{
									ModuleName:        "ModA",
									ModuleScheme:      "junit",
									ModuleVariantHash: "vhash1",
									CoarseName:        "pkg",
									FineName:          "Cls",
									CaseName:          "testOne",
								},
								Results: []*pb.TestResult{
									{
										Name:        "rootInvocations/ants-i123/workUnits/wu-1/tests/:ModA!junit:pkg:Cls%23testOne/results/res-1",
										TestId:      ":ModA!junit:pkg:Cls#testOne",
										ResultId:    "res-1",
										VariantHash: "vhash1",
										StatusV2:    pb.TestResult_FAILED,
									},
								},
							},
						},
					}, nil
				},
			}
			var buf bytes.Buffer
			run := getCmd.CommandRun().(*verdictGetRun)
			run.client = client
			run.out = &buf
			rc := run.Run(nil, []string{"rootInvocations/ants-i123/workUnits/wu-1/tests/:ModA!junit:pkg:Cls%23testOne/results/res-1"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     ants-i123"))
			assert.Loosely(t, out, should.ContainSubstring("Module Name:       ModA"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit ID:      wu-1"))
			assert.Loosely(t, out, should.ContainSubstring("Test ID:           :ModA!junit:pkg:Cls#testOne"))
			assert.Loosely(t, out, should.ContainSubstring("Result ID:         res-1"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation:   ants-i123"))
		})

		t.Run(`verdictGetRun suggests verdict list when no results are found for a test ID`, func(t *ftt.Test) {
			client := &mockAggResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{}, nil
				},
			}
			var outBuf, errBuf bytes.Buffer
			run := getCmd.CommandRun().(*verdictGetRun)
			run.client = client
			run.out = &outBuf
			run.errOut = &errBuf
			run.invocationID = "ants-i123"
			run.testID = ":ModA!junit:pkg:Cls#testMissing"
			rc := run.Run(nil, nil, nil)
			assert.Loosely(t, rc, should.Equal(1))
			assert.Loosely(t, outBuf.String(), should.BeEmpty)
			assert.Loosely(t, errBuf.String(), should.Equal(
				"no results found for test \":ModA!junit:pkg:Cls#testMissing\" in invocation \"ants-i123\" (try 'luci verdict list -invocationid ants-i123' to list failing verdicts in this invocation)\n",
			))
		})

		t.Run(`verdictGetRun suggests verdict list and module get for module-style test ID with no results`, func(t *ftt.Test) {
			client := &mockAggResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{}, nil
				},
			}
			var outBuf, errBuf bytes.Buffer
			run := getCmd.CommandRun().(*verdictGetRun)
			run.client = client
			run.out = &outBuf
			run.errOut = &errBuf
			run.invocationID = "ants-i123"
			run.testID = ":MyModule#."
			rc := run.Run(nil, nil, nil)
			assert.Loosely(t, rc, should.Equal(1))
			assert.Loosely(t, outBuf.String(), should.BeEmpty)
			assert.Loosely(t, errBuf.String(), should.Equal(
				"no results found for test \":MyModule#.\" in invocation \"ants-i123\" (try 'luci verdict list -invocationid ants-i123' to list failing verdicts in this invocation)\n"+
					"Hint: if this was a module error, try: luci module get -invocationid ants-i123 -modulename MyModule\n",
			))
		})

		t.Run(`verdictGetRun suggests verdict list for flat test ID without # when no results are found`, func(t *ftt.Test) {
			client := &mockAggResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{}, nil
				},
			}
			var outBuf, errBuf bytes.Buffer
			run := getCmd.CommandRun().(*verdictGetRun)
			run.client = client
			run.out = &outBuf
			run.errOut = &errBuf
			run.invocationID = "build-123"
			run.testID = "ninja://chrome/test:browser_tests/Suite.Test"
			rc := run.Run(nil, nil, nil)
			assert.Loosely(t, rc, should.Equal(1))
			assert.Loosely(t, outBuf.String(), should.BeEmpty)
			assert.Loosely(t, errBuf.String(), should.Equal(
				"no results found for test \"ninja://chrome/test:browser_tests/Suite.Test\" in invocation \"build-123\" (try 'luci verdict list -invocationid build-123' to list failing verdicts in this invocation)\n"+
					"Hint: if this was a module error, try: luci module get -invocationid build-123 -modulename ninja://chrome/test:browser_tests/Suite.Test\n",
			))
		})
	})
}

func TestQueryRootInvocationVerdicts(t *testing.T) {
	t.Parallel()

	ftt.Run(`queryRootInvocationVerdicts`, t, func(t *ftt.Test) {
		ctx := context.Background()
		testID := `://\:CtsMediaAudioTestCases!cts:android.media.audio.cts:AudioFocusTest#testAudioFocusRequest_gainTransientMayDuck`

		t.Run(`uses TestPrefixFilter and omits OrderBy when variantHash is provided`, func(t *ftt.Test) {
			calls := 0
			client := &mockAggResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					calls++
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i26000010636495227"))
					assert.Loosely(t, in.OrderBy, should.BeEmpty)
					assert.Loosely(t, in.Predicate.GetContainsTestResultFilter(), should.BeEmpty)
					prefix := in.Predicate.GetTestPrefixFilter()
					assert.Loosely(t, prefix, should.NotBeNil)
					assert.Loosely(t, prefix.Level, should.Equal(pb.AggregationLevel_CASE))
					assert.Loosely(t, prefix.Id.ModuleName, should.Equal("//:CtsMediaAudioTestCases"))
					assert.Loosely(t, prefix.Id.ModuleScheme, should.Equal("cts"))
					assert.Loosely(t, prefix.Id.ModuleVariantHash, should.Equal("b7de9035241e76cc"))
					assert.Loosely(t, prefix.Id.CoarseName, should.Equal("android.media.audio.cts"))
					assert.Loosely(t, prefix.Id.FineName, should.Equal("AudioFocusTest"))
					assert.Loosely(t, prefix.Id.CaseName, should.Equal("testAudioFocusRequest_gainTransientMayDuck"))

					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: testID,
								TestIdStructured: &pb.TestIdentifier{
									ModuleName:        "//:CtsMediaAudioTestCases",
									ModuleScheme:      "cts",
									ModuleVariantHash: "b7de9035241e76cc",
									CoarseName:        "android.media.audio.cts",
									FineName:          "AudioFocusTest",
									CaseName:          "testAudioFocusRequest_gainTransientMayDuck",
								},
								Results: []*pb.TestResult{
									{
										Name:     "rootInvocations/ants-i26000010636495227/workUnits/wu-1/tests/t1/results/3bc936dc-02514",
										ResultId: "3bc936dc-02514",
										StatusV2: pb.TestResult_FAILED,
									},
								},
							},
						},
					}, nil
				},
			}

			results, _, hasMore, err := QueryVerdictResultsAndExonerations(ctx, client, "ants-i26000010636495227", testID, "b7de9035241e76cc", false, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calls, should.Equal(1))
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, len(results), should.Equal(1))
			assert.Loosely(t, results[0].ResultId, should.Equal("3bc936dc-02514"))
		})

		t.Run(`stops after priority RPC when verdict is found without following synthetic MaxUIPriority token`, func(t *ftt.Test) {
			calls := 0
			client := &mockAggResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					calls++
					if calls > 1 {
						t.Fatalf("expected only 1 RPC call, got second call with pageToken=%q", in.PageToken)
					}
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: testID,
								Results: []*pb.TestResult{
									{
										Name:     "rootInvocations/ants-i26000010636495227/workUnits/wu-1/tests/t1/results/3bc936dc-02514",
										ResultId: "3bc936dc-02514",
										StatusV2: pb.TestResult_FAILED,
									},
								},
							},
						},
						NextPageToken: "synthetic-max-ui-priority-token",
					}, nil
				},
			}

			results, _, _, err := QueryVerdictResultsAndExonerations(ctx, client, "ants-i26000010636495227", testID, "", false, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calls, should.Equal(1))
			assert.Loosely(t, len(results), should.Equal(1))
		})

		t.Run(`includes AndroidAccessHint on PermissionDenied only for Android invocations`, func(t *ftt.Test) {
			client := &mockAggResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return nil, status.Error(codes.PermissionDenied, "caller does not have permission")
				},
			}

			_, _, _, errBuild := QueryVerdictResultsAndExonerations(ctx, client, "build-123", testID, "", false, 10)
			assert.Loosely(t, errBuild, should.ErrLike("missing ResultDB realm permission"))
			assert.Loosely(t, errBuild.Error(), should.NotContainSubstring("mdb/android-build-access"))

			_, _, _, errAndroid := QueryVerdictResultsAndExonerations(ctx, client, "ants-i26000010636495227", testID, "", false, 10)
			assert.Loosely(t, errAndroid, should.ErrLike("missing ResultDB realm permission"))
			assert.Loosely(t, errAndroid, should.ErrLike(base.AndroidAccessHint))
		})
	})
}

func TestPrintVerdictSummaryInlineArtifacts(t *testing.T) {
	t.Parallel()

	ftt.Run(`printVerdictSummary surfaces inline failure artifacts within budget`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`surfaces direct and ancestor failure artifacts within 1.5 KB cap on FAILED verdict`, func(t *ftt.Test) {
			trName := "rootInvocations/ants-i123/workUnits/wu-leaf/tests/:ModA!junit:pkg:Cls%23testOne/results/res-1"
			leafWU := "rootInvocations/ants-i123/workUnits/wu-leaf"
			rootWU := "rootInvocations/ants-i123/workUnits/root"

			noisyRoot := make([]*pb.Artifact, 0, 200)
			for i := range 200 {
				noisyRoot = append(noisyRoot, &pb.Artifact{
					Name:       fmt.Sprintf("%s/artifacts/injectables_%03d.json", rootWU, i),
					ArtifactId: fmt.Sprintf("injectables_%03d.json", i),
					SizeBytes:  2048,
				})
			}

			client := &mockAggResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{{Name: rootWU}},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					switch in.Parent {
					case trName:
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         trName + "/artifacts/logcat-on-failure.txt",
									ArtifactId:   "logcat-on-failure.txt",
									ArtifactType: "LOGCAT",
									ContentType:  "text/plain",
									SizeBytes:    131072,
								},
							},
						}, nil
					case leafWU:
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         leafWU + "/artifacts/host_log.txt",
									ArtifactId:   "host_log.txt",
									ArtifactType: "HOST_LOG",
									ContentType:  "text/plain",
									SizeBytes:    32768,
								},
							},
						}, nil
					case rootWU:
						return &pb.ListArtifactsResponse{Artifacts: noisyRoot}, nil
					default:
						return &pb.ListArtifactsResponse{}, nil
					}
				},
			}

			vg := &VerdictGroup{
				Key: VerdictKey{TestID: ":ModA!junit:pkg:Cls#testOne", VariantHash: "vhash1"},
				Results: []*pb.TestResult{
					{
						Name:        trName,
						TestId:      ":ModA!junit:pkg:Cls#testOne",
						ResultId:    "res-1",
						VariantHash: "vhash1",
						StatusV2:    pb.TestResult_FAILED,
					},
				},
			}

			var buf bytes.Buffer
			printVerdictSummary(&buf, ctx, nil, client, nil, "ants-i123", vg, false, false)
			out := buf.String()

			idx := strings.Index(out, "Test Result Artifacts")
			assert.Loosely(t, idx > 0, should.BeTrue)
			inlineSection := out[idx:]
			assert.Loosely(t, inlineSection, should.ContainSubstring("Test Result Artifacts (result_id: res-1, 1 artifact):"))
			assert.Loosely(t, inlineSection, should.ContainSubstring("logcat-on-failure.txt"))
			assert.Loosely(t, inlineSection, should.ContainSubstring("To fetch a test result artifact:"))
			assert.Loosely(t, inlineSection, should.ContainSubstring("e.g. luci test-result artifact get -invocationid=ants-i123 -workunitid=wu-leaf -testid=':ModA!junit:pkg:Cls#testOne' -resultid=res-1 -artifactid=logcat-on-failure.txt"))
			assert.Loosely(t, inlineSection, should.ContainSubstring("host_log.txt"))
			assert.Loosely(t, inlineSection, should.ContainSubstring("e.g. luci artifact get -invocationid=ants-i123 -workunitid=wu-leaf -artifactid=host_log.txt"))
			assert.Loosely(t, len(inlineSection) <= format.DefaultInlineGetArtifactBudget, should.BeTrue)
			assert.Loosely(t, len(out) <= format.DefaultMaxOutputBytes, should.BeTrue)
		})

		t.Run(`FLAKY verdict only queries artifacts for non-passing attempts`, func(t *ftt.Test) {
			passTR := "rootInvocations/ants-i123/workUnits/wu-pass/tests/t1/results/r-pass"
			failTR := "rootInvocations/ants-i123/workUnits/wu-fail/tests/t1/results/r-fail"

			var mu sync.Mutex
			queriedParents := make(map[string]bool)
			client := &mockAggResultDBClient{
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					mu.Lock()
					queriedParents[in.Parent] = true
					mu.Unlock()
					if in.Parent == failTR {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         failTR + "/artifacts/crash_stderr.txt",
									ArtifactId:   "crash_stderr.txt",
									ContentType:  "text/plain",
									SizeBytes:    2048,
									ArtifactType: "STDERR",
								},
							},
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			vg := &VerdictGroup{
				Key: VerdictKey{TestID: "t1", VariantHash: "vhash1"},
				Results: []*pb.TestResult{
					{Name: passTR, TestId: "t1", ResultId: "r-pass", StatusV2: pb.TestResult_PASSED},
					{Name: failTR, TestId: "t1", ResultId: "r-fail", StatusV2: pb.TestResult_FAILED},
				},
			}

			var buf bytes.Buffer
			printVerdictSummary(&buf, ctx, nil, client, nil, "ants-i123", vg, false, false)
			out := buf.String()

			mu.Lock()
			sawPassTR := queriedParents[passTR]
			sawFailTR := queriedParents[failTR]
			mu.Unlock()
			assert.Loosely(t, sawPassTR, should.BeFalse)
			assert.Loosely(t, sawFailTR, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("Test Result Artifacts (result_id: r-fail, 1 artifact):"))
			assert.Loosely(t, out, should.ContainSubstring("crash_stderr.txt"))
		})

		t.Run(`PASSED and SKIPPED verdicts make zero test-result artifact RPCs`, func(t *ftt.Test) {
			trName := "rootInvocations/ants-i123/workUnits/wu-1/tests/t1/results/r1"
			var trArtCalls atomic.Int32
			client := &mockAggResultDBClient{
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == trName {
						trArtCalls.Add(1)
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			for _, st := range []pb.TestResult_Status{pb.TestResult_PASSED, pb.TestResult_SKIPPED} {
				vg := &VerdictGroup{
					Key: VerdictKey{TestID: "t1", VariantHash: "vhash1"},
					Results: []*pb.TestResult{
						{
							Name:     trName,
							TestId:   "t1",
							ResultId: "r1",
							StatusV2: st,
						},
					},
				}
				var buf bytes.Buffer
				printVerdictSummary(&buf, ctx, nil, client, nil, "ants-i123", vg, false, false)
				assert.Loosely(t, buf.String(), should.NotContainSubstring("Test Result Artifacts"))
				assert.Loosely(t, buf.String(), should.NotContainSubstring("Work Unit Artifacts:"))
			}

			assert.Loosely(t, trArtCalls.Load(), should.Equal(int32(0)))
		})

		t.Run(`caps oversized default verdict get output at 9.5 KB with inline artifacts while explicit -max-results bypasses cap`, func(t *ftt.Test) {
			testID := ":ModA!junit:pkg:Cls#testManyRetries"
			results := make([]*pb.TestResult, 0, 50)
			for i := range 50 {
				resID := fmt.Sprintf("r-%02d", i)
				trName := fmt.Sprintf("rootInvocations/ants-i123/workUnits/wu-long-shard-identifier-%02d/tests/t1/results/%s", i, resID)
				results = append(results, &pb.TestResult{
					Name:        trName,
					TestId:      testID,
					ResultId:    resID,
					VariantHash: "vhash1",
					StatusV2:    pb.TestResult_FAILED,
					FailureReason: &pb.FailureReason{
						PrimaryErrorMessage: fmt.Sprintf("AssertionError on attempt %02d: expected success during retry execution", i),
					},
				})
			}
			exonerations := []*pb.TestExoneration{
				{
					TestId:          testID,
					VariantHash:     "vhash1",
					Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
					ExplanationHtml: "<pre>" + strings.Repeat("Flake analysis detail line 1234567890\n", 230) + "</pre>",
				},
			}

			firstTR := results[0].Name
			client := &mockAggResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					limit := len(results)
					if in.PageSize > 0 && int(in.PageSize) < limit {
						limit = int(in.PageSize)
					}
					nextTok := ""
					if limit < len(results) {
						nextTok = "more"
					}
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId:       testID,
								Results:      results[:limit],
								Exonerations: exonerations,
							},
						},
						NextPageToken: nextTok,
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == firstTR {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         firstTR + "/artifacts/crash_stderr.txt",
									ArtifactId:   "crash_stderr.txt",
									ArtifactType: "STDERR",
									ContentType:  "text/plain",
									SizeBytes:    2048,
								},
							},
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			runGet := func(args ...string) string {
				var buf bytes.Buffer
				cmd := GetCmd(nil)
				run := cmd.CommandRun().(*verdictGetRun)
				run.client = client
				run.out = &buf
				assert.Loosely(t, run.Run(nil, args, nil), should.Equal(0))
				return buf.String()
			}

			// 1. Default verdict get caps oversized primary output at <= 9,500 B and retains inline artifacts.
			defaultOut := runGet("-invocationid", "ants-i123", "-testid", testID)
			assert.Loosely(t, len(defaultOut) <= format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, defaultOut, should.ContainSubstring("bytes truncated; pass -show-metadata to view full output"))
			assert.Loosely(t, defaultOut, should.ContainSubstring("Test Result Artifacts"))
			assert.Loosely(t, defaultOut, should.ContainSubstring("crash_stderr.txt"))

			// 2. Explicit -max-results 0 bypasses the 9.5 KB cap and displays all 50 results.
			zeroOut := runGet("-invocationid", "ants-i123", "-testid", testID, "-max-results", "0")
			assert.Loosely(t, len(zeroOut) > format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, zeroOut, should.NotContainSubstring("bytes truncated"))
			assert.Loosely(t, zeroOut, should.ContainSubstring("AssertionError on attempt 49"))
			assert.Loosely(t, zeroOut, should.ContainSubstring("crash_stderr.txt"))

			// 3. Explicit -max-results 50 also bypasses the 9.5 KB cap.
			fiftyOut := runGet("-invocationid", "ants-i123", "-testid", testID, "-max-results", "50")
			assert.Loosely(t, len(fiftyOut) > format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, fiftyOut, should.NotContainSubstring("bytes truncated"))
			assert.Loosely(t, fiftyOut, should.ContainSubstring("AssertionError on attempt 49"))
		})
	})
}
