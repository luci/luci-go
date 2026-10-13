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

package workunit

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

type mockWorkUnitResultDBClient struct {
	pb.ResultDBClient
	getRootInvocation func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation     func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
	getWorkUnit       func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error)
}

func (m *mockWorkUnitResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in)
	}
	return &pb.RootInvocation{Name: in.Name}, nil
}

func (m *mockWorkUnitResultDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in)
	}
	return &pb.Invocation{Name: in.Name}, nil
}

func (m *mockWorkUnitResultDBClient) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
	if m.getWorkUnit != nil {
		return m.getWorkUnit(ctx, in)
	}
	return &pb.WorkUnit{Name: in.Name, WorkUnitId: "wu-1", State: pb.WorkUnit_SUCCEEDED}, nil
}

func TestCmdWorkUnit(t *testing.T) {
	t.Parallel()

	ftt.Run(`Cmd`, t, func(t *ftt.Test) {
		cmd := Cmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("work-unit <subcommand>"))

		getCmd := GetCmd(nil)
		assert.Loosely(t, getCmd, should.NotBeNil)
		assert.Loosely(t, getCmd.UsageLine, should.Equal("get -invocationid <invocation_id> -workunitid <work_unit_id>"))

		t.Run(`rejects legacy invocation`, func(t *ftt.Test) {
			r := &workUnitGetRun{
				client: &mockWorkUnitResultDBClient{
					getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
						return nil, status.Error(codes.NotFound, "not found")
					},
				},
				invocationID: "u-swarming-123",
				workUnitID:   "wu-1",
			}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(1))
		})

		t.Run(`normalizes I<digits> to ants-i<digits> and queries root invocation`, func(t *ftt.Test) {
			var gotName string
			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					gotName = in.Name
					return &pb.WorkUnit{Name: in.Name, WorkUnitId: "ants-wu123", State: pb.WorkUnit_SUCCEEDED, SummaryMarkdown: "ok"}, nil
				},
			}
			r := &workUnitGetRun{
				client:       client,
				invocationID: "I77100010600769898",
				workUnitID:   "WU123",
			}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			assert.Loosely(t, gotName, should.Equal("rootInvocations/ants-i77100010600769898/workUnits/ants-wu123"))
		})

		t.Run(`fails immediately on PermissionDenied`, func(t *ftt.Test) {
			client := &mockWorkUnitResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.PermissionDenied, "caller does not have permission")
				},
			}
			r := &workUnitGetRun{
				client:       client,
				invocationID: "build-123",
				workUnitID:   "wu-1",
			}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(1))
		})
	})
}
