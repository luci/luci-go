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

package artifact

import (
	"bytes"
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockResultDBClient struct {
	rdbpb.ResultDBClient
	getRootInvocation     func(ctx context.Context, in *rdbpb.GetRootInvocationRequest) (*rdbpb.RootInvocation, error)
	getInvocation         func(ctx context.Context, in *rdbpb.GetInvocationRequest) (*rdbpb.Invocation, error)
	listArtifacts         func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error)
	queryArtifacts        func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error)
	getArtifact           func(ctx context.Context, in *rdbpb.GetArtifactRequest) (*rdbpb.Artifact, error)
	queryWorkUnits        func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest) (*rdbpb.QueryWorkUnitsResponse, error)
	getWorkUnit           func(ctx context.Context, in *rdbpb.GetWorkUnitRequest) (*rdbpb.WorkUnit, error)
	queryTestVerdicts     func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest) (*rdbpb.QueryTestVerdictsResponse, error)
	queryTestResults      func(ctx context.Context, in *rdbpb.QueryTestResultsRequest) (*rdbpb.QueryTestResultsResponse, error)
	queryTestExonerations func(ctx context.Context, in *rdbpb.QueryTestExonerationsRequest) (*rdbpb.QueryTestExonerationsResponse, error)
}

func (m *mockResultDBClient) GetRootInvocation(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in)
	}
	return &rdbpb.RootInvocation{Name: in.Name}, nil
}

func (m *mockResultDBClient) GetInvocation(ctx context.Context, in *rdbpb.GetInvocationRequest, opts ...grpc.CallOption) (*rdbpb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in)
	}
	return &rdbpb.Invocation{Name: in.Name}, nil
}

func (m *mockResultDBClient) ListArtifacts(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
	if m.listArtifacts != nil {
		return m.listArtifacts(ctx, in)
	}
	return &rdbpb.ListArtifactsResponse{}, nil
}

func (m *mockResultDBClient) QueryArtifacts(ctx context.Context, in *rdbpb.QueryArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.QueryArtifactsResponse, error) {
	if m.queryArtifacts != nil {
		return m.queryArtifacts(ctx, in)
	}
	return &rdbpb.QueryArtifactsResponse{}, nil
}

