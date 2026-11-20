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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/testutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockWorkUnitResultDBClient struct {
	pb.ResultDBClient
	getRootInvocation     func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation         func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
	getWorkUnit           func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error)
	batchGetWorkUnits     func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error)
	queryTestAggregations func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error)
	queryWorkUnits        func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error)
	listArtifacts         func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error)
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

func (m *mockWorkUnitResultDBClient) BatchGetWorkUnits(ctx context.Context, in *pb.BatchGetWorkUnitsRequest, opts ...grpc.CallOption) (*pb.BatchGetWorkUnitsResponse, error) {
	if m.batchGetWorkUnits != nil {
		return m.batchGetWorkUnits(ctx, in)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockWorkUnitResultDBClient) QueryTestAggregations(ctx context.Context, in *pb.QueryTestAggregationsRequest, opts ...grpc.CallOption) (*pb.QueryTestAggregationsResponse, error) {
	if m.queryTestAggregations != nil {
		return m.queryTestAggregations(ctx, in)
	}
	return &pb.QueryTestAggregationsResponse{}, nil
}

func (m *mockWorkUnitResultDBClient) QueryWorkUnits(ctx context.Context, in *pb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*pb.QueryWorkUnitsResponse, error) {
	if m.queryWorkUnits != nil {
		return m.queryWorkUnits(ctx, in)
	}
	return &pb.QueryWorkUnitsResponse{}, nil
}

func (m *mockWorkUnitResultDBClient) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
	if m.listArtifacts != nil {
		return m.listArtifacts(ctx, in)
	}
	return &pb.ListArtifactsResponse{}, nil
}

func (m *mockWorkUnitResultDBClient) QueryArtifacts(ctx context.Context, in *pb.QueryArtifactsRequest, opts ...grpc.CallOption) (*pb.QueryArtifactsResponse, error) {
	return &pb.QueryArtifactsResponse{}, nil
}

