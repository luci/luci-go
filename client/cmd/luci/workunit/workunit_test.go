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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/client/cmd/luci/base"
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
	return &pb.QueryWorkUnitsResponse{}, nil
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
			assert.Loosely(t, out, should.ContainSubstring("  ... and 2 more child work units"))
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
			assert.Loosely(t, out, should.ContainSubstring("Kind:         ATP_INVOCATION\n"))
			assert.Loosely(t, out, should.ContainSubstring("State:        FAILED\n"))
			assert.Loosely(t, out, should.ContainSubstring("Finalization: FINALIZED\n"))
			assert.Loosely(t, out, should.ContainSubstring("Module Statuses: [1 failed, 1 flaky, 18 succeeded] (20 total)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Test Verdicts:   [3 failed, 2 flaky, 145 passed, 5 skipped] (155 total)\n"))

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
	})
}