func (m *mockResultDBClient) GetArtifact(ctx context.Context, in *rdbpb.GetArtifactRequest, opts ...grpc.CallOption) (*rdbpb.Artifact, error) {
	if m.getArtifact != nil {
		return m.getArtifact(ctx, in)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockResultDBClient) QueryWorkUnits(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*rdbpb.QueryWorkUnitsResponse, error) {
	if m.queryWorkUnits != nil {
		return m.queryWorkUnits(ctx, in)
	}
	return &rdbpb.QueryWorkUnitsResponse{}, nil
}

func (m *mockResultDBClient) GetWorkUnit(ctx context.Context, in *rdbpb.GetWorkUnitRequest, opts ...grpc.CallOption) (*rdbpb.WorkUnit, error) {
	if m.getWorkUnit != nil {
		return m.getWorkUnit(ctx, in)
	}
	return nil, errors.New("not implemented")
}

func (m *mockResultDBClient) QueryTestVerdicts(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestVerdictsResponse, error) {
	if m.queryTestVerdicts != nil {
		return m.queryTestVerdicts(ctx, in)
	}
	return &rdbpb.QueryTestVerdictsResponse{}, nil
}

func (m *mockResultDBClient) QueryTestResults(ctx context.Context, in *rdbpb.QueryTestResultsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestResultsResponse, error) {
	if m.queryTestResults != nil {
		return m.queryTestResults(ctx, in)
	}
	return &rdbpb.QueryTestResultsResponse{}, nil
}

func (m *mockResultDBClient) QueryTestExonerations(ctx context.Context, in *rdbpb.QueryTestExonerationsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestExonerationsResponse, error) {
	if m.queryTestExonerations != nil {
		return m.queryTestExonerations(ctx, in)
	}
	return &rdbpb.QueryTestExonerationsResponse{}, nil
}

func TestListCmd(t *testing.T) {
	t.Parallel()

	ftt.Run(`ListCmd`, t, func(t *ftt.Test) {
		trCmd := ListCmd(nil, ParentTypeTestResult)
		assert.Loosely(t, trCmd, should.NotBeNil)
		assert.Loosely(t, trCmd.ShortDesc, should.Equal("List artifacts for a test result"))
		trRun := trCmd.CommandRun().(*artifactListRun)
		assert.Loosely(t, trRun.maxArtifacts, should.Equal(DefaultMaxArtifacts))
		assert.Loosely(t, trRun.Flags.Lookup("filter"), should.NotBeNil)
		assert.Loosely(t, trRun.Flags.Lookup("glob"), should.NotBeNil)

		wuCmd := ListCmd(nil, ParentTypeWorkUnit)
		assert.Loosely(t, wuCmd, should.NotBeNil)
		assert.Loosely(t, wuCmd.ShortDesc, should.Equal("List artifacts for a work unit"))
		wuRun := wuCmd.CommandRun().(*artifactListRun)
		assert.Loosely(t, wuRun.maxArtifacts, should.Equal(DefaultMaxArtifacts))
		assert.Loosely(t, wuRun.Flags.Lookup("filter"), should.NotBeNil)
		assert.Loosely(t, wuRun.Flags.Lookup("glob"), should.NotBeNil)

		unknownCmd := ListCmd(nil, ParentTypeUnknown)
		assert.Loosely(t, unknownCmd, should.NotBeNil)
		assert.Loosely(t, unknownCmd.ShortDesc, should.Equal("List artifacts for a work unit or test result"))
		unknownRun := unknownCmd.CommandRun().(*artifactListRun)
		assert.Loosely(t, unknownRun.maxArtifacts, should.Equal(DefaultMaxArtifacts))
		assert.Loosely(t, unknownRun.Flags.Lookup("invocationid"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("workunitid"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("testid"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("resultid"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("legacy"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("filter"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("glob"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("max-artifacts"), should.NotBeNil)

		t.Run(`fails immediately on PermissionDenied`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest) (*rdbpb.RootInvocation, error) {
					return nil, status.Error(codes.PermissionDenied, "caller does not have permission")
				},
			}
			run := &artifactListRun{
				artifactCommonRun: artifactCommonRun{
					client:       client,
					parentType:   ParentTypeInvocation,
					invocationID: "build-123",
				},
			}
			assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(1))
		})

		t.Run(`rejects mutually exclusive -filter and -glob`, func(t *ftt.Test) {
			runBoth := &artifactListRun{
				artifactCommonRun: artifactCommonRun{
					parentType:   ParentTypeInvocation,
					invocationID: "build-123",
				},
				filter:       "logcat",
				glob:         "*.txt",
				maxArtifacts: DefaultMaxArtifacts,
			}
			assert.Loosely(t, runBoth.Run(nil, nil, nil), should.Equal(1))
		})

		t.Run(`rejects negative -max-artifacts and invalid -filter regex`, func(t *ftt.Test) {
			runNeg := &artifactListRun{
				artifactCommonRun: artifactCommonRun{
					parentType:   ParentTypeInvocation,
					invocationID: "build-123",
				},
				maxArtifacts: -1,
			}
			assert.Loosely(t, runNeg.Run(nil, nil, nil), should.Equal(1))

			runBadRegex := &artifactListRun{
				artifactCommonRun: artifactCommonRun{
					parentType:   ParentTypeInvocation,
					invocationID: "build-123",
				},
				filter:       "[unclosed",
				maxArtifacts: DefaultMaxArtifacts,
			}
			assert.Loosely(t, runBadRegex.Run(nil, nil, nil), should.Equal(1))
		})

		t.Run(`lists invocation-wide artifacts on root invocation via QueryArtifacts including bare I<digits> and i<digits>`, func(t *ftt.Test) {
			for _, rawInv := range []string{"ants-i05300010637202289", "I05300010637202289", "i05300010637202289"} {
				var gotParent string
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						gotParent = in.Parent
						assert.Loosely(t, in.ReadMask.GetPaths(), should.Resemble([]string{
							"name", "artifact_id", "content_type", "artifact_type", "size_bytes", "has_lines",
						}))
						return &rdbpb.QueryArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{
									Name:         "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/bugreport.zip",
									ArtifactId:   "bugreport.zip",
									SizeBytes:    1024,
									ArtifactType: "BUGREPORTZ",
								},
								{
									Name:         "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202290/tests/com.android.FooTest%23testBar/results/0/artifacts/logcat.txt",
									ArtifactId:   "logcat.txt",
									SizeBytes:    512,
									ArtifactType: "LOGCAT",
								},
							},
						}, nil
					},
				}
				run := &artifactListRun{
					artifactCommonRun: artifactCommonRun{
						client:       client,
						parentType:   ParentTypeUnknown,
						invocationID: rawInv,
					},
					maxArtifacts: DefaultMaxArtifacts,
				}
				assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(0))
				assert.Loosely(t, gotParent, should.Equal("rootInvocations/ants-i05300010637202289"))
			}
		})

		t.Run(`filters invocation-wide, work-unit, and test-result artifacts with -glob and -filter`, func(t *ftt.Test) {
			// 1. Invocation-wide passes Predicate.ArtifactIdRegexp to QueryArtifacts for -glob.
			var gotRegexp string
			invClient := &mockResultDBClient{
				queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
					gotRegexp = in.GetPredicate().GetArtifactIdRegexp()
					return &rdbpb.QueryArtifactsResponse{
						Artifacts: []*rdbpb.Artifact{
							{
								Name:       "rootInvocations/ants-i123/workUnits/wu-1/tests/t1/results/0/artifacts/stub/foo-logcat-on-failure_123.txt",
								ArtifactId: "stub/foo-logcat-on-failure_123.txt",
							},
						},
					}, nil
				},
			}
			invRun := &artifactListRun{
				artifactCommonRun: artifactCommonRun{
					client:       invClient,
					parentType:   ParentTypeUnknown,
					invocationID: "I123",
				},
				glob:         "*-logcat-on-failure_*.txt",
				maxArtifacts: DefaultMaxArtifacts,
			}
			assert.Loosely(t, invRun.Run(nil, nil, nil), should.Equal(0))
			assert.Loosely(t, gotRegexp, should.Equal(`(?i).*(?:-logcat-on-failure_.*\.txt)`))

			// 2. Work unit filters ListArtifacts results client-side with -filter before maxArtifacts truncation.
			wuClient := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					return &rdbpb.ListArtifactsResponse{
						Artifacts: []*rdbpb.Artifact{
							{ArtifactId: "stdout.txt", Name: in.Parent + "/artifacts/stdout.txt"},
							{ArtifactId: "bugreport_1.zip", Name: in.Parent + "/artifacts/bugreport_1.zip"},
							{ArtifactId: "HOST_LOG.txt", Name: in.Parent + "/artifacts/HOST_LOG.txt"},
						},
					}, nil
				},
			}
			wuRun := &artifactListRun{
				artifactCommonRun: artifactCommonRun{
					client:       wuClient,
					parentType:   ParentTypeWorkUnit,
					invocationID: "ants-i123",
					workUnitID:   "wu-1",
				},
				filter:       "bugreport|host_log",
				maxArtifacts: DefaultMaxArtifacts,
			}
			assert.Loosely(t, wuRun.Run(nil, nil, nil), should.Equal(0))

			// 3. Test result filters ListArtifacts results client-side with -filter.
			trClient := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					return &rdbpb.ListArtifactsResponse{
						Artifacts: []*rdbpb.Artifact{
							{ArtifactId: "screenshot.png", Name: in.Parent + "/artifacts/screenshot.png"},
							{ArtifactId: "test_logcat.txt", Name: in.Parent + "/artifacts/test_logcat.txt"},
						},
					}, nil
				},
			}
			trRun := &artifactListRun{
				artifactCommonRun: artifactCommonRun{
					client:       trClient,
					parentType:   ParentTypeTestResult,
					invocationID: "ants-i123",
					workUnitID:   "wu-1",
					testID:       "t1",
					resultID:     "0",
				},
				filter:       "LOGCAT",
				maxArtifacts: DefaultMaxArtifacts,
			}
			assert.Loosely(t, trRun.Run(nil, nil, nil), should.Equal(0))
		})
	})
}

