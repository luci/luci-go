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
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockResultDBClient struct {
	pb.ResultDBClient
	queryTestVerdicts func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error)
}

func (m *mockResultDBClient) QueryTestVerdicts(ctx context.Context, in *pb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*pb.QueryTestVerdictsResponse, error) {
	if m.queryTestVerdicts != nil {
		return m.queryTestVerdicts(ctx, in)
	}
	return &pb.QueryTestVerdictsResponse{}, nil
}

func TestExtractIDs(t *testing.T) {
	ftt.Run(`ExtractIDs`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Milo structured verdict URL with query params`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8676886509240051393/modules/%2F%2Fchrome%3Achrome_private_code_test/schemes/single/variants/b7de9035241e76cc/cases/*fixture?artifact=summary_node&result=0"
			ids, err := ExtractIDs(ctx, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8676886509240051393"))
			assert.Loosely(t, ids.VariantHash, should.Equal("b7de9035241e76cc"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("summary_node"))
			assert.Loosely(t, ids.ResultID, should.Equal("0"))
		})

		t.Run(`Canonical test result artifact name`, func(t *ftt.Test) {
			name := "invocations/build-123/tests/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case/results/0/artifacts/stdout"
			ids, err := ExtractIDs(ctx, nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.TestID, should.Equal("ninja://chrome/test:browser_tests/MyTest.Case"))
			assert.Loosely(t, ids.ResultID, should.Equal("0"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("stdout"))
		})

		t.Run(`Canonical test result name`, func(t *ftt.Test) {
			name := "invocations/build-123/tests/ninja%3A%2F%2Fchrome%2Ftest/results/1"
			ids, err := ExtractIDs(ctx, nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.TestID, should.Equal("ninja://chrome/test"))
			assert.Loosely(t, ids.ResultID, should.Equal("1"))
		})

		t.Run(`Canonical work unit artifact name`, func(t *ftt.Test) {
			name := "rootInvocations/ants-i123/workUnits/wu-1/artifacts/log.txt"
			ids, err := ExtractIDs(ctx, nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i123"))
			assert.Loosely(t, ids.WorkUnitID, should.Equal("wu-1"))
			assert.Loosely(t, ids.ArtifactID, should.Equal("log.txt"))
		})

		t.Run(`Canonical work unit name`, func(t *ftt.Test) {
			name := "rootInvocations/build-123/workUnits/run-tests"
			ids, err := ExtractIDs(ctx, nil, name, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))
			assert.Loosely(t, ids.WorkUnitID, should.Equal("run-tests"))
		})

		t.Run(`Milo build URL`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/b/8676886509240051393"
			ids, err := ExtractIDs(ctx, nil, url, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-8676886509240051393"))

			urlBuilder := "https://ci.chromium.org/ui/p/chromium/builders/ci/linux-rel/8676886509240051393"
			ids2, err := ExtractIDs(ctx, nil, urlBuilder, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.InvocationID, should.Equal("build-8676886509240051393"))
		})

		t.Run(`Standalone invocation resource names`, func(t *ftt.Test) {
			ids, err := ExtractIDs(ctx, nil, "invocations/build-123", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("build-123"))

			ids2, err := ExtractIDs(ctx, nil, "rootInvocations/ants-i456", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.InvocationID, should.Equal("ants-i456"))
		})

		t.Run(`Standalone AnTS Invocation and Work Unit IDs`, func(t *ftt.Test) {
			ids, err := ExtractIDs(ctx, nil, "I77100010600769898", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.InvocationID, should.Equal("ants-i77100010600769898"))

			ids2, err := ExtractIDs(ctx, nil, "WU17100269020689387", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids2.WorkUnitID, should.Equal("ants-wu17100269020689387"))
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
		})
	})
}
