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
	"net/http"
	"net/http/httptest"
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
	getWorkUnit           func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error)
	batchGetWorkUnits     func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error)
	listArtifacts         func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error)
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

func (m *mockInvocationResultDBClient) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
	if m.getWorkUnit != nil {
		return m.getWorkUnit(ctx, in)
	}
	return nil, nil
}

func (m *mockInvocationResultDBClient) BatchGetWorkUnits(ctx context.Context, in *pb.BatchGetWorkUnitsRequest, opts ...grpc.CallOption) (*pb.BatchGetWorkUnitsResponse, error) {
	if m.batchGetWorkUnits != nil {
		return m.batchGetWorkUnits(ctx, in)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockInvocationResultDBClient) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
	if m.listArtifacts != nil {
		return m.listArtifacts(ctx, in)
	}
	return &pb.ListArtifactsResponse{}, nil
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
			printInvocationDetails(ctx, &buf, client, nil, resolved, true)
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

		t.Run(`surfaces root and failed child TFC_COMMAND_TASK work unit summaries and invocationFailed_trace artifacts`, func(t *ftt.Test) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/root-summary":
					_, _ = w.Write([]byte("Full root summary from invocationFailed_summary_markdown."))
				default:
					_, _ = w.Write([]byte("com.android.tradefed.device.DeviceNotAvailableException: Device 127.0.0.1:41235 offline"))
				}
			}))
			defer srv.Close()

			rootName := "rootInvocations/ants-i11700010583257168/workUnits/root"
			childName := "rootInvocations/ants-i11700010583257168/workUnits/ants-wu11700010583257168"
			passChildName := "rootInvocations/ants-i11700010583257168/workUnits/ants-wu11700010583257169"

			client := &mockInvocationResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{Name: in.Name, State: pb.RootInvocation_FAILED, SummaryMarkdown: "Test command failed (truncated)."}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{Name: rootName, WorkUnitId: "root", State: pb.WorkUnit_FAILED, SummaryMarkdown: "Test command failed (truncated).", ChildWorkUnits: []string{childName, passChildName}}, nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return &pb.BatchGetWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{Name: childName, WorkUnitId: "ants-wu11700010583257168", Kind: "TFC_COMMAND_TASK", State: pb.WorkUnit_FAILED, SummaryMarkdown: "- **error_name**: DEVICE_UNAVAILABLE"},
							{Name: passChildName, WorkUnitId: "ants-wu11700010583257169", Kind: "TFC_COMMAND_TASK", State: pb.WorkUnit_SUCCEEDED, SummaryMarkdown: "ok"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == rootName {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{{ArtifactId: "invocationFailed_summary_markdown", FetchUrl: srv.URL + "/root-summary"}},
						}, nil
					}
					return &pb.ListArtifactsResponse{
						Artifacts: []*pb.Artifact{{ArtifactId: "invocationFailed_trace", FetchUrl: srv.URL + "/trace"}},
					}, nil
				},
			}

			var buf bytes.Buffer
			r := &invocationGetRun{client: client, httpClient: srv.Client(), out: &buf, invocationID: "ants-i11700010583257168"}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Summary:\n  Full root summary from invocationFailed_summary_markdown.\n"))
			assert.Loosely(t, out, should.ContainSubstring("luci artifact get -invocationid=ants-i11700010583257168 -workunitid=root -artifactid=invocationFailed_summary_markdown"))
			assert.Loosely(t, out, should.ContainSubstring("Failed Child Work Unit: ants-wu11700010583257168 (kind: TFC_COMMAND_TASK, state: FAILED)\n"))
			assert.Loosely(t, out, should.NotContainSubstring("ants-wu11700010583257169"))
			assert.Loosely(t, out, should.ContainSubstring("- **error_name**: DEVICE_UNAVAILABLE"))
			assert.Loosely(t, out, should.ContainSubstring("DeviceNotAvailableException: Device 127.0.0.1:41235 offline"))
			assert.Loosely(t, out, should.ContainSubstring("luci artifact get -invocationid=ants-i11700010583257168 -workunitid=ants-wu11700010583257168 -artifactid=invocationFailed_trace"))
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