func TestQueryInvocationArtifacts(t *testing.T) {
	t.Parallel()

	ftt.Run(`QueryInvocationArtifacts`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`multi-page pagination for root invocation excludes fetch_url in ReadMask`, func(t *ftt.Test) {
			calls := 0
			client := &mockResultDBClient{
				queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
					calls++
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i123"))
					assert.Loosely(t, in.Invocations, should.BeEmpty)
					assert.Loosely(t, in.ReadMask.GetPaths(), should.Resemble([]string{
						"name", "artifact_id", "content_type", "artifact_type", "size_bytes", "has_lines",
					}))
					if in.PageToken == "" {
						assert.Loosely(t, in.PageSize, should.Equal(int32(maxQueryArtifactsPageSize)))
						return &rdbpb.QueryArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "art-1", Name: "rootInvocations/ants-i123/workUnits/wu-1/artifacts/art-1"},
							},
							NextPageToken: "page-2",
						}, nil
					}
					assert.Loosely(t, in.PageSize, should.Equal(int32(maxQueryArtifactsPageSize-1)))
					return &rdbpb.QueryArtifactsResponse{
						Artifacts: []*rdbpb.Artifact{
							{ArtifactId: "art-2", Name: "rootInvocations/ants-i123/workUnits/wu-2/tests/t1/results/0/artifacts/art-2"},
						},
					}, nil
				},
			}
			arts, hasMore, err := QueryInvocationArtifacts(ctx, client, "rootInvocations/ants-i123", "", DefaultMaxArtifacts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, calls, should.Equal(2))
			assert.Loosely(t, len(arts), should.Equal(2))
			// Test-result artifact (art-2) sorts before work-unit artifact (art-1) within the same tier.
			assert.Loosely(t, arts[0].ArtifactId, should.Equal("art-2"))
			assert.Loosely(t, arts[1].ArtifactId, should.Equal("art-1"))
		})

		t.Run(`sorts before truncating to maxArtifacts and sets hasMore when limit is exceeded`, func(t *ftt.Test) {
			calls := 0
			client := &mockResultDBClient{
				queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
					calls++
					assert.Loosely(t, in.PageSize, should.Equal(int32(maxQueryArtifactsPageSize)))
					return &rdbpb.QueryArtifactsResponse{
						Artifacts: []*rdbpb.Artifact{
							{ArtifactId: "injectables.txt", Name: "rootInvocations/ants-i123/workUnits/wu-1/artifacts/injectables.txt", ArtifactType: "TEXT"},
							{ArtifactId: "bugreport.zip", Name: "rootInvocations/ants-i123/workUnits/wu-1/artifacts/bugreport.zip", ArtifactType: "BUGREPORTZ"},
							{ArtifactId: "logcat.txt", Name: "rootInvocations/ants-i123/workUnits/wu-1/artifacts/logcat.txt", ArtifactType: "LOGCAT"},
						},
					}, nil
				},
			}
			arts, hasMore, err := QueryInvocationArtifacts(ctx, client, "rootInvocations/ants-i123", "", 2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeTrue)
			assert.Loosely(t, calls, should.Equal(1))
			assert.Loosely(t, len(arts), should.Equal(2))
			assert.Loosely(t, arts[0].ArtifactId, should.Equal("logcat.txt"))
			assert.Loosely(t, arts[1].ArtifactId, should.Equal("bugreport.zip"))
		})

		t.Run(`artifactIDRegexp sets Predicate.ArtifactIdRegexp`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
					assert.Loosely(t, in.GetPredicate().GetArtifactIdRegexp(), should.Equal(`(?i).*(?:ants-publish).*`))
					return &rdbpb.QueryArtifactsResponse{
						Artifacts: []*rdbpb.Artifact{
							{ArtifactId: "injectables-191970884/ants-publish.json", Name: "rootInvocations/ants-i123/workUnits/wu-1/artifacts/injectables-191970884%2Fants-publish.json"},
						},
					}, nil
				},
			}
			serverRegexp, _, err := CompileArtifactFilter("ants-publish")
			assert.Loosely(t, err, should.BeNil)
			arts, hasMore, err := QueryInvocationArtifacts(ctx, client, "rootInvocations/ants-i123", serverRegexp, DefaultMaxArtifacts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, len(arts), should.Equal(1))
		})
	})
}

