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

package artifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockTestHistoryClient struct {
	analysispb.TestHistoryClient
	queryRecentPasses func(ctx context.Context, in *analysispb.QueryRecentPassesRequest) (*analysispb.QueryRecentPassesResponse, error)
}

func (m *mockTestHistoryClient) QueryRecentPasses(ctx context.Context, in *analysispb.QueryRecentPassesRequest, opts ...grpc.CallOption) (*analysispb.QueryRecentPassesResponse, error) {
	if m.queryRecentPasses != nil {
		return m.queryRecentPasses(ctx, in)
	}
	return &analysispb.QueryRecentPassesResponse{}, nil
}

func TestCompareCmd(t *testing.T) {
	t.Parallel()

	ftt.Run(`CompareCmd`, t, func(t *ftt.Test) {
		tmpDir := t.TempDir()

		t.Run(`root invocation test result artifact comparison`, func(t *ftt.Test) {
			outFile := filepath.Join(tmpDir, "root_tr.txt")
			var capturedCompareReq *rdbpb.CompareArtifactLinesRequest
			var capturedPassesReq *analysispb.QueryRecentPassesRequest
			queryTestVerdictsCalls := 0

			rdb := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest) (*rdbpb.RootInvocation, error) {
					return &rdbpb.RootInvocation{
						Name:  "rootInvocations/ants-i123",
						Realm: "android:ci",
						Sources: &rdbpb.Sources{
							BaseSources: &rdbpb.Sources_SubmittedAndroidBuild{
								SubmittedAndroidBuild: &rdbpb.SubmittedAndroidBuild{
									DataRealm: "prod",
									Branch:    "git_main",
									BuildId:   12345678,
								},
							},
						},
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest) (*rdbpb.QueryTestVerdictsResponse, error) {
					queryTestVerdictsCalls++
					return &rdbpb.QueryTestVerdictsResponse{
						TestVerdicts: []*rdbpb.TestVerdict{
							{
								TestId: ":module!junit:pkg:Class#testFoo",
								Results: []*rdbpb.TestResult{
									{
										Name:        "rootInvocations/ants-i123/workUnits/wu-1/tests/:module%21junit:pkg:Class%23testFoo/results/r1",
										TestId:      ":module!junit:pkg:Class#testFoo",
										ResultId:    "r1",
										VariantHash: "0123456789abcdef",
									},
								},
							},
						},
					}, nil
				},
				compareArtifactLines: func(ctx context.Context, in *rdbpb.CompareArtifactLinesRequest) (*rdbpb.CompareArtifactLinesResponse, error) {
					capturedCompareReq = in
					return &rdbpb.CompareArtifactLinesResponse{
						Artifacts: []string{
							"rootInvocations/ants-i100/workUnits/wu-pass/tests/:module%21junit:pkg:Class%23testFoo/results/r0/artifacts/logcat.txt",
						},
						FailureOnlyRanges: []*rdbpb.CompareArtifactLinesResponse_FailureOnlyRange{
							{
								StartLine: 10,
								EndLine:   12,
								StartByte: 100,
								EndByte:   200,
								Lines:     []string{"E AndroidRuntime: FATAL EXCEPTION", "E AndroidRuntime: NullPointerException"},
							},
						},
						NextPageToken: "next-page-123",
					}, nil
				},
			}

			th := &mockTestHistoryClient{
				queryRecentPasses: func(ctx context.Context, in *analysispb.QueryRecentPassesRequest) (*analysispb.QueryRecentPassesResponse, error) {
					capturedPassesReq = in
					return &analysispb.QueryRecentPassesResponse{
						PassingResults: []*analysispb.QueryRecentPassesResponse_PassingResult{
							{Name: "rootInvocations/ants-i100/workUnits/wu-pass/tests/:module%21junit:pkg:Class%23testFoo/results/r0"},
						},
					}, nil
				},
			}

			cmd := CompareCmd(nil, ParentTypeUnknown)
			run := cmd.CommandRun().(*artifactCompareRun)
			run.client = rdb
			run.thClient = th
			run.invocationID = "I123"
			run.testID = ":module!junit:pkg:Class#testFoo"
			run.resultID = "r1"
			run.artifactID = "logcat.txt"
			run.pageToken = "prev-page-000"
			run.outputFile = outFile

			code := run.Run(nil, nil, nil)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, queryTestVerdictsCalls, should.Equal(1))
			assert.Loosely(t, capturedPassesReq, should.NotBeNil)
			assert.Loosely(t, capturedPassesReq.Project, should.Equal("android"))
			assert.Loosely(t, capturedPassesReq.TestId, should.Equal(":module!junit:pkg:Class#testFoo"))
			assert.Loosely(t, capturedPassesReq.VariantHash, should.Equal("0123456789abcdef"))
			assert.Loosely(t, capturedPassesReq.Sources.GetSubmittedAndroidBuild().GetBuildId(), should.Equal(int64(12345678)))

			assert.Loosely(t, capturedCompareReq, should.NotBeNil)
			assert.Loosely(t, capturedCompareReq.Name, should.Equal("rootInvocations/ants-i123/workUnits/wu-1/tests/:module%21junit:pkg:Class%23testFoo/results/r1/artifacts/logcat.txt"))
			assert.Loosely(t, capturedCompareReq.PageToken, should.Equal("prev-page-000"))
			assert.Loosely(t, capturedCompareReq.PassingResults, should.Match([]string{
				"rootInvocations/ants-i100/workUnits/wu-pass/tests/:module%21junit:pkg:Class%23testFoo/results/r0",
			}))

			data, err := os.ReadFile(outFile)
			assert.Loosely(t, err, should.BeNil)
			out := string(data)
			assert.Loosely(t, out, should.ContainSubstring("Removed lines matching these 1 passing artifact(s):"))
			assert.Loosely(t, out, should.ContainSubstring("Note: Lines are compared after stripping numbers"))
			assert.Loosely(t, out, should.ContainSubstring("--- Lines 11..12 (bytes 100..200) ---"))
			assert.Loosely(t, out, should.ContainSubstring("E AndroidRuntime: FATAL EXCEPTION"))
			assert.Loosely(t, out, should.ContainSubstring("(More ranges available; pass -page-token=next-page-123 to fetch the next page)"))
		})

		t.Run(`root invocation test result artifact with -workunitid matches resultid variant hash`, func(t *ftt.Test) {
			outFile := filepath.Join(tmpDir, "root_tr_wu.txt")
			var capturedPassesReq *analysispb.QueryRecentPassesRequest

			rdb := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest) (*rdbpb.RootInvocation, error) {
					return &rdbpb.RootInvocation{
						Name:  "rootInvocations/ants-i123",
						Realm: "android:ci",
						Sources: &rdbpb.Sources{
							BaseSources: &rdbpb.Sources_SubmittedAndroidBuild{
								SubmittedAndroidBuild: &rdbpb.SubmittedAndroidBuild{
									DataRealm: "prod",
									Branch:    "git_main",
									BuildId:   12345678,
								},
							},
						},
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest) (*rdbpb.QueryTestVerdictsResponse, error) {
					return &rdbpb.QueryTestVerdictsResponse{
						TestVerdicts: []*rdbpb.TestVerdict{
							{
								TestId: ":module!junit:pkg:Class#testFoo",
								Results: []*rdbpb.TestResult{
									{
										Name:        "rootInvocations/ants-i123/workUnits/wu-other/tests/:module%21junit:pkg:Class%23testFoo/results/r0",
										TestId:      ":module!junit:pkg:Class#testFoo",
										ResultId:    "r0",
										VariantHash: "aaaa1111bbbb2222",
									},
									{
										Name:        "rootInvocations/ants-i123/workUnits/wu-target/tests/:module%21junit:pkg:Class%23testFoo/results/r2",
										TestId:      ":module!junit:pkg:Class#testFoo",
										ResultId:    "r2",
										VariantHash: "cccc3333dddd4444",
									},
								},
							},
						},
					}, nil
				},
				compareArtifactLines: func(ctx context.Context, in *rdbpb.CompareArtifactLinesRequest) (*rdbpb.CompareArtifactLinesResponse, error) {
					return &rdbpb.CompareArtifactLinesResponse{}, nil
				},
			}

			th := &mockTestHistoryClient{
				queryRecentPasses: func(ctx context.Context, in *analysispb.QueryRecentPassesRequest) (*analysispb.QueryRecentPassesResponse, error) {
					capturedPassesReq = in
					return &analysispb.QueryRecentPassesResponse{
						PassingResults: []*analysispb.QueryRecentPassesResponse_PassingResult{
							{Name: "rootInvocations/ants-i100/workUnits/wu-pass/tests/:module%21junit:pkg:Class%23testFoo/results/r0"},
						},
					}, nil
				},
			}

			cmd := CompareCmd(nil, ParentTypeTestResult)
			run := cmd.CommandRun().(*artifactCompareRun)
			run.client = rdb
			run.thClient = th
			run.invocationID = "ants-i123"
			run.workUnitID = "wu-target"
			run.testID = ":module!junit:pkg:Class#testFoo"
			run.resultID = "r2"
			run.artifactID = "logcat.txt"
			run.outputFile = outFile

			code := run.Run(nil, nil, nil)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, capturedPassesReq, should.NotBeNil)
			assert.Loosely(t, capturedPassesReq.VariantHash, should.Equal("cccc3333dddd4444"))
		})

		t.Run(`root invocation work unit artifact without -testid`, func(t *ftt.Test) {
			outFile := filepath.Join(tmpDir, "root_wu.txt")
			var capturedCompareReq *rdbpb.CompareArtifactLinesRequest

			rdb := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest) (*rdbpb.RootInvocation, error) {
					return &rdbpb.RootInvocation{
						Name:  "rootInvocations/ants-i123",
						Realm: "android:ci",
						Sources: &rdbpb.Sources{
							BaseSources: &rdbpb.Sources_SubmittedAndroidBuild{
								SubmittedAndroidBuild: &rdbpb.SubmittedAndroidBuild{
									DataRealm: "prod",
									Branch:    "git_main",
									BuildId:   12345678,
								},
							},
						},
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *rdbpb.QueryTestVerdictsRequest) (*rdbpb.QueryTestVerdictsResponse, error) {
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i123"))
					assert.Loosely(t, in.OrderBy, should.Equal("ui_priority"))
					return &rdbpb.QueryTestVerdictsResponse{
						TestVerdicts: []*rdbpb.TestVerdict{
							{
								TestId: ":module!junit:pkg:Class#testBar",
								TestIdStructured: &rdbpb.TestIdentifier{
									ModuleVariantHash: "fedcba9876543210",
								},
							},
						},
					}, nil
				},
				compareArtifactLines: func(ctx context.Context, in *rdbpb.CompareArtifactLinesRequest) (*rdbpb.CompareArtifactLinesResponse, error) {
					capturedCompareReq = in
					return &rdbpb.CompareArtifactLinesResponse{
						Artifacts: []string{
							"rootInvocations/ants-i100/workUnits/wu-attempt-0/artifacts/host_log_999.txt",
						},
						FailureOnlyRanges: []*rdbpb.CompareArtifactLinesResponse_FailureOnlyRange{
							{
								StartLine: 0,
								EndLine:   1,
								StartByte: 0,
								EndByte:   20,
								Lines:     []string{"Host failure line"},
							},
						},
					}, nil
				},
			}

			th := &mockTestHistoryClient{
				queryRecentPasses: func(ctx context.Context, in *analysispb.QueryRecentPassesRequest) (*analysispb.QueryRecentPassesResponse, error) {
					return &analysispb.QueryRecentPassesResponse{
						PassingResults: []*analysispb.QueryRecentPassesResponse_PassingResult{
							{Name: "rootInvocations/ants-i100/workUnits/wu-pass/tests/:module%21junit:pkg:Class%23testBar/results/r0"},
						},
					}, nil
				},
			}

			cmd := CompareCmd(nil, ParentTypeWorkUnit)
			run := cmd.CommandRun().(*artifactCompareRun)
			run.client = rdb
			run.thClient = th
			run.invocationID = "ants-i123"
			run.workUnitID = "wu-attempt-0"
			run.artifactID = "host_log_111.txt"
			run.outputFile = outFile

			code := run.Run(nil, nil, nil)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, capturedCompareReq, should.NotBeNil)
			assert.Loosely(t, capturedCompareReq.Name, should.Equal("rootInvocations/ants-i123/workUnits/wu-attempt-0/artifacts/host_log_111.txt"))
		})

		t.Run(`legacy invocation test result and invocation artifact comparison`, func(t *ftt.Test) {
			outFile := filepath.Join(tmpDir, "legacy_inv.txt")
			var capturedCompareReq *rdbpb.CompareArtifactLinesRequest

			rdb := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *rdbpb.GetRootInvocationRequest) (*rdbpb.RootInvocation, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
				getInvocation: func(ctx context.Context, in *rdbpb.GetInvocationRequest) (*rdbpb.Invocation, error) {
					return &rdbpb.Invocation{
						Name:  "invocations/build-888",
						Realm: "chromium:ci",
					}, nil
				},
				queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
					return &rdbpb.QueryArtifactsResponse{
						Artifacts: []*rdbpb.Artifact{
							{
								Name:       "invocations/u-task-1/artifacts/tast%2Ffull.txt",
								ArtifactId: "tast/full.txt",
							},
						},
					}, nil
				},
				queryTestVariants: func(ctx context.Context, in *rdbpb.QueryTestVariantsRequest) (*rdbpb.QueryTestVariantsResponse, error) {
					assert.Loosely(t, in.Parent, should.BeEmpty)
					assert.Loosely(t, in.Invocations, should.Match([]string{"invocations/build-888"}))
					return &rdbpb.QueryTestVariantsResponse{
						TestVariants: []*rdbpb.TestVariant{
							{
								TestId:      "ninja://chrome/test:browser_tests/MyTest.Foo",
								VariantHash: "abcdef0123456789",
								SourcesId:   "src_git",
							},
						},
						Sources: map[string]*rdbpb.Sources{
							"src_git": {
								BaseSources: &rdbpb.Sources_GitilesCommit{
									GitilesCommit: &rdbpb.GitilesCommit{
										Host:       "chromium.googlesource.com",
										Project:    "chromium/src",
										Ref:        "refs/heads/main",
										CommitHash: "1234567890abcdef1234567890abcdef12345678",
										Position:   999999,
									},
								},
							},
						},
					}, nil
				},
				compareArtifactLines: func(ctx context.Context, in *rdbpb.CompareArtifactLinesRequest) (*rdbpb.CompareArtifactLinesResponse, error) {
					capturedCompareReq = in
					return &rdbpb.CompareArtifactLinesResponse{
						Artifacts: []string{"invocations/u-task-pass/artifacts/tast%2Ffull.txt"},
					}, nil
				},
			}

			th := &mockTestHistoryClient{
				queryRecentPasses: func(ctx context.Context, in *analysispb.QueryRecentPassesRequest) (*analysispb.QueryRecentPassesResponse, error) {
					assert.Loosely(t, in.Project, should.Equal("chromium"))
					assert.Loosely(t, in.Sources.GetGitilesCommit().GetPosition(), should.Equal(int64(999999)))
					return &analysispb.QueryRecentPassesResponse{
						PassingResults: []*analysispb.QueryRecentPassesResponse_PassingResult{
							{Name: "invocations/u-task-pass/tests/ninja:%2F%2Fchrome%2Ftest:browser_tests%2FMyTest.Foo/results/r0"},
						},
					}, nil
				},
			}

			cmd := CompareCmd(nil, ParentTypeUnknown)
			run := cmd.CommandRun().(*artifactCompareRun)
			run.client = rdb
			run.thClient = th
			run.invocationID = "build-888"
			run.artifactID = "tast/full.txt"
			run.outputFile = outFile

			code := run.Run(nil, nil, nil)
			assert.Loosely(t, code, should.Equal(0))
			assert.Loosely(t, capturedCompareReq, should.NotBeNil)
			assert.Loosely(t, capturedCompareReq.Name, should.Equal("invocations/u-task-1/artifacts/tast%2Ffull.txt"))
		})
	})
}
