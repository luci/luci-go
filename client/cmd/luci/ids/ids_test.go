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

package ids

import (
	"bytes"
	"context"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockResultDBClient struct {
	pb.ResultDBClient
	queryTestVerdicts     func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error)
	queryTestResults      func(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error)
	queryTestExonerations func(ctx context.Context, in *pb.QueryTestExonerationsRequest) (*pb.QueryTestExonerationsResponse, error)
	getRootInvocation     func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation         func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
	getWorkUnit           func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error)
	batchGetWorkUnits     func(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error)
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

func (m *mockResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in)
	}
	return &pb.RootInvocation{}, nil
}

func (m *mockResultDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in)
	}
	return &pb.Invocation{}, nil
}

func (m *mockResultDBClient) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
	if m.getWorkUnit != nil {
		return m.getWorkUnit(ctx, in)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func (m *mockResultDBClient) BatchGetWorkUnits(ctx context.Context, in *pb.BatchGetWorkUnitsRequest, opts ...grpc.CallOption) (*pb.BatchGetWorkUnitsResponse, error) {
	if m.batchGetWorkUnits != nil {
		return m.batchGetWorkUnits(ctx, in)
	}
	return nil, status.Error(codes.NotFound, "not found")
}

func legacyMockClient() *mockResultDBClient {
	return &mockResultDBClient{
		getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
			return nil, status.Error(codes.NotFound, "not found")
		},
		getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
			return &pb.Invocation{Name: in.Name}, nil
		},
	}
}

type mockBuildsClient struct {
	grpcpb.BuildsClient
	getBuild func(ctx context.Context, in *bbpb.GetBuildRequest) (*bbpb.Build, error)
}

func (m *mockBuildsClient) GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	if m.getBuild != nil {
		return m.getBuild(ctx, in)
	}
	return &bbpb.Build{}, nil
}

func testBuildsClient() *mockBuildsClient {
	return &mockBuildsClient{
		getBuild: func(ctx context.Context, in *bbpb.GetBuildRequest) (*bbpb.Build, error) {
			if in.Builder != nil && in.Builder.Project == "chromium" && in.Builder.Bucket == "ci" && in.Builder.Builder == "linux-rel" && in.BuildNumber == 12345 {
				return &bbpb.Build{
					Id: 8738491827364512345,
				}, nil
			}
			if in.Builder != nil && in.Builder.Project == "chromium" && in.Builder.Bucket == "ci" && in.Builder.Builder == "linux-win-cross-rel" && in.BuildNumber == 10470 {
				return &bbpb.Build{
					Id: 8712345678901234567,
				}, nil
			}
			if in.Builder != nil && in.Builder.Project == "chromeos" && in.Builder.Bucket == "general" && in.Builder.Builder == "Full Builder" && in.BuildNumber == 12345 {
				return &bbpb.Build{
					Id: 8799999999999999999,
				}, nil
			}
			if in.Builder != nil && in.Builder.Project == "chromium" && in.Builder.Bucket == "ci" && in.Builder.Builder == "Win10 Tests x64 (1)" && in.BuildNumber == 54321 {
				return &bbpb.Build{
					Id: 8788888888888888888,
				}, nil
			}
			return nil, status.Errorf(codes.InvalidArgument, "unexpected builder %v", in.GetBuilder())
		},
	}
}

// newMockAntsServer returns a mock HTTP server simulating the AnTS v4 OnePlatform API
// across method results, invocation errors, and module errors for unit tests.
func newMockAntsServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v4/tests/TR13830335277435395":
			// Method test result with full testIdentifier (module, testClass, method)
			_, _ = w.Write([]byte(`{
				"testResultId": "TR13830335277435395",
				"invocationId": "I77100010600769898",
				"workUnitId": "WU17100269020689387",
				"testStatus": "PASS",
				"testIdentifier": {
					"module": "CellBroadcastServiceTests",
					"testClass": "com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest",
					"method": "testResetAreaInfoWithDefaultSubChanged"
				}
			}`))
		case "/v4/tests/TR98730375213734414":
			// Invocation-level placeholder test result without testIdentifier
			_, _ = w.Write([]byte(`{
				"testResultId": "TR98730375213734414",
				"invocationId": "I56900010616552061",
				"testStatus": "TEST_ERROR"
			}`))
		case "/v4/tests/TR73630363201872508":
			// Invocation-level placeholder test result with workUnitId populated
			_, _ = w.Write([]byte(`{
				"testResultId": "TR73630363201872508",
				"invocationId": "I18000010612961162",
				"workUnitId": "WU51300269529723640",
				"testStatus": "TEST_ERROR"
			}`))
		case "/v4/tests/TR36130379899461232":
			// Method test result without module name
			_, _ = w.Write([]byte(`{
				"testResultId": "TR36130379899461232",
				"invocationId": "I39300010618643739",
				"workUnitId": "WU28600269761659804",
				"testStatus": "FAIL",
				"testIdentifier": {
					"testClass": "SdvFWBaselineDHIOneVMTest",
					"method": "test_stop_start_vm"
				}
			}`))
		case "/v4/tests/TR08930357531778741":
			// Module-level error aggregate with module and aggregationLevel: MODULE
			_, _ = w.Write([]byte(`{
				"testResultId": "TR08930357531778741",
				"invocationId": "I14600010609614895",
				"workUnitId": "WU98100269380597657",
				"testStatus": "TEST_ERROR",
				"testIdentifier": {
					"module": "CellBroadcastReceiverMTS"
				},
				"aggregationDetail": {
					"aggregationLevel": "MODULE"
				}
			}`))
		case "/v4/workunits/WU17100269020689387":
			_, _ = w.Write([]byte(`{
				"id": "WU17100269020689387",
				"invocationId": "I77100010600769898",
				"name": "CellBroadcastServiceTests",
				"type": "TF_MODULE",
				"state": "COMPLETED"
			}`))
		case "/v4/workunits/WU89700270010105897":
			_, _ = w.Write([]byte(`{
				"id": "WU89700270010105897",
				"invocationId": "I71400010624864031",
				"parentId": "WU97500270010256987",
				"name": "x86_64 bluetooth_test_gd_unit",
				"type": "TF_TEST_RUN",
				"state": "ERROR"
			}`))
		case "/v4/workunits/WU97500270010256987":
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
			_, _ = w.Write([]byte(`{
				"id": "WU50500270009982574",
				"invocationId": "I71400010624864031",
				"parentId": "WU98400270010026294",
				"name": "test-task-1",
				"type": "TFC_COMMAND_TASK",
				"state": "COMPLETED",
				"properties": [
					{"name": "resultdb_root_invocation_id", "value": "ants-i71400010624864031"},
					{"name": "resultdb_work_unit_id", "value": "ants-wu50500270009982574"}
				]
			}`))
		case "/v4/workunits/WU98400270010026294":
			_, _ = w.Write([]byte(`{
				"id": "WU98400270010026294",
				"invocationId": "I71400010624864031",
				"name": "root-invocation-wu",
				"type": "TF_INVOCATION",
				"state": "COMPLETED"
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": {"code": 404, "message": "not found"}}`))
		}
	}))
}