func TestQueryAllArtifacts(t *testing.T) {
	t.Parallel()

	ftt.Run(`QueryAllArtifacts`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`empty`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}
			arts, err := QueryAllArtifacts(ctx, client, "parent")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(arts), should.BeZero)
		})

		t.Run(`multi-page pagination`, func(t *ftt.Test) {
			calls := 0
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					calls++
					if in.PageToken == "" {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "art-1", Name: "parent/artifacts/art-1"},
								{ArtifactId: "art-2", Name: "parent/artifacts/art-2"},
							},
							NextPageToken: "token-page-2",
						}, nil
					}
					if in.PageToken == "token-page-2" {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "art-3", Name: "parent/artifacts/art-3"},
							},
							NextPageToken: "",
						}, nil
					}
					return nil, errors.New("unexpected token")
				},
			}
			arts, err := QueryAllArtifacts(ctx, client, "parent")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(arts), should.Equal(3))
			assert.Loosely(t, calls, should.Equal(2))
			assert.Loosely(t, arts[0].ArtifactId, should.Equal("art-1"))
			assert.Loosely(t, arts[1].ArtifactId, should.Equal("art-2"))
			assert.Loosely(t, arts[2].ArtifactId, should.Equal("art-3"))
		})

		t.Run(`error propagation`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					return nil, errors.New("permission denied")
				},
			}
			_, err := QueryAllArtifacts(ctx, client, "parent")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("permission denied"))
		})
	})
}

