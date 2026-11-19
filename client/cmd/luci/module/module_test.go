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

package module

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/testutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockResultDBClient struct {
	pb.ResultDBClient
	queryTestAggregations func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error)
	getWorkUnit           func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error)
	batchGetWorkUnits     func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error)
	queryWorkUnits        func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error)
	getRootInvocation     func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation         func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
	listArtifacts         func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error)
}

func (m *mockResultDBClient) QueryTestAggregations(ctx context.Context, in *pb.QueryTestAggregationsRequest, opts ...grpc.CallOption) (*pb.QueryTestAggregationsResponse, error) {
	if m.queryTestAggregations != nil {
		return m.queryTestAggregations(ctx, in)
	}
	return &pb.QueryTestAggregationsResponse{}, nil
}

func (m *mockResultDBClient) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
	if m.getWorkUnit != nil {
		return m.getWorkUnit(ctx, in)
	}
	return nil, nil
}

func (m *mockResultDBClient) BatchGetWorkUnits(ctx context.Context, in *pb.BatchGetWorkUnitsRequest, opts ...grpc.CallOption) (*pb.BatchGetWorkUnitsResponse, error) {
	if m.batchGetWorkUnits != nil {
		return m.batchGetWorkUnits(ctx, in)
	}
	return &pb.BatchGetWorkUnitsResponse{}, nil
}

func (m *mockResultDBClient) QueryWorkUnits(ctx context.Context, in *pb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*pb.QueryWorkUnitsResponse, error) {
	if m.queryWorkUnits != nil {
		return m.queryWorkUnits(ctx, in)
	}
	return &pb.QueryWorkUnitsResponse{}, nil
}

func (m *mockResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in)
	}
	return &pb.RootInvocation{}, nil
}

func (m *mockResultDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in)
	}
	return &pb.Invocation{}, nil
}

func (m *mockResultDBClient) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
	if m.listArtifacts != nil {
		return m.listArtifacts(ctx, in)
	}
	return &pb.ListArtifactsResponse{}, nil
}