func TestExtractIDs(t *testing.T) {
	ftt.Run(`ExtractIDs`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Milo structured verdict URL with query params`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8676886509240051393/modules/%2F%2Fchrome%3Achrome_private_code_test/schemes/single/variants/b7de9035241e76cc/cases/*fixture?artifact=summary_node&result=0"
			ids, err := ExtractIDs(ctx, legacyMockClient(), nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8676886509240051393"))
			assert.Loosely(t, ids.VariantHash, should.Equal("b7de9035241e76cc"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("summary_node"))
			assert.Loosely(t, ids.ResultID, should.Equal("0"))
			assert.Loosely(t, ids.TestID, should.Equal("*fixture"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)

			idsNoClient, err := ExtractIDs(ctx, nil, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsNoClient.Legacy, should.BeFalse)
		})

		t.Run(`Canonical test result artifact name`, func(t *ftt.Test) {
			name := "invocations/build-123/tests/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case/results/0/artifacts/stdout"
			ids, err := ExtractIDs(ctx, legacyMockClient(), nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.TestID, should.Equal("ninja://chrome/test:browser_tests/MyTest.Case"))
			assert.Loosely(t, ids.ResultID, should.Equal("0"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("stdout"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)
		})

		t.Run(`Canonical test result name`, func(t *ftt.Test) {
			name := "invocations/build-123/tests/ninja%3A%2F%2Fchrome%2Ftest/results/1"
			ids, err := ExtractIDs(ctx, legacyMockClient(), nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.TestID, should.Equal("ninja://chrome/test"))
			assert.Loosely(t, ids.ResultID, should.Equal("1"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)
		})

		t.Run(`Canonical work unit artifact name`, func(t *ftt.Test) {
			name := "rootInvocations/ants-i123/workUnits/wu-1/artifacts/log.txt"
			ids, err := ExtractIDs(ctx, nil, nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i123"))
			assert.Loosely(t, ids.WorkUnitID, should.Equal("wu-1"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("log.txt"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)
		})

		t.Run(`Canonical work unit name`, func(t *ftt.Test) {
			name := "rootInvocations/build-123/workUnits/run-tests"
			ids, err := ExtractIDs(ctx, nil, nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.WorkUnitID, should.Equal("run-tests"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)
		})

		t.Run(`Milo build URL`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/b/8676886509240051393"
			ids, err := ExtractIDs(ctx, legacyMockClient(), nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.BuildID, should.Equal("8676886509240051393"))
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8676886509240051393"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)

			urlShort := "https://ci.chromium.org/b/8676886509240051393"
			idsShort, err := ExtractIDs(ctx, legacyMockClient(), nil, urlShort, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsShort.BuildID, should.Equal("8676886509240051393"))
			assert.Loosely(t, idsShort.InvocationID, should.Equal("build-8676886509240051393"))

			urlBuild := "https://ci.chromium.org/build/8676886509240051393"
			idsBuild, err := ExtractIDs(ctx, legacyMockClient(), nil, urlBuild, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsBuild.BuildID, should.Equal("8676886509240051393"))
			assert.Loosely(t, idsBuild.InvocationID, should.Equal("build-8676886509240051393"))

			urlUIBuild := "https://ci.chromium.org/ui/build/8676886509240051393"
			idsUIBuild, err := ExtractIDs(ctx, legacyMockClient(), nil, urlUIBuild, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsUIBuild.BuildID, should.Equal("8676886509240051393"))
			assert.Loosely(t, idsUIBuild.InvocationID, should.Equal("build-8676886509240051393"))

			urlBuilder := "https://ci.chromium.org/ui/p/chromium/builders/ci/linux-rel/8676886509240051393"
			ids2, err := ExtractIDs(ctx, legacyMockClient(), nil, urlBuilder, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.BuildID, should.Equal("8676886509240051393"))
			assert.Loosely(t, ids2.InvocationID, should.Equal("build-8676886509240051393"))
			assert.Loosely(t, ids2.Legacy, should.BeTrue)

			urlBuilderTab := "https://ci.chromium.org/ui/p/chromium/builders/ci/linux-rel/8676886509240051393/test-results"
			ids3, err := ExtractIDs(ctx, legacyMockClient(), nil, urlBuilderTab, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids3.BuildID, should.Equal("8676886509240051393"))
			assert.Loosely(t, ids3.InvocationID, should.Equal("build-8676886509240051393"))

			urlBuilderBId := "https://ci.chromium.org/ui/p/chromium/builders/ci/linux-rel/b8676886509240051393/overview"
			ids4, err := ExtractIDs(ctx, legacyMockClient(), nil, urlBuilderBId, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids4.BuildID, should.Equal("8676886509240051393"))
			assert.Loosely(t, ids4.InvocationID, should.Equal("build-8676886509240051393"))

			urlBuilderNumber := "https://ci.chromium.org/p/chromium/builders/ci/linux-rel/12345"
			ids5, err := ExtractIDs(ctx, legacyMockClient(), testBuildsClient(), urlBuilderNumber, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids5.BuildID, should.Equal("8738491827364512345"))
			assert.Loosely(t, ids5.InvocationID, should.Equal("build-8738491827364512345"))

			pathBuilderNumber := "chromium/ci/linux-rel/12345"
			ids6, err := ExtractIDs(ctx, legacyMockClient(), testBuildsClient(), pathBuilderNumber, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids6.BuildID, should.Equal("8738491827364512345"))
			assert.Loosely(t, ids6.InvocationID, should.Equal("build-8738491827364512345"))

			urlWinCross := "https://ci.chromium.org/ui/p/chromium/builders/ci/linux-win-cross-rel/10470/overview"
			idsWin, err := ExtractIDs(ctx, legacyMockClient(), testBuildsClient(), urlWinCross, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsWin.BuildID, should.Equal("8712345678901234567"))
			assert.Loosely(t, idsWin.InvocationID, should.Equal("build-8712345678901234567"))

			urlEscapedSpace := "https://ci.chromium.org/ui/p/chromeos/builders/general/Full%20Builder/12345"
			idsEscapedSpace, err := ExtractIDs(ctx, legacyMockClient(), testBuildsClient(), urlEscapedSpace, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsEscapedSpace.BuildID, should.Equal("8799999999999999999"))
			assert.Loosely(t, idsEscapedSpace.InvocationID, should.Equal("build-8799999999999999999"))

			pathEscapedSpace := "chromeos/general/Full%20Builder/12345"
			idsPathEscaped, err := ExtractIDs(ctx, legacyMockClient(), testBuildsClient(), pathEscapedSpace, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsPathEscaped.BuildID, should.Equal("8799999999999999999"))
			assert.Loosely(t, idsPathEscaped.InvocationID, should.Equal("build-8799999999999999999"))

			urlEscapedParens := "https://ci.chromium.org/ui/p/chromium/builders/ci/Win10%20Tests%20x64%20%281%29/54321/overview"
			idsEscapedParens, err := ExtractIDs(ctx, legacyMockClient(), testBuildsClient(), urlEscapedParens, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsEscapedParens.BuildID, should.Equal("8788888888888888888"))
			assert.Loosely(t, idsEscapedParens.InvocationID, should.Equal("build-8788888888888888888"))

			_, errNoClient := ExtractIDs(ctx, nil, nil, urlWinCross, false)
			assert.Loosely(t, errNoClient, should.NotBeNil)
		})

		t.Run(`Buildbucket build URL`, func(t *ftt.Test) {
			url := "https://cr-buildbucket.appspot.com/build/8671656129082602433"
			ids, err := ExtractIDs(ctx, legacyMockClient(), nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.BuildID, should.Equal("8671656129082602433"))
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8671656129082602433"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)

			urlOverview := "https://cr-buildbucket.appspot.com/build/8671656129082602433/overview"
			idsOverview, err := ExtractIDs(ctx, legacyMockClient(), nil, urlOverview, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsOverview.BuildID, should.Equal("8671656129082602433"))
			assert.Loosely(t, idsOverview.InvocationID, should.Equal("build-8671656129082602433"))

			urlBId := "https://cr-buildbucket.appspot.com/build/b8671656129082602433"
			idsBId, err := ExtractIDs(ctx, legacyMockClient(), nil, urlBId, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsBId.BuildID, should.Equal("8671656129082602433"))
			assert.Loosely(t, idsBId.InvocationID, should.Equal("build-8671656129082602433"))

			urlBuilds := "https://cr-buildbucket.appspot.com/builds/8671656129082602433"
			idsBuilds, err := ExtractIDs(ctx, legacyMockClient(), nil, urlBuilds, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsBuilds.BuildID, should.Equal("8671656129082602433"))
			assert.Loosely(t, idsBuilds.InvocationID, should.Equal("build-8671656129082602433"))
		})

		t.Run(`Milo test investigation and test history URLs`, func(t *ftt.Test) {
			// Invocation subtabs
			invTests := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/tests"
			idsTests, err := ExtractIDs(ctx, legacyMockClient(), nil, invTests, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsTests.InvocationID, should.Equal("build-8671749950226328289"))

			invDetails := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/details"
			idsDetails, err := ExtractIDs(ctx, legacyMockClient(), nil, invDetails, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsDetails.InvocationID, should.Equal("build-8671749950226328289"))

			invArtifacts := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/artifacts"
			idsArtifacts, err := ExtractIDs(ctx, legacyMockClient(), nil, invArtifacts, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsArtifacts.InvocationID, should.Equal("build-8671749950226328289"))

			// Old test investigation route: invocations/:invId/tests/:testId/variants/:variantHash
			oldTI := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/tests/ninja%3A%2F%2Ftest/variants/a24ef92542e11200"
			idsOldTI, err := ExtractIDs(ctx, nil, nil, oldTI, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsOldTI.InvocationID, should.Equal("build-8671749950226328289"))
			assert.Loosely(t, idsOldTI.TestID, should.Equal("ninja://test"))
			assert.Loosely(t, idsOldTI.VariantHash, should.Equal("a24ef92542e11200"))

			// Test history route: test/:projectOrRealm/:testId
			testHistory := "https://ci.chromium.org/ui/test/chromium/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case?q=V%3Aos%3DLinux"
			idsHistory, err := ExtractIDs(ctx, nil, nil, testHistory, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsHistory.TestID, should.Equal("ninja://chrome/test:browser_tests/MyTest.Case"))

			// Blamelist route: p/:project/tests/:testId/variants/:variantHash/refs/:refHash/blamelist
			blamelist := "https://ci.chromium.org/ui/labs/p/chromium/tests/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case/variants/a24ef92542e11200/refs/deadbeef/blamelist#CP-12345"
			idsBlamelist, err := ExtractIDs(ctx, nil, nil, blamelist, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsBlamelist.TestID, should.Equal("ninja://chrome/test:browser_tests/MyTest.Case"))
			assert.Loosely(t, idsBlamelist.VariantHash, should.Equal("a24ef92542e11200"))

			// Artifact routes
			artRaw := "https://ci.chromium.org/ui/artifact/raw/invocations/build-12345/artifacts/my-art"
			idsRaw, err := ExtractIDs(ctx, nil, nil, artRaw, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsRaw.InvocationID, should.Equal("build-12345"))
			assert.Loosely(t, idsRaw.ArtifactID, should.Equal("my-art"))

			artDiff := "https://ci.chromium.org/ui/artifact/text-diff/invocations/build-12345/tests/ninja%3A%2F%2Ftest/results/r1/artifacts/stdout"
			idsDiff, err := ExtractIDs(ctx, nil, nil, artDiff, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsDiff.InvocationID, should.Equal("build-12345"))
			assert.Loosely(t, idsDiff.TestID, should.Equal("ninja://test"))
			assert.Loosely(t, idsDiff.ResultID, should.Equal("r1"))
			assert.Loosely(t, idsDiff.ArtifactID, should.Equal("stdout"))
		})

		t.Run(`Standalone invocation resource names`, func(t *ftt.Test) {
			ids, err := ExtractIDs(ctx, legacyMockClient(), nil, "invocations/build-123", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)

			ids2, err := ExtractIDs(ctx, nil, nil, "rootInvocations/ants-i456", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.InvocationID, should.Equal("ants-i456"))
			assert.Loosely(t, ids2.Legacy, should.BeFalse)
		})

		t.Run(`Standalone AnTS Invocation and Work Unit IDs`, func(t *ftt.Test) {
			srv := newMockAntsServer()
			defer srv.Close()

			ctxWithAnts := base.WithAntsHTTPClient(ctx, srv.Client(), srv.URL)

			// 1. Standalone invocation ID
			ids, err := ExtractIDs(ctxWithAnts, nil, nil, "I77100010600769898", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)

			// 2. Standalone WU17100269020689387 resolves both InvocationID and WorkUnitID
			ids2, err := ExtractIDs(ctxWithAnts, nil, nil, "WU17100269020689387", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, ids2.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, ids2.ModuleName, should.Equal("CellBroadcastServiceTests"))
			assert.Loosely(t, ids2.Legacy, should.BeFalse)

			ids3, err := ExtractIDs(ctxWithAnts, nil, nil, "ants-wu17100269020689387", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids3.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, ids3.WorkUnitID, should.Equal("ants-wu17100269020689387"))

			// 3. Standalone leaf WU89700270010105897 with mock ResultDB GetWorkUnit + BatchGetWorkUnits resolves
			// to InvocationID: "ants-i71400010624864031", ModuleName: "bluetooth_test_gd_unit",
			// and composite WorkUnitID: "ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64".
			taskWUName := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574"
			modWUName := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-db583479-b6d6-4167-88b7-e4ce8af99beb"
			testRunWUName := "rootInvocations/ants-i71400010624864031/workUnits/ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"

			client := &mockResultDBClient{
				getWorkUnit: func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
					if in.Name == taskWUName {
						return &pb.WorkUnit{
							Name:           taskWUName,
							WorkUnitId:     "ants-wu50500270009982574",
							ChildWorkUnits: []string{modWUName},
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
									WorkUnitId: "ants-wu50500270009982574:u-db583479-b6d6-4167-88b7-e4ce8af99beb",
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

			idsLeaf, err := ExtractIDs(ctxWithAnts, client, nil, "WU89700270010105897", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsLeaf.InvocationID, should.Equal("ants-i71400010624864031"))
			assert.Loosely(t, idsLeaf.ModuleName, should.Equal("bluetooth_test_gd_unit"))
			assert.Loosely(t, idsLeaf.WorkUnitID, should.Equal("ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"))

			// 4. Standalone composite ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64
			// resolves InvocationID: "ants-i71400010624864031" and preserves WorkUnitID: "ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64".
			idsComp, err := ExtractIDs(ctxWithAnts, nil, nil, "ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsComp.InvocationID, should.Equal("ants-i71400010624864031"))
			assert.Loosely(t, idsComp.WorkUnitID, should.Equal("ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"))

			// 5. ATI URL with /workunit/WU... and ?workUnitId=WU...
			urlWUPath := "https://android-build.corp.google.com/test_investigate/invocation/I71400010624864031/workunit/WU50500270009982574"
			idsURLPath, err := ExtractIDs(ctxWithAnts, client, nil, urlWUPath, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsURLPath.InvocationID, should.Equal("ants-i71400010624864031"))
			assert.Loosely(t, idsURLPath.WorkUnitID, should.Equal("ants-wu50500270009982574"))

			urlWUQuery := "https://android-build.googleplex.com/builds/tests/view?invocationId=I71400010624864031&workUnitId=WU50500270009982574"
			idsURLQuery, err := ExtractIDs(ctxWithAnts, client, nil, urlWUQuery, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsURLQuery.InvocationID, should.Equal("ants-i71400010624864031"))
			assert.Loosely(t, idsURLQuery.WorkUnitID, should.Equal("ants-wu50500270009982574"))

			// 6. Standalone composite work unit without AnTS API availability preserves WorkUnitID
			srv404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			}))
			defer srv404.Close()
			ctx404 := base.WithAntsHTTPClient(ctx, srv404.Client(), srv404.URL)
			idsCompOffline, err := ExtractIDs(ctx404, nil, nil, "ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsCompOffline.WorkUnitID, should.Equal("ants-wu50500270009982574:u-354dc939-8efe-4e98-96f7-df0e9bb26b64"))
		})

		t.Run(`AnTS TR ID and ATI URL with mock AnTS API and ResultDB`, func(t *ftt.Test) {
			srv := newMockAntsServer()
			defer srv.Close()

			ctx := base.WithAntsHTTPClient(ctx, srv.Client(), srv.URL)

			client := &mockResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i77100010600769898"))
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: ":CellBroadcastServiceTests!junit:test#testResetAreaInfoWithDefaultSubChanged",
								Results: []*pb.TestResult{
									{
										Name:        "rootInvocations/ants-i77100010600769898/workUnits/ants-wu17100269020689387/tests/t1/results/r1",
										ResultId:    "r1",
										VariantHash: "varhash123",
										StatusV2:    pb.TestResult_PASSED,
									},
								},
							},
						},
					}, nil
				},
			}

			// 1. From ATI URL
			atiURL := "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898/test/TR13830335277435395/"
			ids, err := ExtractIDs(ctx, client, nil, atiURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, ids.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, ids.TestID, should.Equal(":CellBroadcastServiceTests!junit:test#testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, ids.ResultID, should.Equal("r1"))
			assert.Loosely(t, ids.VariantHash, should.Equal("varhash123"))

			// 2. From direct TR ID
			idsTR, err := ExtractIDs(ctx, client, nil, "TR13830335277435395", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsTR.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, idsTR.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, idsTR.TestID, should.Equal(":CellBroadcastServiceTests!junit:test#testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, idsTR.ResultID, should.Equal("r1"))

			// 3. From ATI URL with query params
			atiURLWithParams := "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898/test/TR13830335277435395/?artifact=stdout&result=custom-res"
			idsParams, err := ExtractIDs(ctx, client, nil, atiURLWithParams, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsParams.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, idsParams.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, idsParams.TestID, should.Equal(":CellBroadcastServiceTests!junit:test#testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, idsParams.ResultID, should.Equal("custom-res"))
			assert.Loosely(t, idsParams.ArtifactID, should.Equal("stdout"))

			// 4. Without ResultDB resolution, AnTS WorkUnitID is preserved as fallback
			idsNoRDB, err := ExtractIDs(ctx, nil, nil, "TR13830335277435395", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsNoRDB.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, idsNoRDB.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, idsNoRDB.TestID, should.Equal("CellBroadcastServiceTests#com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest.testResetAreaInfoWithDefaultSubChanged"))

			// 4b. When ResultDB query returns an error, AnTS WorkUnitID and fallback TestID are preserved
			errClient := &mockResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return nil, status.Error(codes.Unavailable, "transient error")
				},
			}
			idsErrRDB, err := ExtractIDs(ctx, errClient, nil, "TR13830335277435395", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsErrRDB.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, idsErrRDB.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, idsErrRDB.TestID, should.Equal("CellBroadcastServiceTests#com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest.testResetAreaInfoWithDefaultSubChanged"))

			// 5. From placeholder test case "#." (e.g. invocation-level summary node in AnTS)
			idsPlaceholder, err := ExtractIDs(ctx, nil, nil, "TR98730375213734414", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsPlaceholder.InvocationID, should.Equal("ants-i56900010616552061"))
			assert.Loosely(t, idsPlaceholder.WorkUnitID, should.Equal(""))
			assert.Loosely(t, idsPlaceholder.ModuleName, should.Equal(""))
			assert.Loosely(t, idsPlaceholder.TestID, should.Equal(""))
			assert.Loosely(t, idsPlaceholder.ResultID, should.Equal(""))
			assert.Loosely(t, idsPlaceholder.SummaryPlaceholder, should.BeTrue)

			// 5b. From ATI URL with placeholder test case "#." and Work Unit ID populated
			atiSummaryURL := "https://android-build.corp.google.com/test_investigate/invocation/I18000010612961162/test/TR73630363201872508/?status=none"
			idsPlaceholderWU, err := ExtractIDs(ctx, client, nil, atiSummaryURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsPlaceholderWU.InvocationID, should.Equal("ants-i18000010612961162"))
			assert.Loosely(t, idsPlaceholderWU.WorkUnitID, should.Equal("ants-wu51300269529723640"))
			assert.Loosely(t, idsPlaceholderWU.ModuleName, should.Equal(""))
			assert.Loosely(t, idsPlaceholderWU.TestID, should.Equal(""))
			assert.Loosely(t, idsPlaceholderWU.ResultID, should.Equal(""))
			assert.Loosely(t, idsPlaceholderWU.SummaryPlaceholder, should.BeTrue)

			// 6. From AnTS test result without module name (matching ResultDB no-module-name)
			clientNoModule := &mockResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i39300010618643739"))
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: ":no-module-name!junit:no-package-name:SdvFWBaselineDHIOneVMTest#test_stop_start_vm",
								TestIdStructured: &pb.TestIdentifier{
									ModuleName:   "no-module-name",
									ModuleScheme: "junit",
									CoarseName:   "no-package-name",
									FineName:     "SdvFWBaselineDHIOneVMTest",
									CaseName:     "test_stop_start_vm",
								},
								Results: []*pb.TestResult{
									{
										Name:        "rootInvocations/ants-i39300010618643739/workUnits/ants-wu74900269761565426:u-376817f8-577f-435f-ae49-d04ecb2c2d1f/tests/:no-module-name%21junit:no-package-name:SdvFWBaselineDHIOneVMTest%23test_stop_start_vm/results/15e59e62-00003",
										ResultId:    "15e59e62-00003",
										VariantHash: "varhash-nomod",
										StatusV2:    pb.TestResult_FAILED,
									},
								},
							},
						},
					}, nil
				},
			}
			idsNoModule, err := ExtractIDs(ctx, clientNoModule, nil, "TR36130379899461232", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsNoModule.InvocationID, should.Equal("ants-i39300010618643739"))
			assert.Loosely(t, idsNoModule.WorkUnitID, should.Equal("ants-wu74900269761565426:u-376817f8-577f-435f-ae49-d04ecb2c2d1f"))
			assert.Loosely(t, idsNoModule.TestID, should.Equal(":no-module-name!junit:no-package-name:SdvFWBaselineDHIOneVMTest#test_stop_start_vm"))
			assert.Loosely(t, idsNoModule.ResultID, should.Equal("15e59e62-00003"))
			assert.Loosely(t, idsNoModule.VariantHash, should.Equal("varhash-nomod"))
		})

		t.Run(`Android Build tests view URL with query param`, func(t *ftt.Test) {
			url := "https://android-build.googleplex.com/builds/tests/view?invocationId=I30800010616707848"
			ids, err := ExtractIDs(ctx, nil, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i30800010616707848"))
			assert.Loosely(t, ids.TestID, should.Equal(""))

			urlCorp := "https://android-build.corp.google.com/builds/tests/view?invocation_id=I30800010616707848"
			idsCorp, err := ExtractIDs(ctx, nil, nil, urlCorp, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsCorp.InvocationID, should.Equal("ants-i30800010616707848"))
			assert.Loosely(t, idsCorp.TestID, should.Equal(""))
		})

		t.Run(`Milo legacy test-investigate URL without client`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/modules/legacy/schemes/legacy/variants/a24ef92542e11200/cases/ninja%3A%2F%2Fchromeos%3Achrome_all_tast_tests%2Ftast.cryptohome.UssMigrationPasswordPin?artifact=summary_node"
			ids, err := ExtractIDs(ctx, nil, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8671749950226328289"))
			assert.Loosely(t, ids.VariantHash, should.Equal("a24ef92542e11200"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("summary_node"))
			assert.Loosely(t, ids.TestID, should.Equal("ninja://chromeos:chrome_all_tast_tests/tast.cryptohome.UssMigrationPasswordPin"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)

			idsLegacy, err := ExtractIDs(ctx, nil, nil, url, true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsLegacy.Legacy, should.BeTrue)
		})

		t.Run(`Milo legacy test-investigate URL with client`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/modules/legacy/schemes/legacy/variants/a24ef92542e11200/cases/ninja%3A%2F%2Fchromeos%3Achrome_all_tast_tests%2Ftast.cryptohome.UssMigrationPasswordPin?artifact=summary_node"
			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
				getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
					return &pb.Invocation{Name: in.Name}, nil
				},
				queryTestResults: func(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
					assert.Loosely(t, in.Invocations, should.Resemble([]string{"invocations/build-8671749950226328289"}))
					return &pb.QueryTestResultsResponse{
						TestResults: []*pb.TestResult{
							{
								Name:        "invocations/build-8671749950226328289/tests/ninja%3A%2F%2Fchromeos%3Achrome_all_tast_tests%2Ftast.cryptohome.UssMigrationPasswordPin/results/res-legacy-1",
								TestId:      "ninja://chromeos:chrome_all_tast_tests/tast.cryptohome.UssMigrationPasswordPin",
								ResultId:    "res-legacy-1",
								VariantHash: "a24ef92542e11200",
							},
						},
					}, nil
				},
			}
			ids, err := ExtractIDs(ctx, client, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8671749950226328289"))
			assert.Loosely(t, ids.VariantHash, should.Equal("a24ef92542e11200"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("summary_node"))
			assert.Loosely(t, ids.TestID, should.Equal("ninja://chromeos:chrome_all_tast_tests/tast.cryptohome.UssMigrationPasswordPin"))
			assert.Loosely(t, ids.ResultID, should.Equal("res-legacy-1"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)
		})

		t.Run(`Milo modern structured URL with client resolves legacy idempotently`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/modules/modern/schemes/scheme/variants/a24ef92542e11200/cases/test.Foo"
			getRootCallCount := 0
			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					getRootCallCount++
					return &pb.RootInvocation{Name: in.Name}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: "test.Foo",
								Results: []*pb.TestResult{
									{
										TestId:      "test.Foo",
										ResultId:    "result-1",
										VariantHash: "a24ef92542e11200",
									},
								},
							},
						},
					}, nil
				},
			}
			ids, err := ExtractIDs(ctx, client, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8671749950226328289"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)
			assert.Loosely(t, getRootCallCount, should.Equal(1))
		})

		t.Run(`Milo invocation URL`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/inv/build-8671749950226328289"
			ids, err := ExtractIDs(ctx, legacyMockClient(), nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8671749950226328289"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)
		})

		t.Run(`RPC fallback detects legacy invocation`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
				getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
					return &pb.Invocation{Name: in.Name}, nil
				},
			}
			ids, err := ExtractIDs(ctx, client, nil, "custom-legacy-inv", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("custom-legacy-inv"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)
		})

		t.Run(`AnTS TR ID representing a module error`, func(t *ftt.Test) {
			srv := newMockAntsServer()
			defer srv.Close()

			ctx := base.WithAntsHTTPClient(ctx, srv.Client(), srv.URL)

			ids, err := ExtractIDs(ctx, nil, nil, "TR08930357531778741", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i14600010609614895"))
			assert.Loosely(t, ids.ModuleName, should.Equal("CellBroadcastReceiverMTS"))
			assert.Loosely(t, ids.WorkUnitID, should.Equal("ants-wu98100269380597657"))
			assert.Loosely(t, ids.TestID, should.Equal(""))
			assert.Loosely(t, ids.ResultID, should.Equal(""))
			assert.Loosely(t, ids.SummaryPlaceholder, should.BeFalse)
		})

		t.Run(`Milo module URL without test cases`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8676886509240051393/modules/my_module"
			ids, err := ExtractIDs(ctx, nil, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8676886509240051393"))
			assert.Loosely(t, ids.ModuleName, should.Equal("my_module"))

			escapedURL := "https://ci.chromium.org/ui/test-investigate/invocations/build-8676886509240051393/modules/%2F%2Fchrome%3Achrome_private_code_test"
			idsEscaped, err := ExtractIDs(ctx, nil, nil, escapedURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsEscaped.InvocationID, should.Equal("build-8676886509240051393"))
			assert.Loosely(t, idsEscaped.ModuleName, should.Equal("//chrome:chrome_private_code_test"))
		})
	})
}

func TestPrintExtractedIDs(t *testing.T) {
	ftt.Run(`PrintExtractedIDs`, t, func(t *ftt.Test) {
		t.Run(`Human-readable legacy output`, func(t *ftt.Test) {
			extracted := &ExtractedIDs{
				InvocationID: "build-8671749950226328289",
				TestID:       "ninja://chromeos:chrome_all_tast_tests/tast.cryptohome.UssMigrationPasswordPin",
				ResultID:     "ee50005a-00024",
				ArtifactID:   "summary_node",
				VariantHash:  "a24ef92542e11200",
				Legacy:       true,
			}
			var buf bytes.Buffer
			err := printExtractedIDs(&buf, extracted, false)
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     build-8671749950226328289\n"))
			assert.Loosely(t, out, should.ContainSubstring("Test ID:           ninja://chromeos:chrome_all_tast_tests/tast.cryptohome.UssMigrationPasswordPin\n"))
			assert.Loosely(t, out, should.ContainSubstring("Result ID:         ee50005a-00024\n"))
			assert.Loosely(t, out, should.ContainSubstring("Artifact ID:       summary_node\n"))
			assert.Loosely(t, out, should.ContainSubstring("Variant Hash:      a24ef92542e11200\n"))
			assert.Loosely(t, out, should.ContainSubstring("Legacy:            true\n"))
			assert.Loosely(t, out, should.NotContainSubstring("subsequent commands require -legacy"))
		})

		t.Run(`Human-readable non-legacy output`, func(t *ftt.Test) {
			extracted := &ExtractedIDs{
				InvocationID: "ants-i77100010600769898",
				WorkUnitID:   "ants-wu17100269020689387",
				TestID:       ":CellBroadcastServiceTests!junit:test#testCase",
				ResultID:     "r1",
				VariantHash:  "varhash123",
				Legacy:       false,
			}
			var buf bytes.Buffer
			err := printExtractedIDs(&buf, extracted, false)
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     ants-i77100010600769898\n"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit ID:      ants-wu17100269020689387\n"))
			assert.Loosely(t, out, should.NotContainSubstring("Legacy:"))
			assert.Loosely(t, out, should.NotContainSubstring("Note: This is a legacy invocation"))
		})

		t.Run(`Human-readable output with module name`, func(t *ftt.Test) {
			extracted := &ExtractedIDs{
				InvocationID: "build-8676886509240051393",
				ModuleName:   "my_module",
			}
			var buf bytes.Buffer
			err := printExtractedIDs(&buf, extracted, false)
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     build-8676886509240051393\n"))
			assert.Loosely(t, out, should.ContainSubstring("Module Name:       my_module\n"))
		})

		t.Run(`Human-readable and JSON output for AnTS summary placeholder`, func(t *ftt.Test) {
			extracted := &ExtractedIDs{
				InvocationID:       "ants-i18000010612961162",
				WorkUnitID:         "ants-wu51300269529723640",
				SummaryPlaceholder: true,
			}
			var buf bytes.Buffer
			err := printExtractedIDs(&buf, extracted, false)
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     ants-i18000010612961162\n"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit ID:      ants-wu51300269529723640\n"))
			assert.Loosely(t, out, should.ContainSubstring("Note: The AnTS test result in this target is an invocation-level summary placeholder (Test Case: #.), so -testid and -resultid are not emitted."))
			assert.Loosely(t, out, should.ContainSubstring("luci verdict list -invocationid=ants-i18000010612961162"))
			assert.Loosely(t, out, should.ContainSubstring("luci work-unit get -invocationid=ants-i18000010612961162 -workunitid=ants-wu51300269529723640"))
			assert.Loosely(t, out, should.ContainSubstring("luci artifact list -invocationid=ants-i18000010612961162"))

			// Without WorkUnitID, defaults to -workunitid=root in guidance
			extractedNoWU := &ExtractedIDs{
				InvocationID:       "ants-i56900010616552061",
				SummaryPlaceholder: true,
			}
			var bufNoWU bytes.Buffer
			assert.Loosely(t, printExtractedIDs(&bufNoWU, extractedNoWU, false), should.BeNil)
			assert.Loosely(t, bufNoWU.String(), should.ContainSubstring("luci work-unit get -invocationid=ants-i56900010616552061 -workunitid=root"))

			// JSON output includes summary_placeholder: true
			var jsonBuf bytes.Buffer
			assert.Loosely(t, printExtractedIDs(&jsonBuf, extracted, true), should.BeNil)
			assert.Loosely(t, jsonBuf.String(), should.ContainSubstring(`"summary_placeholder": true`))
		})

		t.Run(`Human-readable output with build ID does not print builder or build number`, func(t *ftt.Test) {
			extracted := &ExtractedIDs{
				BuildID:      "8738491827364512345",
				InvocationID: "build-8738491827364512345",
			}
			var buf bytes.Buffer
			err := printExtractedIDs(&buf, extracted, false)
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Build ID:          8738491827364512345\n"))
			assert.Loosely(t, out, should.NotContainSubstring("Builder:"))
			assert.Loosely(t, out, should.NotContainSubstring("Build Number:"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     build-8738491827364512345\n"))
		})

		t.Run(`JSON output with legacy`, func(t *ftt.Test) {
			extracted := &ExtractedIDs{
				InvocationID: "build-8671749950226328289",
				TestID:       "ninja://test",
				Legacy:       true,
			}
			var buf bytes.Buffer
			err := printExtractedIDs(&buf, extracted, true)
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring(`"legacy": true`))
		})

		t.Run(`JSON output without legacy`, func(t *ftt.Test) {
			extracted := &ExtractedIDs{
				InvocationID: "ants-i77100010600769898",
				Legacy:       false,
			}
			var buf bytes.Buffer
			err := printExtractedIDs(&buf, extracted, true)
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()
			assert.Loosely(t, out, should.NotContainSubstring(`"legacy"`))
		})

		t.Run(`JSON output with build ID does not contain builder or build_number`, func(t *ftt.Test) {
			extracted := &ExtractedIDs{
				BuildID:      "8738491827364512345",
				InvocationID: "build-8738491827364512345",
			}
			var buf bytes.Buffer
			err := printExtractedIDs(&buf, extracted, true)
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring(`"build_id": "8738491827364512345"`))
			assert.Loosely(t, out, should.NotContainSubstring(`"builder"`))
			assert.Loosely(t, out, should.NotContainSubstring(`"build_number"`))
		})

		t.Run(`ExtractIDs unescapes '#' and '/' in canonical artifact resource names and URLs`, func(t *ftt.Test) {
			ctx := context.Background()
			escapedURL := "https://results.usercontent.cr.dev/invocations/build-8674628400464446193/artifacts/tradefed%2Fpkg.Class%23Method-screenshot.png?token=abc"
			ids, err := ExtractIDs(ctx, nil, nil, escapedURL, true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8674628400464446193"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("tradefed/pkg.Class#Method-screenshot.png"))

			escapedResourceName := "invocations/build-8674628400464446193/artifacts/tradefed%2Fpkg.Class%23Method-screenshot.png"
			ids2, err := ExtractIDs(ctx, nil, nil, escapedResourceName, true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.InvocationID, should.Equal("build-8674628400464446193"))
			assert.Loosely(t, ids2.ArtifactID, should.Equal("tradefed/pkg.Class#Method-screenshot.png"))
		})

		t.Run(`ExtractIDs extracts BuildID, InvocationID, StepName, and LogName from LogDog buildbucket URLs`, func(t *ftt.Test) {
			ctx := context.Background()
			viewURL := "https://logs.chromium.org/logs/chrome/buildbucket/cr-buildbucket/8669362094518613393/+/u/generate_build_files/stdout"
			ids, err := ExtractIDs(ctx, nil, nil, viewURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.BuildID, should.Equal("8669362094518613393"))
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8669362094518613393"))
			assert.Loosely(t, ids.StepName, should.Equal("generate_build_files"))
			assert.Loosely(t, ids.LogName, should.Equal("stdout"))

			logdogURL := "logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket/8738491827364512345/+/u/compile/stdout"
			ids2, err := ExtractIDs(ctx, nil, nil, logdogURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.BuildID, should.Equal("8738491827364512345"))
			assert.Loosely(t, ids2.InvocationID, should.Equal("build-8738491827364512345"))
			assert.Loosely(t, ids2.StepName, should.Equal("compile"))
			assert.Loosely(t, ids2.LogName, should.Equal("stdout"))

			// Resolves canonical Buildbucket step and log names when sanitized in LogDog URL
			sanitizedURL := "https://logs.chromium.org/logs/chrome/buildbucket/cr-buildbucket/8669362094518613393/+/u/builder_cache/check_if_empty/l_execution_details"
			bbMock := &mockBuildsClient{
				getBuild: func(ctx context.Context, in *bbpb.GetBuildRequest) (*bbpb.Build, error) {
					return &bbpb.Build{
						Id: 8669362094518613393,
						Steps: []*bbpb.Step{
							{
								Name: "builder cache|check if empty",
								Logs: []*bbpb.Log{
									{
										Name:    "$execution details",
										ViewUrl: sanitizedURL,
										Url:     "logdog://logs.chromium.org/chrome/buildbucket/cr-buildbucket/8669362094518613393/+/u/builder_cache/check_if_empty/l_execution_details",
									},
								},
							},
						},
					}, nil
				},
			}
			ids3, err := ExtractIDs(ctx, nil, bbMock, sanitizedURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids3.BuildID, should.Equal("8669362094518613393"))
			assert.Loosely(t, ids3.StepName, should.Equal("builder cache|check if empty"))
			assert.Loosely(t, ids3.LogName, should.Equal("$execution details"))

			var buf bytes.Buffer
			assert.Loosely(t, printExtractedIDs(&buf, ids3, false), should.BeNil)
			assert.Loosely(t, buf.String(), should.ContainSubstring("Step Name:         builder cache|check if empty\n"))
			assert.Loosely(t, buf.String(), should.ContainSubstring("Log Name:          $execution details\n"))
		})

		t.Run(`ExtractIDs extracts LUCI Analysis rule/cluster URLs and resource names`, func(t *ftt.Test) {
			ctx := context.Background()

			// Milo / LUCI Analysis rule URLs
			miloRuleURL := "https://luci-milo.appspot.com/ui/tests/p/turquoise/rules/32ae75255a4aa807897d8fa20137e7d5"
			idsMilo, err := ExtractIDs(ctx, nil, nil, miloRuleURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsMilo.Project, should.Equal("turquoise"))
			assert.Loosely(t, idsMilo.RuleID, should.Equal("32ae75255a4aa807897d8fa20137e7d5"))
			assert.Loosely(t, idsMilo.ClusterAlgorithm, should.Equal("rules"))
			assert.Loosely(t, idsMilo.ClusterID, should.Equal("32ae75255a4aa807897d8fa20137e7d5"))

			analysisRuleURL := "https://luci-analysis.appspot.com/p/chromium/rules/deadbeef0123456789abcdef01234567"
			idsAnalysis, err := ExtractIDs(ctx, nil, nil, analysisRuleURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsAnalysis.Project, should.Equal("chromium"))
			assert.Loosely(t, idsAnalysis.RuleID, should.Equal("deadbeef0123456789abcdef01234567"))
			assert.Loosely(t, idsAnalysis.ClusterAlgorithm, should.Equal("rules"))
			assert.Loosely(t, idsAnalysis.ClusterID, should.Equal("deadbeef0123456789abcdef01234567"))

			ciRuleURL := "https://ci.chromium.org/ui/tests/p/chromium/rules/0123456789abcdef0123456789abcdef?selectedMetrics=human-cls-failed-presubmit,builds-failed-due-to-flaky-tests"
			idsCI, err := ExtractIDs(ctx, nil, nil, ciRuleURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsCI.Project, should.Equal("chromium"))
			assert.Loosely(t, idsCI.RuleID, should.Equal("0123456789abcdef0123456789abcdef"))
			assert.Loosely(t, idsCI.ClusterAlgorithm, should.Equal("rules"))
			assert.Loosely(t, idsCI.ClusterID, should.Equal("0123456789abcdef0123456789abcdef"))

			// Suggested cluster URL (non-rule algorithm)
			clusterURL := "https://luci-milo.appspot.com/ui/tests/p/chromium/clusters/reason-v6/fedcba9876543210fedcba9876543210"
			idsCluster, err := ExtractIDs(ctx, nil, nil, clusterURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsCluster.Project, should.Equal("chromium"))
			assert.Loosely(t, idsCluster.RuleID, should.Equal(""))
			assert.Loosely(t, idsCluster.ClusterAlgorithm, should.Equal("reason-v6"))
			assert.Loosely(t, idsCluster.ClusterID, should.Equal("fedcba9876543210fedcba9876543210"))
			assert.Loosely(t, idsCluster.IsEmpty(), should.BeFalse)

			// Rule cluster URL (/clusters/rules/<id>) also populates RuleID
			ruleClusterURL := "https://luci-analysis.appspot.com/p/turquoise/clusters/rules/32ae75255a4aa807897d8fa20137e7d5/"
			idsRuleCluster, err := ExtractIDs(ctx, nil, nil, ruleClusterURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsRuleCluster.Project, should.Equal("turquoise"))
			assert.Loosely(t, idsRuleCluster.RuleID, should.Equal("32ae75255a4aa807897d8fa20137e7d5"))
			assert.Loosely(t, idsRuleCluster.ClusterAlgorithm, should.Equal("rules"))
			assert.Loosely(t, idsRuleCluster.ClusterID, should.Equal("32ae75255a4aa807897d8fa20137e7d5"))

			// Canonical LUCI Analysis resource names (including embedded /projects/... path and suggested cluster)
			ruleRes, err := ExtractIDs(ctx, nil, nil, "projects/turquoise/rules/32ae75255a4aa807897d8fa20137e7d5", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ruleRes.Project, should.Equal("turquoise"))
			assert.Loosely(t, ruleRes.RuleID, should.Equal("32ae75255a4aa807897d8fa20137e7d5"))
			assert.Loosely(t, ruleRes.ClusterAlgorithm, should.Equal("rules"))
			assert.Loosely(t, ruleRes.ClusterID, should.Equal("32ae75255a4aa807897d8fa20137e7d5"))

			urlEmbeddedRes, err := ExtractIDs(ctx, nil, nil, "https://analysis.api.luci.app/projects/chromium/clusters/testname-v4/abcdef0123456789abcdef0123456789", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, urlEmbeddedRes.Project, should.Equal("chromium"))
			assert.Loosely(t, urlEmbeddedRes.RuleID, should.Equal(""))
			assert.Loosely(t, urlEmbeddedRes.ClusterAlgorithm, should.Equal("testname-v4"))
			assert.Loosely(t, urlEmbeddedRes.ClusterID, should.Equal("abcdef0123456789abcdef0123456789"))

			clusterFailuresRes, err := ExtractIDs(ctx, nil, nil, "projects/turquoise/clusters/rules/32ae75255a4aa807897d8fa20137e7d5/failures", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, clusterFailuresRes.Project, should.Equal("turquoise"))
			assert.Loosely(t, clusterFailuresRes.RuleID, should.Equal("32ae75255a4aa807897d8fa20137e7d5"))
			assert.Loosely(t, clusterFailuresRes.ClusterAlgorithm, should.Equal("rules"))
			assert.Loosely(t, clusterFailuresRes.ClusterID, should.Equal("32ae75255a4aa807897d8fa20137e7d5"))

			// Test history URL also populates Project
			testHistoryRealm := "https://ci.chromium.org/ui/test/chromium:ci/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case"
			idsHistory, err := ExtractIDs(ctx, nil, nil, testHistoryRealm, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsHistory.Project, should.Equal("chromium"))
			assert.Loosely(t, idsHistory.TestID, should.Equal("ninja://chrome/test:browser_tests/MyTest.Case"))

			// Print output formatting (text & JSON)
			var textBuf bytes.Buffer
			assert.Loosely(t, printExtractedIDs(&textBuf, idsMilo, false), should.BeNil)
			assert.Loosely(t, textBuf.String(), should.ContainSubstring("Project:           turquoise\n"))
			assert.Loosely(t, textBuf.String(), should.ContainSubstring("Rule ID:           32ae75255a4aa807897d8fa20137e7d5\n"))
			assert.Loosely(t, textBuf.String(), should.ContainSubstring("Cluster Algorithm: rules\n"))
			assert.Loosely(t, textBuf.String(), should.ContainSubstring("Cluster ID:        32ae75255a4aa807897d8fa20137e7d5\n"))

			var jsonBuf bytes.Buffer
			assert.Loosely(t, printExtractedIDs(&jsonBuf, idsMilo, true), should.BeNil)
			assert.Loosely(t, jsonBuf.String(), should.ContainSubstring(`"project": "turquoise"`))
			assert.Loosely(t, jsonBuf.String(), should.ContainSubstring(`"rule_id": "32ae75255a4aa807897d8fa20137e7d5"`))
		})

		t.Run(`Bug URL and shorthand extraction`, func(t *ftt.Test) {
			ctx := context.Background()
			cases := []struct {
				input  string
				wantID string
			}{
				{"b/553974454", "553974454"},
				{"http://b/553974454", "553974454"},
				{"https://b/553974454", "553974454"},
				{"http://b.corp.google.com/issues/553974454", "553974454"},
				{"https://b.corp.google.com/issues/553974454", "553974454"},
				{"http://issuetracker.google.com/issues/553974454", "553974454"},
				{"https://issuetracker.google.com/issues/553974454", "553974454"},
				{"http://issues.chromium.org/issues/553974454", "553974454"},
				{"https://issues.chromium.org/issues/553974454", "553974454"},
				{"http://crbug.com/553974454", "553974454"},
				{"https://crbug.com/553974454", "553974454"},
				{"crbug.com/553974454", "553974454"},
				{"b/553974454#comment3", "553974454"},
				{"https://issues.chromium.org/issues/553974454#comment2", "553974454"},
			}
			for _, tc := range cases {
				extracted, err := ExtractIDs(ctx, nil, nil, tc.input, false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, extracted.BugID, should.Equal(tc.wantID))
				assert.Loosely(t, extracted.IsEmpty(), should.BeFalse)
			}

			// Bare numbers must NOT be matched as BugID (treated as build/inv ID)
			bare, err := ExtractIDs(ctx, nil, nil, "8673802696052024673", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bare.BugID, should.Equal(""))
			assert.Loosely(t, bare.InvocationID, should.Equal("build-8673802696052024673"))

			// Invalid bug shorthand does not extract BugID
			invalid, err := ExtractIDs(ctx, nil, nil, "b/not-a-number", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invalid.BugID, should.Equal(""))

			// Output formatting for BugID
			ids := &ExtractedIDs{BugID: "553974454"}
			var textBuf bytes.Buffer
			assert.Loosely(t, printExtractedIDs(&textBuf, ids, false), should.BeNil)
			assert.Loosely(t, textBuf.String(), should.ContainSubstring("Bug ID:            553974454\n"))

			var derivedBuf bytes.Buffer
			PrintDerivedIDs(&derivedBuf, ids)
			assert.Loosely(t, derivedBuf.String(), should.Equal("Bug ID:            553974454\n\n"))

			var jsonBuf bytes.Buffer
			assert.Loosely(t, printExtractedIDs(&jsonBuf, ids, true), should.BeNil)
			assert.Loosely(t, jsonBuf.String(), should.ContainSubstring(`"bug_id": "553974454"`))
		})

		t.Run(`Gerrit CL URL detection and hint`, func(t *ftt.Test) {
			ctx := context.Background()

			t.Run(`Gerrit URL emits Gerrit CL`, func(t *ftt.Test) {
				url := "https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529/10"
				extracted, err := ExtractIDs(ctx, nil, nil, url, false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, extracted.GerritURL, should.Equal(url))
				assert.Loosely(t, extracted.IsEmpty(), should.BeFalse)

				var textBuf bytes.Buffer
				assert.Loosely(t, printExtractedIDs(&textBuf, extracted, false), should.BeNil)
				assert.Loosely(t, textBuf.String(), should.Equal("Gerrit CL:         https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529/10\n"))

				var jsonBuf bytes.Buffer
				assert.Loosely(t, printExtractedIDs(&jsonBuf, extracted, true), should.BeNil)
				assert.Loosely(t, jsonBuf.String(), should.ContainSubstring(`"gerrit_url": "https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529/10"`))
			})

			t.Run(`Short Gerrit URL crrev.com/c/...`, func(t *ftt.Test) {
				url := "https://crrev.com/c/8565529"
				extracted, err := ExtractIDs(ctx, nil, nil, url, false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, extracted.GerritURL, should.Equal(url))

				var textBuf bytes.Buffer
				assert.Loosely(t, printExtractedIDs(&textBuf, extracted, false), should.BeNil)
				assert.Loosely(t, textBuf.String(), should.Equal("Gerrit CL:         https://crrev.com/c/8565529\n"))
			})

			t.Run(`ids command prints Gerrit CL and hint`, func(t *ftt.Test) {
				cmd := Cmd(nil)
				run := cmd.CommandRun().(*idsRun)
				var out bytes.Buffer
				run.out = &out

				code := run.Run(&cli.Application{Name: "luci"}, []string{
					"https://crrev.com/c/8565529",
				}, nil)
				assert.Loosely(t, code, should.Equal(0))
				expected := "Gerrit CL:         https://crrev.com/c/8565529\n\nHint: a Gerrit CL has multiple tryjob builds; to list them, run:\n  luci build list -cl https://crrev.com/c/8565529\n"
				assert.Loosely(t, out.String(), should.Equal(expected))
			})
		})

		t.Run(`PrintMissingFlagsError prints derived IDs and next-step hints`, func(t *ftt.Test) {
			var buf bytes.Buffer
			PrintMissingFlagsError(&buf, &ExtractedIDs{GerritURL: "https://crrev.com/c/8565529"}, "flag -buildid (or a positional build URL/ID) is required")
			expected := "Gerrit CL:         https://crrev.com/c/8565529\n\nflag -buildid (or a positional build URL/ID) is required\nHint: a Gerrit CL has multiple tryjob builds; to list them, run:\n  luci build list -cl https://crrev.com/c/8565529\n"
			assert.Loosely(t, buf.String(), should.Equal(expected))

			buf.Reset()
			PrintMissingFlagsError(&buf, &ExtractedIDs{BugID: "553974454"}, "flag -buildid (or a positional build URL/ID) is required")
			assert.Loosely(t, buf.String(), should.ContainSubstring("Bug ID:            553974454\n"))
			assert.Loosely(t, buf.String(), should.ContainSubstring("Hint: to find failing invocations and tests for this bug, run:\n  luci analysis failures -bugid 553974454\n"))

			buf.Reset()
			PrintMissingFlagsError(&buf, &ExtractedIDs{InvocationID: "ants-i123"}, "flags -invocationid and -testid (or a positional URL/ID containing them) are required")
			assert.Loosely(t, buf.String(), should.ContainSubstring("Invocation ID:     ants-i123\n"))
			assert.Loosely(t, buf.String(), should.ContainSubstring("Hint: to list failing test verdicts and test IDs in this invocation, run:\n  luci verdict list -invocationid ants-i123\n"))

			buf.Reset()
			PrintMissingFlagsError(&buf, &ExtractedIDs{InvocationID: "ants-i123", TestID: ":Mod!junit:pkg:Cls#m", VariantHash: "abcd"}, "flags -invocationid, -testid, and -resultid (or a positional URL/ID containing them) are required")
			assert.Loosely(t, buf.String(), should.ContainSubstring(`luci verdict get -invocationid ants-i123 -testid ":Mod!junit:pkg:Cls#m" -varianthash abcd`))

			buf.Reset()
			PrintMissingFlagsError(&buf, &ExtractedIDs{InvocationID: "ants-i123", TestID: ":Mod!junit:pkg:Cls#m", ResultID: "r1"}, "flag -artifactid is required (or a positional URL/ID containing an artifact ID)")
			assert.Loosely(t, buf.String(), should.ContainSubstring(`luci artifact list -invocationid ants-i123 -testid ":Mod!junit:pkg:Cls#m" -resultid r1`))
		})
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCustomHostsForwarding(t *testing.T) {
	t.Parallel()

	ftt.Run(`Lazy client creation respects custom ResultDB and Buildbucket hosts`, t, func(t *ftt.Test) {
		var seenRDBHost, seenBBHost string
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "application/prpc; encoding=binary")
			header.Set("X-Prpc-Grpc-Code", "0")
			var msg proto.Message
			switch req.URL.Path {
			case "/prpc/buildbucket.v2.Builds/GetBuild":
				seenBBHost = req.URL.Host
				msg = &bbpb.Build{Id: 8765432109876543210}
			case "/prpc/luci.resultdb.v1.ResultDB/GetRootInvocation":
				seenRDBHost = req.URL.Host
				msg = &pb.RootInvocation{Name: "rootInvocations/build-8765432109876543210"}
			case "/prpc/luci.resultdb.v1.ResultDB/QueryTestVerdicts":
				seenRDBHost = req.URL.Host
				msg = &pb.QueryTestVerdictsResponse{}
			default:
				msg = &pb.QueryTestVerdictsResponse{}
			}
			rawBody, err := proto.Marshal(msg)
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(bytes.NewReader(rawBody)),
				Request:    req,
			}, nil
		})

		newTestAuthFlags := func() *base.AuthFlags {
			opts := chromeinfra.DefaultAuthOptions()
			opts.Method = auth.UserCredentialsMethod
			opts.SecretsDir = t.TempDir()
			opts.Transport = transport
			return &base.AuthFlags{DefaultOpts: opts}
		}

		t.Run(`idsRun.Run forwards -host and -bb-host flags`, func(t *ftt.Test) {
			seenRDBHost, seenBBHost = "", ""
			af := newTestAuthFlags()
			cmd := Cmd(af)
			run := cmd.CommandRun().(*idsRun)
			var out bytes.Buffer
			run.out = &out

			code := run.Run(&cli.Application{Name: "luci"}, []string{
				"-host=custom-rdb.example.com",
				"-bb-host=custom-bb.example.com",
				"https://ci.chromium.org/ui/p/chromium/builders/ci/Linux%20Tests/12345",
			}, nil)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, seenBBHost, should.Equal("custom-bb.example.com"))
			assert.Loosely(t, seenRDBHost, should.Equal("custom-rdb.example.com"))
			assert.Loosely(t, out.String(), should.ContainSubstring("Build ID:          8765432109876543210"))
		})

		t.Run(`ExtractWithAuth forwards TargetBindings ResultDBHost and BuildbucketHost`, func(t *ftt.Test) {
			seenRDBHost, seenBBHost = "", ""
			af := newTestAuthFlags()
			var fs flag.FlagSet
			af.Register(&fs)
			ctx := context.Background()

			var buildID string
			_, err := ExtractWithAuth(ctx, af, nil, nil, "chromium/ci/Linux Tests/12345", TargetBindings{
				BuildID:         &buildID,
				BuildbucketHost: "staging-bb.example.com",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, seenBBHost, should.Equal("staging-bb.example.com"))
			assert.Loosely(t, buildID, should.Equal("8765432109876543210"))

			var testID string
			_, err = ExtractWithAuth(ctx, af, nil, nil, "https://ci.chromium.org/ui/test-investigate/invocations/build-8765432109876543210/modules/ModA/schemes/junit/variants/abcd1234abcd1234/cases/pkg.Cls%23method", TargetBindings{
				TestID:       &testID,
				ResultDBHost: "staging-rdb.example.com",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, seenRDBHost, should.Equal("staging-rdb.example.com"))
		})
	})
}