func TestQueryAncestorWorkUnits(t *testing.T) {
	t.Parallel()

	ftt.Run(`queryAncestorWorkUnits`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`successful query with AncestorsOf`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest) (*rdbpb.QueryWorkUnitsResponse, error) {
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/build-123"))
					assert.Loosely(t, in.Predicate.AncestorsOf, should.Equal("rootInvocations/build-123/workUnits/step-1"))
					return &rdbpb.QueryWorkUnitsResponse{
						WorkUnits: []*rdbpb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/root"},
							{Name: "rootInvocations/build-123/workUnits/parent-step"},
						},
					}, nil
				},
			}

			ancestors := format.QueryAncestorWorkUnits(ctx, client, "rootInvocations/build-123/workUnits/step-1")
			assert.Loosely(t, len(ancestors), should.Equal(2))
			assert.Loosely(t, ancestors[0].Name, should.Equal("rootInvocations/build-123/workUnits/root"))
			assert.Loosely(t, ancestors[1].Name, should.Equal("rootInvocations/build-123/workUnits/parent-step"))
		})

		t.Run(`query error returns nil`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest) (*rdbpb.QueryWorkUnitsResponse, error) {
					return nil, errors.New("QueryWorkUnits error")
				},
			}

			ancestors := format.QueryAncestorWorkUnits(ctx, client, "rootInvocations/build-123/workUnits/child")
			assert.Loosely(t, len(ancestors), should.BeZero)
		})

		t.Run(`QueryWorkUnits with zero ancestors returns empty`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest) (*rdbpb.QueryWorkUnitsResponse, error) {
					return &rdbpb.QueryWorkUnitsResponse{WorkUnits: nil}, nil
				},
			}

			ancestors := format.QueryAncestorWorkUnits(ctx, client, "rootInvocations/build-123/workUnits/root")
			assert.Loosely(t, len(ancestors), should.BeZero)
		})

		t.Run(`invalid target work unit format`, func(t *ftt.Test) {
			client := &mockResultDBClient{}
			ancestors := format.QueryAncestorWorkUnits(ctx, client, "invalid")
			assert.Loosely(t, len(ancestors), should.BeZero)
		})
	})
}

