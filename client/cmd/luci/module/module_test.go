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

type mockResultDBClient struct {
	pb.ResultDBClient
	queryTestAggregations func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error)
	getWorkUnit           func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error)
	batchGetWorkUnits     func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error)
	queryWorkUnits        func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error)
	getRootInvocation     func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
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

			// Verify children of targetWU, otherWU, and targetShard2WU were NOT fetched
			assert.Loosely(t, fetchedNames["rootInvocations/inv-tree/workUnits/target-child-1"], should.BeFalse)
			assert.Loosely(t, fetchedNames["rootInvocations/inv-tree/workUnits/other-child-1"], should.BeFalse)
			assert.Loosely(t, fetchedNames["rootInvocations/inv-tree/workUnits/target-shard2-child-1"], should.BeFalse)
		})

		t.Run(`QueryTestAggregations error propagated`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return nil, status.Errorf(codes.PermissionDenied, "access denied")
				},
			}
			details, err := FetchModuleDetails(ctx, client, nil, "inv-1", "MyModule")
			assert.Loosely(t, details, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("failed to query test aggregations"))
			assert.Loosely(t, err, should.ErrLike("access denied"))
		})

		t.Run(`discoverModuleWorkUnits root WU error propagated`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return nil, status.Errorf(codes.PermissionDenied, "permission denied")
				},
			}
			details, err := FetchModuleDetails(ctx, client, nil, "inv-1", "MyModule")
			assert.Loosely(t, details, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("failed to discover module work units"))
			assert.Loosely(t, err, should.ErrLike("failed to get root work unit"))
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
			assert.Loosely(t, err, should.ErrLike("failed to discover module work units"))
			assert.Loosely(t, err, should.ErrLike("failed to batch get work units"))
			assert.Loosely(t, err, should.ErrLike("quota exceeded or network failure"))
		})
	})
}