func TestModuleDetails(t *testing.T) {
	ftt.Run(`FetchModuleDetails`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Module failure with 0 tests ran and failed work unit`, func(t *ftt.Test) {
			failedWU := &pb.WorkUnit{
				Name:       "rootInvocations/ants-i14600010609614895/workUnits/ants-wu98100269380597657",
				WorkUnitId: "ants-wu98100269380597657",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_FAILED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "CellBroadcastReceiverMTS",
				},
				ModuleShardKey:  "0",
				SummaryMarkdown: "TradeFed harness error: DeviceNotAvailableException",
			}

			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i14600010609614895"))
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{
										ModuleName: "CellBroadcastReceiverMTS",
									},
								},
								ModuleStatus: pb.TestAggregation_FAILED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
									Passed: 0,
									Failed: 0,
								},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == "rootInvocations/ants-i14600010609614895/workUnits/root" {
						return &pb.WorkUnit{
							Name: "rootInvocations/ants-i14600010609614895/workUnits/root",
							ChildWorkUnits: []string{
								"rootInvocations/ants-i14600010609614895/workUnits/ants-wu98100269380597657",
							},
						}, nil
					}
					if in.Name == failedWU.Name {
						return failedWU, nil
					}
					return nil, nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return &pb.BatchGetWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{failedWU},
					}, nil
				},
			}

			details, err := FetchModuleDetails(ctx, client, nil, "ants-i14600010609614895", "CellBroadcastReceiverMTS")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, details.ModuleName, should.Equal("CellBroadcastReceiverMTS"))
			assert.Loosely(t, details.InvocationID, should.Equal("ants-i14600010609614895"))
			assert.Loosely(t, details.ModuleStatus, should.Equal("FAILED"))
			assert.Loosely(t, details.TotalVerdicts.Total, should.Equal(0))
			assert.Loosely(t, len(details.WorkUnits), should.Equal(1))
			assert.Loosely(t, details.WorkUnits[0].WorkUnitID, should.Equal("ants-wu98100269380597657"))
			assert.Loosely(t, details.WorkUnits[0].State, should.Equal("FAILED"))
			assert.Loosely(t, details.WorkUnits[0].ShardKey, should.Equal("0"))
			assert.Loosely(t, details.WorkUnits[0].Error, should.NotBeNil)
			assert.Loosely(t, details.WorkUnits[0].Error.RawSummary, should.Equal("TradeFed harness error: DeviceNotAvailableException"))
		})

		t.Run(`Sharded module with 1 passed shard and 1 failed shard`, func(t *ftt.Test) {
			shard0 := &pb.WorkUnit{
				Name:       "rootInvocations/build-123/workUnits/shard-0",
				WorkUnitId: "shard-0",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_SUCCEEDED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "MyShardedModule",
				},
				ModuleShardKey: "0",
			}
			shard1 := &pb.WorkUnit{
				Name:       "rootInvocations/build-123/workUnits/shard-1",
				WorkUnitId: "shard-1",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_FAILED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "MyShardedModule",
				},
				ModuleShardKey:  "1",
				SummaryMarkdown: "Shard crashed during test execution",
			}

			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{
										ModuleName: "MyShardedModule",
									},
								},
								ModuleStatus: pb.TestAggregation_FAILED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
									Passed: 20,
									Failed: 0,
								},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == "rootInvocations/build-123/workUnits/root" {
						return &pb.WorkUnit{
							Name: "rootInvocations/build-123/workUnits/root",
							ChildWorkUnits: []string{
								"rootInvocations/build-123/workUnits/shard-0",
								"rootInvocations/build-123/workUnits/shard-1",
							},
						}, nil
					}
					if in.Name == shard0.Name {
						return shard0, nil
					}
					if in.Name == shard1.Name {
						return shard1, nil
					}
					return nil, nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return &pb.BatchGetWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{shard0, shard1},
					}, nil
				},
			}

			details, err := FetchModuleDetails(ctx, client, nil, "build-123", "MyShardedModule")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, details.ModuleStatus, should.Equal("FAILED"))
			assert.Loosely(t, details.TotalVerdicts.Passed, should.Equal(20))
			assert.Loosely(t, details.TotalVerdicts.Total, should.Equal(20))
			assert.Loosely(t, len(details.WorkUnits), should.Equal(2))
			assert.Loosely(t, details.WorkUnits[0].State, should.Equal("SUCCEEDED"))
			assert.Loosely(t, details.WorkUnits[1].State, should.Equal("FAILED"))
		})

		t.Run(`Module with failed work unit and 0 tests ran`, func(t *ftt.Test) {
			wuWithSetupErr := &pb.WorkUnit{
				Name:       "rootInvocations/test-inv/workUnits/wu-1",
				WorkUnitId: "wu-1",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_FAILED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "desktop-pnp-multitasking-chrome-media",
				},
				SummaryMarkdown: "TargetSetupError: Failed to install apk",
			}

			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{
										ModuleName: "desktop-pnp-multitasking-chrome-media",
									},
								},
								ModuleStatus: pb.TestAggregation_SUCCEEDED, // ResultDB aggregated as SUCCEEDED because WU was SUCCEEDED and 0 verdicts
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
									Passed: 0,
									Failed: 0,
								},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == "rootInvocations/test-inv/workUnits/root" {
						return &pb.WorkUnit{
							Name: "rootInvocations/test-inv/workUnits/root",
							ChildWorkUnits: []string{
								"rootInvocations/test-inv/workUnits/wu-1",
							},
						}, nil
					}
					if in.Name == wuWithSetupErr.Name {
						return wuWithSetupErr, nil
					}
					return nil, nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return &pb.BatchGetWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{wuWithSetupErr},
					}, nil
				},
			}

			details, err := FetchModuleDetails(ctx, client, nil, "test-inv", "desktop-pnp-multitasking-chrome-media")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, details.ModuleStatus, should.Equal("FAILED"))
			assert.Loosely(t, details.TotalVerdicts.Total, should.Equal(0))
			assert.Loosely(t, len(details.WorkUnits), should.Equal(1))
			assert.Loosely(t, details.WorkUnits[0].State, should.Equal("FAILED"))
			assert.Loosely(t, details.WorkUnits[0].Error, should.NotBeNil)
		})

		t.Run(`Tree traversal early termination preserves invariant and prunes subtrees`, func(t *ftt.Test) {
			targetWU := &pb.WorkUnit{
				Name:       "rootInvocations/inv-tree/workUnits/target-module",
				WorkUnitId: "target-module",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_SUCCEEDED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "TargetModule",
				},
				ChildWorkUnits: []string{"rootInvocations/inv-tree/workUnits/target-child-1"},
			}
			otherWU := &pb.WorkUnit{
				Name:       "rootInvocations/inv-tree/workUnits/other-module",
				WorkUnitId: "other-module",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_FAILED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "OtherModule",
				},
				ChildWorkUnits: []string{"rootInvocations/inv-tree/workUnits/other-child-1"},
			}
			intermediateWU := &pb.WorkUnit{
				Name:           "rootInvocations/inv-tree/workUnits/intermediate-shard",
				WorkUnitId:     "intermediate-shard",
				Kind:           "RUNNER",
				State:          pb.WorkUnit_SUCCEEDED,
				ModuleId:       nil, // container work unit
				ChildWorkUnits: []string{"rootInvocations/inv-tree/workUnits/target-module-shard2"},
			}
			targetShard2WU := &pb.WorkUnit{
				Name:       "rootInvocations/inv-tree/workUnits/target-module-shard2",
				WorkUnitId: "target-module-shard2",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_SUCCEEDED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "TargetModule",
				},
				ModuleShardKey: "1",
				ChildWorkUnits: []string{"rootInvocations/inv-tree/workUnits/target-shard2-child-1"},
			}
			targetChildWU := &pb.WorkUnit{
				Name:           "rootInvocations/inv-tree/workUnits/target-child-1",
				WorkUnitId:     "target-child-1",
				Kind:           "TF_TEST_RUN",
				State:          pb.WorkUnit_SUCCEEDED,
				ChildWorkUnits: []string{"rootInvocations/inv-tree/workUnits/target-grandchild-1"},
			}
			targetShard2ChildWU := &pb.WorkUnit{
				Name:       "rootInvocations/inv-tree/workUnits/target-shard2-child-1",
				WorkUnitId: "target-shard2-child-1",
				Kind:       "TF_TEST_RUN",
				State:      pb.WorkUnit_SUCCEEDED,
			}

			fetchedNames := make(map[string]bool)
			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{ModuleName: "TargetModule"},
								},
								ModuleStatus: pb.TestAggregation_SUCCEEDED,
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == "rootInvocations/inv-tree/workUnits/root" {
						return &pb.WorkUnit{
							Name: "rootInvocations/inv-tree/workUnits/root",
							ChildWorkUnits: []string{
								targetWU.Name,
								otherWU.Name,
								intermediateWU.Name,
							},
						}, nil
					}
					return nil, nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					var res []*pb.WorkUnit
					for _, name := range in.Names {
						fetchedNames[name] = true
						switch name {
						case targetWU.Name:
							res = append(res, targetWU)
						case otherWU.Name:
							res = append(res, otherWU)
						case intermediateWU.Name:
							res = append(res, intermediateWU)
						case targetShard2WU.Name:
							res = append(res, targetShard2WU)
						case targetChildWU.Name:
							res = append(res, targetChildWU)
						case targetShard2ChildWU.Name:
							res = append(res, targetShard2ChildWU)
						default:
							t.Fatalf("unexpected work unit fetched: %s", name)
						}
					}
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: res}, nil
				},
			}

			details, err := FetchModuleDetails(ctx, client, nil, "inv-tree", "TargetModule")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(details.WorkUnits), should.Equal(2))

			// Direct TF_TEST_RUN children of matched TF_MODULE work units are fetched,
			// while children of other modules and grandchildren under TF_TEST_RUN are pruned.
			assert.Loosely(t, fetchedNames["rootInvocations/inv-tree/workUnits/target-child-1"], should.BeTrue)
			assert.Loosely(t, fetchedNames["rootInvocations/inv-tree/workUnits/target-shard2-child-1"], should.BeTrue)
			assert.Loosely(t, fetchedNames["rootInvocations/inv-tree/workUnits/other-child-1"], should.BeFalse)
			assert.Loosely(t, fetchedNames["rootInvocations/inv-tree/workUnits/target-grandchild-1"], should.BeFalse)
		})

		t.Run(`SUCCEEDED TF_MODULE with FAILED child TF_TEST_RUN aggregates failure state, summary, and full summary artifact`, func(t *ftt.Test) {
			fullSummary := "Instrumentation run failed due to 'Process crashed.'\nFull untruncated native crash stack trace"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/full-summary" {
					_, _ = w.Write([]byte(fullSummary))
					return
				}
				http.NotFound(w, r)
			}))
			defer srv.Close()

			modWUName := "rootInvocations/ants-i63900010575653989/workUnits/ants-wu019:u-mod-1"
			runWUName := "rootInvocations/ants-i63900010575653989/workUnits/ants-wu019:u-run-1"
			modWU := &pb.WorkUnit{
				Name:       modWUName,
				WorkUnitId: "ants-wu019:u-mod-1",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_SUCCEEDED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "CtsGraphicsTestCases",
				},
				ModuleShardKey: "0",
				ChildWorkUnits: []string{runWUName},
			}
			runWU := &pb.WorkUnit{
				Name:            runWUName,
				Parent:          modWUName,
				WorkUnitId:      "ants-wu019:u-run-1",
				Kind:            "TF_TEST_RUN",
				State:           pb.WorkUnit_FAILED,
				SummaryMarkdown: "Instrumentation run failed due to 'Process crashed.'",
			}

			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{ModuleName: "CtsGraphicsTestCases"},
								},
								ModuleStatus: pb.TestAggregation_SUCCEEDED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
									Passed: 10,
								},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					switch in.Name {
					case "rootInvocations/ants-i63900010575653989/workUnits/root":
						return &pb.WorkUnit{
							Name:           in.Name,
							ChildWorkUnits: []string{modWUName},
						}, nil
					case modWUName:
						return modWU, nil
					case runWUName:
						return runWU, nil
					default:
						return nil, nil
					}
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					var res []*pb.WorkUnit
					for _, name := range in.Names {
						switch name {
						case modWUName:
							res = append(res, modWU)
						case runWUName:
							res = append(res, runWU)
						}
					}
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: res}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == runWUName {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:       runWUName + "/artifacts/testRunFailed_full_summary_markdown",
									ArtifactId: "testRunFailed_full_summary_markdown",
									FetchUrl:   srv.URL + "/full-summary",
									SizeBytes:  int64(len(fullSummary)),
								},
							},
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			details, err := FetchModuleDetails(ctx, client, srv.Client(), "ants-i63900010575653989", "CtsGraphicsTestCases")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, details.ModuleStatus, should.Equal("FAILED"))
			assert.Loosely(t, len(details.WorkUnits), should.Equal(1))
			assert.Loosely(t, details.WorkUnits[0].State, should.Equal("FAILED"))
			assert.Loosely(t, details.WorkUnits[0].Summary, should.Equal(fullSummary))
			assert.Loosely(t, details.WorkUnits[0].Error, should.NotBeNil)
			assert.Loosely(t, details.WorkUnits[0].Error.RawSummary, should.Equal(fullSummary))

			var buf bytes.Buffer
			fprintModuleDetails(&buf, details, false)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Status:        FAILED\n"))
			assert.Loosely(t, out, should.ContainSubstring("  [FAILED] ants-wu019:u-mod-1 (Shard: 0, Kind: TF_MODULE)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Full untruncated native crash stack trace"))
			assert.Loosely(t, out, should.ContainSubstring("To fetch full failure summary (testRunFailed_full_summary_markdown):\n      luci artifact get -invocationid=ants-i63900010575653989 -workunitid=ants-wu019:u-run-1 -artifactid=testRunFailed_full_summary_markdown"))
		})

		t.Run(`SUCCEEDED TF_MODULE with failed or execution-errored verdicts runs DiscoverWorkUnitError and reports FAILED`, func(t *ftt.Test) {
			modWUName := "rootInvocations/ants-i63900010575653989/workUnits/ants-wu019:u-mod-1"
			taskWUName := "rootInvocations/ants-i63900010575653989/workUnits/ants-wu019"
			modWU := &pb.WorkUnit{
				Name:       modWUName,
				Parent:     taskWUName,
				WorkUnitId: "ants-wu019:u-mod-1",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_SUCCEEDED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "CtsGraphicsTestCases",
				},
			}

			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{ModuleName: "CtsGraphicsTestCases"},
								},
								ModuleStatus: pb.TestAggregation_SUCCEEDED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
									Passed:           3169,
									Failed:           1,
									ExecutionErrored: 705,
								},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == "rootInvocations/ants-i63900010575653989/workUnits/root" {
						return &pb.WorkUnit{
							Name:           in.Name,
							ChildWorkUnits: []string{modWUName},
						}, nil
					}
					return modWU, nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: []*pb.WorkUnit{modWU}}, nil
				},
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{
								Name:            taskWUName,
								WorkUnitId:      "ants-wu019",
								SummaryMarkdown: "Instrumentation run failed due to HWASan crash",
							},
						},
					}, nil
				},
			}

			details, err := FetchModuleDetails(ctx, client, nil, "ants-i63900010575653989", "CtsGraphicsTestCases")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, details.ModuleStatus, should.Equal("FAILED"))
			assert.Loosely(t, len(details.WorkUnits), should.Equal(1))
			assert.Loosely(t, details.WorkUnits[0].Error, should.NotBeNil)
			assert.Loosely(t, details.WorkUnits[0].Error.RawSummary, should.Equal("Instrumentation run failed due to HWASan crash"))
		})

		t.Run(`QueryTestAggregations error propagated`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return nil, status.Errorf(codes.PermissionDenied, "access denied")
				},
			}
			details, err := FetchModuleDetails(ctx, client, nil, "inv-1", "MyModule")
			assert.Loosely(t, details, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("query test aggregations"))
			assert.Loosely(t, err, should.ErrLike("missing ResultDB realm permission"))
			assert.Loosely(t, err.Error(), should.NotContainSubstring("mdb/android-build-access"))
			assert.Loosely(t, err, should.ErrLike("access denied"))

			_, errAndroid := FetchModuleDetails(ctx, client, nil, "I77100010600769898", "MyModule")
			assert.Loosely(t, errAndroid, should.ErrLike("missing ResultDB realm permission"))
			assert.Loosely(t, errAndroid, should.ErrLike(base.AndroidAccessHint))
		})

		t.Run(`discoverModuleWorkUnits root WU error propagated`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return nil, status.Errorf(codes.PermissionDenied, "permission denied")
				},
			}
			details, err := FetchModuleDetails(ctx, client, nil, "inv-1", "MyModule")
			assert.Loosely(t, details, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("discover module work units"))
			assert.Loosely(t, err, should.ErrLike("get root work unit"))
			assert.Loosely(t, err, should.ErrLike("permission denied"))
		})

		t.Run(`discoverModuleWorkUnits batch RPC error propagated`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{
						Name:           "rootInvocations/inv-1/workUnits/root",
						ChildWorkUnits: []string{"rootInvocations/inv-1/workUnits/child-1"},
					}, nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return nil, status.Errorf(codes.Unavailable, "quota exceeded or network failure")
				},
			}
			details, err := FetchModuleDetails(ctx, client, nil, "inv-1", "MyModule")
			assert.Loosely(t, details, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("discover module work units"))
			assert.Loosely(t, err, should.ErrLike("batch get work units"))
			assert.Loosely(t, err, should.ErrLike("quota exceeded or network failure"))
		})
	})

	ftt.Run(`moduleGetRun`, t, func(t *ftt.Test) {
		t.Run(`flag aliases -invocation-id and -module-name`, func(t *ftt.Test) {
			r := GetCmd(nil).CommandRun().(*moduleGetRun)
			assert.Loosely(t, r.Flags.Parse([]string{"-invocation-id", "inv-1", "-module-name", "ModA"}), should.BeNil)
			assert.Loosely(t, r.invocationID, should.Equal("inv-1"))
			assert.Loosely(t, r.moduleName, should.Equal("ModA"))
		})

		t.Run(`rejects legacy invocation`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
				getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
					return &pb.Invocation{Name: in.Name}, nil
				},
			}
			r := &moduleGetRun{
				client:       client,
				invocationID: "u-swarming-123",
				moduleName:   "ModA",
			}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(1))
		})

		t.Run(`fails immediately on PermissionDenied`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.PermissionDenied, "caller does not have permission")
				},
			}
			r := &moduleGetRun{
				client:       client,
				invocationID: "build-123",
				moduleName:   "ModA",
			}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(1))
		})

		t.Run(`accepts positional URL and prints derived IDs`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{ModuleName: "CellBroadcastReceiverMTS"},
								},
								ModuleStatus: pb.TestAggregation_FAILED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
									Passed: 5,
									Failed: 1,
								},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{Name: in.Name}, nil
				},
			}
			cmd := GetCmd(nil)
			r := cmd.CommandRun().(*moduleGetRun)
			r.client = client
			out := testutil.CaptureStdout(func() {
				rc := r.Run(nil, []string{"rootInvocations/ants-i14600010609614895/workUnits/ants-wu1/tests/:CellBroadcastReceiverMTS!junit:pkg:Cls%23testOne/results/res-1"}, nil)
				assert.Loosely(t, rc, should.Equal(0))
			})
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     ants-i14600010609614895"))
			assert.Loosely(t, out, should.ContainSubstring("Module Name:       CellBroadcastReceiverMTS"))
			assert.Loosely(t, out, should.ContainSubstring("Module:        CellBroadcastReceiverMTS"))
		})

		t.Run(`surfaces RootInvocation metadata and honors -show-metadata`, func(t *ftt.Test) {
			wuProps, _ := structpb.NewStruct(map[string]any{"abi": "x86_64"})
			invProps, _ := structpb.NewStruct(map[string]any{"scheduler": "ATP"})
			wu := &pb.WorkUnit{
				Name:       "rootInvocations/ants-i14600010609614895/workUnits/ants-wu981",
				Parent:     "rootInvocations/ants-i14600010609614895/workUnits/root",
				WorkUnitId: "ants-wu981",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_SUCCEEDED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName:        "CellBroadcastReceiverMTS",
					ModuleScheme:      "junit",
					ModuleVariantHash: "abcd1234",
					ModuleVariant: &pb.Variant{
						Def: map[string]string{"abi": "x86_64"},
					},
				},
				ModuleShardKey: "0",
				Tags: []*pb.StringPair{
					{Key: "module_tag", Value: "val1"},
				},
				Properties: wuProps,
			}
			rootInv := &pb.RootInvocation{
				Name:              "rootInvocations/ants-i14600010609614895",
				State:             pb.RootInvocation_SUCCEEDED,
				FinalizationState: pb.RootInvocation_FINALIZED,
				Realm:             "google:android-test-results",
				PrimaryBuild: &pb.BuildDescriptor{
					Definition: &pb.BuildDescriptor_AndroidBuild{
						AndroidBuild: &pb.AndroidBuildDescriptor{
							DataRealm:   "prod",
							Branch:      "git_main",
							BuildTarget: "cf_x86_64_phone-trunk_staging-userdebug",
							BuildId:     "16133789",
						},
					},
				},
				Definition: &pb.RootInvocationDefinition{
					System: "atp",
					Name:   "v2/android-mts/mts-cellbroadcast",
				},
				Properties: invProps,
			}

			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return rootInv, nil
				},
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{ModuleName: "CellBroadcastReceiverMTS"},
								},
								ModuleStatus: pb.TestAggregation_SUCCEEDED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
									Passed: 10,
								},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == "rootInvocations/ants-i14600010609614895/workUnits/root" {
						return &pb.WorkUnit{
							Name:           in.Name,
							ChildWorkUnits: []string{wu.Name},
						}, nil
					}
					return wu, nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: []*pb.WorkUnit{wu}}, nil
				},
			}

			details, err := FetchModuleDetails(context.Background(), client, nil, "ants-i14600010609614895", "CellBroadcastReceiverMTS")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, details.RootInvocation, should.Equal(rootInv))
			assert.Loosely(t, len(details.WorkUnits), should.Equal(1))
			assert.Loosely(t, details.WorkUnits[0].ModuleScheme, should.Equal("junit"))
			assert.Loosely(t, details.WorkUnits[0].VariantHash, should.Equal("abcd1234"))
			assert.Loosely(t, len(details.WorkUnits[0].Tags), should.Equal(1))
			assert.Loosely(t, details.WorkUnits[0].Properties, should.NotBeNil)

			var defaultBuf bytes.Buffer
			fprintModuleDetails(&defaultBuf, details, false)
			defaultOut := defaultBuf.String()
			assert.Loosely(t, defaultOut, should.NotContainSubstring("Primary Build:"))
			assert.Loosely(t, defaultOut, should.NotContainSubstring("Definition:"))
			assert.Loosely(t, defaultOut, should.NotContainSubstring("Scheme:"))
			assert.Loosely(t, defaultOut, should.NotContainSubstring("Realm:"))

			var buf bytes.Buffer
			fprintModuleDetails(&buf, details, true)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Primary Build: branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=16133789\n"))
			assert.Loosely(t, out, should.ContainSubstring("Definition:    system=atp name=v2/android-mts/mts-cellbroadcast\n"))
			assert.Loosely(t, out, should.ContainSubstring("    Scheme:       junit\n"))
			assert.Loosely(t, out, should.ContainSubstring("    Variant:      abi=x86_64 (hash: abcd1234)\n"))
			assert.Loosely(t, out, should.ContainSubstring("    Tags:\n      module_tag: val1\n"))
			assert.Loosely(t, out, should.ContainSubstring("Realm:        google:android-test-results\n"))
			assert.Loosely(t, out, should.ContainSubstring(`"scheduler"`))
			assert.Loosely(t, out, should.ContainSubstring(`"ATP"`))
		})

		t.Run(`surfaces direct and ancestor work unit artifact notices for failed module`, func(t *ftt.Test) {
			modWUName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu505:u-mod-1"
			taskWUName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu505"
			rootWUName := "rootInvocations/ants-i14600010609614895/workUnits/root"

			modWU := &pb.WorkUnit{
				Name:       modWUName,
				Parent:     taskWUName,
				WorkUnitId: "ants-wu505:u-mod-1",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_FAILED,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName: "CellBroadcastReceiverMTS",
				},
				ModuleShardKey:  "0",
				SummaryMarkdown: "TradeFed harness error: DeviceNotAvailableException",
			}

			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{ModuleName: "CellBroadcastReceiverMTS"},
								},
								ModuleStatus:       pb.TestAggregation_FAILED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					switch in.Name {
					case rootWUName:
						return &pb.WorkUnit{
							Name:           rootWUName,
							ChildWorkUnits: []string{modWUName},
						}, nil
					case modWUName:
						return modWU, nil
					default:
						return nil, nil
					}
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: []*pb.WorkUnit{modWU}}, nil
				},
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					if in.GetPredicate().GetAncestorsOf() == modWUName {
						return &pb.QueryWorkUnitsResponse{
							WorkUnits: []*pb.WorkUnit{
								{Name: taskWUName, WorkUnitId: "ants-wu505", Parent: rootWUName, Kind: "TFC_COMMAND_TASK"},
								{Name: rootWUName, WorkUnitId: "root", Kind: "ATP_INVOCATION"},
							},
						}, nil
					}
					return &pb.QueryWorkUnitsResponse{}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					switch in.Parent {
					case modWUName:
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         modWUName + "/artifacts/module_logcat.txt",
									ArtifactId:   "module_logcat.txt",
									ArtifactType: "LOGCAT",
									ContentType:  "text/plain",
									SizeBytes:    4096,
								},
							},
						}, nil
					case taskWUName:
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         taskWUName + "/artifacts/host_log_999.txt",
									ArtifactId:   "host_log_999.txt",
									ArtifactType: "HOST_LOG",
									ContentType:  "text/plain",
									SizeBytes:    16384,
								},
								{
									Name:         taskWUName + "/artifacts/bugreportz_999.zip",
									ArtifactId:   "bugreportz_999.zip",
									ArtifactType: "BUGREPORTZ",
									ContentType:  "application/zip",
									SizeBytes:    1048576,
								},
							},
						}, nil
					default:
						return &pb.ListArtifactsResponse{}, nil
					}
				},
			}

			details, err := FetchModuleDetails(context.Background(), client, nil, "ants-i14600010609614895", "CellBroadcastReceiverMTS")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(details.WorkUnits), should.Equal(1))
			assert.Loosely(t, details.WorkUnits[0].ArtifactList, should.Match([]string{"module_logcat.txt"}))

			var buf bytes.Buffer
			fprintModuleDetails(&buf, details, false)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:\n"))
			assert.Loosely(t, out, should.ContainSubstring("  Work unit ants-wu505:u-mod-1 (1 artifact):\n"))
			assert.Loosely(t, out, should.ContainSubstring("module_logcat.txt"))
			assert.Loosely(t, out, should.ContainSubstring("To fetch a work unit artifact:\n  luci artifact <get|head|tail> -invocationid=ants-i14600010609614895 -workunitid=ants-wu505:u-mod-1 -artifactid=<ARTIFACT_ID>"))
			assert.Loosely(t, out, should.ContainSubstring("Ancestor Work Unit Artifacts:\n"))
			assert.Loosely(t, out, should.ContainSubstring("  Work unit ants-wu505 (2 artifacts):\n"))
			assert.Loosely(t, out, should.ContainSubstring("bugreportz_999.zip"))
			assert.Loosely(t, out, should.ContainSubstring("host_log_999.txt"))
			assert.Loosely(t, out, should.ContainSubstring("To fetch an ancestor work unit artifact:\n  luci artifact <get|head|tail> -invocationid=ants-i14600010609614895 -workunitid=ants-wu505 -artifactid=<ARTIFACT_ID>"))
		})

		t.Run(`bounds output to DefaultMaxOutputBytes and deduplicates ancestor RPCs for 100 failed shards with artifacts`, func(t *ftt.Test) {
			taskWUName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu505"
			rootWUName := "rootInvocations/ants-i14600010609614895/workUnits/root"

			const numShards = 100
			childNames := make([]string, numShards)
			shardWUs := make([]*pb.WorkUnit, numShards)
			shardByName := make(map[string]*pb.WorkUnit, numShards)
			for i := 0; i < numShards; i++ {
				wuID := fmt.Sprintf("ants-wu505:u-mod-%d", i)
				wuName := "rootInvocations/ants-i14600010609614895/workUnits/" + wuID
				childNames[i] = wuName
				wu := &pb.WorkUnit{
					Name:       wuName,
					Parent:     taskWUName,
					WorkUnitId: wuID,
					Kind:       "TF_MODULE",
					State:      pb.WorkUnit_FAILED,
					ModuleId: &pb.ModuleIdentifier{
						ModuleName: "CellBroadcastReceiverMTS",
					},
					ModuleShardKey:  fmt.Sprintf("%d", i),
					SummaryMarkdown: "TradeFed harness error: DeviceNotAvailableException",
				}
				shardWUs[i] = wu
				shardByName[wuName] = wu
			}

			var queryWorkUnitsCalls atomic.Int32
			var listArtifactsCalls atomic.Int32

			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id: &pb.TestIdentifierPrefix{
									Id: &pb.TestIdentifier{ModuleName: "CellBroadcastReceiverMTS"},
								},
								ModuleStatus:       pb.TestAggregation_FAILED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == rootWUName {
						return &pb.WorkUnit{
							Name:           rootWUName,
							ChildWorkUnits: childNames,
						}, nil
					}
					return shardByName[in.Name], nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: shardWUs}, nil
				},
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					queryWorkUnitsCalls.Add(1)
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{Name: taskWUName, WorkUnitId: "ants-wu505", Parent: rootWUName, Kind: "TFC_COMMAND_TASK"},
							{Name: rootWUName, WorkUnitId: "root", Kind: "ATP_INVOCATION"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					listArtifactsCalls.Add(1)
					if in.Parent == taskWUName {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         taskWUName + "/artifacts/host_log_999.txt",
									ArtifactId:   "host_log_999.txt",
									ArtifactType: "HOST_LOG",
									ContentType:  "text/plain",
									SizeBytes:    16384,
								},
							},
						}, nil
					}
					if _, ok := shardByName[in.Parent]; ok {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         in.Parent + "/artifacts/module_logcat.txt",
									ArtifactId:   "module_logcat.txt",
									ArtifactType: "LOGCAT",
									ContentType:  "text/plain",
									SizeBytes:    4096,
								},
							},
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			}))
			defer srv.Close()

			details, err := FetchModuleDetails(context.Background(), client, srv.Client(), "ants-i14600010609614895", "CellBroadcastReceiverMTS")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(details.WorkUnits), should.Equal(numShards))
			assert.Loosely(t, queryWorkUnitsCalls.Load(), should.Equal(int32(1)))
			assert.Loosely(t, listArtifactsCalls.Load() <= 22, should.BeTrue)

			var buf bytes.Buffer
			fprintModuleDetails(&buf, details, false)
			out := buf.String()
			assert.Loosely(t, len(out) <= format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("  ... and 80 more work units\n"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:\n"))
			assert.Loosely(t, out, should.ContainSubstring("more work units with artifacts; run 'luci artifact list -invocationid=ants-i14600010609614895' to view all"))
			assert.Loosely(t, out, should.ContainSubstring("Ancestor Work Unit Artifacts:\n"))
			idx := strings.Index(out, "Work Unit Artifacts:\n")
			assert.Loosely(t, idx > 0, should.BeTrue)
			assert.Loosely(t, len(out[idx:]) <= format.DefaultInlineGetArtifactBudget, should.BeTrue)
		})

		t.Run(`includes derived IDs prefix within DefaultMaxOutputBytes cap`, func(t *ftt.Test) {
			derivedPrefix := "Invocation ID: ants-i14600010609614895\n" + strings.Repeat("d", 1200) + "\n\n"
			details := &ModuleDetails{
				ModuleName:   "CellBroadcastReceiverMTS",
				InvocationID: "ants-i14600010609614895",
				ModuleStatus: "FAILED",
				WorkUnits: []*WorkUnitInfo{
					{
						WorkUnitID: "ants-wu001",
						State:      "FAILED",
						Error: &format.DiscoveredError{
							Message:    "java.lang.RuntimeException: fatal failure",
							StackTrace: strings.Repeat("at com.android.Foo.bar(Foo.java:42)\n", 260),
						},
					},
				},
				ArtifactNotices: "Work Unit Artifacts:\n  host_log.txt  HOST_LOG  4.0 KiB  text/plain\n",
			}

			var buf bytes.Buffer
			fprintModuleDetailsWithDerived(&buf, derivedPrefix, details, false)
			out := buf.String()
			assert.Loosely(t, len(out) <= format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, strings.HasPrefix(out, "Invocation ID: ants-i14600010609614895\n"), should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("bytes truncated; pass -show-metadata to view full output"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:\n"))
			assert.Loosely(t, out, should.ContainSubstring("host_log.txt"))
		})

		t.Run(`discovers and displays failed shard beyond maxDisplayedModuleWorkUnits`, func(t *ftt.Test) {
			rootWUName := "rootInvocations/ants-i14600010609614895/workUnits/root"
			taskWUName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu505"
			taskWU := &pb.WorkUnit{
				Name:       taskWUName,
				Parent:     rootWUName,
				WorkUnitId: "ants-wu505",
				Kind:       "TFC_COMMAND_TASK",
			}

			const numShards = 25
			shardNames := make([]string, numShards)
			allWUs := []*pb.WorkUnit{taskWU}
			wuByName := map[string]*pb.WorkUnit{taskWUName: taskWU}
			for i := 0; i < numShards; i++ {
				wuID := fmt.Sprintf("ants-wu505:u-mod-%d", i)
				wuName := "rootInvocations/ants-i14600010609614895/workUnits/" + wuID
				shardNames[i] = wuName
				wu := &pb.WorkUnit{
					Name:           wuName,
					Parent:         taskWUName,
					WorkUnitId:     wuID,
					Kind:           "TF_MODULE",
					State:          pb.WorkUnit_SUCCEEDED,
					ModuleId:       &pb.ModuleIdentifier{ModuleName: "CellBroadcastReceiverMTS"},
					ModuleShardKey: fmt.Sprintf("%d", i),
				}
				if i == 22 {
					runName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu505:u-run-22"
					wu.ChildWorkUnits = []string{runName}
					runWU := &pb.WorkUnit{
						Name:            runName,
						Parent:          wuName,
						WorkUnitId:      "ants-wu505:u-run-22",
						Kind:            "TF_TEST_RUN",
						State:           pb.WorkUnit_FAILED,
						SummaryMarkdown: "Shard 22 native crash",
					}
					allWUs = append(allWUs, runWU)
					wuByName[runName] = runWU
				}
				allWUs = append(allWUs, wu)
				wuByName[wuName] = wu
			}
			taskWU.ChildWorkUnits = shardNames

			var queryWorkUnitsCalls atomic.Int32
			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id:                 &pb.TestIdentifierPrefix{Id: &pb.TestIdentifier{ModuleName: "CellBroadcastReceiverMTS"}},
								ModuleStatus:       pb.TestAggregation_SUCCEEDED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Passed: 500, Failed: 1},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == rootWUName {
						return &pb.WorkUnit{Name: rootWUName, ChildWorkUnits: []string{taskWUName}}, nil
					}
					return wuByName[in.Name], nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					var res []*pb.WorkUnit
					for _, n := range in.Names {
						if w := wuByName[n]; w != nil {
							res = append(res, w)
						}
					}
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: res}, nil
				},
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					queryWorkUnitsCalls.Add(1)
					return &pb.QueryWorkUnitsResponse{}, nil
				},
			}
			_ = allWUs

			details, err := FetchModuleDetails(context.Background(), client, nil, "ants-i14600010609614895", "CellBroadcastReceiverMTS")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, details.ModuleStatus, should.Equal("FAILED"))
			assert.Loosely(t, queryWorkUnitsCalls.Load(), should.Equal(int32(0)))
			assert.Loosely(t, details.WorkUnits[22].State, should.Equal("FAILED"))
			assert.Loosely(t, details.WorkUnits[22].Error, should.NotBeNil)
			assert.Loosely(t, details.WorkUnits[22].Error.RawSummary, should.Equal("Shard 22 native crash"))

			var buf bytes.Buffer
			fprintModuleDetails(&buf, details, false)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("  [FAILED] ants-wu505:u-mod-22 (Shard: 22, Kind: TF_MODULE)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Shard 22 native crash"))
		})

		t.Run(`bounds Layer 2 ListArtifacts fan-out on 100 SUCCEEDED shards when verdict fails and httpClient is non-nil`, func(t *ftt.Test) {
			rootWUName := "rootInvocations/ants-i14600010609614895/workUnits/root"
			const numTasks = 20
			const numShards = 100

			var xmlFetches atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				xmlFetches.Add(1)
				_, _ = w.Write([]byte(`<?xml version="1.0"?><TestResult><Module name="CellBroadcastReceiverMTS" /></TestResult>`))
			}))
			defer srv.Close()

			taskNames := make([]string, numTasks)
			wuByName := make(map[string]*pb.WorkUnit, numTasks+numShards)
			for k := 0; k < numTasks; k++ {
				tName := fmt.Sprintf("rootInvocations/ants-i14600010609614895/workUnits/ants-wu-task-%d", k)
				taskNames[k] = tName
				wuByName[tName] = &pb.WorkUnit{
					Name:       tName,
					Parent:     rootWUName,
					WorkUnitId: fmt.Sprintf("ants-wu-task-%d", k),
					Kind:       "TFC_COMMAND_TASK",
					State:      pb.WorkUnit_SUCCEEDED,
				}
			}
			for i := 0; i < numShards; i++ {
				tName := taskNames[i%numTasks]
				wuID := fmt.Sprintf("ants-wu-task-%d:u-mod-%d", i%numTasks, i)
				wuName := "rootInvocations/ants-i14600010609614895/workUnits/" + wuID
				wuByName[tName].ChildWorkUnits = append(wuByName[tName].ChildWorkUnits, wuName)
				wuByName[wuName] = &pb.WorkUnit{
					Name:           wuName,
					Parent:         tName,
					WorkUnitId:     wuID,
					Kind:           "TF_MODULE",
					State:          pb.WorkUnit_SUCCEEDED,
					ModuleId:       &pb.ModuleIdentifier{ModuleName: "CellBroadcastReceiverMTS"},
					ModuleShardKey: fmt.Sprintf("%d", i),
				}
			}

			var listArtifactsCalls atomic.Int32
			var xmlListArtifactsCalls atomic.Int32
			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id:                 &pb.TestIdentifierPrefix{Id: &pb.TestIdentifier{ModuleName: "CellBroadcastReceiverMTS"}},
								ModuleStatus:       pb.TestAggregation_FAILED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Passed: 4999, Failed: 1},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == rootWUName {
						return &pb.WorkUnit{Name: rootWUName, ChildWorkUnits: taskNames}, nil
					}
					return wuByName[in.Name], nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					var res []*pb.WorkUnit
					for _, n := range in.Names {
						if w := wuByName[n]; w != nil {
							res = append(res, w)
						}
					}
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: res}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					listArtifactsCalls.Add(1)
					if in.GetPredicate() != nil {
						xmlListArtifactsCalls.Add(1)
					}
					if w := wuByName[in.Parent]; w != nil && w.Kind == "TFC_COMMAND_TASK" {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:       in.Parent + "/artifacts/subprocess-test_result.xml",
									ArtifactId: "subprocess-test_result.xml",
									FetchUrl:   srv.URL + "/xml",
								},
							},
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			details, err := FetchModuleDetails(context.Background(), client, srv.Client(), "ants-i14600010609614895", "CellBroadcastReceiverMTS")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, details.ModuleStatus, should.Equal("FAILED"))
			assert.Loosely(t, len(details.WorkUnits), should.Equal(numShards))
			// Bounded to 5 shard work units + their 5 tasks + 1 root = 11 Layer 2 XML ListArtifacts RPCs (plus 11 metadata-only ReadMask RPCs, rather than 121+) and <= 5 XML fetches (rather than 20).
			assert.Loosely(t, xmlListArtifactsCalls.Load() <= 11, should.BeTrue)
			assert.Loosely(t, listArtifactsCalls.Load() <= 22, should.BeTrue)
			assert.Loosely(t, xmlFetches.Load() <= 5, should.BeTrue)
		})

		t.Run(`preserves SKIPPED module status and informational SUCCEEDED summaries`, func(t *ftt.Test) {
			rootWUName := "rootInvocations/ants-i14600010609614895/workUnits/root"
			skipModName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu-skip:u-mod"
			skipRunName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu-skip:u-run"
			skipMod := &pb.WorkUnit{
				Name:            skipModName,
				Parent:          rootWUName,
				WorkUnitId:      "ants-wu-skip:u-mod",
				Kind:            "TF_MODULE",
				State:           pb.WorkUnit_SKIPPED,
				ModuleId:        &pb.ModuleIdentifier{ModuleName: "SkippedMod"},
				SummaryMarkdown: "Skipped by ABI filter",
				ChildWorkUnits:  []string{skipRunName},
			}
			skipRun := &pb.WorkUnit{
				Name:            skipRunName,
				Parent:          skipModName,
				WorkUnitId:      "ants-wu-skip:u-run",
				Kind:            "TF_TEST_RUN",
				State:           pb.WorkUnit_SKIPPED,
				SummaryMarkdown: "Skipped run",
			}

			passModName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu-pass:u-mod"
			passMod := &pb.WorkUnit{
				Name:            passModName,
				Parent:          rootWUName,
				WorkUnitId:      "ants-wu-pass:u-mod",
				Kind:            "TF_MODULE",
				State:           pb.WorkUnit_SUCCEEDED,
				ModuleId:        &pb.ModuleIdentifier{ModuleName: "PassedMod"},
				SummaryMarkdown: "Completed 10 tests cleanly",
			}

			wuByName := map[string]*pb.WorkUnit{
				skipModName: skipMod,
				skipRunName: skipRun,
				passModName: passMod,
			}
			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					if strings.Contains(in.GetPredicate().GetContentsFilter(), `"SkippedMod"`) {
						return &pb.QueryTestAggregationsResponse{
							Aggregations: []*pb.TestAggregation{
								{
									Id:                 &pb.TestIdentifierPrefix{Id: &pb.TestIdentifier{ModuleName: "SkippedMod"}},
									ModuleStatus:       pb.TestAggregation_SKIPPED,
									TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
								},
							},
						}, nil
					}
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id:                 &pb.TestIdentifierPrefix{Id: &pb.TestIdentifier{ModuleName: "PassedMod"}},
								ModuleStatus:       pb.TestAggregation_SUCCEEDED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Passed: 10},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == rootWUName {
						return &pb.WorkUnit{Name: rootWUName, ChildWorkUnits: []string{skipModName, passModName}}, nil
					}
					return wuByName[in.Name], nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					var res []*pb.WorkUnit
					for _, n := range in.Names {
						if w := wuByName[n]; w != nil {
							res = append(res, w)
						}
					}
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: res}, nil
				},
			}

			skipDetails, err := FetchModuleDetails(context.Background(), client, nil, "ants-i14600010609614895", "SkippedMod")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, skipDetails.ModuleStatus, should.Equal("SKIPPED"))
			assert.Loosely(t, skipDetails.WorkUnits[0].State, should.Equal("SKIPPED"))
			assert.Loosely(t, skipDetails.WorkUnits[0].Error, should.BeNil)

			passDetails, err := FetchModuleDetails(context.Background(), client, nil, "ants-i14600010609614895", "PassedMod")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, passDetails.ModuleStatus, should.Equal("SUCCEEDED"))
			assert.Loosely(t, passDetails.WorkUnits[0].Error, should.BeNil)
		})

		t.Run(`sorts SUCCEEDED shard with ancestor Error ahead of healthy shards and keeps real ancestor summary over generic child wrapper`, func(t *ftt.Test) {
			rootWUName := "rootInvocations/ants-i14600010609614895/workUnits/root"
			okTaskName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu-ok"
			badTaskName := "rootInvocations/ants-i14600010609614895/workUnits/ants-wu-bad"

			okTask := &pb.WorkUnit{Name: okTaskName, Parent: rootWUName, WorkUnitId: "ants-wu-ok", Kind: "TFC_COMMAND_TASK", State: pb.WorkUnit_SUCCEEDED}
			badTask := &pb.WorkUnit{
				Name:            badTaskName,
				Parent:          rootWUName,
				WorkUnitId:      "ants-wu-bad",
				Kind:            "TFC_COMMAND_TASK",
				State:           pb.WorkUnit_FAILED,
				SummaryMarkdown: "Device rebooted unexpectedly during command task",
			}

			const numShards = 25
			wuByName := map[string]*pb.WorkUnit{
				okTaskName:  okTask,
				badTaskName: badTask,
			}
			for i := 0; i < numShards; i++ {
				parentName := okTaskName
				if i == 22 || i == 23 {
					parentName = badTaskName
				}
				wuID := fmt.Sprintf("shard-%d", i)
				wuName := "rootInvocations/ants-i14600010609614895/workUnits/" + wuID
				wuByName[parentName].ChildWorkUnits = append(wuByName[parentName].ChildWorkUnits, wuName)
				wu := &pb.WorkUnit{
					Name:           wuName,
					Parent:         parentName,
					WorkUnitId:     wuID,
					Kind:           "TF_MODULE",
					State:          pb.WorkUnit_SUCCEEDED,
					ModuleId:       &pb.ModuleIdentifier{ModuleName: "CellBroadcastReceiverMTS"},
					ModuleShardKey: fmt.Sprintf("%d", i),
				}
				if i == 23 {
					runName := wuName + ":u-run"
					wu.ChildWorkUnits = []string{runName}
					wuByName[runName] = &pb.WorkUnit{
						Name:            runName,
						Parent:          wuName,
						WorkUnitId:      wuID + ":u-run",
						Kind:            "TF_TEST_RUN",
						State:           pb.WorkUnit_FAILED,
						SummaryMarkdown: "MobileHarness wrapper: ANDROID_TRADEFED_TEST_HAS_FAIL_SUBTEST",
					}
				}
				wuByName[wuName] = wu
			}

			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								Id:                 &pb.TestIdentifierPrefix{Id: &pb.TestIdentifier{ModuleName: "CellBroadcastReceiverMTS"}},
								ModuleStatus:       pb.TestAggregation_SUCCEEDED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Passed: 400, ExecutionErrored: 10},
							},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == rootWUName {
						return &pb.WorkUnit{Name: rootWUName, ChildWorkUnits: []string{okTaskName, badTaskName}}, nil
					}
					return wuByName[in.Name], nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					var res []*pb.WorkUnit
					for _, n := range in.Names {
						if w := wuByName[n]; w != nil {
							res = append(res, w)
						}
					}
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: res}, nil
				},
			}

			details, err := FetchModuleDetails(context.Background(), client, nil, "ants-i14600010609614895", "CellBroadcastReceiverMTS")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, details.ModuleStatus, should.Equal("FAILED"))

			var shard22, shard23 *WorkUnitInfo
			for _, w := range details.WorkUnits {
				if w.WorkUnitID == "shard-22" {
					shard22 = w
				} else if w.WorkUnitID == "shard-23" {
					shard23 = w
				}
			}
			assert.Loosely(t, shard22, should.NotBeNil)
			assert.Loosely(t, shard22.State, should.Equal("SUCCEEDED"))
			assert.Loosely(t, shard22.Error, should.NotBeNil)
			assert.Loosely(t, shard22.Error.RawSummary, should.Equal("Device rebooted unexpectedly during command task"))

			assert.Loosely(t, shard23, should.NotBeNil)
			assert.Loosely(t, shard23.State, should.Equal("FAILED"))
			assert.Loosely(t, shard23.Error, should.NotBeNil)
			assert.Loosely(t, shard23.Error.RawSummary, should.Equal("Device rebooted unexpectedly during command task"))

			// Truncate multi-byte UTF-8 summary at maxModuleSummaryBytes cleanly on rune boundary.
			shard23.Error.RawSummary = strings.Repeat("a", maxModuleSummaryBytes-1) + "€extra"

			var buf bytes.Buffer
			fprintModuleDetails(&buf, details, false)
			out := buf.String()
			assert.Loosely(t, utf8.ValidString(out), should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("  [FAILED] shard-23 (Shard: 23, Kind: TF_MODULE)\n"))
			assert.Loosely(t, out, should.ContainSubstring("  [SUCCEEDED] shard-22 (Shard: 22, Kind: TF_MODULE)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Device rebooted unexpectedly during command task"))
			assert.Loosely(t, out, should.NotContainSubstring("ANDROID_TRADEFED_TEST_HAS_FAIL_SUBTEST"))
		})

		t.Run(`preserves FLAKY on retried shards, ignores cross-module root summaries on SUCCEEDED shards, and caps child artifact RPCs`, func(t *ftt.Test) {
			rootWUName := "rootInvocations/ants-i14600010609614895/workUnits/root"
			atiURL := "https://android-build.corp.google.com/test_investigate/invocation/I14600010609614895"

			// 1. Retried shard: Attempt 0 failed at TF_TEST_RUN, Attempt 1 on same ShardKey "0" succeeded.
			attempt0Mod := &pb.WorkUnit{
				Name:            "rootInvocations/ants-i14600010609614895/workUnits/attempt0:u-mod",
				Parent:          rootWUName,
				WorkUnitId:      "attempt0:u-mod",
				Kind:            "TF_MODULE",
				State:           pb.WorkUnit_SUCCEEDED,
				ModuleId:        &pb.ModuleIdentifier{ModuleName: "FlakyMod"},
				ModuleShardKey:  "0",
				SummaryMarkdown: "ANDROID_TRADEFED_TEST_HAS_FAIL_SUBTEST",
				ChildWorkUnits:  []string{"rootInvocations/ants-i14600010609614895/workUnits/attempt0:u-run"},
			}
			attempt0Run := &pb.WorkUnit{
				Name:       "rootInvocations/ants-i14600010609614895/workUnits/attempt0:u-run",
				Parent:     attempt0Mod.Name,
				WorkUnitId: "attempt0:u-run",
				Kind:       "TF_TEST_RUN",
				State:      pb.WorkUnit_FAILED,
			}
			attempt1Mod := &pb.WorkUnit{
				Name:           "rootInvocations/ants-i14600010609614895/workUnits/attempt1:u-mod",
				Parent:         rootWUName,
				WorkUnitId:     "attempt1:u-mod",
				Kind:           "TF_MODULE",
				State:          pb.WorkUnit_SUCCEEDED,
				ModuleId:       &pb.ModuleIdentifier{ModuleName: "FlakyMod"},
				ModuleShardKey: "0",
			}

			// 2. Healthy 0-test SUCCEEDED module in an invocation where another module failed and set RootInvocation.SummaryMarkdown.
			zeroTestMod := &pb.WorkUnit{
				Name:       "rootInvocations/ants-i14600010609614895/workUnits/zero:u-mod",
				Parent:     rootWUName,
				WorkUnitId: "zero:u-mod",
				Kind:       "TF_MODULE",
				State:      pb.WorkUnit_SUCCEEDED,
				ModuleId:   &pb.ModuleIdentifier{ModuleName: "ZeroTestMod"},
			}

			// 3. 100-shard module where all 100 child TF_TEST_RUN work units failed.
			const numShards = 100
			wuByName := map[string]*pb.WorkUnit{
				attempt0Mod.Name: attempt0Mod,
				attempt0Run.Name: attempt0Run,
				attempt1Mod.Name: attempt1Mod,
				zeroTestMod.Name: zeroTestMod,
			}
			rootChildren := []string{attempt0Mod.Name, attempt1Mod.Name, zeroTestMod.Name}
			for i := 0; i < numShards; i++ {
				mName := fmt.Sprintf("rootInvocations/ants-i14600010609614895/workUnits/crash-%d:u-mod", i)
				rName := fmt.Sprintf("rootInvocations/ants-i14600010609614895/workUnits/crash-%d:u-run", i)
				rootChildren = append(rootChildren, mName)
				wuByName[mName] = &pb.WorkUnit{
					Name:           mName,
					Parent:         rootWUName,
					WorkUnitId:     fmt.Sprintf("crash-%d:u-mod", i),
					Kind:           "TF_MODULE",
					State:          pb.WorkUnit_SUCCEEDED,
					ModuleId:       &pb.ModuleIdentifier{ModuleName: "HundredCrashMod"},
					ModuleShardKey: fmt.Sprintf("%d", i),
					ChildWorkUnits: []string{rName},
				}
				wuByName[rName] = &pb.WorkUnit{
					Name:            rName,
					Parent:          mName,
					WorkUnitId:      fmt.Sprintf("crash-%d:u-run", i),
					Kind:            "TF_TEST_RUN",
					State:           pb.WorkUnit_FAILED,
					SummaryMarkdown: fmt.Sprintf("Shard %d APK install failed", i),
				}
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("Attempt 0 full crash trace"))
			}))
			defer srv.Close()

			var childRunListArtifactCalls atomic.Int32
			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:            in.Name,
						State:           pb.RootInvocation_FAILED,
						SummaryMarkdown: "OtherModule failed during device setup",
						Tags:            []*pb.StringPair{{Key: "result_link", Value: atiURL}},
					}, nil
				},
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					filter := in.GetPredicate().GetContentsFilter()
					switch {
					case strings.Contains(filter, `"FlakyMod"`):
						return &pb.QueryTestAggregationsResponse{
							Aggregations: []*pb.TestAggregation{{
								Id:                 &pb.TestIdentifierPrefix{Id: &pb.TestIdentifier{ModuleName: "FlakyMod"}},
								ModuleStatus:       pb.TestAggregation_SUCCEEDED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Passed: 9, Flaky: 1},
							}},
						}, nil
					case strings.Contains(filter, `"ZeroTestMod"`):
						return &pb.QueryTestAggregationsResponse{
							Aggregations: []*pb.TestAggregation{{
								Id:                 &pb.TestIdentifierPrefix{Id: &pb.TestIdentifier{ModuleName: "ZeroTestMod"}},
								ModuleStatus:       pb.TestAggregation_SUCCEEDED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
							}},
						}, nil
					default:
						return &pb.QueryTestAggregationsResponse{
							Aggregations: []*pb.TestAggregation{{
								Id:                 &pb.TestIdentifierPrefix{Id: &pb.TestIdentifier{ModuleName: "HundredCrashMod"}},
								ModuleStatus:       pb.TestAggregation_SUCCEEDED,
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
							}},
						}, nil
					}
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == rootWUName {
						return &pb.WorkUnit{
							Name:            rootWUName,
							WorkUnitId:      "root",
							State:           pb.WorkUnit_FAILED,
							SummaryMarkdown: atiURL,
							ChildWorkUnits:  rootChildren,
						}, nil
					}
					return wuByName[in.Name], nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					var res []*pb.WorkUnit
					for _, n := range in.Names {
						if w := wuByName[n]; w != nil {
							res = append(res, w)
						}
					}
					return &pb.BatchGetWorkUnitsResponse{WorkUnits: res}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == attempt0Run.Name {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{{
								Name:       attempt0Run.Name + "/artifacts/testRunFailed_full_summary_markdown",
								ArtifactId: "testRunFailed_full_summary_markdown",
								FetchUrl:   srv.URL + "/full",
								SizeBytes:  26,
							}},
						}, nil
					}
					if strings.HasSuffix(in.Parent, ":u-run") {
						childRunListArtifactCalls.Add(1)
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			// FlakyMod: Attempt 0 failed at TF_TEST_RUN (with FullSummaryText replacing generic TF_MODULE summary)
			// and Attempt 1 succeeded on ShardKey "0" -> upgraded to FLAKY, and JSON omits internal ChildFailures.
			flakyDetails, err := FetchModuleDetails(context.Background(), client, srv.Client(), "ants-i14600010609614895", "FlakyMod")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, flakyDetails.ModuleStatus, should.Equal("FLAKY"))
			assert.Loosely(t, flakyDetails.WorkUnits[0].Summary, should.Equal("Attempt 0 full crash trace"))
			assert.Loosely(t, flakyDetails.WorkUnits[0].Error, should.NotBeNil)
			assert.Loosely(t, flakyDetails.WorkUnits[0].Error.RawSummary, should.Equal("Attempt 0 full crash trace"))
			jsonBytes, err := json.Marshal(flakyDetails)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(jsonBytes), should.NotContainSubstring("ChildFailures"))
			assert.Loosely(t, string(jsonBytes), should.NotContainSubstring("child_failures"))

			// ZeroTestMod: 0 tests ran on a healthy SUCCEEDED module ignores cross-module RootInvocation summary.
			zeroDetails, err := FetchModuleDetails(context.Background(), client, srv.Client(), "ants-i14600010609614895", "ZeroTestMod")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, zeroDetails.ModuleStatus, should.Equal("SUCCEEDED"))
			assert.Loosely(t, zeroDetails.WorkUnits[0].Error, should.BeNil)

			// HundredCrashMod: all 100 shards promoted to FAILED, while child ListArtifacts RPCs are capped to 20.
			crashDetails, err := FetchModuleDetails(context.Background(), client, srv.Client(), "ants-i14600010609614895", "HundredCrashMod")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, crashDetails.ModuleStatus, should.Equal("FAILED"))
			assert.Loosely(t, childRunListArtifactCalls.Load(), should.Equal(int32(maxDisplayedModuleWorkUnits)))
			assert.Loosely(t, crashDetails.WorkUnits[99].State, should.Equal("FAILED"))
			assert.Loosely(t, crashDetails.WorkUnits[99].Summary, should.Equal("Shard 99 APK install failed"))
		})
	})
}
