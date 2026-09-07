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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockResultDBClient struct {
	pb.ResultDBClient
	queryTestVerdicts     func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error)
	queryTestResults      func(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error)
	queryTestExonerations func(ctx context.Context, in *pb.QueryTestExonerationsRequest) (*pb.QueryTestExonerationsResponse, error)
	getRootInvocation     func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation         func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
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

func TestExtractIDs(t *testing.T) {
	ftt.Run(`ExtractIDs`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Milo structured verdict URL with query params`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8676886509240051393/modules/%2F%2Fchrome%3Achrome_private_code_test/schemes/single/variants/b7de9035241e76cc/cases/*fixture?artifact=summary_node&result=0"
			ids, err := ExtractIDs(ctx, legacyMockClient(), url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8676886509240051393"))
			assert.Loosely(t, ids.VariantHash, should.Equal("b7de9035241e76cc"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("summary_node"))
			assert.Loosely(t, ids.ResultID, should.Equal("0"))
			assert.Loosely(t, ids.TestID, should.Equal("*fixture"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)

			idsNoClient, err := ExtractIDs(ctx, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsNoClient.Legacy, should.BeFalse)
		})

		t.Run(`Canonical test result artifact name`, func(t *ftt.Test) {
			name := "invocations/build-123/tests/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case/results/0/artifacts/stdout"
			ids, err := ExtractIDs(ctx, legacyMockClient(), name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.TestID, should.Equal("ninja://chrome/test:browser_tests/MyTest.Case"))
			assert.Loosely(t, ids.ResultID, should.Equal("0"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("stdout"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)
		})

		t.Run(`Canonical test result name`, func(t *ftt.Test) {
			name := "invocations/build-123/tests/ninja%3A%2F%2Fchrome%2Ftest/results/1"
			ids, err := ExtractIDs(ctx, legacyMockClient(), name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.TestID, should.Equal("ninja://chrome/test"))
			assert.Loosely(t, ids.ResultID, should.Equal("1"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)
		})

		t.Run(`Canonical work unit artifact name`, func(t *ftt.Test) {
			name := "rootInvocations/ants-i123/workUnits/wu-1/artifacts/log.txt"
			ids, err := ExtractIDs(ctx, nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i123"))
			assert.Loosely(t, ids.WorkUnitID, should.Equal("wu-1"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("log.txt"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)
		})

		t.Run(`Canonical work unit name`, func(t *ftt.Test) {
			name := "rootInvocations/build-123/workUnits/run-tests"
			ids, err := ExtractIDs(ctx, nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.WorkUnitID, should.Equal("run-tests"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)
		})

		t.Run(`Milo build URL`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/b/8676886509240051393"
			ids, err := ExtractIDs(ctx, legacyMockClient(), url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8676886509240051393"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)

			urlShort := "https://ci.chromium.org/b/8676886509240051393"
			idsShort, err := ExtractIDs(ctx, legacyMockClient(), urlShort, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsShort.InvocationID, should.Equal("build-8676886509240051393"))

			urlBuilder := "https://ci.chromium.org/ui/p/chromium/builders/ci/linux-rel/8676886509240051393"
			ids2, err := ExtractIDs(ctx, legacyMockClient(), urlBuilder, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.InvocationID, should.Equal("build-8676886509240051393"))
			assert.Loosely(t, ids2.Legacy, should.BeTrue)

			urlBuilderTab := "https://ci.chromium.org/ui/p/chromium/builders/ci/linux-rel/8676886509240051393/test-results"
			ids3, err := ExtractIDs(ctx, legacyMockClient(), urlBuilderTab, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids3.InvocationID, should.Equal("build-8676886509240051393"))

			urlBuilderBId := "https://ci.chromium.org/ui/p/chromium/builders/ci/linux-rel/b8676886509240051393/overview"
			ids4, err := ExtractIDs(ctx, legacyMockClient(), urlBuilderBId, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids4.InvocationID, should.Equal("build-8676886509240051393"))
		})

		t.Run(`Milo test investigation and test history URLs`, func(t *ftt.Test) {
			// Invocation subtabs
			invTests := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/tests"
			idsTests, err := ExtractIDs(ctx, legacyMockClient(), invTests, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsTests.InvocationID, should.Equal("build-8671749950226328289"))

			invDetails := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/details"
			idsDetails, err := ExtractIDs(ctx, legacyMockClient(), invDetails, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsDetails.InvocationID, should.Equal("build-8671749950226328289"))

			invArtifacts := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/artifacts"
			idsArtifacts, err := ExtractIDs(ctx, legacyMockClient(), invArtifacts, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsArtifacts.InvocationID, should.Equal("build-8671749950226328289"))

			// Old test investigation route: invocations/:invId/tests/:testId/variants/:variantHash
			oldTI := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/tests/ninja%3A%2F%2Ftest/variants/a24ef92542e11200"
			idsOldTI, err := ExtractIDs(ctx, nil, oldTI, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsOldTI.InvocationID, should.Equal("build-8671749950226328289"))
			assert.Loosely(t, idsOldTI.TestID, should.Equal("ninja://test"))
			assert.Loosely(t, idsOldTI.VariantHash, should.Equal("a24ef92542e11200"))

			// Test history route: test/:projectOrRealm/:testId
			testHistory := "https://ci.chromium.org/ui/test/chromium/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case?q=V%3Aos%3DLinux"
			idsHistory, err := ExtractIDs(ctx, nil, testHistory, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsHistory.TestID, should.Equal("ninja://chrome/test:browser_tests/MyTest.Case"))

			// Blamelist route: p/:project/tests/:testId/variants/:variantHash/refs/:refHash/blamelist
			blamelist := "https://ci.chromium.org/ui/labs/p/chromium/tests/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case/variants/a24ef92542e11200/refs/deadbeef/blamelist#CP-12345"
			idsBlamelist, err := ExtractIDs(ctx, nil, blamelist, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsBlamelist.TestID, should.Equal("ninja://chrome/test:browser_tests/MyTest.Case"))
			assert.Loosely(t, idsBlamelist.VariantHash, should.Equal("a24ef92542e11200"))

			// Artifact routes
			artRaw := "https://ci.chromium.org/ui/artifact/raw/invocations/build-12345/artifacts/my-art"
			idsRaw, err := ExtractIDs(ctx, nil, artRaw, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsRaw.InvocationID, should.Equal("build-12345"))
			assert.Loosely(t, idsRaw.ArtifactID, should.Equal("my-art"))

			artDiff := "https://ci.chromium.org/ui/artifact/text-diff/invocations/build-12345/tests/ninja%3A%2F%2Ftest/results/r1/artifacts/stdout"
			idsDiff, err := ExtractIDs(ctx, nil, artDiff, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsDiff.InvocationID, should.Equal("build-12345"))
			assert.Loosely(t, idsDiff.TestID, should.Equal("ninja://test"))
			assert.Loosely(t, idsDiff.ResultID, should.Equal("r1"))
			assert.Loosely(t, idsDiff.ArtifactID, should.Equal("stdout"))
		})

		t.Run(`Standalone invocation resource names`, func(t *ftt.Test) {
			ids, err := ExtractIDs(ctx, legacyMockClient(), "invocations/build-123", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)

			ids2, err := ExtractIDs(ctx, nil, "rootInvocations/ants-i456", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.InvocationID, should.Equal("ants-i456"))
			assert.Loosely(t, ids2.Legacy, should.BeFalse)
		})

		t.Run(`Standalone AnTS Invocation and Work Unit IDs`, func(t *ftt.Test) {
			ids, err := ExtractIDs(ctx, nil, "I77100010600769898", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)

			ids2, err := ExtractIDs(ctx, nil, "WU17100269020689387", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, ids2.Legacy, should.BeFalse)
		})

		t.Run(`AnTS TR ID and ATI URL with mock ants_cli and ResultDB`, func(t *ftt.Test) {
			if runtime.GOOS == "windows" {
				t.Skip("ants_cli mock shell script execution is not supported on Windows")
			}
			tempDir := t.TempDir()
			fakeBin := filepath.Join(tempDir, "ants_cli")
			script := `#!/bin/sh
cat <<EOF
Test Result ID: TR13830335277435395
Test Case: CellBroadcastServiceTests#com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest.testResetAreaInfoWithDefaultSubChanged
Status: pass
Work Unit ID: WU17100269020689387
Invocation ID: I77100010600769898
Run Number: 0
Attempt Number: 0
EOF
`
			_ = os.WriteFile(fakeBin, []byte(script), 0755)
			t.Setenv("ANTS_CLI_PATH", fakeBin)

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
			ids, err := ExtractIDs(ctx, client, atiURL, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, ids.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, ids.TestID, should.Equal(":CellBroadcastServiceTests!junit:test#testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, ids.ResultID, should.Equal("r1"))
			assert.Loosely(t, ids.VariantHash, should.Equal("varhash123"))

			// 2. From direct TR ID
			idsTR, err := ExtractIDs(ctx, client, "TR13830335277435395", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsTR.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, idsTR.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, idsTR.TestID, should.Equal(":CellBroadcastServiceTests!junit:test#testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, idsTR.ResultID, should.Equal("r1"))

			// 3. From ATI URL with query params
			atiURLWithParams := "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898/test/TR13830335277435395/?artifact=stdout&result=custom-res"
			idsParams, err := ExtractIDs(ctx, client, atiURLWithParams, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsParams.InvocationID, should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, idsParams.WorkUnitID, should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, idsParams.TestID, should.Equal(":CellBroadcastServiceTests!junit:test#testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, idsParams.ResultID, should.Equal("custom-res"))
			assert.Loosely(t, idsParams.ArtifactID, should.Equal("stdout"))

			// 4. From placeholder test case "#." (e.g. build failure in AnTS)
			scriptPlaceholder := `#!/bin/sh
cat <<EOF
Test Result ID: TR98730375213734414
Test Case: #.
Status: testError
Work Unit ID:
Invocation ID: I56900010616552061
Run Number: 0
Attempt Number: 0
EOF
`
			_ = os.WriteFile(fakeBin, []byte(scriptPlaceholder), 0755)
			idsPlaceholder, err := ExtractIDs(ctx, nil, "TR98730375213734414", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsPlaceholder.InvocationID, should.Equal("ants-i56900010616552061"))
			assert.Loosely(t, idsPlaceholder.TestID, should.Equal(""))
		})

		t.Run(`Android Build tests view URL with query param`, func(t *ftt.Test) {
			url := "https://android-build.googleplex.com/builds/tests/view?invocationId=I30800010616707848"
			ids, err := ExtractIDs(ctx, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i30800010616707848"))
			assert.Loosely(t, ids.TestID, should.Equal(""))

			urlCorp := "https://android-build.corp.google.com/builds/tests/view?invocation_id=I30800010616707848"
			idsCorp, err := ExtractIDs(ctx, nil, urlCorp, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsCorp.InvocationID, should.Equal("ants-i30800010616707848"))
			assert.Loosely(t, idsCorp.TestID, should.Equal(""))
		})

		t.Run(`Milo legacy test-investigate URL without client`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8671749950226328289/modules/legacy/schemes/legacy/variants/a24ef92542e11200/cases/ninja%3A%2F%2Fchromeos%3Achrome_all_tast_tests%2Ftast.cryptohome.UssMigrationPasswordPin?artifact=summary_node"
			ids, err := ExtractIDs(ctx, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8671749950226328289"))
			assert.Loosely(t, ids.VariantHash, should.Equal("a24ef92542e11200"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("summary_node"))
			assert.Loosely(t, ids.TestID, should.Equal("ninja://chromeos:chrome_all_tast_tests/tast.cryptohome.UssMigrationPasswordPin"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)

			idsLegacy, err := ExtractIDs(ctx, nil, url, true)
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
			ids, err := ExtractIDs(ctx, client, url, false)
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
			ids, err := ExtractIDs(ctx, client, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8671749950226328289"))
			assert.Loosely(t, ids.Legacy, should.BeFalse)
			assert.Loosely(t, getRootCallCount, should.Equal(1))
		})

		t.Run(`Milo invocation URL`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/inv/build-8671749950226328289"
			ids, err := ExtractIDs(ctx, legacyMockClient(), url, false)
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
			ids, err := ExtractIDs(ctx, client, "custom-legacy-inv", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("custom-legacy-inv"))
			assert.Loosely(t, ids.Legacy, should.BeTrue)
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
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID: build-8671749950226328289\n"))
			assert.Loosely(t, out, should.ContainSubstring("Test ID:       ninja://chromeos:chrome_all_tast_tests/tast.cryptohome.UssMigrationPasswordPin\n"))
			assert.Loosely(t, out, should.ContainSubstring("Result ID:     ee50005a-00024\n"))
			assert.Loosely(t, out, should.ContainSubstring("Artifact ID:   summary_node\n"))
			assert.Loosely(t, out, should.ContainSubstring("Variant Hash:  a24ef92542e11200\n"))
			assert.Loosely(t, out, should.ContainSubstring("Legacy:        true (subsequent commands require -legacy)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Note: This is a legacy invocation. You will need to pass the -legacy flag to subsequent commands (e.g. 'luci verdict', 'luci test-result', 'luci test-result artifact').\n"))
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
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID: ants-i77100010600769898\n"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit ID:  ants-wu17100269020689387\n"))
			assert.Loosely(t, out, should.NotContainSubstring("Legacy:"))
			assert.Loosely(t, out, should.NotContainSubstring("Note: This is a legacy invocation"))
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
	})
}