func TestCmdWorkUnit(t *testing.T) {
	ftt.Run(`Cmd`, t, func(t *ftt.Test) {
		cmd := Cmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("work-unit <subcommand>"))

		getCmd := GetCmd(nil)
		assert.Loosely(t, getCmd, should.NotBeNil)
		assert.Loosely(t, getCmd.UsageLine, should.Equal("get [-invocationid <invocation_id> -workunitid <work_unit_id>] [<target>]"))

		t.Run(`flag aliases -id, -work-unit-id, and -invocation-id`, func(t *ftt.Test) {
			r1 := GetCmd(nil).CommandRun().(*workUnitGetRun)
			assert.Loosely(t, r1.Flags.Parse([]string{"-invocation-id", "inv-1", "-id", "wu-1"}), should.BeNil)
			assert.Loosely(t, r1.invocationID, should.Equal("inv-1"))
			assert.Loosely(t, r1.workUnitID, should.Equal("wu-1"))

			r2 := GetCmd(nil).CommandRun().(*workUnitGetRun)
			assert.Loosely(t, r2.Flags.Parse([]string{"-invocation-id", "inv-2", "-work-unit-id", "wu-2"}), should.BeNil)
			assert.Loosely(t, r2.invocationID, should.Equal("inv-2"))
			assert.Loosely(t, r2.workUnitID, should.Equal("wu-2"))
		})

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

		t.Run(`displays Module and ChildWorkUnits with truncation past 10 children`, func(t *ftt.Test) {
			children := make([]string, 12)
			for i := 0; i < 12; i++ {
				children[i] = fmt.Sprintf("rootInvocations/build-123/workUnits/step-%d", i+1)
			}

			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{
						Name:              in.Name,
						WorkUnitId:        "build-root",
						Kind:              "STEP",
						State:             pb.WorkUnit_SUCCEEDED,
						FinalizationState: pb.WorkUnit_FINALIZED,
						SummaryMarkdown:   "all passed",
						ModuleId: &pb.ModuleIdentifier{
							ModuleName: "bluetooth_test_gd_unit",
						},
						ChildWorkUnits: children,
					}, nil
				},
			}

			r := &workUnitGetRun{
				client:       client,
				invocationID: "build-123",
				workUnitID:   "build-root",
			}

			out := testutil.CaptureStdout(func() {
				assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			})

			assert.Loosely(t, out, should.ContainSubstring("Module:       bluetooth_test_gd_unit"))
			assert.Loosely(t, out, should.ContainSubstring("Child Work Units (12):"))
			assert.Loosely(t, out, should.ContainSubstring("  - step-1\n"))
			assert.Loosely(t, out, should.ContainSubstring("  - step-10\n"))
			assert.Loosely(t, out, should.ContainSubstring("  ... and 2 more child work units (run 'luci work-unit list -invocationid=build-123 -parent=build-root' to list all)"))
		})

		t.Run(`gates RootInvocation and work unit metadata behind -show-metadata`, func(t *ftt.Test) {
			client := &mockWorkUnitResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:  in.Name,
						State: pb.RootInvocation_SUCCEEDED,
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
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{
						Name:            in.Name,
						WorkUnitId:      "ants-wu123",
						Kind:            "TF_MODULE",
						State:           pb.WorkUnit_SUCCEEDED,
						SummaryMarkdown: "ok",
						ModuleId: &pb.ModuleIdentifier{
							ModuleName:        "CtsModule",
							ModuleScheme:      "junit",
							ModuleVariantHash: "abcd1234",
							ModuleVariant:     &pb.Variant{Def: map[string]string{"abi": "x86_64"}},
						},
					}, nil
				},
			}

			defaultRun := &workUnitGetRun{
				client:       client,
				invocationID: "ants-i123",
				workUnitID:   "ants-wu123",
				showMetadata: false,
			}
			defaultOut := testutil.CaptureStdout(func() {
				assert.Loosely(t, defaultRun.Run(nil, nil, nil), should.Equal(0))
			})
			assert.Loosely(t, defaultOut, should.NotContainSubstring("Primary Build:"))
			assert.Loosely(t, defaultOut, should.NotContainSubstring("Scheme:"))
			assert.Loosely(t, defaultOut, should.NotContainSubstring("Realm:"))

			metaRun := &workUnitGetRun{
				client:       client,
				invocationID: "ants-i123",
				workUnitID:   "ants-wu123",
				showMetadata: true,
			}
			metaOut := testutil.CaptureStdout(func() {
				assert.Loosely(t, metaRun.Run(nil, nil, nil), should.Equal(0))
			})
			assert.Loosely(t, metaOut, should.ContainSubstring("Full Name:    rootInvocations/ants-i123/workUnits/ants-wu123\n"))
			assert.Loosely(t, metaOut, should.ContainSubstring("Scheme:       junit\n"))
			assert.Loosely(t, metaOut, should.ContainSubstring("Variant:      abi=x86_64 (hash: abcd1234)\n"))
			assert.Loosely(t, metaOut, should.ContainSubstring("Primary Build: branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=16133789\n"))
			assert.Loosely(t, metaOut, should.ContainSubstring("Realm:        google:android-test-results\n"))
		})

		t.Run(`automatic fallback resolution when GetWorkUnit returns NotFound on leaf AnTS work unit`, func(t *ftt.Test) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v4/workunits/WU89700270010105897":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
							"id": "WU89700270010105897",
							"invocationId": "I71400010624864031",
							"parentId": "WU97500270010256987",
							"name": "x86_64 bluetooth_test_gd_unit",
							"type": "TF_TEST_RUN",
							"state": "ERROR"
						}`))
				case "/v4/workunits/WU97500270010256987":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
							"id": "WU97500270010256987",
							"invocationId": "I71400010624864031",
							"parentId": "WU50500270009982574",
							"name": "bluetooth_test_gd_unit",
							"type": "TF_MODULE",
							"state": "COMPLETED",
							"moduleInfo": {
								"name": "bluetooth_test_gd_unit",
								"moduleParameters": [
									{"name": "module-abi", "value": "x86_64"}
								]
							}
						}`))
				case "/v4/workunits/WU50500270009982574":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
							"id": "WU50500270009982574",
							"name": "test-task-1",
							"type": "TFC_COMMAND_TASK",
							"invocationId": "I71400010624864031",
							"state": "COMPLETED",
							"properties": [
								{"name": "resultdb_root_invocation_id", "value": "ants-i71400010624864031"},
								{"name": "resultdb_work_unit_id", "value": "ants-wu50500270009982574"}
							]
						}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			ctxWithAnts := base.WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)

			taskWUName := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574"
			modWUName := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-mod-1"
			testRunWUName := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"

			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == "rootInvocations/ants-i71400010624864031/workUnits/ants-wu89700270010105897" {
						return nil, status.Error(codes.NotFound, "not found")
					}
					if in.Name == taskWUName {
						return &pb.WorkUnit{
							Name:           taskWUName,
							WorkUnitId:     "ants-wu50500270009982574",
							ChildWorkUnits: []string{modWUName},
						}, nil
					}
					if in.Name == testRunWUName {
						return &pb.WorkUnit{
							Name:            testRunWUName,
							WorkUnitId:      "ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64",
							Kind:            "TF_TEST_RUN",
							State:           pb.WorkUnit_FAILED,
							SummaryMarkdown: "bluetooth test failed",
						}, nil
					}
					return nil, status.Error(codes.NotFound, "not found")
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					if len(in.Names) == 1 && in.Names[0] == modWUName {
						return &pb.BatchGetWorkUnitsResponse{
							WorkUnits: []*pb.WorkUnit{
								{
									Name:       modWUName,
									WorkUnitId: "ants-wu50500270009982574:u-mod-1",
									Kind:       "TF_MODULE",
									ModuleId: &pb.ModuleIdentifier{
										ModuleName: "bluetooth_test_gd_unit",
										ModuleVariant: &pb.Variant{
											Def: map[string]string{"module_abi": "x86_64"},
										},
									},
									ChildWorkUnits: []string{testRunWUName},
								},
							},
						}, nil
					}
					return nil, status.Error(codes.NotFound, "not found")
				},
			}

			r := &workUnitGetRun{
				client:       client,
				invocationID: "I71400010624864031",
				workUnitID:   "WU89700270010105897",
				customCtx:    ctxWithAnts,
			}

			var exitCode int
			var stdoutStr, stderrStr string

			stdoutStr, stderrStr = testutil.CaptureOutput(func() {
				exitCode = r.Run(nil, nil, nil)
			})

			assert.Loosely(t, exitCode, should.Equal(0))
			assert.Loosely(t, stderrStr, should.ContainSubstring(`Note: resolved AnTS work unit "WU89700270010105897" to ResultDB work unit "ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"`))
			assert.Loosely(t, stdoutStr, should.ContainSubstring("Work Unit ID: ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"))
			assert.Loosely(t, stdoutStr, should.ContainSubstring("Summary:\nbluetooth test failed"))
		})

		t.Run(`actionable NotFound guidance message on unresolvable ants-wu... ID`, func(t *ftt.Test) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			}))
			defer srv.Close()

			ctxWithAnts := base.WithAntsHTTPClient(context.Background(), srv.Client(), srv.URL)

			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
			}

			r := &workUnitGetRun{
				client:       client,
				invocationID: "ants-i71400010624864031",
				workUnitID:   "ants-wu99999999999999999",
				customCtx:    ctxWithAnts,
			}

			var exitCode int
			stderrStr := testutil.CaptureStderr(func() {
				exitCode = r.Run(nil, nil, nil)
			})

			assert.Loosely(t, exitCode, should.Equal(1))
			assert.Loosely(t, stderrStr, should.ContainSubstring(`Work unit "ants-wu99999999999999999" not found in invocation "ants-i71400010624864031".`))
			assert.Loosely(t, stderrStr, should.ContainSubstring("Note: In Android CI (AnTS / TradeFed), module and test-run work units are stored in ResultDB"))
			assert.Loosely(t, stderrStr, should.ContainSubstring("luci ids <TR_ID_or_ATI_URL>"))
			assert.Loosely(t, stderrStr, should.ContainSubstring("luci verdict list -invocationid=ants-i71400010624864031"))
			assert.Loosely(t, stderrStr, should.ContainSubstring("luci artifact list -invocationid=ants-i71400010624864031"))
		})

		t.Run(`accepts positional work unit resource name and prints derived IDs`, func(t *ftt.Test) {
			var gotName string
			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					gotName = in.Name
					return &pb.WorkUnit{
						Name:            in.Name,
						WorkUnitId:      "ants-wu123",
						State:           pb.WorkUnit_SUCCEEDED,
						SummaryMarkdown: "ok",
						ModuleId:        &pb.ModuleIdentifier{ModuleName: "ModA", ModuleScheme: "junit"},
					}, nil
				},
			}
			r := getCmd.CommandRun().(*workUnitGetRun)
			r.client = client
			out := testutil.CaptureStdout(func() {
				rc := r.Run(nil, []string{"rootInvocations/ants-i77100010600769898/workUnits/ants-wu123"}, nil)
				assert.Loosely(t, rc, should.Equal(0))
			})
			assert.Loosely(t, gotName, should.Equal("rootInvocations/ants-i77100010600769898/workUnits/ants-wu123"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     ants-i77100010600769898"))
			assert.Loosely(t, out, should.ContainSubstring("Module Name:       ModA"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit ID:      ants-wu123"))
			assert.Loosely(t, out, should.NotContainSubstring("Work Unit ID: ants-wu123"))
		})

		t.Run(`displays RootInvocation metadata and invocation-level verdict and module counts on root work unit only`, func(t *ftt.Test) {
			var gotAggReq *pb.QueryTestAggregationsRequest
			client := &mockWorkUnitResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:  in.Name,
						Realm: "android:prod",
						PrimaryBuild: &pb.BuildDescriptor{
							Definition: &pb.BuildDescriptor_AndroidBuild{
								AndroidBuild: &pb.AndroidBuildDescriptor{
									DataRealm:   "prod",
									Branch:      "git_main",
									BuildTarget: "cf_x86_64_phone-trunk_staging-userdebug",
									BuildId:     "12345678",
								},
							},
						},
						ProducerResource: &pb.ProducerResource{
							System: "atp",
							Name:   "testRuns/98765432",
						},
						Tags: []*pb.StringPair{
							{Key: "runner_url", Value: "https://tradefed.example.com/run/123"},
							{Key: "test_uri", Value: ""},
							{Key: "runner", Value: "Tradefed"},
							{Key: "result_link", Value: "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898"},
							{Key: "sponge_invocation_id", Value: "d1234567-89ab-cdef-0123-456789abcdef"},
							{Key: "failure_status", Value: "presubmit_error"},
							{Key: "presubmit_failure_status", Value: "ERROR"},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == "rootInvocations/ants-i77100010600769898/workUnits/root" {
						return &pb.WorkUnit{
							Name:              in.Name,
							WorkUnitId:        "root",
							Kind:              "ATP_INVOCATION",
							State:             pb.WorkUnit_FAILED,
							FinalizationState: pb.WorkUnit_FINALIZED,
							SummaryMarkdown:   "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898",
							ChildWorkUnits: []string{
								"rootInvocations/ants-i77100010600769898/workUnits/ants-wu1",
								"rootInvocations/ants-i77100010600769898/workUnits/ants-wu2",
							},
						}, nil
					}
					return &pb.WorkUnit{
						Name:       in.Name,
						WorkUnitId: "ants-wu1",
						State:      pb.WorkUnit_SUCCEEDED,
					}, nil
				},
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					gotAggReq = in
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{
									Failed:  3,
									Flaky:   2,
									Passed:  145,
									Skipped: 5,
								},
								ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{
									Failed:    1,
									Flaky:     1,
									Succeeded: 18,
								},
							},
						},
					}, nil
				},
			}

			r := &workUnitGetRun{
				client:       client,
				invocationID: "I77100010600769898",
				workUnitID:   "root",
			}

			out := testutil.CaptureStdout(func() {
				assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			})

			assert.Loosely(t, gotAggReq, should.NotBeNil)
			assert.Loosely(t, gotAggReq.Parent, should.Equal("rootInvocations/ants-i77100010600769898"))
			assert.Loosely(t, gotAggReq.Predicate.GetAggregationLevel(), should.Equal(pb.AggregationLevel_INVOCATION))

			assert.Loosely(t, out, should.ContainSubstring("Work Unit ID: root\n"))
			assert.Loosely(t, out, should.ContainSubstring("Realm:        android:prod\n"))
			assert.Loosely(t, out, should.ContainSubstring("Branch:       git_main\n"))
			assert.Loosely(t, out, should.ContainSubstring("Target:       cf_x86_64_phone-trunk_staging-userdebug\n"))
			assert.Loosely(t, out, should.ContainSubstring("Build ID:     12345678\n"))
			assert.Loosely(t, out, should.ContainSubstring("Scheduler:    atp\n"))
			assert.Loosely(t, out, should.ContainSubstring("Runner:       Tradefed\n"))
			assert.Loosely(t, out, should.ContainSubstring("Result Link:  https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898\n"))
			assert.Loosely(t, out, should.ContainSubstring("Sponge Invocation ID: d1234567-89ab-cdef-0123-456789abcdef\n"))
			assert.Loosely(t, out, should.NotContainSubstring("Test URI:"))
			assert.Loosely(t, out, should.ContainSubstring("Runner URL:   https://tradefed.example.com/run/123\n"))
			assert.Loosely(t, out, should.ContainSubstring("Failure Policy: failure_status=presubmit_error presubmit_failure_status=ERROR\n"))
			assert.Loosely(t, out, should.ContainSubstring("Kind:         ATP_INVOCATION\n"))
			assert.Loosely(t, out, should.ContainSubstring("State:        FAILED\n"))
			assert.Loosely(t, out, should.ContainSubstring("Finalization: FINALIZED\n"))
			assert.Loosely(t, out, should.ContainSubstring("Module Statuses: [1 failed, 1 flaky, 18 succeeded] (20 total)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Test Verdicts:   [3 failed, 2 flaky, 145 passed, 5 skipped] (155 total)\n"))
			assert.Loosely(t, out, should.NotContainSubstring("Summary:"))

			// -show-metadata on root work unit prints external metadata fields only once.
			rRootMeta := &workUnitGetRun{
				client:       client,
				invocationID: "I77100010600769898",
				workUnitID:   "root",
				showMetadata: true,
			}
			outRootMeta := testutil.CaptureStdout(func() {
				assert.Loosely(t, rRootMeta.Run(nil, nil, nil), should.Equal(0))
			})
			assert.Loosely(t, strings.Count(outRootMeta, "Result Link:"), should.Equal(1))
			assert.Loosely(t, strings.Count(outRootMeta, "Runner URL:"), should.Equal(1))
			assert.Loosely(t, strings.Count(outRootMeta, "Failure Policy:"), should.Equal(1))
			assert.Loosely(t, outRootMeta, should.ContainSubstring("Primary Build: branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=12345678\n"))

			// Zero-verdict root invocation prints contextual Note and Next steps without duplicate work-unit get.
			clientZero := &mockWorkUnitResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:              in.Name,
						State:             pb.RootInvocation_SUCCEEDED,
						FinalizationState: pb.RootInvocation_FINALIZED,
						Tags: []*pb.StringPair{
							{Key: "result_link", Value: "http://sponge2/outer-111"},
							{Key: "sponge_invocation_id", Value: "inner-222"},
							{Key: "test_uri", Value: "http://fusion2/inner-222"},
							{Key: "failure_status", Value: "presubmit_error"},
						},
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{Name: in.Name, WorkUnitId: "root", State: pb.WorkUnit_SUCCEEDED}, nil
				},
				queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
					return &pb.QueryTestAggregationsResponse{
						Aggregations: []*pb.TestAggregation{
							{
								ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{Skipped: 1},
								TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{},
							},
						},
					}, nil
				},
			}
			rZero := &workUnitGetRun{client: clientZero, invocationID: "ants-i90900010658207079", workUnitID: "root"}
			outZero := testutil.CaptureStdout(func() {
				assert.Loosely(t, rZero.Run(nil, nil, nil), should.Equal(0))
			})
			assert.Loosely(t, outZero, should.ContainSubstring("Note: Inv State: SUCCEEDED with 0 ResultDB verdicts"))
			assert.Loosely(t, outZero, should.NotContainSubstring("luci work-unit get"))
			assert.Loosely(t, outZero, should.ContainSubstring("luci artifact list -invocationid=ants-i90900010658207079"))
			assert.Loosely(t, outZero, should.ContainSubstring("--invocation_id=inner-222 via sponge skill; http://fusion2/inner-222"))
			assert.Loosely(t, outZero, should.ContainSubstring("--invocation_id=outer-111; http://sponge2/outer-111"))

			// Empty wu.SummaryMarkdown on root does not leak redundant RootInvocation.SummaryMarkdown via Layer 3.
			origGetRootInv := client.getRootInvocation
			origGetWU := client.getWorkUnit
			client.getRootInvocation = func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
				ri, err := origGetRootInv(ctx, in)
				if ri != nil {
					ri.SummaryMarkdown = "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898"
				}
				return ri, err
			}
			client.getWorkUnit = func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
				wu, err := origGetWU(ctx, in)
				if wu != nil && wu.WorkUnitId == "root" {
					wu.SummaryMarkdown = ""
				}
				return wu, err
			}
			rRootLayer3 := &workUnitGetRun{
				client:       client,
				invocationID: "I77100010600769898",
				workUnitID:   "root",
			}
			outRootLayer3 := testutil.CaptureStdout(func() {
				assert.Loosely(t, rRootLayer3.Run(nil, nil, nil), should.Equal(0))
			})
			assert.Loosely(t, outRootLayer3, should.NotContainSubstring("Summary"))
			client.getRootInvocation = origGetRootInv
			client.getWorkUnit = origGetWU

			gotAggReq = nil
			rChild := &workUnitGetRun{
				client:       client,
				invocationID: "I77100010600769898",
				workUnitID:   "ants-wu1",
			}

			outChild := testutil.CaptureStdout(func() {
				assert.Loosely(t, rChild.Run(nil, nil, nil), should.Equal(0))
			})

			assert.Loosely(t, gotAggReq, should.BeNil)
			assert.Loosely(t, outChild, should.ContainSubstring("Work Unit ID: ants-wu1\n"))
			assert.Loosely(t, outChild, should.NotContainSubstring("Realm:"))
			assert.Loosely(t, outChild, should.NotContainSubstring("Branch:"))
			assert.Loosely(t, outChild, should.NotContainSubstring("Target:"))
			assert.Loosely(t, outChild, should.NotContainSubstring("Build ID:"))
		})

		t.Run(`surfaces direct and ancestor work unit artifact notices`, func(t *ftt.Test) {
			targetWUName := "rootInvocations/ants-i77100010600769898/workUnits/ants-wu505:u-mod-1"
			taskWUName := "rootInvocations/ants-i77100010600769898/workUnits/ants-wu505"
			rootWUName := "rootInvocations/ants-i77100010600769898/workUnits/root"

			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{
						Name:            targetWUName,
						WorkUnitId:      "ants-wu505:u-mod-1",
						Parent:          taskWUName,
						Kind:            "TF_MODULE",
						State:           pb.WorkUnit_FAILED,
						SummaryMarkdown: "TargetSetupError: Failed to install apk",
					}, nil
				},
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					if in.GetPredicate().GetAncestorsOf() == targetWUName {
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
					case targetWUName:
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         targetWUName + "/artifacts/module_setup_error.txt",
									ArtifactId:   "module_setup_error.txt",
									ArtifactType: "TEXT",
									ContentType:  "text/plain",
									SizeBytes:    1024,
								},
							},
						}, nil
					case taskWUName:
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         taskWUName + "/artifacts/host_log_123.txt",
									ArtifactId:   "host_log_123.txt",
									ArtifactType: "HOST_LOG",
									ContentType:  "text/plain",
									SizeBytes:    20480,
								},
								{
									Name:         taskWUName + "/artifacts/device_logcat_setup_123.txt",
									ArtifactId:   "device_logcat_setup_123.txt",
									ArtifactType: "LOGCAT",
									ContentType:  "text/plain",
									SizeBytes:    65536,
								},
							},
						}, nil
					default:
						return &pb.ListArtifactsResponse{}, nil
					}
				},
			}

			r := &workUnitGetRun{
				client:       client,
				invocationID: "I77100010600769898",
				workUnitID:   "ants-wu505:u-mod-1",
			}

			out := testutil.CaptureStdout(func() {
				assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			})

			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:\n"))
			assert.Loosely(t, out, should.ContainSubstring("  Work unit ants-wu505:u-mod-1 (1 artifact):\n"))
			assert.Loosely(t, out, should.ContainSubstring("module_setup_error.txt"))
			assert.Loosely(t, out, should.ContainSubstring("To fetch a work unit artifact:\n  luci artifact <get|head|tail> -invocationid=ants-i77100010600769898 -workunitid=ants-wu505:u-mod-1 -artifactid=<ARTIFACT_ID>"))
			assert.Loosely(t, out, should.ContainSubstring("Ancestor Work Unit Artifacts:\n"))
			assert.Loosely(t, out, should.ContainSubstring("  Work unit ants-wu505 (2 artifacts):\n"))
			assert.Loosely(t, out, should.ContainSubstring("device_logcat_setup_123.txt"))
			assert.Loosely(t, out, should.ContainSubstring("host_log_123.txt"))
			assert.Loosely(t, out, should.ContainSubstring("To fetch an ancestor work unit artifact:\n  luci artifact <get|head|tail> -invocationid=ants-i77100010600769898 -workunitid=ants-wu505 -artifactid=<ARTIFACT_ID>"))
		})

		t.Run(`surfaces failed child TFC_COMMAND_TASK summary and invocationFailed_* artifacts on root and child work-unit get`, func(t *ftt.Test) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/trace" {
					_, _ = w.Write([]byte("com.android.tradefed.device.DeviceNotAvailableException: Device lost"))
				} else {
					_, _ = w.Write([]byte("Full summary from invocationFailed_summary_markdown"))
				}
			}))
			defer srv.Close()

			rootName := "rootInvocations/ants-i11400010573721720/workUnits/root"
			childName := "rootInvocations/ants-i11400010573721720/workUnits/ants-wu11400010573721720"
			passChildName := "rootInvocations/ants-i11400010573721720/workUnits/ants-wu11400010573721721"

			var childListArtCalls atomic.Int32
			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					switch in.Name {
					case rootName:
						return &pb.WorkUnit{Name: rootName, WorkUnitId: "root", State: pb.WorkUnit_FAILED, SummaryMarkdown: "stack trace:", ChildWorkUnits: []string{childName, passChildName}}, nil
					case passChildName:
						return &pb.WorkUnit{Name: passChildName, WorkUnitId: "ants-wu11400010573721721", Kind: "TFC_COMMAND_TASK", State: pb.WorkUnit_SUCCEEDED, SummaryMarkdown: "ok"}, nil
					default:
						return &pb.WorkUnit{Name: childName, WorkUnitId: "ants-wu11400010573721720", Kind: "TFC_COMMAND_TASK", State: pb.WorkUnit_FAILED, SummaryMarkdown: "truncated"}, nil
					}
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == childName {
						childListArtCalls.Add(1)
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{Name: childName + "/artifacts/invocationFailed_summary_markdown", ArtifactId: "invocationFailed_summary_markdown", FetchUrl: srv.URL + "/summary"},
								{Name: childName + "/artifacts/invocationFailed_trace", ArtifactId: "invocationFailed_trace", FetchUrl: srv.URL + "/trace"},
							},
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			rRoot := &workUnitGetRun{client: client, httpClient: srv.Client(), invocationID: "ants-i11400010573721720", workUnitID: "root"}
			outRoot := testutil.CaptureStdout(func() {
				assert.Loosely(t, rRoot.Run(nil, nil, nil), should.Equal(0))
			})
			assert.Loosely(t, outRoot, should.ContainSubstring("Failed Child Work Unit: ants-wu11400010573721720 (kind: TFC_COMMAND_TASK, state: FAILED)\n"))
			assert.Loosely(t, outRoot, should.NotContainSubstring("Failed Child Work Unit: ants-wu11400010573721721"))
			assert.Loosely(t, outRoot, should.ContainSubstring("Full summary from invocationFailed_summary_markdown"))
			assert.Loosely(t, outRoot, should.ContainSubstring("DeviceNotAvailableException: Device lost"))
			assert.Loosely(t, outRoot, should.ContainSubstring("luci artifact get -invocationid=ants-i11400010573721720 -workunitid=ants-wu11400010573721720 -artifactid=invocationFailed_trace"))

			childListArtCalls.Store(0)
			rChild := &workUnitGetRun{client: client, httpClient: srv.Client(), invocationID: "ants-i11400010573721720", workUnitID: "ants-wu11400010573721720"}
			outChild := testutil.CaptureStdout(func() {
				assert.Loosely(t, rChild.Run(nil, nil, nil), should.Equal(0))
			})
			assert.Loosely(t, childListArtCalls.Load(), should.Equal(int32(2)))
			assert.Loosely(t, outChild, should.ContainSubstring("Full summary from invocationFailed_summary_markdown"))
			assert.Loosely(t, outChild, should.ContainSubstring("luci artifact get -invocationid=ants-i11400010573721720 -workunitid=ants-wu11400010573721720 -artifactid=invocationFailed_summary_markdown"))
			assert.Loosely(t, outChild, should.NotContainSubstring("Work Unit Artifacts:"))
		})

		t.Run(`caps noisy work-unit artifacts to 1.5 KB and deduplicates invocationFailed_trace`, func(t *ftt.Test) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("TargetSetupError: failed to flash device"))
			}))
			defer srv.Close()

			wuName := "rootInvocations/ants-i77100010600769898/workUnits/ants-wu900"
			arts := make([]*pb.Artifact, 0, 202)
			arts = append(arts,
				&pb.Artifact{
					Name:       wuName + "/artifacts/invocationFailed_trace",
					ArtifactId: "invocationFailed_trace",
					FetchUrl:   srv.URL + "/trace",
					SizeBytes:  40,
				},
				&pb.Artifact{
					Name:         wuName + "/artifacts/host_log_setup.txt",
					ArtifactId:   "host_log_setup.txt",
					ArtifactType: "HOST_LOG",
					ContentType:  "text/plain",
					SizeBytes:    65536,
				},
			)
			for i := range 200 {
				arts = append(arts, &pb.Artifact{
					Name:       fmt.Sprintf("%s/artifacts/injectables_%03d.json", wuName, i),
					ArtifactId: fmt.Sprintf("injectables_%03d.json", i),
					SizeBytes:  2048,
				})
			}

			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{
						Name:            wuName,
						WorkUnitId:      "ants-wu900",
						Kind:            "TFC_COMMAND_TASK",
						State:           pb.WorkUnit_FAILED,
						SummaryMarkdown: "TargetSetupError",
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == wuName {
						return &pb.ListArtifactsResponse{Artifacts: arts}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			var buf bytes.Buffer
			r := &workUnitGetRun{
				client:       client,
				httpClient:   srv.Client(),
				invocationID: "ants-i77100010600769898",
				workUnitID:   "ants-wu900",
				out:          &buf,
			}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("TargetSetupError: failed to flash device"))
			idx := strings.Index(out, "Work Unit Artifacts:\n")
			assert.Loosely(t, idx > 0, should.BeTrue)
			inlineSection := out[idx:]
			assert.Loosely(t, inlineSection, should.ContainSubstring("host_log_setup.txt"))
			assert.Loosely(t, inlineSection, should.NotContainSubstring("invocationFailed_trace"))
			assert.Loosely(t, len(inlineSection) <= format.DefaultInlineGetArtifactBudget, should.BeTrue)
			assert.Loosely(t, len(out) <= format.DefaultMaxOutputBytes, should.BeTrue)
		})

		t.Run(`skips artifact RPCs for SUCCEEDED and benign SKIPPED work units, while inlining CANCELLED`, func(t *ftt.Test) {
			rootWUName := "rootInvocations/ants-i77100010600769898/workUnits/root"
			wuName := "rootInvocations/ants-i77100010600769898/workUnits/ants-wu100"

			for _, tc := range []struct {
				name            string
				wu              *pb.WorkUnit
				rootAgg         *pb.TestAggregation
				expectListArts  bool
				expectQueryWU   bool
				expectArtifacts bool
			}{
				{
					name: "non-root SUCCEEDED with empty summary makes zero artifact RPCs",
					wu: &pb.WorkUnit{
						Name:       wuName,
						WorkUnitId: "ants-wu100",
						Parent:     rootWUName,
						State:      pb.WorkUnit_SUCCEEDED,
					},
					expectListArts:  false,
					expectQueryWU:   false,
					expectArtifacts: false,
				},
				{
					name: "root SUCCEEDED with verdicts makes zero artifact RPCs",
					wu: &pb.WorkUnit{
						Name:       rootWUName,
						WorkUnitId: "root",
						State:      pb.WorkUnit_SUCCEEDED,
					},
					rootAgg: &pb.TestAggregation{
						TotalVerdictCounts: &pb.TestAggregation_VerdictCounts{Passed: 5},
					},
					expectListArts:  false,
					expectQueryWU:   false,
					expectArtifacts: false,
				},
				{
					name: "benign SKIPPED without failure artifacts makes zero ancestor QueryWorkUnits RPCs and omits inline artifact block",
					wu: &pb.WorkUnit{
						Name:       wuName,
						WorkUnitId: "ants-wu100",
						Parent:     rootWUName,
						State:      pb.WorkUnit_SKIPPED,
					},
					expectListArts:  true,
					expectQueryWU:   false,
					expectArtifacts: false,
				},
				{
					name: "CANCELLED work unit inlines work-unit and ancestor artifacts within 1.5 KB",
					wu: &pb.WorkUnit{
						Name:            wuName,
						WorkUnitId:      "ants-wu100",
						Parent:          rootWUName,
						State:           pb.WorkUnit_CANCELLED,
						SummaryMarkdown: "Cancelled due to timeout",
					},
					expectListArts:  true,
					expectQueryWU:   true,
					expectArtifacts: true,
				},
			} {
				t.Run(tc.name, func(t *ftt.Test) {
					var listArtCalls, queryWUCalls atomic.Int32
					client := &mockWorkUnitResultDBClient{
						getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
							return tc.wu, nil
						},
						queryTestAggregations: func(ctx context.Context, in *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
							if tc.rootAgg != nil {
								return &pb.QueryTestAggregationsResponse{Aggregations: []*pb.TestAggregation{tc.rootAgg}}, nil
							}
							return &pb.QueryTestAggregationsResponse{}, nil
						},
						queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
							queryWUCalls.Add(1)
							if in.GetPredicate().GetAncestorsOf() == wuName {
								return &pb.QueryWorkUnitsResponse{
									WorkUnits: []*pb.WorkUnit{{Name: rootWUName, WorkUnitId: "root"}},
								}, nil
							}
							return &pb.QueryWorkUnitsResponse{}, nil
						},
						listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
							listArtCalls.Add(1)
							if in.Parent == wuName {
								return &pb.ListArtifactsResponse{
									Artifacts: []*pb.Artifact{
										{
											Name:         wuName + "/artifacts/host_log_cancelled.txt",
											ArtifactId:   "host_log_cancelled.txt",
											ArtifactType: "HOST_LOG",
											ContentType:  "text/plain",
											SizeBytes:    4096,
										},
									},
								}, nil
							}
							if in.Parent == rootWUName {
								return &pb.ListArtifactsResponse{
									Artifacts: []*pb.Artifact{
										{
											Name:         rootWUName + "/artifacts/root_bugreport.zip",
											ArtifactId:   "root_bugreport.zip",
											ArtifactType: "BUGREPORTZ",
											SizeBytes:    8192,
										},
									},
								}, nil
							}
							return &pb.ListArtifactsResponse{}, nil
						},
					}

					wuID := tc.wu.GetWorkUnitId()
					var buf bytes.Buffer
					r := &workUnitGetRun{
						client:       client,
						invocationID: "ants-i77100010600769898",
						workUnitID:   wuID,
						out:          &buf,
					}
					assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
					out := buf.String()

					if !tc.expectListArts {
						assert.Loosely(t, listArtCalls.Load(), should.Equal(int32(0)))
					}
					if !tc.expectQueryWU {
						assert.Loosely(t, queryWUCalls.Load(), should.Equal(int32(0)))
					}
					if tc.expectArtifacts {
						assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:\n"))
						assert.Loosely(t, out, should.ContainSubstring("host_log_cancelled.txt"))
						assert.Loosely(t, out, should.ContainSubstring("Ancestor Work Unit Artifacts:\n"))
						assert.Loosely(t, out, should.ContainSubstring("root_bugreport.zip"))
						idx := strings.Index(out, "Work Unit Artifacts:\n")
						assert.Loosely(t, len(out[idx:]) <= format.DefaultInlineGetArtifactBudget, should.BeTrue)
					} else {
						assert.Loosely(t, out, should.NotContainSubstring("Work Unit Artifacts:"))
						assert.Loosely(t, out, should.NotContainSubstring("Ancestor Work Unit Artifacts:"))
					}
				})
			}
		})

		t.Run(`benign SKIPPED child work unit does not leak FAILED parent or RootInvocation error summary`, func(t *ftt.Test) {
			rootWUName := "rootInvocations/ants-i77100010600769898/workUnits/root"
			parentWUName := "rootInvocations/ants-i77100010600769898/workUnits/ants-wu-parent"
			skippedChildName := "rootInvocations/ants-i77100010600769898/workUnits/ants-wu-skipped"

			var queryWUCalls atomic.Int32
			client := &mockWorkUnitResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:            in.Name,
						State:           pb.RootInvocation_FAILED,
						SummaryMarkdown: "Root invocation harness crash: infrastructure failure",
					}, nil
				},
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					switch in.Name {
					case skippedChildName:
						return &pb.WorkUnit{
							Name:       skippedChildName,
							WorkUnitId: "ants-wu-skipped",
							Parent:     parentWUName,
							State:      pb.WorkUnit_SKIPPED,
						}, nil
					case parentWUName:
						return &pb.WorkUnit{
							Name:            parentWUName,
							WorkUnitId:      "ants-wu-parent",
							Parent:          rootWUName,
							State:           pb.WorkUnit_FAILED,
							SummaryMarkdown: "Parent TradeFed crash: DeviceNotAvailableException",
						}, nil
					default:
						return &pb.WorkUnit{
							Name:            rootWUName,
							WorkUnitId:      "root",
							State:           pb.WorkUnit_FAILED,
							SummaryMarkdown: "Root work unit failed",
						}, nil
					}
				},
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					queryWUCalls.Add(1)
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{
								Name:            parentWUName,
								WorkUnitId:      "ants-wu-parent",
								Parent:          rootWUName,
								State:           pb.WorkUnit_FAILED,
								SummaryMarkdown: "Parent TradeFed crash: DeviceNotAvailableException",
							},
							{
								Name:            rootWUName,
								WorkUnitId:      "root",
								State:           pb.WorkUnit_FAILED,
								SummaryMarkdown: "Root work unit failed",
							},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					return &pb.ListArtifactsResponse{
						Artifacts: []*pb.Artifact{
							{
								Name:         in.Parent + "/artifacts/host_log.txt",
								ArtifactId:   "host_log.txt",
								ArtifactType: "HOST_LOG",
								SizeBytes:    2048,
							},
						},
					}, nil
				},
			}

			var buf bytes.Buffer
			r := &workUnitGetRun{
				client:       client,
				httpClient:   &http.Client{},
				invocationID: "ants-i77100010600769898",
				workUnitID:   "ants-wu-skipped",
				out:          &buf,
			}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()

			assert.Loosely(t, queryWUCalls.Load(), should.Equal(int32(0)))
			assert.Loosely(t, out, should.ContainSubstring("State:        SKIPPED"))
			assert.Loosely(t, out, should.NotContainSubstring("Parent TradeFed crash"))
			assert.Loosely(t, out, should.NotContainSubstring("Root invocation harness crash"))
			assert.Loosely(t, out, should.NotContainSubstring("Work Unit Artifacts:"))
			assert.Loosely(t, out, should.NotContainSubstring("Ancestor Work Unit Artifacts:"))
		})

		t.Run(`caps oversized work-unit primary summary at 9.5 KB while retaining inline failure artifacts`, func(t *ftt.Test) {
			wuName := "rootInvocations/ants-i77100010600769898/workUnits/ants-wu-huge"
			hugeSummary := "TOP_SETUP_FAILURE\n" + strings.Repeat("  at com.android.tradefed.targetprep.DeviceFlashPreparer.setUp(DeviceFlashPreparer.java:180)\n", 140)
			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{
						Name:            wuName,
						WorkUnitId:      "ants-wu-huge",
						State:           pb.WorkUnit_FAILED,
						SummaryMarkdown: hugeSummary,
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == wuName {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									Name:         wuName + "/artifacts/host_log_flash.txt",
									ArtifactId:   "host_log_flash.txt",
									ArtifactType: "HOST_LOG",
									ContentType:  "text/plain",
									SizeBytes:    16384,
								},
							},
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			var buf bytes.Buffer
			r := &workUnitGetRun{
				client:       client,
				invocationID: "ants-i77100010600769898",
				workUnitID:   "ants-wu-huge",
				out:          &buf,
			}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()

			assert.Loosely(t, len(out) <= format.DefaultMaxOutputBytes, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("TOP_SETUP_FAILURE"))
			assert.Loosely(t, out, should.ContainSubstring("bytes truncated; pass -show-metadata to view full output"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:\n"))
			assert.Loosely(t, out, should.ContainSubstring("host_log_flash.txt"))
		})

		t.Run(`prints work unit timestamps and computed duration`, func(t *ftt.Test) {
			created := time.Date(2026, 10, 6, 7, 0, 0, 0, time.UTC)
			finalizeStart := created.Add(3*time.Hour + 5*time.Minute)
			finalized := created.Add(3*time.Hour + 5*time.Minute + 12*time.Second)

			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{
						Name:              in.Name,
						WorkUnitId:        "wu-timed",
						State:             pb.WorkUnit_SUCCEEDED,
						FinalizationState: pb.WorkUnit_FINALIZED,
						CreateTime:        timestamppb.New(created),
						FinalizeStartTime: timestamppb.New(finalizeStart),
						FinalizeTime:      timestamppb.New(finalized),
					}, nil
				},
			}

			r := &workUnitGetRun{client: client, invocationID: "build-123", workUnitID: "wu-timed"}
			out := testutil.CaptureStdout(func() {
				assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			})
			assert.Loosely(t, out, should.ContainSubstring("Created:      2026-10-06T07:00:00Z\n"))
			assert.Loosely(t, out, should.ContainSubstring("Finalize Start: 2026-10-06T10:05:00Z\n"))
			assert.Loosely(t, out, should.ContainSubstring("Finalized:    2026-10-06T10:05:12Z\n"))
			assert.Loosely(t, out, should.ContainSubstring("Duration:     3h 05m 00s\n"))
		})
	})
}

