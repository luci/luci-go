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
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestParseInvocationContext(t *testing.T) {
	t.Parallel()

	ftt.Run(`ParseInvocationContext`, t, func(t *ftt.Test) {
		t.Run(`build invocation`, func(t *ftt.Test) {
			res := ParseInvocationContext("invocations/build-8676971682343117873/tests/test1/results/res1")
			assert.Loosely(t, res, should.Equal("build 8676971682343117873"))
		})

		t.Run(`swarming task invocation`, func(t *ftt.Test) {
			res := ParseInvocationContext("invocations/task-chromium-swarm.appspot.com-5a1b2c3d4e5f/tests/test1/results/res1")
			assert.Loosely(t, res, should.Equal("task 5a1b2c3d4e5f"))
		})

		t.Run(`generic invocation`, func(t *ftt.Test) {
			res := ParseInvocationContext("invocations/u-my-invocation/tests/test1/results/res1")
			assert.Loosely(t, res, should.Equal("invocation u-my-invocation"))
		})

		t.Run(`invalid name`, func(t *ftt.Test) {
			res := ParseInvocationContext("invalid")
			assert.Loosely(t, res, should.Equal(""))
		})
	})

	ftt.Run(`Breadcrumbs`, t, func(t *ftt.Test) {
		t.Run(`TestResult breadcrumb`, func(t *ftt.Test) {
			tr := "invocations/task-chromium-swarm.appspot.com-7a07b808bfa95b11/tests/my_test/results/0c30c334-01920"
			res := FormatTestResultBreadcrumb(tr)
			assert.Loosely(t, res, should.Equal("Result 0c30c334-01920 in task 7a07b808bfa95b11"))
		})

		t.Run(`WorkUnit breadcrumb`, func(t *ftt.Test) {
			wu := "rootInvocations/build-8673802696052024673/workUnits/run-tests"
			res := FormatWorkUnitBreadcrumb(wu)
			assert.Loosely(t, res, should.Equal("Work Unit run-tests in build 8673802696052024673"))
		})

		t.Run(`Invocation breadcrumb`, func(t *ftt.Test) {
			inv := "invocations/build-8673802696052024673"
			res := FormatInvocationBreadcrumb(inv)
			assert.Loosely(t, res, should.Equal("build 8673802696052024673"))

			inv2 := "invocations/u-root-2026-09-07"
			res2 := FormatInvocationBreadcrumb(inv2)
			assert.Loosely(t, res2, should.Equal("invocation u-root-2026-09-07"))
		})

		t.Run(`GetParentGroup`, func(t *ftt.Test) {
			pgTask := GetParentGroup("invocations/task-chromium-swarm.appspot.com-7a07b808bfa95b11/tests/test1/results/res1")
			assert.Loosely(t, pgTask.Label, should.Equal("Task 7a07b808bfa95b11"))
			assert.Loosely(t, pgTask.ID, should.Equal("invocations/task-chromium-swarm.appspot.com-7a07b808bfa95b11"))

			pgWU := GetParentGroup("rootInvocations/build-8673802696052024673/workUnits/run-tests/tests/test1/results/res1")
			assert.Loosely(t, pgWU.Label, should.Equal("Work Unit run-tests"))
			assert.Loosely(t, pgWU.ID, should.Equal("rootInvocations/build-8673802696052024673/workUnits/run-tests"))

			pgBuild := GetParentGroup("invocations/build-8673802696052024673/tests/test1/results/res1")
			assert.Loosely(t, pgBuild.Label, should.Equal("build-8673802696052024673"))
			assert.Loosely(t, pgBuild.ID, should.Equal("invocations/build-8673802696052024673"))

			pgGeneric := GetParentGroup("invocations/u-test-inv/tests/test1/results/res1")
			assert.Loosely(t, pgGeneric.Label, should.Equal("u-test-inv"))
			assert.Loosely(t, pgGeneric.ID, should.Equal("invocations/u-test-inv"))
		})
	})
}

