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

package testresult

import (
	"bytes"
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockResultDBClient struct {
	pb.ResultDBClient
	getRootInvocation     func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation         func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
	getTestResult         func(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error)
	queryTestVerdicts     func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error)
	queryTestResults      func(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error)
	queryTestExonerations func(ctx context.Context, in *pb.QueryTestExonerationsRequest) (*pb.QueryTestExonerationsResponse, error)
	listArtifacts         func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error)
	queryWorkUnits        func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error)
}

func (m *mockResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in)
	}
	return &pb.RootInvocation{Name: in.Name}, nil
}

func (m *mockResultDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in)
	}
	return &pb.Invocation{Name: in.Name}, nil
}

func (m *mockResultDBClient) QueryTestAggregations(ctx context.Context, in *pb.QueryTestAggregationsRequest, opts ...grpc.CallOption) (*pb.QueryTestAggregationsResponse, error) {
	return &pb.QueryTestAggregationsResponse{}, nil
}

func (m *mockResultDBClient) GetTestResult(ctx context.Context, in *pb.GetTestResultRequest, opts ...grpc.CallOption) (*pb.TestResult, error) {
	if m.getTestResult != nil {
		return m.getTestResult(ctx, in)
	}
	return nil, errors.New("not implemented")
}

func (m *mockResultDBClient) QueryTestVerdicts(ctx context.Context, in *pb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*pb.QueryTestVerdictsResponse, error) {
	if m.queryTestVerdicts != nil {
		return m.queryTestVerdicts(ctx, in)
	}
	return &pb.QueryTestVerdictsResponse{}, nil
}

func (m *mockResultDBClient) QueryTestResults(ctx context.Context, in *pb.QueryTestResultsRequest, opts ...grpc.CallOption) (*pb.QueryTestResultsResponse, error) {
	if m.queryTestResults != nil {
		return m.queryTestResults(ctx, in)
	}
	return &pb.QueryTestResultsResponse{}, nil
}

func (m *mockResultDBClient) QueryTestExonerations(ctx context.Context, in *pb.QueryTestExonerationsRequest, opts ...grpc.CallOption) (*pb.QueryTestExonerationsResponse, error) {
	if m.queryTestExonerations != nil {
		return m.queryTestExonerations(ctx, in)
	}
	return &pb.QueryTestExonerationsResponse{}, nil
}

func (m *mockResultDBClient) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
	if m.listArtifacts != nil {
		return m.listArtifacts(ctx, in)
	}
	return &pb.ListArtifactsResponse{}, nil
}

func (m *mockResultDBClient) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
	return &pb.WorkUnit{Name: in.Name}, nil
}

func (m *mockResultDBClient) QueryWorkUnits(ctx context.Context, in *pb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*pb.QueryWorkUnitsResponse, error) {
	if m.queryWorkUnits != nil {
		return m.queryWorkUnits(ctx, in)
	}
	return &pb.QueryWorkUnitsResponse{}, nil
}

func (m *mockResultDBClient) QueryArtifacts(ctx context.Context, in *pb.QueryArtifactsRequest, opts ...grpc.CallOption) (*pb.QueryArtifactsResponse, error) {
	return &pb.QueryArtifactsResponse{}, nil
}

