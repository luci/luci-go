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
	"context"
	"testing"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockAggResultDBClient struct {
	pb.ResultDBClient
	queryTestAggregations func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error)
}

func (m *mockAggResultDBClient) QueryTestAggregations(ctx context.Context, in *pb.QueryTestAggregationsRequest, opts ...grpc.CallOption) (*pb.QueryTestAggregationsResponse, error) {
	if m.queryTestAggregations != nil {
		return m.queryTestAggregations(ctx, in)
	}
	return &pb.QueryTestAggregationsResponse{}, nil
}

func (m *mockAggResultDBClient) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
	return &pb.WorkUnit{Name: in.Name}, nil
}

func (m *mockAggResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	return &pb.RootInvocation{Name: in.Name}, nil
}

func (m *mockAggResultDBClient) QueryWorkUnits(ctx context.Context, in *pb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*pb.QueryWorkUnitsResponse, error) {
	return &pb.QueryWorkUnitsResponse{}, nil
}

func (m *mockAggResultDBClient) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
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

		out := captureStdout(func() {
			printVerdictSummary(ctx, &mockSchemasClient{}, client, nil, "ants-i85300010634749400", vg, false, false, false)
		})

		assert.Loosely(t, out, should.ContainSubstring("Invocation:   ants-i85300010634749400 [312 failed, 4498 passed]"))
		assert.Loosely(t, out, should.ContainSubstring("Module (JUnit): VtsAidlKeyMintTargetTest [245 failed, 218 passed, 7 skipped]"))
		assert.Loosely(t, out, should.ContainSubstring("Package: no-package-name [245 failed, 218 passed, 7 skipped]"))
		assert.Loosely(t, out, should.ContainSubstring("Class: PerInstance/NewKeyGenerationTest [42 failed, 36 passed]"))
		assert.Loosely(t, out, should.ContainSubstring("Method: TripleDesInvalidSize/0_strongbox"))

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
		assert.Loosely(t, getCmd.UsageLine, should.Equal("get -invocationid <invocation_id> -testid <test_id> [-varianthash <hash>]"))
	})
}
