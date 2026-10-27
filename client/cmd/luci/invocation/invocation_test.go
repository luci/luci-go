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

package invocation

import (
	"bytes"
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockInvocationResultDBClient struct {
	pb.ResultDBClient
	getRootInvocation     func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation         func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
	queryTestAggregations func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error)
}

func (m *mockInvocationResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in)
	}
	return &pb.RootInvocation{Name: in.Name}, nil
}

func (m *mockInvocationResultDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in)
	}
	return &pb.Invocation{Name: in.Name}, nil
}

func (m *mockInvocationResultDBClient) QueryTestAggregations(ctx context.Context, in *pb.QueryTestAggregationsRequest, opts ...grpc.CallOption) (*pb.QueryTestAggregationsResponse, error) {
	if m.queryTestAggregations != nil {
		return m.queryTestAggregations(ctx, in)
	}
	return &pb.QueryTestAggregationsResponse{}, nil
}

func TestInvocationGet(t *testing.T) {
	t.Parallel()

	ftt.Run(`luci invocation get`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Cmd and GetCmd`, func(t *ftt.Test) {
			cmd := Cmd(nil)
			assert.Loosely(t, cmd, should.NotBeNil)
			assert.Loosely(t, cmd.UsageLine, should.Equal("invocation <subcommand>"))

			getCmd := GetCmd(nil)
			assert.Loosely(t, getCmd, should.NotBeNil)
			assert.Loosely(t, getCmd.UsageLine, should.Equal("get [-invocationid <invocation_id>] [<target>]"))
		})

		t.Run(`accepts positional URL and prints derived IDs`, func(t *ftt.Test) {
			var gotName string
			client := &mockInvocationResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					gotName = in.Name
					return &pb.RootInvocation{
						Name:              in.Name,
						State:             pb.RootInvocation_SUCCEEDED,
						FinalizationState: pb.RootInvocation_FINALIZED,
					}, nil
				},
			}
			var buf bytes.Buffer
			getCmd := GetCmd(nil)
			r := getCmd.CommandRun().(*invocationGetRun)
			r.client = client
			r.out = &buf
			rc := r.Run(nil, []string{"https://ci.chromium.org/ui/p/chromium/builders/try/linux-rel/b8765432109876543210"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := buf.String()
			assert.Loosely(t, gotName, should.Equal("rootInvocations/build-8765432109876543210"))
			assert.Loosely(t, out, should.ContainSubstring("Build ID:          8765432109876543210"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     build-8765432109876543210"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation:    rootInvocations/build-8765432109876543210"))
		})

		t.Run(`prints RootInvocation build, branch, target, sources, definition, state, and metadata`, func(t *ftt.Test) {
			props, _ := structpb.NewStruct(map[string]any{"runner": "tradefed"})
			rootInv := &pb.RootInvocation{
				Name:              "rootInvocations/ants-i14600010609614895",
				RootInvocationId:  "ants-i14600010609614895",
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
				ExtraBuilds: []*pb.BuildDescriptor{
					{
						Definition: &pb.BuildDescriptor_AndroidBuild{
							AndroidBuild: &pb.AndroidBuildDescriptor{
								DataRealm:   "prod",
								Branch:      "git_main",
								BuildTarget: "test_suites_x86_64",
								BuildId:     "16133790",
							},
						},
					},
				},
				Sources: &pb.Sources{
					BaseSources: &pb.Sources_SubmittedAndroidBuild{
						SubmittedAndroidBuild: &pb.SubmittedAndroidBuild{
							DataRealm: "prod",
							Branch:    "git_main",
							BuildId:   16133789,
						},
					},
				},
				Definition: &pb.RootInvocationDefinition{
					System: "atp",
					Name:   "v2/android-mts/mts-cellbroadcast",
					Properties: &pb.RootInvocationDefinition_Properties{
						Def: map[string]string{
							"run_target": "cf_x86_64_phone",
						},
					},
				},
				ProducerResource: &pb.ProducerResource{
					System: "ants",
					Name:   "ants-i14600010609614895",
				},
				Tags: []*pb.StringPair{
					{Key: "test_type", Value: "mts"},
				},
				Properties: props,
			}

			client := &mockInvocationResultDBClient{
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
									Failed: 2,
									Passed: 40,
								},
							},
						},
					}, nil
				},
			}

			resolved := &base.ResolvedInvocation{
				ID:             "ants-i14600010609614895",
				Legacy:         false,
				RootInvocation: rootInv,
			}

			var buf bytes.Buffer
			printInvocationDetails(ctx, &buf, client, resolved, true)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Invocation:    rootInvocations/ants-i14600010609614895 [2 failed, 40 passed]\n"))
			assert.Loosely(t, out, should.ContainSubstring("Inv State:     SUCCEEDED (FINALIZED)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Primary Build: branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=16133789\n"))
			assert.Loosely(t, out, should.ContainSubstring("Extra Builds:  branch=git_main build_target=test_suites_x86_64 build_id=16133790\n"))
			assert.Loosely(t, out, should.ContainSubstring("Sources:       submitted_android_build(branch=git_main build_id=16133789)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Definition:    system=atp name=v2/android-mts/mts-cellbroadcast [run_target=cf_x86_64_phone]\n"))
			assert.Loosely(t, out, should.ContainSubstring("Producer:      system=ants name=ants-i14600010609614895\n"))
			assert.Loosely(t, out, should.ContainSubstring("Realm:        google:android-test-results\n"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation Tags:\n  test_type: mts\n"))
			assert.Loosely(t, out, should.ContainSubstring(`"runner"`))
			assert.Loosely(t, out, should.ContainSubstring(`"tradefed"`))
		})

		t.Run(`fails immediately on PermissionDenied`, func(t *ftt.Test) {
			client := &mockInvocationResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.PermissionDenied, "caller does not have permission")
				},
			}
			r := &invocationGetRun{
				client:       client,
				invocationID: "ants-i123",
			}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(1))
		})
	})
}