func TestCmdTestResult(t *testing.T) {
	t.Parallel()

	ftt.Run(`Cmd`, t, func(t *ftt.Test) {
		cmd := Cmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("test-result <subcommand>"))

		getCmd := GetCmd(nil)
		assert.Loosely(t, getCmd, should.NotBeNil)
		assert.Loosely(t, getCmd.UsageLine, should.Equal("get [-invocationid <invocation_id> -testid <test_id> -resultid <result_id>] [<target>]"))

		t.Run(`testResultGetRun accepts positional test result resource name and prints derived IDs`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: ":ModA!junit:pkg:Cls#testOne",
								Results: []*pb.TestResult{
									{
										Name:        "rootInvocations/ants-i123/workUnits/wu-1/tests/:ModA!junit:pkg:Cls%23testOne/results/res-1",
										TestId:      ":ModA!junit:pkg:Cls#testOne",
										ResultId:    "res-1",
										VariantHash: "vhash1",
										StatusV2:    pb.TestResult_FAILED,
									},
								},
							},
						},
					}, nil
				},
			}
			var buf bytes.Buffer
			run := getCmd.CommandRun().(*testResultGetRun)
			run.client = client
			run.out = &buf
			rc := run.Run(nil, []string{"rootInvocations/ants-i123/workUnits/wu-1/tests/:ModA!junit:pkg:Cls%23testOne/results/res-1"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     ants-i123"))
			assert.Loosely(t, out, should.ContainSubstring("Module Name:       ModA"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit ID:      wu-1"))
			assert.Loosely(t, out, should.ContainSubstring("Test ID:           :ModA!junit:pkg:Cls#testOne"))
			assert.Loosely(t, out, should.ContainSubstring("Result ID:         res-1"))
			assert.Loosely(t, out, should.ContainSubstring("Result ID: res-1"))
		})
	})
}