func TestPrintAncestorWorkUnitNotices(t *testing.T) {
	t.Parallel()

	ftt.Run(`printAncestorWorkUnitNotices`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`test result with parent and ancestor artifacts`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest) (*rdbpb.QueryWorkUnitsResponse, error) {
					return &rdbpb.QueryWorkUnitsResponse{
						WorkUnits: []*rdbpb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/build-root"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					if in.Parent == "rootInvocations/build-123/workUnits/step-1" {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "stdout"},
								{ArtifactId: "stderr"},
							},
						}, nil
					}
					if in.Parent == "rootInvocations/build-123/workUnits/build-root" {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "build-summary"},
							},
							NextPageToken: "more",
						}, nil
					}
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}

			var buf bytes.Buffer
			PrintAncestorWorkUnitNotices(ctx, client, "rootInvocations/build-123/workUnits/step-1/tests/test1/results/0", ParentTypeTestResult, &buf)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:"))
			assert.Loosely(t, out, should.ContainSubstring("Work unit step-1 contains 2 artifacts. Run 'luci work-unit artifact list -invocationid build-123 -workunitid step-1' to view."))
			assert.Loosely(t, out, should.ContainSubstring("Work unit build-root contains 1+ artifacts. Run 'luci work-unit artifact list -invocationid build-123 -workunitid build-root' to view."))
		})

		t.Run(`work unit only checks ancestors, not target itself`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest) (*rdbpb.QueryWorkUnitsResponse, error) {
					return &rdbpb.QueryWorkUnitsResponse{
						WorkUnits: []*rdbpb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/step-1"}, // target itself
							{Name: "rootInvocations/build-123/workUnits/build-root"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					if in.Parent == "rootInvocations/build-123/workUnits/build-root" {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{{ArtifactId: "root-log"}},
						}, nil
					}
					if in.Parent == "rootInvocations/build-123/workUnits/step-1" {
						t.Fatalf("target work unit itself should not be queried for ancestor notices")
					}
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}

			var buf bytes.Buffer
			PrintAncestorWorkUnitNotices(ctx, client, "rootInvocations/build-123/workUnits/step-1", ParentTypeWorkUnit, &buf)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Work unit build-root contains 1 artifact. Run 'luci work-unit artifact list -invocationid build-123 -workunitid build-root' to view."))
			assert.Loosely(t, out, should.NotContainSubstring("step-1"))
		})

		t.Run(`root work unit notice formatting`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest) (*rdbpb.QueryWorkUnitsResponse, error) {
					return &rdbpb.QueryWorkUnitsResponse{
						WorkUnits: []*rdbpb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/root"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					if in.Parent == "rootInvocations/build-123/workUnits/root" {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{{ArtifactId: "root.log"}},
						}, nil
					}
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}

			var buf bytes.Buffer
			PrintAncestorWorkUnitNotices(ctx, client, "rootInvocations/build-123/workUnits/step-1", ParentTypeWorkUnit, &buf)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Root work unit contains 1 artifact. Run 'luci work-unit artifact list -invocationid build-123 -workunitid root' to view."))
		})

		t.Run(`no notice printed when ancestors have zero artifacts`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest) (*rdbpb.QueryWorkUnitsResponse, error) {
					return &rdbpb.QueryWorkUnitsResponse{
						WorkUnits: []*rdbpb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/parent"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}

			var buf bytes.Buffer
			PrintAncestorWorkUnitNotices(ctx, client, "rootInvocations/build-123/workUnits/step/tests/t/results/0", ParentTypeTestResult, &buf)
			out := buf.String()

			assert.Loosely(t, out, should.Equal(""))
		})
	})
}