func TestFormatVariant(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatVariant`, t, func(t *ftt.Test) {
		t.Run(`nil variant`, func(t *ftt.Test) {
			assert.Loosely(t, FormatVariant(nil), should.Equal(""))
		})

		t.Run(`empty variant`, func(t *ftt.Test) {
			v := &rdbpb.Variant{Def: map[string]string{}}
			assert.Loosely(t, FormatVariant(v), should.Equal(""))
		})

		t.Run(`populated variant`, func(t *ftt.Test) {
			v := &rdbpb.Variant{
				Def: map[string]string{
					"builder":    "win-rel",
					"os":         "Windows-10",
					"test_suite": "browser_tests",
				},
			}
			assert.Loosely(t, FormatVariant(v), should.Equal("builder=win-rel os=Windows-10 test_suite=browser_tests"))
		})
	})
}

func TestStripHTML(t *testing.T) {
	t.Parallel()

	ftt.Run(`StripHTML`, t, func(t *ftt.Test) {
		t.Run(`plain text`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML("hello world"), should.Equal("hello world"))
		})

		t.Run(`simple tags`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML("<p>hello <b>world</b></p>"), should.Equal("hello world"))
		})

		t.Run(`links`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML("click <a href=\"http://example.com\">here</a>"), should.Equal("click here"))
		})

		t.Run(`complex tags and entities`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML(`<div title="foo > bar">text &lt; with &amp; entities</div>`), should.Equal("text < with & entities"))
		})

		t.Run(`empty`, func(t *ftt.Test) {
			assert.Loosely(t, StripHTML(""), should.BeEmpty)
		})
	})
}

type fakeRDBClient struct {
	rdbpb.ResultDBClient
	getArtifactFunc func(ctx context.Context, in *rdbpb.GetArtifactRequest, opts ...grpc.CallOption) (*rdbpb.Artifact, error)
}

func (f *fakeRDBClient) GetArtifact(ctx context.Context, in *rdbpb.GetArtifactRequest, opts ...grpc.CallOption) (*rdbpb.Artifact, error) {
	if f.getArtifactFunc != nil {
		return f.getArtifactFunc(ctx, in, opts...)
	}
	return nil, nil
}

func TestFormatVerdictCountsInline(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatVerdictCountsInline`, t, func(t *ftt.Test) {
		t.Run(`nil or zero counts`, func(t *ftt.Test) {
			assert.Loosely(t, FormatVerdictCountsInline(nil), should.Equal(""))
			assert.Loosely(t, FormatVerdictCountsInline(&rdbpb.TestAggregation_VerdictCounts{}), should.Equal(""))
		})

		t.Run(`failed with zero passed explicitly shows 0 passed`, func(t *ftt.Test) {
			vc := &rdbpb.TestAggregation_VerdictCounts{
				Failed: 16,
				Passed: 0,
			}
			assert.Loosely(t, FormatVerdictCountsInline(vc), should.Equal("[16 failed, 0 passed]"))
		})

		t.Run(`failed, passed, and skipped`, func(t *ftt.Test) {
			vc := &rdbpb.TestAggregation_VerdictCounts{
				Failed:  245,
				Passed:  218,
				Skipped: 7,
			}
			assert.Loosely(t, FormatVerdictCountsInline(vc), should.Equal("[245 failed, 218 passed, 7 skipped]"))
		})

		t.Run(`only passed`, func(t *ftt.Test) {
			vc := &rdbpb.TestAggregation_VerdictCounts{
				Passed: 50,
			}
			assert.Loosely(t, FormatVerdictCountsInline(vc), should.Equal("[50 passed]"))
		})
	})

	ftt.Run(`FormatVerdictCountsSummary`, t, func(t *ftt.Test) {
		t.Run(`nil returns empty`, func(t *ftt.Test) {
			assert.Loosely(t, FormatVerdictCountsSummary(nil), should.Equal(""))
		})

		t.Run(`zero counts returns 0 tests ran`, func(t *ftt.Test) {
			assert.Loosely(t, FormatVerdictCountsSummary(&rdbpb.TestAggregation_VerdictCounts{}), should.Equal("0 tests ran"))
		})

		t.Run(`populated counts includes total`, func(t *ftt.Test) {
			vc := &rdbpb.TestAggregation_VerdictCounts{
				Failed:           3,
				ExecutionErrored: 1,
				Flaky:            2,
				Passed:           144,
				Skipped:          5,
			}
			assert.Loosely(t, FormatVerdictCountsSummary(vc), should.Equal("[3 failed, 1 execution_errored, 2 flaky, 144 passed, 5 skipped] (155 total)"))
		})
	})

	ftt.Run(`FormatModuleStatusCountsSummary`, t, func(t *ftt.Test) {
		t.Run(`nil or zero returns empty`, func(t *ftt.Test) {
			assert.Loosely(t, FormatModuleStatusCountsSummary(nil), should.Equal(""))
			assert.Loosely(t, FormatModuleStatusCountsSummary(&rdbpb.TestAggregation_ModuleStatusCounts{}), should.Equal(""))
		})

		t.Run(`failed with zero succeeded explicitly shows 0 succeeded`, func(t *ftt.Test) {
			mc := &rdbpb.TestAggregation_ModuleStatusCounts{
				Failed: 2,
			}
			assert.Loosely(t, FormatModuleStatusCountsSummary(mc), should.Equal("[2 failed, 0 succeeded] (2 total)"))
		})

		t.Run(`mixed module statuses`, func(t *ftt.Test) {
			mc := &rdbpb.TestAggregation_ModuleStatusCounts{
				Failed:    1,
				Flaky:     1,
				Succeeded: 18,
				Skipped:   2,
			}
			assert.Loosely(t, FormatModuleStatusCountsSummary(mc), should.Equal("[1 failed, 1 flaky, 18 succeeded, 2 skipped] (22 total)"))
		})
	})
}