func TestFetchTestResult(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchTestResult`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`legacy uses QueryTestResults`, func(t *ftt.Test) {
			calledQuery := false
			client := &mockResultDBClient{
				getTestResult: func(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error) {
					t.Fatalf("GetTestResult should not be called in legacy mode")
					return nil, nil
				},
				queryTestResults: func(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
					calledQuery = true
					assert.Loosely(t, in.Invocations, should.Resemble([]string{"invocations/build-123"}))
					return &pb.QueryTestResultsResponse{
						TestResults: []*pb.TestResult{
							{
								Name:     "invocations/u-leaf/tests/test1/results/res1",
								TestId:   "test1",
								ResultId: "res1",
								StatusV2: pb.TestResult_PASSED,
							},
						},
					}, nil
				},
			}

			tr, err := FetchTestResult(ctx, client, "build-123", "test1", "res1", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calledQuery, should.BeTrue)
			assert.Loosely(t, tr.ResultId, should.Equal("res1"))
			assert.Loosely(t, tr.Name, should.Equal("invocations/u-leaf/tests/test1/results/res1"))
		})

		t.Run(`root invocation uses QueryTestVerdicts directly`, func(t *ftt.Test) {
			calledQuery := false
			client := &mockResultDBClient{
				getTestResult: func(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error) {
					t.Fatalf("GetTestResult should not be called in root invocation mode")
					return nil, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					calledQuery = true
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i123"))
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: "test1",
								Results: []*pb.TestResult{
									{
										Name:     "rootInvocations/ants-i123/workUnits/wu-1/tests/test1/results/res1",
										TestId:   "test1",
										ResultId: "res1",
										StatusV2: pb.TestResult_PASSED,
									},
								},
							},
						},
					}, nil
				},
			}

			tr, err := FetchTestResult(ctx, client, "ants-i123", "test1", "res1", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calledQuery, should.BeTrue)
			assert.Loosely(t, tr.ResultId, should.Equal("res1"))
			assert.Loosely(t, tr.Name, should.Equal("rootInvocations/ants-i123/workUnits/wu-1/tests/test1/results/res1"))
		})

		t.Run(`testResultGetRun fails immediately on PermissionDenied`, func(t *ftt.Test) {
			legacyQueried := false
			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.PermissionDenied, "caller does not have permission")
				},
				queryTestResults: func(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
					legacyQueried = true
					return &pb.QueryTestResultsResponse{}, nil
				},
			}
			run := &testResultGetRun{
				client:       client,
				invocationID: "build-123",
				testID:       "test1",
				resultID:     "res1",
			}
			rc := run.Run(nil, nil, nil)
			assert.Loosely(t, rc, should.Equal(1))
			assert.Loosely(t, legacyQueried, should.BeFalse)
		})
	})
}

func TestPrintTestResult(t *testing.T) {
	t.Parallel()

	ftt.Run(`printTestResult prints test result and parent work unit artifact counts`, t, func(t *ftt.Test) {
		ctx := context.Background()
		trName := "rootInvocations/ants-i123/workUnits/wu-1/tests/test1/results/res1"

		t.Run(`with test result and work unit artifacts`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					switch in.Parent {
					case trName:
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{
									ArtifactId:   "logcat-on-failure-123.txt",
									SizeBytes:    194987,
									ContentType:  "text/plain",
									ArtifactType: "LOGCAT",
								},
							},
						}, nil
					case "rootInvocations/ants-i123/workUnits/wu-1":
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{ArtifactId: "host_log.txt"},
								{ArtifactId: "tradefed-expanded.xml"},
							},
						}, nil
					default:
						return &pb.ListArtifactsResponse{}, nil
					}
				},
			}

			res := &pb.TestResult{
				Name:     trName,
				TestId:   "test1",
				ResultId: "res1",
				StatusV2: pb.TestResult_FAILED,
			}

			var buf bytes.Buffer
			run := &testResultGetRun{invocationID: "ants-i123", out: &buf}
			run.printTestResult(ctx, &buf, client, nil, nil, "ants-i123", res, false)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring(`Artifact Count: 1 (run 'luci test-result artifact list -invocationid ants-i123 -testid "test1" -resultid res1' to view)`))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:"))
			assert.Loosely(t, out, should.ContainSubstring("Work unit wu-1 contains 2 artifacts. Run 'luci work-unit artifact list -invocationid ants-i123 -workunitid wu-1' to view."))
		})

		t.Run(`with 0 test result artifacts and artifacts only on ancestor wu_1 (wu_1 -> wu_2 -> wu_3 -> tr_1)`, func(t *ftt.Test) {
			nestedTRName := "rootInvocations/ants-i123/workUnits/wu_3/tests/test1/results/tr_1"
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					assert.Loosely(t, in.Predicate.GetAncestorsOf(), should.Equal("rootInvocations/ants-i123/workUnits/wu_3"))
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{Name: "rootInvocations/ants-i123/workUnits/wu_2"},
							{Name: "rootInvocations/ants-i123/workUnits/wu_1"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == "rootInvocations/ants-i123/workUnits/wu_1" {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{ArtifactId: "host_log.txt"},
							},
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			res := &pb.TestResult{
				Name:     nestedTRName,
				TestId:   "test1",
				ResultId: "tr_1",
				StatusV2: pb.TestResult_FAILED,
			}

			var buf bytes.Buffer
			run := &testResultGetRun{invocationID: "ants-i123", out: &buf}
			run.printTestResult(ctx, &buf, client, nil, nil, "ants-i123", res, false)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Artifact Count: 0\n"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:"))
			assert.Loosely(t, out, should.ContainSubstring("Work unit wu_1 contains 1 artifact. Run 'luci work-unit artifact list -invocationid ants-i123 -workunitid wu_1' to view."))
			assert.Loosely(t, out, should.NotContainSubstring("wu_2"))
			assert.Loosely(t, out, should.NotContainSubstring("wu_3 contains"))
		})

		t.Run(`gates RootInvocation metadata behind -show-metadata`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:  in.Name,
						State: pb.RootInvocation_FAILED,
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
			}
			res := &pb.TestResult{
				Name:     trName,
				TestId:   "test1",
				ResultId: "res1",
				StatusV2: pb.TestResult_FAILED,
			}

			var defaultBuf bytes.Buffer
			defaultRun := &testResultGetRun{invocationID: "ants-i123", showMetadata: false, out: &defaultBuf}
			defaultRun.printTestResult(ctx, &defaultBuf, client, nil, nil, "ants-i123", res, false)
			defaultOut := defaultBuf.String()
			assert.Loosely(t, defaultOut, should.NotContainSubstring("Primary Build:"))
			assert.Loosely(t, defaultOut, should.NotContainSubstring("Realm:"))

			var metaBuf bytes.Buffer
			metaRun := &testResultGetRun{invocationID: "ants-i123", showMetadata: true, out: &metaBuf}
			metaRun.printTestResult(ctx, &metaBuf, client, nil, nil, "ants-i123", res, false)
			metaOut := metaBuf.String()
			assert.Loosely(t, metaOut, should.ContainSubstring("Primary Build: branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=16133789\n"))
			assert.Loosely(t, metaOut, should.ContainSubstring("Realm:        google:android-test-results\n"))
		})
	})
}
