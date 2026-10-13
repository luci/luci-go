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
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockTargetsResultDBClient struct {
	pb.ResultDBClient
	getRootInvocation func(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error)
	getInvocation     func(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error)
}

func (m *mockTargetsResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in, opts...)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockTargetsResultDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in, opts...)
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
		})

		t.Run("ResolveInvocation", func(t *ftt.Test) {
			ctx := context.Background()

			t.Run("Queries both GetRootInvocation and GetInvocation in parallel and returns RootInvocation when found", func(t *ftt.Test) {
				expectedRoot := &pb.RootInvocation{Name: "rootInvocations/ants-i77100010600769898", SummaryMarkdown: "root summary"}
				client := &mockTargetsResultDBClient{
					getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
						return expectedRoot, nil
					},
					getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
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
				expectedLegacy := &pb.Invocation{Name: "invocations/build-8697111068122866129"}
				client := &mockTargetsResultDBClient{
					getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
						return nil, status.Error(codes.NotFound, "root not found")
					},
					getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
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
					getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
						return nil, status.Error(codes.PermissionDenied, "caller does not have permission resultdb.rootInvocations.get")
					},
					getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
						return nil, status.Error(codes.NotFound, "not found")
					},
				}
				_, err := ResolveInvocation(ctx, client, "build-123")
				assert.Loosely(t, err, should.ErrLike("missing ResultDB realm permission"))
			})

			t.Run("PermissionDenied on GetInvocation fails immediately", func(t *ftt.Test) {
				client := &mockTargetsResultDBClient{
					getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
						return nil, status.Error(codes.NotFound, "not found")
					},
					getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
						return nil, status.Error(codes.PermissionDenied, "caller does not have permission resultdb.invocations.get")
					},
				}
				_, err := ResolveInvocation(ctx, client, "build-123")
				assert.Loosely(t, err, should.ErrLike("missing ResultDB realm permission"))
			})
		})
	})
}
