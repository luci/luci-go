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

package base

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockTargetsResultDBClient struct {
	rdbpb.ResultDBClient
	getRootInvocation func(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error)
	getInvocation     func(ctx context.Context, in *rdbpb.GetInvocationRequest, opts ...grpc.CallOption) (*rdbpb.Invocation, error)
	getWorkUnit       func(ctx context.Context, in *rdbpb.GetWorkUnitRequest, opts ...grpc.CallOption) (*rdbpb.WorkUnit, error)
	batchGetWorkUnits func(ctx context.Context, in *rdbpb.BatchGetWorkUnitsRequest, opts ...grpc.CallOption) (*rdbpb.BatchGetWorkUnitsResponse, error)
	queryTestVerdicts func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestVerdictsResponse, error)
}

func (m *mockTargetsResultDBClient) GetRootInvocation(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in, opts...)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockTargetsResultDBClient) GetInvocation(ctx context.Context, in *rdbpb.GetInvocationRequest, opts ...grpc.CallOption) (*rdbpb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in, opts...)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockTargetsResultDBClient) GetWorkUnit(ctx context.Context, in *rdbpb.GetWorkUnitRequest, opts ...grpc.CallOption) (*rdbpb.WorkUnit, error) {
	if m.getWorkUnit != nil {
		return m.getWorkUnit(ctx, in, opts...)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockTargetsResultDBClient) BatchGetWorkUnits(ctx context.Context, in *rdbpb.BatchGetWorkUnitsRequest, opts ...grpc.CallOption) (*rdbpb.BatchGetWorkUnitsResponse, error) {
	if m.batchGetWorkUnits != nil {
		return m.batchGetWorkUnits(ctx, in, opts...)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockTargetsResultDBClient) QueryTestVerdicts(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestVerdictsResponse, error) {
	if m.queryTestVerdicts != nil {
		return m.queryTestVerdicts(ctx, in, opts...)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func TestTargets(t *testing.T) {
	ftt.Run("Targets parsing and formatting", t, func(t *ftt.Test) {
		t.Run("TrimResourceURL", func(t *ftt.Test) {
			assert.Loosely(t, TrimResourceURL("https://ci.chromium.org/ui/test-investigate/invocations/build-123/tests/test1?result=r1#top"), should.Equal("invocations/build-123/tests/test1"))
			assert.Loosely(t, TrimResourceURL("https://ci.chromium.org/ui/test-investigate/rootInvocations/ants-123/workUnits/wu-456"), should.Equal("rootInvocations/ants-123/workUnits/wu-456"))
		})

		t.Run("FormatTestResultResourceName", func(t *ftt.Test) {
			assert.Loosely(t, FormatTestResultResourceName("build-123", "test/one", "res1"), should.Equal("invocations/build-123/tests/test%2Fone/results/res1"))
			assert.Loosely(t, FormatTestResultResourceName("8673802696052024673", "test/one", "res1"), should.Equal("invocations/build-8673802696052024673/tests/test%2Fone/results/res1"))
		})

		t.Run("FormatWorkUnitResourceName", func(t *ftt.Test) {
			assert.Loosely(t, FormatWorkUnitResourceName("ants-123", "wu-456"), should.Equal("rootInvocations/ants-123/workUnits/wu-456"))
			assert.Loosely(t, FormatWorkUnitResourceName("8673802696052024673", "wu-456"), should.Equal("rootInvocations/build-8673802696052024673/workUnits/wu-456"))
		})

		t.Run("ExtractTestResultComponents", func(t *ftt.Test) {
			inv, testID, resID := ExtractTestResultComponents("rootInvocations/ants-i123/workUnits/wu-1/tests/:MTS!junit:Test#case/results/res-01")
			assert.Loosely(t, inv, should.Equal("ants-i123"))
			assert.Loosely(t, testID, should.Equal(":MTS!junit:Test#case"))
			assert.Loosely(t, resID, should.Equal("res-01"))

			inv2, testID2, resID2 := ExtractTestResultComponents("invocations/build-123/tests/test_foo/results/0")
			assert.Loosely(t, inv2, should.Equal("build-123"))
			assert.Loosely(t, testID2, should.Equal("test_foo"))
			assert.Loosely(t, resID2, should.Equal("0"))
		})

		t.Run("NormalizeInvocation", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeInvocation("b8676886509240051393"), should.Equal("build-8676886509240051393"))
			assert.Loosely(t, NormalizeInvocation("8676886509240051393"), should.Equal("build-8676886509240051393"))
			assert.Loosely(t, NormalizeInvocation("I77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("i77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("ants-I77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("invocations/build-123"), should.Equal("build-123"))
			assert.Loosely(t, NormalizeInvocation("rootInvocations/build-123"), should.Equal("build-123"))
		})

		t.Run("NormalizeWorkUnit", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeWorkUnit("WU17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("wu17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("workUnits/wu-1"), should.Equal("wu-1"))
			assert.Loosely(t, NormalizeWorkUnit("WU50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"), should.Equal("ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"))
			assert.Loosely(t, NormalizeWorkUnit("ants-wu78200270526224282-attempt__0:u-f5bacc67-4d7a-428d-a417-be4fe61e05a8"), should.Equal("ants-wu78200270526224282-attempt__0:u-f5bacc67-4d7a-428d-a417-be4fe61e05a8"))
			assert.Loosely(t, NormalizeWorkUnit("WU78200270526224282-attempt__0"), should.Equal("ants-wu78200270526224282-attempt__0"))
			assert.Loosely(t, NormalizeWorkUnit("ANTS-WU123-ATTEMPT__0:U-ABC"), should.Equal("ants-wu123-attempt__0:u-abc"))
			assert.Loosely(t, NormalizeWorkUnit("workUnits/WU123:u-abc"), should.Equal("ants-wu123:u-abc"))
		})

		t.Run("ResolveInvocation", func(t *ftt.Test) {
			ctx := context.Background()

			t.Run("Queries both GetRootInvocation and GetInvocation in parallel and returns RootInvocation when found", func(t *ftt.Test) {
				expectedRoot := &rdbpb.RootInvocation{Name: "rootInvocations/ants-i77100010600769898", SummaryMarkdown: "root summary"}
				client := &mockTargetsResultDBClient{
					getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
						return expectedRoot, nil
					},
					getInvocation: func(ctx context.Context, in *rdbpb.GetInvocationRequest, opts ...grpc.CallOption) (*rdbpb.Invocation, error) {
						return nil, status.Error(codes.NotFound, "not found")
					},
				}
				resolved, err := ResolveInvocation(ctx, client, "I77100010600769898")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resolved.ID, should.Equal("ants-i77100010600769898"))
				assert.Loosely(t, resolved.Legacy, should.BeFalse)
				assert.Loosely(t, resolved.RootInvocation, should.Equal(expectedRoot))
			})

			t.Run("Resolves legacy Invocation when GetRootInvocation returns NotFound", func(t *ftt.Test) {
				expectedLegacy := &rdbpb.Invocation{Name: "invocations/build-8697111068122866129"}
				client := &mockTargetsResultDBClient{
					getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
						return nil, status.Error(codes.NotFound, "root not found")
					},
					getInvocation: func(ctx context.Context, in *rdbpb.GetInvocationRequest, opts ...grpc.CallOption) (*rdbpb.Invocation, error) {
						return expectedLegacy, nil
					},
				}
				resolved, err := ResolveInvocation(ctx, client, "8697111068122866129")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resolved.ID, should.Equal("build-8697111068122866129"))
				assert.Loosely(t, resolved.Legacy, should.BeTrue)
				assert.Loosely(t, resolved.Invocation, should.Equal(expectedLegacy))
			})

			t.Run("PermissionDenied on GetRootInvocation fails immediately", func(t *ftt.Test) {
				client := &mockTargetsResultDBClient{
					getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
						return nil, status.Error(codes.PermissionDenied, "caller does not have permission resultdb.rootInvocations.get")
					},
					getInvocation: func(ctx context.Context, in *rdbpb.GetInvocationRequest, opts ...grpc.CallOption) (*rdbpb.Invocation, error) {
						return nil, status.Error(codes.NotFound, "not found")
					},
				}
				_, err := ResolveInvocation(ctx, client, "build-123")
				assert.Loosely(t, err, should.ErrLike("missing ResultDB realm permission"))
			})

			t.Run("PermissionDenied on GetInvocation fails immediately", func(t *ftt.Test) {
				client := &mockTargetsResultDBClient{
					getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest, opts ...grpc.CallOption) (*rdbpb.RootInvocation, error) {
						return nil, status.Error(codes.NotFound, "not found")
					},
					getInvocation: func(ctx context.Context, in *rdbpb.GetInvocationRequest, opts ...grpc.CallOption) (*rdbpb.Invocation, error) {
						return nil, status.Error(codes.PermissionDenied, "caller does not have permission resultdb.invocations.get")
					},
				}
				_, err := ResolveInvocation(ctx, client, "build-123")
				assert.Loosely(t, err, should.ErrLike("missing ResultDB realm permission"))
			})
		})

		t.Run("ExtractArtifactMetadata and FormatFlags", func(t *ftt.Test) {
			t.Run("root invocation work unit artifact", func(t *ftt.Test) {
				meta := ExtractArtifactMetadata("rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/bugreport.zip")
				assert.Loosely(t, meta, should.Equal(ArtifactMetadata{
					InvocationID: "ants-i05300010637202289",
					WorkUnitID:   "ants-wu56200010637202289",
					ArtifactID:   "bugreport.zip",
				}))
				assert.Loosely(t, meta.FormatFlags(), should.Equal("-workunitid=ants-wu56200010637202289"))
			})

			t.Run("root invocation test result artifact with special chars in test ID", func(t *ftt.Test) {
				meta := ExtractArtifactMetadata("rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202290/tests/com.android.FooTest%23testBar/results/0/artifacts/logcat.txt")
				assert.Loosely(t, meta, should.Equal(ArtifactMetadata{
					InvocationID: "ants-i05300010637202289",
					WorkUnitID:   "ants-wu56200010637202290",
					TestID:       "com.android.FooTest#testBar",
					ResultID:     "0",
					ArtifactID:   "logcat.txt",
				}))
				assert.Loosely(t, meta.FormatFlags(), should.Equal(`-workunitid=ants-wu56200010637202290 -testid='com.android.FooTest#testBar' -resultid=0`))
			})

			t.Run("resource IDs matching collection keywords are not confused with keys", func(t *ftt.Test) {
				meta := ExtractArtifactMetadata("rootInvocations/ants-i123/workUnits/tests/tests/artifacts/results/workUnits/artifacts/dir%2Flog.txt")
				assert.Loosely(t, meta, should.Equal(ArtifactMetadata{
					InvocationID: "ants-i123",
					WorkUnitID:   "tests",
					TestID:       "artifacts",
					ResultID:     "workUnits",
					ArtifactID:   "dir/log.txt",
				}))
			})
		})
	})
}

func TestShellQuoteIfNeeded(t *testing.T) {
	t.Parallel()

	ftt.Run("ShellQuoteIfNeeded", t, func(t *ftt.Test) {
		assert.Loosely(t, ShellQuoteIfNeeded(""), should.Equal("''"))
		assert.Loosely(t, ShellQuoteIfNeeded("ants-wu56200010637202289"), should.Equal("ants-wu56200010637202289"))
		assert.Loosely(t, ShellQuoteIfNeeded("dir_a/log.txt"), should.Equal("dir_a/log.txt"))
		assert.Loosely(t, ShellQuoteIfNeeded("com.android.FooTest$Inner#testBar"), should.Equal(`'com.android.FooTest$Inner#testBar'`))
		assert.Loosely(t, ShellQuoteIfNeeded(":CellBroadcastServiceTests!junit:com.android.Foo$Inner#testBar"), should.Equal(`':CellBroadcastServiceTests!junit:com.android.Foo$Inner#testBar'`))
		assert.Loosely(t, ShellQuoteIfNeeded("test'with'quotes"), should.Equal(`'test'\''with'\''quotes'`))
	})
}