func TestPrintImmediateParentInvocationNotice(t *testing.T) {
	t.Parallel()

	ftt.Run(`printImmediateParentInvocationNotice`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`test result with immediate parent invocation artifacts`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					if in.Parent == "invocations/u-root-123" {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "minidump"},
								{ArtifactId: "stderr"},
							},
						}, nil
					}
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}

			var buf bytes.Buffer
			PrintImmediateParentInvocationNotice(ctx, client, "invocations/u-root-123/tests/test1/results/0", &buf)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Invocation Artifacts:"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation u-root-123 contains 2 artifacts. Run 'luci artifact list -invocationid u-root-123' to view."))
		})

		t.Run(`singular artifact count and paginated`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					if in.Parent == "invocations/u-root-1" {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "minidump"},
							},
						}, nil
					}
					if in.Parent == "invocations/u-root-more" {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "minidump"},
							},
							NextPageToken: "page-2",
						}, nil
					}
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}

			var buf bytes.Buffer
			PrintImmediateParentInvocationNotice(ctx, client, "invocations/u-root-1/tests/test1/results/0", &buf)
			assert.Loosely(t, buf.String(), should.ContainSubstring("Invocation u-root-1 contains 1 artifact."))

			buf.Reset()
			PrintImmediateParentInvocationNotice(ctx, client, "invocations/u-root-more/tests/test1/results/0", &buf)
			assert.Loosely(t, buf.String(), should.ContainSubstring("Invocation u-root-more contains 1+ artifacts."))
		})

		t.Run(`no artifacts on parent invocation`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *rdbpb.ListArtifactsRequest) (*rdbpb.ListArtifactsResponse, error) {
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}

			var buf bytes.Buffer
			PrintImmediateParentInvocationNotice(ctx, client, "invocations/u-root-123/tests/test1/results/0", &buf)
			assert.Loosely(t, buf.String(), should.Equal(""))
		})

		t.Run(`invalid target format`, func(t *ftt.Test) {
			client := &mockResultDBClient{}
			var buf bytes.Buffer
			PrintImmediateParentInvocationNotice(ctx, client, "invalid-target", &buf)
			assert.Loosely(t, buf.String(), should.Equal(""))
		})
	})
}

func TestPrintArtifactHeader(t *testing.T) {
	t.Parallel()

	ftt.Run(`printArtifactHeaderTo`, t, func(t *ftt.Test) {
		t.Run(`empty artifacts displays count 0`, func(t *ftt.Test) {
			var buf bytes.Buffer
			displayed := printArtifactHeaderTo(&buf, "Test Result", "test-1/result-0", 0, 100)
			assert.Loosely(t, displayed, should.Equal(0))
			assert.Loosely(t, buf.String(), should.Equal("Test Result Artifacts (test-1/result-0) (0):\n"))
		})

		t.Run(`within limit displays total count`, func(t *ftt.Test) {
			var buf bytes.Buffer
			displayed := printArtifactHeaderTo(&buf, "Invocation", "inv-123", 5, 10)
			assert.Loosely(t, displayed, should.Equal(5))
			assert.Loosely(t, buf.String(), should.Equal("Invocation Artifacts (inv-123) (5):\n"))
		})

		t.Run(`exceeds maxArtifacts displays showing info`, func(t *ftt.Test) {
			var buf bytes.Buffer
			displayed := printArtifactHeaderTo(&buf, "Work Unit", "root/step", 15, 10)
			assert.Loosely(t, displayed, should.Equal(10))
			assert.Loosely(t, buf.String(), should.Equal("Work Unit Artifacts (root/step) (showing 10 of 15, use -all to see all):\n"))
		})

		t.Run(`all artifacts requested with maxArtifacts 0`, func(t *ftt.Test) {
			var buf bytes.Buffer
			displayed := printArtifactHeaderTo(&buf, "Test Result", "test-1/result-0", 15, 0)
			assert.Loosely(t, displayed, should.Equal(15))
			assert.Loosely(t, buf.String(), should.Equal("Test Result Artifacts (test-1/result-0) (15):\n"))
		})
	})
}