func TestFormatSummaryHTML(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatSummaryHTML`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`without client returns placeholder`, func(t *ftt.Test) {
			html := `<p>Test failed</p><br><text-artifact artifact-id="log.txt">`
			res := FormatSummaryHTML(ctx, nil, nil, "invocations/inv1/tests/t1/results/r1", html)
			assert.Loosely(t, res, should.Equal("Test failed\n\n[Embedded Artifact: log.txt - use 'luci artifact get' to view]"))
		})

		t.Run(`expands small text-artifact by default`, func(t *ftt.Test) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Error log content from artifact"))
			}))
			defer server.Close()

			fakeClient := &fakeRDBClient{
				getArtifactFunc: func(ctx context.Context, in *rdbpb.GetArtifactRequest, opts ...grpc.CallOption) (*rdbpb.Artifact, error) {
					return &rdbpb.Artifact{
						Name:       in.Name,
						ArtifactId: "log.txt",
						SizeBytes:  31,
						FetchUrl:   server.URL,
					}, nil
				},
			}

			html := `<p>Test failed</p><text-artifact artifact-id="log.txt">`
			res := FormatSummaryHTML(ctx, fakeClient, server.Client(), "invocations/inv1/tests/t1/results/r1", html)
			assert.Loosely(t, res, should.Equal("Test failed\n\n--- Embedded Artifact: log.txt ---\nError log content from artifact\n--- End Artifact ---"))
		})

		t.Run(`skips large text-artifact exceeding inline threshold`, func(t *ftt.Test) {
			fakeClient := &fakeRDBClient{
				getArtifactFunc: func(ctx context.Context, in *rdbpb.GetArtifactRequest, opts ...grpc.CallOption) (*rdbpb.Artifact, error) {
					return &rdbpb.Artifact{
						Name:       in.Name,
						ArtifactId: "big.txt",
						SizeBytes:  128 * 1024,
						FetchUrl:   "https://example.com/big.txt",
					}, nil
				},
			}

			html := `<p>Test failed</p><text-artifact artifact-id="big.txt">`
			res := FormatSummaryHTML(ctx, fakeClient, http.DefaultClient, "invocations/inv1/tests/t1/results/r1", html)
			assert.Loosely(t, res, should.Equal("Test failed\n[Embedded Artifact: big.txt (128.0 KiB (131072 bytes)) - use 'luci artifact get' to view]"))
		})
	})
}

func TestFetchArtifactContent(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchArtifactContent`, t, func(t *ftt.Test) {
		ctx := context.Background()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("artifact data"))
		}))
		defer server.Close()

		var buf bytes.Buffer
		err := FetchArtifactContent(ctx, server.Client(), server.URL, &buf)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, buf.String(), should.Equal("artifact data"))
	})
}

