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

package format

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestInlineArtifacts(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatTestResultInlineArtifacts`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`enforces DefaultInlineGetArtifactBudget across direct and 200 noisy root work-unit artifacts`, func(t *ftt.Test) {
			testID := ":CtsMediaTestCases!junit:android.media.cts:AudioFocusTest#testDucking"
			trName := "rootInvocations/ants-i999/workUnits/ants-wu505:u-mod-1/tests/:CtsMediaTestCases!junit:android.media.cts:AudioFocusTest%23testDucking/results/res-1"
			rootWU := "rootInvocations/ants-i999/workUnits/root"

			var rootArts []*rdbpb.Artifact
			for i := range 200 {
				id := fmt.Sprintf("16502346/inv_1/injectables-bundle-%03d.json", i)
				rootArts = append(rootArts, &rdbpb.Artifact{
					Name:         rootWU + "/artifacts/" + id,
					ArtifactId:   id,
					ArtifactType: "TEXT",
					ContentType:  "application/json",
					SizeBytes:    2048,
				})
			}
			rootArts = append(rootArts,
				&rdbpb.Artifact{
					Name:         rootWU + "/artifacts/host_log_root.txt",
					ArtifactId:   "host_log_root.txt",
					ArtifactType: "HOST_LOG",
					ContentType:  "text/plain",
					SizeBytes:    1048576,
				},
			)

			var directArts []*rdbpb.Artifact
			for i := range 25 {
				id := fmt.Sprintf("perfetto_chunk_%02d.bin", i)
				directArts = append(directArts, &rdbpb.Artifact{
					Name:        trName + "/artifacts/" + id,
					ArtifactId:  id,
					ContentType: "application/octet-stream",
					SizeBytes:   4096,
				})
			}
			directArts = append(directArts,
				&rdbpb.Artifact{
					Name:         trName + "/artifacts/logcat-on-failure.txt",
					ArtifactId:   "logcat-on-failure.txt",
					ArtifactType: "LOGCAT",
					ContentType:  "text/plain",
					SizeBytes:    194987,
				},
				&rdbpb.Artifact{
					Name:        trName + "/artifacts/summary_expanded.txt",
					ArtifactId:  "summary_expanded.txt",
					ContentType: "text/plain",
					SizeBytes:   256,
				},
			)

			client := &fakeDiscoveryRDBClient{
				queryWorkUnitsFunc: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*rdbpb.QueryWorkUnitsResponse, error) {
					return &rdbpb.QueryWorkUnitsResponse{
						WorkUnits: []*rdbpb.WorkUnit{{Name: rootWU}},
					}, nil
				},
				listArtifactsFunc: func(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
					switch in.Parent {
					case trName:
						return &rdbpb.ListArtifactsResponse{Artifacts: directArts}, nil
					case rootWU:
						return &rdbpb.ListArtifactsResponse{Artifacts: rootArts}, nil
					default:
						return &rdbpb.ListArtifactsResponse{}, nil
					}
				},
			}

			res := &rdbpb.TestResult{
				Name:     trName,
				TestId:   testID,
				ResultId: "res-1",
				StatusV2: rdbpb.TestResult_FAILED,
			}

			out := FormatTestResultInlineArtifacts(
				ctx,
				client,
				"ants-i999",
				res,
				map[string]bool{"summary_expanded.txt": true},
				false,
				DefaultInlineGetArtifactBudget,
			)

			assert.Loosely(t, len(out) <= DefaultInlineGetArtifactBudget, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("Test Result Artifacts (showing 1 of 26 artifacts):"))
			assert.Loosely(t, out, should.ContainSubstring("logcat-on-failure.txt"))
			assert.Loosely(t, out, should.NotContainSubstring("summary_expanded.txt"))
			assert.Loosely(t, out, should.NotContainSubstring("injectables-bundle"))
			assert.Loosely(t, out, should.ContainSubstring("-testid=':CtsMediaTestCases!junit:android.media.cts:AudioFocusTest#testDucking'"))
			assert.Loosely(t, out, should.ContainSubstring("e.g. luci test-result artifact get -invocationid=ants-i999 -workunitid=ants-wu505:u-mod-1 -testid=':CtsMediaTestCases!junit:android.media.cts:AudioFocusTest#testDucking' -resultid=res-1 -artifactid=logcat-on-failure.txt"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:"))
			assert.Loosely(t, out, should.ContainSubstring("host_log_root.txt"))
			assert.Loosely(t, out, should.ContainSubstring("e.g. luci artifact get -invocationid=ants-i999 -workunitid=root -artifactid=host_log_root.txt"))
		})

		t.Run(`formats legacy invocation direct test-result and parent invocation artifacts`, func(t *ftt.Test) {
			trName := "invocations/task-chromium-swarm.appspot.com-123/tests/ninja:%2F%2Fchrome%2Ftest:browser_tests%2FFoo.Bar/results/r1"
			client := &fakeDiscoveryRDBClient{
				listArtifactsFunc: func(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
					switch in.Parent {
					case trName:
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "crash_log.txt", SizeBytes: 2048, ContentType: "text/plain"},
							},
						}, nil
					case "invocations/task-chromium-swarm.appspot.com-123":
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{{ArtifactId: "system_log"}},
						}, nil
					default:
						return &rdbpb.ListArtifactsResponse{}, nil
					}
				},
			}
			res := &rdbpb.TestResult{
				Name:     trName,
				TestId:   "ninja://chrome/test:browser_tests/Foo.Bar",
				ResultId: "r1",
				StatusV2: rdbpb.TestResult_FAILED,
			}
			out := FormatTestResultInlineArtifacts(ctx, client, "build-8765", res, nil, true, DefaultInlineGetArtifactBudget)
			assert.Loosely(t, len(out) <= DefaultInlineGetArtifactBudget, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("crash_log.txt"))
			assert.Loosely(t, out, should.ContainSubstring("luci test-result artifact <get|head|tail> -invocationid=task-chromium-swarm.appspot.com-123 -testid=ninja://chrome/test:browser_tests/Foo.Bar -resultid=r1 -artifactid=<ARTIFACT_ID>"))
			assert.Loosely(t, out, should.ContainSubstring("e.g. luci test-result artifact get -invocationid=task-chromium-swarm.appspot.com-123 -testid=ninja://chrome/test:browser_tests/Foo.Bar -resultid=r1 -artifactid=crash_log.txt"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation Artifacts:"))
		})
	})

	ftt.Run(`FormatVerdictInlineFailureArtifacts`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`makes zero RPCs for PASSED and SKIPPED verdicts`, func(t *ftt.Test) {
			var rpcCalls atomic.Int32
			client := &fakeDiscoveryRDBClient{
				listArtifactsFunc: func(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
					rpcCalls.Add(1)
					return &rdbpb.ListArtifactsResponse{}, nil
				},
				queryWorkUnitsFunc: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*rdbpb.QueryWorkUnitsResponse, error) {
					rpcCalls.Add(1)
					return &rdbpb.QueryWorkUnitsResponse{}, nil
				},
			}

			results := []*rdbpb.TestResult{
				{Name: "rootInvocations/ants-i1/workUnits/wu-1/tests/t1/results/r1", StatusV2: rdbpb.TestResult_PASSED},
				{Name: "rootInvocations/ants-i1/workUnits/wu-1/tests/t1/results/r2", StatusV2: rdbpb.TestResult_SKIPPED},
			}
			out := FormatVerdictInlineFailureArtifacts(ctx, client, "ants-i1", results, false, DefaultInlineGetArtifactBudget)
			assert.Loosely(t, out, should.BeEmpty)
			assert.Loosely(t, rpcCalls.Load(), should.Equal(int32(0)))
		})

		t.Run(`inspects only failed attempt on FLAKY verdict and ranks test-matching work-unit artifacts first`, func(t *ftt.Test) {
			passName := "rootInvocations/ants-i1/workUnits/wu-1/tests/com.example.WidgetTest%23testClick/results/res-pass"
			failName := "rootInvocations/ants-i1/workUnits/wu-1/tests/com.example.WidgetTest%23testClick/results/res-fail"
			wuName := "rootInvocations/ants-i1/workUnits/wu-1"

			var wuArts []*rdbpb.Artifact
			for i := range 20 {
				id := fmt.Sprintf("OtherTest_case%02d_dump.txt", i)
				wuArts = append(wuArts, &rdbpb.Artifact{Name: wuName + "/artifacts/" + id, ArtifactId: id, SizeBytes: 512})
			}
			wuArts = append(wuArts, &rdbpb.Artifact{
				Name:       wuName + "/artifacts/WidgetTest_testClick_logcat.txt",
				ArtifactId: "WidgetTest_testClick_logcat.txt",
				SizeBytes:  65536,
			})

			var passQueried atomic.Bool
			client := &fakeDiscoveryRDBClient{
				listArtifactsFunc: func(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
					switch in.Parent {
					case passName:
						passQueried.Store(true)
						return &rdbpb.ListArtifactsResponse{}, nil
					case failName:
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "screenshot.png", ArtifactType: "SCREENSHOT", SizeBytes: 32768, ContentType: "image/png"},
							},
						}, nil
					case wuName:
						return &rdbpb.ListArtifactsResponse{Artifacts: wuArts}, nil
					default:
						return &rdbpb.ListArtifactsResponse{}, nil
					}
				},
			}

			results := []*rdbpb.TestResult{
				{Name: passName, TestId: "com.example.WidgetTest#testClick", ResultId: "res-pass", StatusV2: rdbpb.TestResult_PASSED},
				{Name: failName, TestId: "com.example.WidgetTest#testClick", ResultId: "res-fail", StatusV2: rdbpb.TestResult_FAILED},
			}
			out := FormatVerdictInlineFailureArtifacts(ctx, client, "ants-i1", results, false, DefaultInlineGetArtifactBudget)
			assert.Loosely(t, passQueried.Load(), should.BeFalse)
			assert.Loosely(t, len(out) <= DefaultInlineGetArtifactBudget, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("Test Result Artifacts (result_id: res-fail, 1 artifact):"))
			assert.Loosely(t, out, should.ContainSubstring("screenshot.png"))
			assert.Loosely(t, out, should.ContainSubstring("WidgetTest_testClick_logcat.txt"))
			assert.Loosely(t, out, should.NotContainSubstring("OtherTest_case00_dump.txt"))
		})

		t.Run(`formats multi-attempt failure artifacts with per-row FLAGS`, func(t *ftt.Test) {
			fail1 := "rootInvocations/ants-i1/workUnits/wu-1/tests/t1/results/r1"
			fail2 := "rootInvocations/ants-i1/workUnits/wu-2/tests/t1/results/r2"
			client := &fakeDiscoveryRDBClient{
				listArtifactsFunc: func(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
					switch in.Parent {
					case fail1:
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{{ArtifactId: "attempt1_logcat.txt", ArtifactType: "LOGCAT", SizeBytes: 4096}},
						}, nil
					case fail2:
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{{ArtifactId: "attempt2_logcat.txt", ArtifactType: "LOGCAT", SizeBytes: 8192}},
						}, nil
					default:
						return &rdbpb.ListArtifactsResponse{}, nil
					}
				},
			}
			results := []*rdbpb.TestResult{
				{Name: fail1, TestId: "t1", ResultId: "r1", StatusV2: rdbpb.TestResult_FAILED},
				{Name: fail2, TestId: "t1", ResultId: "r2", StatusV2: rdbpb.TestResult_EXECUTION_ERRORED},
			}
			out := FormatVerdictInlineFailureArtifacts(ctx, client, "ants-i1", results, false, DefaultInlineGetArtifactBudget)
			assert.Loosely(t, len(out) <= DefaultInlineGetArtifactBudget, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("Test Result Artifacts (across 2 failed runs, 2 artifacts):"))
			assert.Loosely(t, out, should.ContainSubstring("-workunitid=wu-1 -testid=t1 -resultid=r1"))
			assert.Loosely(t, out, should.ContainSubstring("-workunitid=wu-2 -testid=t1 -resultid=r2"))
			assert.Loosely(t, out, should.ContainSubstring("e.g. luci test-result artifact get -invocationid=ants-i1 -workunitid=wu-1 -testid=t1 -resultid=r1 -artifactid=attempt1_logcat.txt"))
		})

		t.Run(`formats legacy multi-attempt failure artifacts across Swarming tasks using per-row leaf task invocation flags and single parent notice header`, func(t *ftt.Test) {
			testID := "ninja://chrome/test:browser_tests/Foo.Bar"
			task1 := "invocations/task-chromium-swarm.appspot.com-111"
			task2 := "invocations/task-chromium-swarm.appspot.com-222"
			fail1 := task1 + "/tests/ninja:%2F%2Fchrome%2Ftest:browser_tests%2FFoo.Bar/results/0"
			fail2 := task2 + "/tests/ninja:%2F%2Fchrome%2Ftest:browser_tests%2FFoo.Bar/results/0"
			client := &fakeDiscoveryRDBClient{
				listArtifactsFunc: func(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
					switch in.Parent {
					case fail1:
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "crash_1.txt", ArtifactType: "", SizeBytes: 1024},
								{ArtifactId: "extra_1.txt", ArtifactType: "", SizeBytes: 512},
							},
							NextPageToken: "next-page",
						}, nil
					case fail2:
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{{ArtifactId: "crash_2.txt", ArtifactType: "", SizeBytes: 2048}},
						}, nil
					case task1:
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{{ArtifactId: "task1_sys.log"}},
						}, nil
					case task2:
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{{ArtifactId: "task2_sys.log"}},
						}, nil
					default:
						return &rdbpb.ListArtifactsResponse{}, nil
					}
				},
			}
			results := []*rdbpb.TestResult{
				{Name: fail1, TestId: testID, ResultId: "0", StatusV2: rdbpb.TestResult_FAILED},
				{Name: fail2, TestId: testID, ResultId: "0", StatusV2: rdbpb.TestResult_FAILED},
			}
			out := FormatVerdictInlineFailureArtifacts(ctx, client, "build-8765", results, true, DefaultInlineGetArtifactBudget)
			assert.Loosely(t, len(out) <= DefaultInlineGetArtifactBudget, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("Test Result Artifacts (across 2 failed runs, showing"))
			assert.Loosely(t, out, should.ContainSubstring("-invocationid=task-chromium-swarm.appspot.com-111 -testid=ninja://chrome/test:browser_tests/Foo.Bar -resultid=0"))
			assert.Loosely(t, out, should.ContainSubstring("-invocationid=task-chromium-swarm.appspot.com-222 -testid=ninja://chrome/test:browser_tests/Foo.Bar -resultid=0"))
			assert.Loosely(t, out, should.ContainSubstring("view all: luci test-result artifact list -invocationid=build-8765 -testid=ninja://chrome/test:browser_tests/Foo.Bar -resultid=<RESULT_ID> -all [-filter=...]"))
			assert.Loosely(t, out, should.ContainSubstring("luci test-result artifact <get|head|tail> -artifactid=<ARTIFACT_ID> [FLAGS]"))
			assert.Loosely(t, out, should.ContainSubstring("e.g. luci test-result artifact get -invocationid=task-chromium-swarm.appspot.com-111 -testid=ninja://chrome/test:browser_tests/Foo.Bar -resultid=0 -artifactid=crash_1.txt"))
			assert.Loosely(t, strings.Count(out, "Invocation Artifacts:\n"), should.Equal(1))
			assert.Loosely(t, out, should.ContainSubstring("Invocation task-chromium-swarm.appspot.com-111 contains 1 artifact."))
			assert.Loosely(t, out, should.ContainSubstring("Invocation task-chromium-swarm.appspot.com-222 contains 1 artifact."))
		})

		t.Run(`reports total failed runs across >3 attempts even when only 1 of the first 3 has artifacts`, func(t *ftt.Test) {
			results := make([]*rdbpb.TestResult, 5)
			for i := range results {
				results[i] = &rdbpb.TestResult{
					Name:     fmt.Sprintf("rootInvocations/ants-i1/workUnits/wu-%d/tests/t1/results/r%d", i+1, i+1),
					TestId:   "t1",
					ResultId: fmt.Sprintf("r%d", i+1),
					StatusV2: rdbpb.TestResult_FAILED,
				}
			}
			client := &fakeDiscoveryRDBClient{
				listArtifactsFunc: func(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
					if in.Parent == results[0].Name {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{{ArtifactId: "attempt1_logcat.txt", ArtifactType: "LOGCAT", SizeBytes: 4096}},
						}, nil
					}
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}
			out := FormatVerdictInlineFailureArtifacts(ctx, client, "ants-i1", results, false, DefaultInlineGetArtifactBudget)
			assert.Loosely(t, len(out) <= DefaultInlineGetArtifactBudget, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("Test Result Artifacts (across 5 failed runs, showing 1 of 1+ artifacts):"))
		})

		t.Run(`single-attempt FAILED verdict includes result_id header and ready artifact get command within budget`, func(t *ftt.Test) {
			failName := "rootInvocations/ants-i1/workUnits/wu-1/tests/t1/results/r-single"
			client := &fakeDiscoveryRDBClient{
				listArtifactsFunc: func(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
					if in.Parent == failName {
						return &rdbpb.ListArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{ArtifactId: "logcat.txt", ArtifactType: "LOGCAT", SizeBytes: 2048},
							},
						}, nil
					}
					return &rdbpb.ListArtifactsResponse{}, nil
				},
			}
			results := []*rdbpb.TestResult{
				{Name: failName, TestId: "t1", ResultId: "r-single", StatusV2: rdbpb.TestResult_FAILED},
			}
			out := FormatVerdictInlineFailureArtifacts(ctx, client, "ants-i1", results, false, DefaultInlineGetArtifactBudget)
			assert.Loosely(t, len(out) <= DefaultInlineGetArtifactBudget, should.BeTrue)
			assert.Loosely(t, out, should.ContainSubstring("Test Result Artifacts (result_id: r-single, 1 artifact):"))
			assert.Loosely(t, out, should.ContainSubstring("e.g. luci test-result artifact get -invocationid=ants-i1 -workunitid=wu-1 -testid=t1 -resultid=r-single -artifactid=logcat.txt"))
		})
	})

	ftt.Run(`FormatAncestorWorkUnitNoticesWithOmitted`, t, func(t *ftt.Test) {
		infos := []WorkUnitArtifactsInfo{
			{
				Name: "rootInvocations/ants-i1/workUnits/wu-a",
				Artifacts: []*rdbpb.Artifact{
					{ArtifactId: "host_log_a.txt", ArtifactType: "HOST_LOG", SizeBytes: 4096},
				},
			},
			{
				Name: "rootInvocations/ants-i1/workUnits/wu-b",
				Artifacts: []*rdbpb.Artifact{
					{ArtifactId: "bugreport_b.zip", ArtifactType: "BUGREPORTZ", SizeBytes: 8192},
				},
			},
		}
		out := FormatAncestorWorkUnitNoticesWithOmitted(infos, 0, "", "", false, DefaultInlineGetArtifactBudget)
		assert.Loosely(t, len(out) <= DefaultInlineGetArtifactBudget, should.BeTrue)
		assert.Loosely(t, out, should.ContainSubstring("e.g. luci artifact get -invocationid=ants-i1 -workunitid=wu-a -artifactid=host_log_a.txt"))
	})

	ftt.Run(`FetchWorkUnitAndAncestorNoticeFormatterExcluding reclaims unused ancestor budget`, t, func(t *ftt.Test) {
		ctx := context.Background()
		longInv := "build-" + strings.Repeat("9", 165)
		directWU := "rootInvocations/" + longInv + "/workUnits/wu-child"
		ancWU := "rootInvocations/" + longInv + "/workUnits/wu-anc"

		directArts := []*rdbpb.Artifact{
			{Name: directWU + "/artifacts/logcat_01.txt", ArtifactId: "logcat_01.txt", ArtifactType: "LOGCAT", SizeBytes: 4096, ContentType: "text/plain"},
			{Name: directWU + "/artifacts/host_log_02.txt", ArtifactId: "host_log_02.txt", ArtifactType: "HOST_LOG", SizeBytes: 8192, ContentType: "text/plain"},
		}
		ancArts := []*rdbpb.Artifact{
			{Name: ancWU + "/artifacts/anc_host_log.txt", ArtifactId: "anc_host_log.txt", ArtifactType: "HOST_LOG", SizeBytes: 4096, ContentType: "text/plain"},
		}

		client := &fakeDiscoveryRDBClient{
			queryWorkUnitsFunc: func(ctx context.Context, in *rdbpb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*rdbpb.QueryWorkUnitsResponse, error) {
				if in.GetPredicate().GetAncestorsOf() == directWU {
					return &rdbpb.QueryWorkUnitsResponse{
						WorkUnits: []*rdbpb.WorkUnit{{Name: ancWU}},
					}, nil
				}
				return &rdbpb.QueryWorkUnitsResponse{}, nil
			},
			listArtifactsFunc: func(ctx context.Context, in *rdbpb.ListArtifactsRequest, opts ...grpc.CallOption) (*rdbpb.ListArtifactsResponse, error) {
				switch in.Parent {
				case directWU:
					return &rdbpb.ListArtifactsResponse{Artifacts: directArts}, nil
				case ancWU:
					return &rdbpb.ListArtifactsResponse{Artifacts: ancArts}, nil
				default:
					return &rdbpb.ListArtifactsResponse{}, nil
				}
			},
		}

		formatter, _ := FetchWorkUnitAndAncestorNoticeFormatterExcluding(ctx, client, []string{directWU}, nil)
		assert.Loosely(t, formatter, should.NotBeNil)

		// At shrunkBudget = 580 (>= minShrunkInlineBudget), directBudget (290) collapses directOut to
		// shownWUs=0 (288 B) while remaining (292 B) cannot fit even the collapsed ancestor summary (297 B).
		// Reclaiming unused ancestor reserve upgrades directOut to the full 580 B budget showing wu-child.
		const shrunkBudget = 580
		directExpected := FormatAncestorWorkUnitNoticesWithOmitted([]WorkUnitArtifactsInfo{{Name: directWU, Artifacts: directArts}}, 0, "", "", false, shrunkBudget)
		got := formatter(shrunkBudget)
		assert.Loosely(t, len(got) <= shrunkBudget, should.BeTrue)
		assert.Loosely(t, got, should.NotContainSubstring("Ancestor Work Unit Artifacts:"))
		assert.Loosely(t, got, should.Equal(directExpected))
		assert.Loosely(t, got, should.ContainSubstring("Work unit wu-child (2 artifacts)"))

		// Verify end-to-end reclamation through FitPrimaryAndInlineArtifacts when remaining == shrunkBudget.
		primaryLen := DefaultMaxOutputBytes - shrunkBudget - 1
		primary := strings.Repeat("x", primaryLen-1) + "\n"
		combined := FitPrimaryAndInlineArtifacts(primary, formatter, true)
		assert.Loosely(t, len(combined) <= DefaultMaxOutputBytes, should.BeTrue)
		assert.Loosely(t, combined, should.NotContainSubstring("bytes truncated"))
		assert.Loosely(t, combined, should.ContainSubstring("Work unit wu-child (2 artifacts)"))
	})

	ftt.Run(`FormatParentInvocationNotices`, t, func(t *ftt.Test) {
		infos := []ParentInvocationNoticeInfo{
			{LeafInvID: "task-chromium-swarm.appspot.com-111", Count: 1},
			{LeafInvID: "task-chromium-swarm.appspot.com-222", Count: 5, More: true},
			{LeafInvID: "task-chromium-swarm.appspot.com-333", Count: 2},
		}

		full := FormatParentInvocationNotices(infos, 0)
		assert.Loosely(t, strings.Count(full, "Invocation Artifacts:\n"), should.Equal(1))
		assert.Loosely(t, full, should.ContainSubstring("Invocation task-chromium-swarm.appspot.com-111 contains 1 artifact. Run 'luci artifact list -invocationid task-chromium-swarm.appspot.com-111' to view."))
		assert.Loosely(t, full, should.ContainSubstring("Invocation task-chromium-swarm.appspot.com-222 contains 5+ artifacts."))
		assert.Loosely(t, full, should.ContainSubstring("Invocation task-chromium-swarm.appspot.com-333 contains 2 artifacts."))

		// Trim to 2 bullets + omitted footer.
		trimmedTwo := FormatParentInvocationNotices(infos, len(full)-20)
		assert.Loosely(t, len(trimmedTwo) <= len(full)-20, should.BeTrue)
		assert.Loosely(t, strings.Count(trimmedTwo, "Invocation Artifacts:\n"), should.Equal(1))
		assert.Loosely(t, trimmedTwo, should.ContainSubstring("task-chromium-swarm.appspot.com-111"))
		assert.Loosely(t, trimmedTwo, should.ContainSubstring("task-chromium-swarm.appspot.com-222"))
		assert.Loosely(t, trimmedTwo, should.NotContainSubstring("task-chromium-swarm.appspot.com-333"))
		assert.Loosely(t, trimmedTwo, should.ContainSubstring("  ... and 1 more invocation with artifacts\n"))

		// Tiny budget below 1 bullet returns "".
		assert.Loosely(t, FormatParentInvocationNotices(infos, 40), should.BeEmpty)
	})

	ftt.Run(`Empty invocation ID guards ShellQuoteIfNeeded and falls back cleanly`, t, func(t *ftt.Test) {
		// 1. Multi-attempt where invID == "" and first attempt has no invocation prefix,
		// while highest-priority displayed artifact carries rootInvocations/ants-i777.
		attempts := []verdictAttemptArtifacts{
			{
				res: &rdbpb.TestResult{Name: "tests/t1/results/r1", TestId: "t1", ResultId: "r1"},
				artifacts: []*rdbpb.Artifact{
					{ArtifactId: "plain.txt", SizeBytes: 128, ContentType: "text/plain"},
					{ArtifactId: "extra.txt", SizeBytes: 64, ContentType: "text/plain"},
				},
				more: true,
			},
			{
				res: &rdbpb.TestResult{Name: "tests/t1/results/r2", TestId: "t1", ResultId: "r2"},
				artifacts: []*rdbpb.Artifact{
					{
						Name:         "rootInvocations/ants-i777/workUnits/wu-2/tests/t1/results/r2/artifacts/logcat.txt",
						ArtifactId:   "logcat.txt",
						ArtifactType: "LOGCAT",
						SizeBytes:    2048,
						ContentType:  "text/plain",
					},
				},
			},
		}
		multiOut := formatMultiAttemptDirectArtifacts("", attempts, 2, 0, false, DefaultInlineGetArtifactBudget)
		assert.Loosely(t, multiOut, should.NotContainSubstring("''"))
		assert.Loosely(t, multiOut, should.ContainSubstring("luci artifact <get|head|tail> -invocationid=ants-i777 -artifactid=<ARTIFACT_ID> [FLAGS]"))

		// 2. Single-result and work-unit formatters with empty invID never emit -invocationid=''.
		singleOut := formatSingleResultDirectArtifacts(
			"Test Result Artifacts",
			&rdbpb.TestResult{Name: "tests/t1/results/r1", TestId: "t1", ResultId: "r1"},
			"",
			[]*rdbpb.Artifact{{
				Name:         "rootInvocations/ants-i777/workUnits/wu-1/tests/t1/results/r1/artifacts/logcat.txt",
				ArtifactId:   "logcat.txt",
				ArtifactType: "LOGCAT",
				SizeBytes:    1024,
			}},
			true,
			DefaultInlineGetArtifactBudget,
		)
		assert.Loosely(t, singleOut, should.NotContainSubstring("''"))
		assert.Loosely(t, singleOut, should.ContainSubstring("-invocationid=ants-i777"))

		wuOut := FormatAncestorWorkUnitNoticesWithOmitted([]WorkUnitArtifactsInfo{
			{
				Name: "",
				Artifacts: []*rdbpb.Artifact{{
					Name:         "rootInvocations/ants-i777/workUnits/wu-1/artifacts/host_log.txt",
					ArtifactId:   "host_log.txt",
					ArtifactType: "HOST_LOG",
					SizeBytes:    1024,
				}},
				More: true,
			},
		}, 1, "", "", false, DefaultInlineGetArtifactBudget)
		assert.Loosely(t, wuOut, should.NotContainSubstring("''"))
		assert.Loosely(t, wuOut, should.ContainSubstring("luci work-unit artifact <get|head|tail> -invocationid=ants-i777 -workunitid=wu-1 -artifactid=<ARTIFACT_ID>"))
	})

	ftt.Run(`FitPrimaryAndInlineArtifacts`, t, func(t *ftt.Test) {
		formatInline := func(maxBytes int) string {
			s := "Test Result Artifacts (1 artifact):\n  logcat.txt  LOGCAT  10.0 KiB  text/plain\n"
			if maxBytes > 0 && len(s) > maxBytes {
				return ""
			}
			return s
		}

		// 1. Primary ~8 KB + inline fits within 9.5 KB without truncating primary.
		eightKB := strings.Repeat("stack frame line 1234567890\n", 285) // ~7,980 B
		gotEight := FitPrimaryAndInlineArtifacts(eightKB, formatInline, true)
		assert.Loosely(t, len(gotEight) <= DefaultMaxOutputBytes, should.BeTrue)
		assert.Loosely(t, gotEight, should.NotContainSubstring("bytes truncated"))
		assert.Loosely(t, gotEight, should.ContainSubstring("logcat.txt"))

		// 2. Primary > 9.5 KB middle-truncates primary while keeping inline artifacts intact.
		twentyKB := "HEADER_START\n" + strings.Repeat("deep stack frame line 1234567890\n", 650) + "BOTTOM_FRAME\n"
		gotTwenty := FitPrimaryAndInlineArtifacts(twentyKB, formatInline, true)
		assert.Loosely(t, len(gotTwenty) <= DefaultMaxOutputBytes, should.BeTrue)
		assert.Loosely(t, gotTwenty, should.ContainSubstring("HEADER_START"))
		assert.Loosely(t, gotTwenty, should.ContainSubstring("bytes truncated; pass -show-metadata to view full output"))
		assert.Loosely(t, gotTwenty, should.ContainSubstring("BOTTOM_FRAME"))
		assert.Loosely(t, gotTwenty, should.ContainSubstring("logcat.txt"))

		// 3. capTotalOutput=false (-show-metadata) preserves full primary output.
		gotUncapped := FitPrimaryAndInlineArtifacts(twentyKB, formatInline, false)
		assert.Loosely(t, len(gotUncapped) > DefaultMaxOutputBytes, should.BeTrue)
		assert.Loosely(t, gotUncapped, should.NotContainSubstring("bytes truncated"))
		assert.Loosely(t, gotUncapped, should.ContainSubstring("logcat.txt"))

		// 4. Oversized multi-byte UTF-8 inline section preserves valid UTF-8 on fallback cap.
		oversizedUTF8Inline := func(int) string {
			return strings.Repeat("日志", DefaultMaxOutputBytes)
		}
		gotUTF8Fallback := FitPrimaryAndInlineArtifacts("header\n", oversizedUTF8Inline, true)
		assert.Loosely(t, len(gotUTF8Fallback) <= DefaultMaxOutputBytes, should.BeTrue)
		assert.Loosely(t, utf8.ValidString(gotUTF8Fallback), should.BeTrue)
	})
}