func TestWorkUnitList(t *testing.T) {
	t.Parallel()

	ftt.Run(`work-unit list`, t, func(t *ftt.Test) {
		baseTime := time.Date(2026, 10, 6, 7, 0, 0, 0, time.UTC)

		t.Run(`lists work units across invocation with pagination and state-priority sorting`, func(t *ftt.Test) {
			var pageTokens []string
			client := &mockWorkUnitResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					pageTokens = append(pageTokens, in.PageToken)
					if in.PageToken == "" {
						return &pb.QueryWorkUnitsResponse{
							WorkUnits: []*pb.WorkUnit{
								{
									WorkUnitId:   "root",
									Kind:         "ATP_INVOCATION",
									State:        pb.WorkUnit_SUCCEEDED,
									CreateTime:   timestamppb.New(baseTime),
									FinalizeTime: timestamppb.New(baseTime.Add(10 * time.Minute)),
								},
							},
							NextPageToken: "page-2",
						}, nil
					}
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{
								WorkUnitId:     "wu-mod-failed",
								Kind:           "TF_MODULE",
								State:          pb.WorkUnit_FAILED,
								ModuleId:       &pb.ModuleIdentifier{ModuleName: "CtsCameraTestCases"},
								ModuleShardKey: "0",
								CreateTime:     timestamppb.New(baseTime),
								FinalizeTime:   timestamppb.New(baseTime.Add(3 * time.Minute)),
							},
							{
								WorkUnitId: "wu-running",
								Kind:       "TFC_COMMAND_TASK",
								State:      pb.WorkUnit_RUNNING,
								CreateTime: timestamppb.New(baseTime),
							},
						},
					}, nil
				},
			}

			r := &workUnitListRun{
				client:       client,
				invocationID: "ants-i123",
				limit:        defaultListLimit,
				sortBy:       "state",
				now:          baseTime.Add(5 * time.Minute),
			}
			var buf strings.Builder
			r.out = &buf
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()
			assert.Loosely(t, pageTokens, should.Match([]string{"", "page-2"}))
			assert.Loosely(t, out, should.ContainSubstring("Work Units in rootInvocations/ants-i123 (3):"))
			assert.Loosely(t, out, should.ContainSubstring("wu-mod-failed"))
			assert.Loosely(t, out, should.ContainSubstring("CtsCameraTestCases [shard:0]"))
			assert.Loosely(t, out, should.ContainSubstring("5m 00s"))
			assert.Loosely(t, out, should.NotContainSubstring("running 5m 00s"))
			assert.Loosely(t, out, should.ContainSubstring("Next step: luci work-unit get -invocationid=ants-i123 -workunitid=wu-mod-failed\n"))
			// FAILED must appear before RUNNING, which must appear before SUCCEEDED.
			assert.Loosely(t, strings.Index(out, "wu-mod-failed"), should.BeLessThan(strings.Index(out, "wu-running")))
			assert.Loosely(t, strings.Index(out, "wu-running"), should.BeLessThan(strings.Index(out, "\nroot ")))
		})

		t.Run(`lists direct child work units via -parent using BatchGetWorkUnits`, func(t *ftt.Test) {
			var queriedAll bool
			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					assert.Loosely(t, in.Name, should.Equal("rootInvocations/ants-i123/workUnits/root"))
					return &pb.WorkUnit{
						Name:       in.Name,
						WorkUnitId: "root",
						ChildWorkUnits: []string{
							"rootInvocations/ants-i123/workUnits/ants-wu1",
							"rootInvocations/ants-i123/workUnits/ants-wu2",
						},
					}, nil
				},
				batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
					return &pb.BatchGetWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{
								WorkUnitId:   "ants-wu1",
								Kind:         "TFC_COMMAND",
								State:        pb.WorkUnit_SUCCEEDED,
								CreateTime:   timestamppb.New(baseTime),
								FinalizeTime: timestamppb.New(baseTime.Add(2 * time.Minute)),
							},
							{
								WorkUnitId:   "ants-wu2",
								Kind:         "TFC_COMMAND",
								State:        pb.WorkUnit_FAILED,
								CreateTime:   timestamppb.New(baseTime),
								FinalizeTime: timestamppb.New(baseTime.Add(1 * time.Minute)),
							},
						},
					}, nil
				},
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					queriedAll = true
					return &pb.QueryWorkUnitsResponse{}, nil
				},
			}

			r := &workUnitListRun{
				client:       client,
				invocationID: "ants-i123",
				parentID:     "root",
				limit:        defaultListLimit,
				sortBy:       "state",
			}
			var buf strings.Builder
			r.out = &buf
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()
			assert.Loosely(t, queriedAll, should.BeFalse)
			assert.Loosely(t, out, should.ContainSubstring("Child Work Units of root in rootInvocations/ants-i123 (2):"))
			assert.Loosely(t, out, should.ContainSubstring("ants-wu1"))
			assert.Loosely(t, out, should.ContainSubstring("ants-wu2"))
		})

		t.Run(`supports -state, -kind, -modulename, -sort duration, and -limit`, func(t *ftt.Test) {
			client := &mockWorkUnitResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{
								WorkUnitId:   "wu-short",
								Kind:         "TF_MODULE",
								State:        pb.WorkUnit_SUCCEEDED,
								ModuleId:     &pb.ModuleIdentifier{ModuleName: "ModA"},
								CreateTime:   timestamppb.New(baseTime),
								FinalizeTime: timestamppb.New(baseTime.Add(1 * time.Minute)),
							},
							{
								WorkUnitId:   "wu-long",
								Kind:         "TF_MODULE",
								State:        pb.WorkUnit_SUCCEEDED,
								ModuleId:     &pb.ModuleIdentifier{ModuleName: "ModA"},
								CreateTime:   timestamppb.New(baseTime),
								FinalizeTime: timestamppb.New(baseTime.Add(9 * time.Hour)),
							},
							{
								WorkUnitId: "wu-other-kind",
								Kind:       "TFC_COMMAND",
								State:      pb.WorkUnit_SUCCEEDED,
							},
						},
					}, nil
				},
			}

			r := &workUnitListRun{
				client:       client,
				invocationID: "ants-i123",
				stateFilter:  "succeeded",
				kindFilter:   "tf_module",
				moduleFilter: "moda",
				sortBy:       "duration",
				limit:        1,
			}
			var buf strings.Builder
			r.out = &buf
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Work Units in rootInvocations/ants-i123 (2 matching, 3 total):"))
			assert.Loosely(t, out, should.ContainSubstring("wu-long"))
			assert.Loosely(t, out, should.NotContainSubstring("wu-short"))
			assert.Loosely(t, out, should.NotContainSubstring("wu-other-kind"))
			assert.Loosely(t, out, should.ContainSubstring("... and 1 more (use -limit=0, -all, or -state/-kind/-modulename)"))
		})

		t.Run(`omits trailing colon and (none) when 0 work units match`, func(t *ftt.Test) {
			client := &mockWorkUnitResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{Name: in.Name, WorkUnitId: "wu-empty"}, nil
				},
			}
			r := &workUnitListRun{client: client, invocationID: "ants-i123", parentID: "wu-empty", limit: defaultListLimit, sortBy: "state"}
			var buf strings.Builder
			r.out = &buf
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()
			assert.Loosely(t, out, should.Equal("Child Work Units of wu-empty in rootInvocations/ants-i123 (0)\n"))
		})

		t.Run(`caps default output at format.DefaultMaxOutputBytes and allows -all or explicit -limit to bypass`, func(t *ftt.Test) {
			wus := make([]*pb.WorkUnit, 100)
			for i := range wus {
				wus[i] = &pb.WorkUnit{
					WorkUnitId: fmt.Sprintf("ants-wu06900268298775285:u-00000000-0000-0000-0000-%012d", i),
					Kind:       "TF_MODULE",
					State:      pb.WorkUnit_FAILED,
					ModuleId:   &pb.ModuleIdentifier{ModuleName: "CtsNotificationTestCases_notification_long_module_name"},
					CreateTime: timestamppb.New(baseTime),
				}
			}
			client := &mockWorkUnitResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return &pb.QueryWorkUnitsResponse{WorkUnits: wus}, nil
				},
			}

			r := &workUnitListRun{
				client:       client,
				invocationID: "ants-i123",
				limit:        defaultListLimit,
				sortBy:       "state",
				now:          baseTime.Add(5 * time.Minute),
			}
			var buf strings.Builder
			r.out = &buf
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			out := buf.String()
			assert.Loosely(t, len(out), should.BeLessThanOrEqual(format.DefaultMaxOutputBytes))
			assert.Loosely(t, out, should.ContainSubstring("... and "))

			// Explicit -limit=100 via flag parsing bypasses the 9.5 KB byte cap.
			buf.Reset()
			r.Flags.IntVar(&r.limit, "limit", defaultListLimit, "")
			assert.Loosely(t, r.Flags.Parse([]string{"-limit", "100"}), should.BeNil)
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
			assert.Loosely(t, buf.Len(), should.BeGreaterThan(format.DefaultMaxOutputBytes))
			assert.Loosely(t, buf.String(), should.NotContainSubstring("... and "))

			// -all also bypasses the 9.5 KB byte cap.
			buf.Reset()
			r2 := &workUnitListRun{
				client:       client,
				invocationID: "ants-i123",
				limit:        defaultListLimit,
				all:          true,
				sortBy:       "state",
				now:          baseTime.Add(5 * time.Minute),
				out:          &buf,
			}
			assert.Loosely(t, r2.Run(nil, nil, nil), should.Equal(0))
			assert.Loosely(t, buf.Len(), should.BeGreaterThan(format.DefaultMaxOutputBytes))
			assert.Loosely(t, buf.String(), should.NotContainSubstring("... and "))
		})

		t.Run(`rejects legacy invocations and invalid flags`, func(t *ftt.Test) {
			legacyClient := &mockWorkUnitResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
				getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
					return &pb.Invocation{Name: in.Name}, nil
				},
			}

			r := &workUnitListRun{client: legacyClient, invocationID: "build-123", sortBy: "state"}
			assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(1))

			rBadState := &workUnitListRun{client: legacyClient, invocationID: "ants-i123", stateFilter: "bogus", sortBy: "state"}
			assert.Loosely(t, rBadState.Run(nil, nil, nil), should.Equal(1))

			rBadSort := &workUnitListRun{client: legacyClient, invocationID: "ants-i123", sortBy: "bogus"}
			assert.Loosely(t, rBadSort.Run(nil, nil, nil), should.Equal(1))

			rNegLimit := &workUnitListRun{client: legacyClient, invocationID: "ants-i123", limit: -1, sortBy: "state"}
			assert.Loosely(t, rNegLimit.Run(nil, nil, nil), should.Equal(1))
		})

		t.Run(`handles truncatedScan, positional target with RPC failure, BatchGetWorkUnits errors/batching, and canceled alias`, func(t *ftt.Test) {
			t.Run(`truncatedScan`, func(t *ftt.Test) {
				calls := 0
				client := &mockWorkUnitResultDBClient{
					queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
						calls++
						return &pb.QueryWorkUnitsResponse{
							WorkUnits:     []*pb.WorkUnit{{WorkUnitId: fmt.Sprintf("wu-%d", calls), State: pb.WorkUnit_SUCCEEDED}},
							NextPageToken: fmt.Sprintf("tok-%d", calls),
						}, nil
					},
				}
				var buf strings.Builder
				r := &workUnitListRun{client: client, invocationID: "ants-i123", limit: defaultListLimit, sortBy: "state", out: &buf}
				assert.Loosely(t, r.Run(nil, nil, nil), should.Equal(0))
				assert.Loosely(t, calls, should.Equal(maxQueryPages))
				assert.Loosely(t, buf.String(), should.ContainSubstring(fmt.Sprintf("Work Units in rootInvocations/ants-i123 (%d+ scanned):", maxQueryPages)))
				assert.Loosely(t, buf.String(), should.ContainSubstring(fmt.Sprintf("stopped after scanning %d pages", maxQueryPages)))
			})

			t.Run(`positional target writes nothing to stdout on RPC failure`, func(t *ftt.Test) {
				client := &mockWorkUnitResultDBClient{
					getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
						return nil, status.Error(codes.PermissionDenied, "denied")
					},
				}
				var buf strings.Builder
				r := &workUnitListRun{client: client, limit: defaultListLimit, sortBy: "state", out: &buf}
				assert.Loosely(t, r.Run(nil, []string{"rootInvocations/ants-i123/workUnits/root"}, nil), should.Equal(1))
				assert.Loosely(t, buf.Len(), should.Equal(0))
			})

			t.Run(`BatchGetWorkUnits error returns directly and batches past maxBatchWorkUnits`, func(t *ftt.Test) {
				getCalls := 0
				batchCalls := 0
				children := make([]string, maxBatchWorkUnits+1)
				for i := range children {
					children[i] = fmt.Sprintf("rootInvocations/ants-i123/workUnits/c-%d", i)
				}
				errClient := &mockWorkUnitResultDBClient{
					getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
						getCalls++
						return &pb.WorkUnit{Name: in.Name, WorkUnitId: "root", ChildWorkUnits: children}, nil
					},
					batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
						return nil, status.Error(codes.Unavailable, "backend down")
					},
				}
				rErr := &workUnitListRun{client: errClient, invocationID: "ants-i123", parentID: "root", limit: defaultListLimit, sortBy: "state"}
				assert.Loosely(t, rErr.Run(nil, nil, nil), should.Equal(1))
				assert.Loosely(t, getCalls, should.Equal(1))

				batchClient := &mockWorkUnitResultDBClient{
					getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
						return &pb.WorkUnit{Name: in.Name, WorkUnitId: "root", ChildWorkUnits: children}, nil
					},
					batchGetWorkUnits: func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
						batchCalls++
						wus := make([]*pb.WorkUnit, len(in.Names))
						for i := range in.Names {
							wus[i] = &pb.WorkUnit{WorkUnitId: fmt.Sprintf("c-%d-%d", batchCalls, i), State: pb.WorkUnit_CANCELLED}
						}
						return &pb.BatchGetWorkUnitsResponse{WorkUnits: wus}, nil
					},
				}
				var buf strings.Builder
				rBatch := &workUnitListRun{client: batchClient, invocationID: "ants-i123", parentID: "root", stateFilter: "canceled", limit: defaultListLimit, sortBy: "state", out: &buf}
				assert.Loosely(t, rBatch.Run(nil, nil, nil), should.Equal(0))
				assert.Loosely(t, batchCalls, should.Equal(2))
				assert.Loosely(t, buf.String(), should.ContainSubstring("Child Work Units of root in rootInvocations/ants-i123 (501):"))
			})
		})
	})
}