func TestFormatSize(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatSize and FormatCompactSize`, t, func(t *ftt.Test) {
		t.Run(`zero and negative bytes`, func(t *ftt.Test) {
			assert.Loosely(t, FormatSize(0), should.Equal("0 B"))
			assert.Loosely(t, FormatSize(-1), should.Equal("0 B"))
			assert.Loosely(t, FormatCompactSize(0), should.Equal("0 B"))
			assert.Loosely(t, FormatCompactSize(-1), should.Equal("0 B"))
		})
		t.Run(`bytes under 1024`, func(t *ftt.Test) {
			assert.Loosely(t, FormatSize(512), should.Equal("512 B"))
			assert.Loosely(t, FormatCompactSize(512), should.Equal("512 B"))
		})
		t.Run(`KiB`, func(t *ftt.Test) {
			assert.Loosely(t, FormatSize(1536), should.Equal("1.5 KiB (1536 bytes)"))
			assert.Loosely(t, FormatCompactSize(1536), should.Equal("1.5 KiB"))
		})
		t.Run(`MiB`, func(t *ftt.Test) {
			assert.Loosely(t, FormatSize(2621440), should.Equal("2.5 MiB (2621440 bytes)"))
			assert.Loosely(t, FormatCompactSize(2621440), should.Equal("2.5 MiB"))
		})
	})
}

func TestPrintFailureReason(t *testing.T) {
	t.Parallel()

	ftt.Run(`PrintFailureReason`, t, func(t *ftt.Test) {
		t.Run(`nil failure reason`, func(t *ftt.Test) {
			PrintFailureReason(nil, "")
		})

		t.Run(`primary error message only`, func(t *ftt.Test) {
			fr := &rdbpb.FailureReason{PrimaryErrorMessage: "Something went wrong"}
			PrintFailureReason(fr, "")
		})

		t.Run(`single error with message and trace`, func(t *ftt.Test) {
			fr := &rdbpb.FailureReason{
				Errors: []*rdbpb.FailureReason_Error{
					{
						Message: "NullPointerException: object was null",
						Trace:   "java.lang.NullPointerException\n\tat com.example.MyClass.run(MyClass.java:42)",
					},
				},
			}
			PrintFailureReason(fr, "")
		})

		t.Run(`multiple errors`, func(t *ftt.Test) {
			fr := &rdbpb.FailureReason{
				Errors: []*rdbpb.FailureReason_Error{
					{
						Message: "Error 1",
						Trace:   "trace 1",
					},
					{
						Message: "Error 2\nsecond line",
					},
				},
			}
			PrintFailureReason(fr, "  ")
		})

		t.Run(`empty error message with PrimaryErrorMessage fallback`, func(t *ftt.Test) {
			fr := &rdbpb.FailureReason{
				PrimaryErrorMessage: "Primary error message here",
				Errors: []*rdbpb.FailureReason_Error{
					{
						Message: "",
						Trace:   "trace 1",
					},
				},
			}
			PrintFailureReason(fr, "  ")
		})
	})

	ftt.Run(`TruncateFirstLine and FormatFailureReasonFirstLine`, t, func(t *ftt.Test) {
		t.Run(`TruncateFirstLine`, func(t *ftt.Test) {
			s1, t1 := TruncateFirstLine("hello\nworld", 0)
			assert.Loosely(t, s1, should.Equal("hello"))
			assert.Loosely(t, t1, should.BeTrue)

			s2, t2 := TruncateFirstLine("hello world this is a long line", 10)
			assert.Loosely(t, s2, should.Equal("hello worl..."))
			assert.Loosely(t, t2, should.BeTrue)

			sRune, tRune := TruncateFirstLine("你好世界，这是一个测试", 4)
			assert.Loosely(t, sRune, should.Equal("你好世界..."))
			assert.Loosely(t, tRune, should.BeTrue)

			s3, t3 := TruncateFirstLine("short", 10)
			assert.Loosely(t, s3, should.Equal("short"))
			assert.Loosely(t, t3, should.BeFalse)

			s4, t4 := TruncateFirstLine("", 10)
			assert.Loosely(t, s4, should.Equal(""))
			assert.Loosely(t, t4, should.BeFalse)
		})

		t.Run(`FormatFailureReasonFirstLine`, func(t *ftt.Test) {
			s0, t0 := FormatFailureReasonFirstLine(nil, 50)
			assert.Loosely(t, s0, should.Equal(""))
			assert.Loosely(t, t0, should.BeFalse)

			fr := &rdbpb.FailureReason{
				Errors: []*rdbpb.FailureReason_Error{
					{
						Message: "AssertionError: expected true but was false\nextra details here",
					},
				},
			}
			s1, t1 := FormatFailureReasonFirstLine(fr, 100)
			assert.Loosely(t, s1, should.Equal("AssertionError: expected true but was false"))
			assert.Loosely(t, t1, should.BeTrue)

			frSingle := &rdbpb.FailureReason{
				Errors: []*rdbpb.FailureReason_Error{
					{
						Message: "Single line error",
					},
				},
			}
			s2, t2 := FormatFailureReasonFirstLine(frSingle, 100)
			assert.Loosely(t, s2, should.Equal("Single line error"))
			assert.Loosely(t, t2, should.BeFalse)

			frFallback := &rdbpb.FailureReason{
				PrimaryErrorMessage: "Fallback primary error",
				Errors: []*rdbpb.FailureReason_Error{
					{
						Message: "",
					},
				},
			}
			s3, t3 := FormatFailureReasonFirstLine(frFallback, 100)
			assert.Loosely(t, s3, should.Equal("Fallback primary error"))
			assert.Loosely(t, t3, should.BeFalse)
		})
	})

	ftt.Run(`SortArtifacts`, t, func(t *ftt.Test) {
		t.Run(`orders by Huckle ATI priority tiers, test-result before work-unit, and alphabetical`, func(t *ftt.Test) {
			arts := []*rdbpb.Artifact{
				{
					Name:       "rootInvocations/ants-i1/workUnits/wu1/artifacts/zebra.bin",
					ArtifactId: "zebra.bin",
				},
				{
					Name:         "rootInvocations/ants-i1/workUnits/wu1/artifacts/injectables.txt",
					ArtifactId:   "injectables.txt",
					ArtifactType: "TEXT",
				},
				{
					Name:         "rootInvocations/ants-i1/workUnits/wu1/artifacts/host_adb_log.txt",
					ArtifactId:   "host_adb_log.txt",
					ArtifactType: "HOST_LOG",
				},
				{
					Name:         "rootInvocations/ants-i1/workUnits/wu1/artifacts/screenshot.png",
					ArtifactId:   "screenshot.png",
					ArtifactType: "SCREENSHOT",
				},
				{
					Name:         "rootInvocations/ants-i1/workUnits/wu1/artifacts/bugreport.zip",
					ArtifactId:   "bugreport.zip",
					ArtifactType: "BUGREPORTZ",
				},
				{
					Name:         "rootInvocations/ants-i1/workUnits/wu1/artifacts/device_logcat_setup.txt",
					ArtifactId:   "device_logcat_setup.txt",
					ArtifactType: "LOGCAT",
				},
				{
					Name:         "rootInvocations/ants-i1/workUnits/wu1/tests/t1/results/0/artifacts/test_logcat.txt",
					ArtifactId:   "test_logcat.txt",
					ArtifactType: "LOGCAT",
				},
				{
					Name:       "rootInvocations/ants-i1/workUnits/wu1/artifacts/untyped_logcat_fallback.txt",
					ArtifactId: "untyped_logcat_fallback.txt",
				},
			}
			SortArtifacts(arts)
			var gotIDs []string
			for _, a := range arts {
				gotIDs = append(gotIDs, a.ArtifactId)
			}
			assert.Loosely(t, gotIDs, should.Resemble([]string{
				"test_logcat.txt",             // LOGCAT (test-result level first)
				"device_logcat_setup.txt",     // LOGCAT (work-unit level)
				"untyped_logcat_fallback.txt", // inferred LOGCAT from filename
				"bugreport.zip",               // BUGREPORTZ
				"screenshot.png",              // SCREENSHOT
				"host_adb_log.txt",            // HOST_LOG
				"injectables.txt",             // other typed (TEXT)
				"zebra.bin",                   // untyped
			}))
		})
	})

	ftt.Run(`PrintArtifactListTo and PrintInvocationArtifactListTo`, t, func(t *ftt.Test) {
		t.Run(`empty artifacts`, func(t *ftt.Test) {
			var buf bytes.Buffer
			PrintInvocationArtifactListTo(&buf, nil)
			assert.Loosely(t, buf.String(), should.Equal("  (no artifacts)\n\n"))

			buf.Reset()
			PrintArtifactListTo(&buf, nil)
			assert.Loosely(t, buf.String(), should.Equal("  (no artifacts)\n\n"))
		})

		t.Run(`single work unit compact table with command template footer`, func(t *ftt.Test) {
			var buf bytes.Buffer
			arts := []*rdbpb.Artifact{
				{
					Name:         "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/injectables-10101285828505166678.txt",
					ArtifactId:   "injectables-10101285828505166678.txt",
					SizeBytes:    671,
					ContentType:  "text/plain",
					ArtifactType: "TEXT",
				},
				{
					Name:         "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/device_logcat_test_oriole.txt",
					ArtifactId:   "device_logcat_test_oriole.txt",
					SizeBytes:    1258291,
					ContentType:  "text/plain",
					ArtifactType: "LOGCAT",
				},
				{
					Name:         "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/bugreport-oriole-123.zip",
					ArtifactId:   "bugreport-oriole-123.zip",
					SizeBytes:    19293798,
					ContentType:  "application/zip",
					ArtifactType: "BUGREPORTZ",
				},
			}
			PrintArtifactListTo(&buf, arts)
			expected := "" +
				"  ARTIFACT ID                           TYPE        SIZE      CONTENT-TYPE\n" +
				"  device_logcat_test_oriole.txt         LOGCAT      1.2 MiB   text/plain\n" +
				"  bugreport-oriole-123.zip              BUGREPORTZ  18.4 MiB  application/zip\n" +
				"  injectables-10101285828505166678.txt  TEXT        671 B     text/plain\n" +
				"\n" +
				"To fetch an artifact:\n" +
				"  luci work-unit artifact <get|head|tail> -invocationid=ants-i05300010637202289 -workunitid=ants-wu56200010637202289 -artifactid=<ARTIFACT_ID>\n\n"
			assert.Loosely(t, buf.String(), should.Equal(expected))
		})

		t.Run(`single test result compact table with command template footer`, func(t *ftt.Test) {
			var buf bytes.Buffer
			arts := []*rdbpb.Artifact{
				{
					Name:         "rootInvocations/ants-i123/workUnits/wu-1/tests/com.android.FooTest%23testBar/results/0/artifacts/logcat.txt",
					ArtifactId:   "logcat.txt",
					SizeBytes:    145715,
					ContentType:  "text/plain",
					ArtifactType: "LOGCAT",
				},
			}
			PrintArtifactListTo(&buf, arts)
			expected := "" +
				"  ARTIFACT ID  TYPE    SIZE       CONTENT-TYPE\n" +
				"  logcat.txt   LOGCAT  142.3 KiB  text/plain\n" +
				"\n" +
				"To fetch an artifact:\n" +
				"  luci test-result artifact <get|head|tail> -invocationid=ants-i123 -workunitid=wu-1 -testid='com.android.FooTest#testBar' -resultid=0 -artifactid=<ARTIFACT_ID>\n\n"
			assert.Loosely(t, buf.String(), should.Equal(expected))
		})

		t.Run(`root invocation work unit and test result artifacts`, func(t *ftt.Test) {
			var buf bytes.Buffer
			arts := []*rdbpb.Artifact{
				{
					Name:         "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/bugreport-oriole-123.zip",
					ArtifactId:   "bugreport-oriole-123.zip",
					SizeBytes:    19293798,
					ContentType:  "application/zip",
					ArtifactType: "BUGREPORTZ",
				},
				{
					Name:         "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202290/tests/com.android.FooTest%23testBar/results/0/artifacts/logcat_test_foo_123.txt",
					ArtifactId:   "logcat_test_foo_123.txt",
					SizeBytes:    145715,
					ContentType:  "text/plain",
					ArtifactType: "LOGCAT",
				},
			}
			PrintInvocationArtifactListTo(&buf, arts)
			expected := "" +
				"  ARTIFACT ID               TYPE        SIZE       CONTENT-TYPE     FLAGS\n" +
				"  logcat_test_foo_123.txt   LOGCAT      142.3 KiB  text/plain       -workunitid=ants-wu56200010637202290 -testid='com.android.FooTest#testBar' -resultid=0\n" +
				"  bugreport-oriole-123.zip  BUGREPORTZ  18.4 MiB   application/zip  -workunitid=ants-wu56200010637202289\n" +
				"\n" +
				"To fetch an artifact:\n" +
				"  luci artifact <get|head|tail> -invocationid=ants-i05300010637202289 -artifactid=<ARTIFACT_ID> [FLAGS]\n\n"
			assert.Loosely(t, buf.String(), should.Equal(expected))
		})
	})

	ftt.Run(`RootInvocation formatting helpers`, t, func(t *ftt.Test) {
		t.Run(`FormatRootInvocationState`, func(t *ftt.Test) {
			assert.Loosely(t, FormatRootInvocationState(nil), should.Equal(""))
			assert.Loosely(t, FormatRootInvocationState(&rdbpb.RootInvocation{}), should.Equal(""))
			assert.Loosely(t, FormatRootInvocationState(&rdbpb.RootInvocation{
				State:             rdbpb.RootInvocation_SUCCEEDED,
				FinalizationState: rdbpb.RootInvocation_FINALIZED,
			}), should.Equal("SUCCEEDED (FINALIZED)"))
			assert.Loosely(t, FormatRootInvocationState(&rdbpb.RootInvocation{
				State: rdbpb.RootInvocation_RUNNING,
			}), should.Equal("RUNNING"))
		})

		t.Run(`FormatBuildDescriptor and FormatExtraBuilds`, func(t *ftt.Test) {
			assert.Loosely(t, FormatBuildDescriptor(nil), should.Equal(""))
			bd1 := &rdbpb.BuildDescriptor{
				Definition: &rdbpb.BuildDescriptor_AndroidBuild{
					AndroidBuild: &rdbpb.AndroidBuildDescriptor{
						DataRealm:   "prod",
						Branch:      "git_main",
						BuildTarget: "cf_x86_64_phone-trunk_staging-userdebug",
						BuildId:     "16133789",
					},
				},
			}
			assert.Loosely(t, FormatBuildDescriptor(bd1), should.Equal("branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=16133789"))

			bd2 := &rdbpb.BuildDescriptor{
				Definition: &rdbpb.BuildDescriptor_AndroidBuild{
					AndroidBuild: &rdbpb.AndroidBuildDescriptor{
						DataRealm:   "staging",
						Branch:      "aosp-main",
						BuildTarget: "test_suites_x86_64",
						BuildId:     "P999999",
					},
				},
			}
			assert.Loosely(t, FormatExtraBuilds([]*rdbpb.BuildDescriptor{bd1, bd2}), should.Equal(
				"branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=16133789; branch=aosp-main build_target=test_suites_x86_64 build_id=P999999 data_realm=staging",
			))
		})

		t.Run(`FormatSources`, func(t *ftt.Test) {
			assert.Loosely(t, FormatSources(nil), should.Equal(""))
			src := &rdbpb.Sources{
				BaseSources: &rdbpb.Sources_SubmittedAndroidBuild{
					SubmittedAndroidBuild: &rdbpb.SubmittedAndroidBuild{
						DataRealm: "prod",
						Branch:    "git_main",
						BuildId:   16133789,
					},
				},
				Changelists: []*rdbpb.GerritChange{
					{
						Host:     "android-review.googlesource.com",
						Project:  "platform/packages/apps/CellBroadcastReceiver",
						Change:   3456789,
						Patchset: 2,
					},
				},
				IsDirty: true,
			}
			assert.Loosely(t, FormatSources(src), should.Equal(
				"submitted_android_build(branch=git_main build_id=16133789); changelist(host=android-review.googlesource.com project=platform/packages/apps/CellBroadcastReceiver change=3456789 patchset=2); is_dirty=true",
			))

			gitSrc := &rdbpb.Sources{
				BaseSources: &rdbpb.Sources_GitilesCommit{
					GitilesCommit: &rdbpb.GitilesCommit{
						Host:       "chromium.googlesource.com",
						Project:    "chromium/src",
						Ref:        "refs/heads/main",
						CommitHash: "abcdef123456",
						Position:   12345,
					},
				},
			}
			assert.Loosely(t, FormatSources(gitSrc), should.Equal(
				"gitiles_commit(host=chromium.googlesource.com project=chromium/src ref=refs/heads/main commit_hash=abcdef123456 position=12345)",
			))
		})

		t.Run(`FormatRootInvocationDefinition and FormatProducerResource`, func(t *ftt.Test) {
			def := &rdbpb.RootInvocationDefinition{
				System: "atp",
				Name:   "v2/android-crystalball-eng/health/microbench/uibench/HotAppStartTest",
				Properties: &rdbpb.RootInvocationDefinition_Properties{
					Def: map[string]string{
						"run_target": "brya_device_on_satlab",
						"cluster_id": "freeform-test-runner",
					},
				},
			}
			assert.Loosely(t, FormatRootInvocationDefinition(def), should.Equal(
				"system=atp name=v2/android-crystalball-eng/health/microbench/uibench/HotAppStartTest [cluster_id=freeform-test-runner run_target=brya_device_on_satlab]",
			))

			pr := &rdbpb.ProducerResource{
				System: "ants",
				Name:   "ants-i14600010609614895",
				Url:    "https://android-build.corp.google.com/test_investigate/invocation/I14600010609614895",
			}
			assert.Loosely(t, FormatProducerResource(pr), should.Equal(
				"system=ants name=ants-i14600010609614895 (https://android-build.corp.google.com/test_investigate/invocation/I14600010609614895)",
			))
		})

		t.Run(`FprintRootInvocationMetadata and FprintRootInvocationAdditionalMetadata`, func(t *ftt.Test) {
			props, _ := structpb.NewStruct(map[string]any{
				"scheduler": "ATP",
			})
			rootInv := &rdbpb.RootInvocation{
				Name:              "rootInvocations/ants-i14600010609614895",
				State:             rdbpb.RootInvocation_SUCCEEDED,
				FinalizationState: rdbpb.RootInvocation_FINALIZED,
				Realm:             "google:android-test-results",
				BaselineId:        "atp:v2/test",
				PrimaryBuild: &rdbpb.BuildDescriptor{
					Definition: &rdbpb.BuildDescriptor_AndroidBuild{
						AndroidBuild: &rdbpb.AndroidBuildDescriptor{
							DataRealm:   "prod",
							Branch:      "git_main",
							BuildTarget: "cf_x86_64_phone-trunk_staging-userdebug",
							BuildId:     "16133789",
						},
					},
				},
				ExtraBuilds: []*rdbpb.BuildDescriptor{
					{
						Definition: &rdbpb.BuildDescriptor_AndroidBuild{
							AndroidBuild: &rdbpb.AndroidBuildDescriptor{
								DataRealm:   "prod",
								Branch:      "git_main",
								BuildTarget: "test_suites_x86_64",
								BuildId:     "16133790",
							},
						},
					},
				},
				Sources: &rdbpb.Sources{
					BaseSources: &rdbpb.Sources_SubmittedAndroidBuild{
						SubmittedAndroidBuild: &rdbpb.SubmittedAndroidBuild{
							DataRealm: "prod",
							Branch:    "git_main",
							BuildId:   16133789,
						},
					},
				},
				Definition: &rdbpb.RootInvocationDefinition{
					System: "atp",
					Name:   "v2/android-mts/mts-cellbroadcast",
					Properties: &rdbpb.RootInvocationDefinition_Properties{
						Def: map[string]string{
							"run_target": "cf_x86_64_phone",
						},
					},
				},
				ProducerResource: &rdbpb.ProducerResource{
					System: "ants",
					Name:   "ants-i14600010609614895",
				},
				Tags: []*rdbpb.StringPair{
					{Key: "test_plan", Value: "mts"},
				},
				Properties: props,
			}

			var buf bytes.Buffer
			FprintRootInvocationMetadata(&buf, rootInv)
			expectedMeta := "" +
				"Inv State:     SUCCEEDED (FINALIZED)\n" +
				"Primary Build: branch=git_main build_target=cf_x86_64_phone-trunk_staging-userdebug build_id=16133789\n" +
				"Extra Builds:  branch=git_main build_target=test_suites_x86_64 build_id=16133790\n" +
				"Sources:       submitted_android_build(branch=git_main build_id=16133789)\n" +
				"Definition:    system=atp name=v2/android-mts/mts-cellbroadcast [run_target=cf_x86_64_phone]\n" +
				"Producer:      system=ants name=ants-i14600010609614895\n"
			assert.Loosely(t, buf.String(), should.Equal(expectedMeta))

			buf.Reset()
			FprintRootInvocationAdditionalMetadata(&buf, rootInv)
			assert.Loosely(t, buf.String(), should.ContainSubstring("Realm:        google:android-test-results\n"))
			assert.Loosely(t, buf.String(), should.ContainSubstring("Baseline ID:  atp:v2/test\n"))
			assert.Loosely(t, buf.String(), should.ContainSubstring("Invocation Tags:\n  test_plan: mts\n"))
			assert.Loosely(t, buf.String(), should.ContainSubstring("Invocation Properties:\n"))
			assert.Loosely(t, buf.String(), should.ContainSubstring(`"scheduler"`))
			assert.Loosely(t, buf.String(), should.ContainSubstring(`"ATP"`))
		})

		t.Run(`FprintWorkUnitMetadata`, func(t *ftt.Test) {
			wuProps, _ := structpb.NewStruct(map[string]any{"abi": "x86_64"})
			wu := &rdbpb.WorkUnit{
				Name:   "rootInvocations/ants-i123/workUnits/ants-wu456",
				Parent: "rootInvocations/ants-i123/workUnits/root",
				ModuleId: &rdbpb.ModuleIdentifier{
					ModuleScheme:      "junit",
					ModuleVariantHash: "abcd1234",
					ModuleVariant: &rdbpb.Variant{
						Def: map[string]string{"abi": "x86_64"},
					},
				},
				Tags: []*rdbpb.StringPair{
					{Key: "module_tag", Value: "val1"},
				},
				Properties: wuProps,
			}

			var buf bytes.Buffer
			FprintWorkUnitMetadata(&buf, wu, "", false)
			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Full Name:    rootInvocations/ants-i123/workUnits/ants-wu456\n"))
			assert.Loosely(t, out, should.NotContainSubstring("Parent:"))
			assert.Loosely(t, out, should.ContainSubstring("Scheme:       junit\n"))
			assert.Loosely(t, out, should.ContainSubstring("Variant:      abi=x86_64 (hash: abcd1234)\n"))
			assert.Loosely(t, out, should.ContainSubstring("Tags:\n  module_tag: val1\n"))
			assert.Loosely(t, out, should.ContainSubstring("Properties:\n"))
			assert.Loosely(t, out, should.ContainSubstring(`"abi"`))
			assert.Loosely(t, out, should.ContainSubstring(`"x86_64"`))

			buf.Reset()
			FprintWorkUnitMetadata(&buf, wu, "    ", true)
			indentedOut := buf.String()
			assert.Loosely(t, indentedOut, should.ContainSubstring("    Full Name:    rootInvocations/ants-i123/workUnits/ants-wu456\n"))
			assert.Loosely(t, indentedOut, should.ContainSubstring("    Parent:       Work Unit root in invocation ants-i123\n"))
			assert.Loosely(t, indentedOut, should.ContainSubstring("    Scheme:       junit\n"))
			assert.Loosely(t, indentedOut, should.ContainSubstring("    Variant:      abi=x86_64 (hash: abcd1234)\n"))
			assert.Loosely(t, indentedOut, should.ContainSubstring("    Tags:\n      module_tag: val1\n"))
			assert.Loosely(t, indentedOut, should.ContainSubstring("    Properties:\n      {"))
		})
	})
}
