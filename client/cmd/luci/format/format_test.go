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
				"  luci test-result artifact <get|head|tail> -invocationid=ants-i123 -workunitid=wu-1 -testid=\"com.android.FooTest#testBar\" -resultid=0 -artifactid=<ARTIFACT_ID>\n\n"
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
				"  logcat_test_foo_123.txt   LOGCAT      142.3 KiB  text/plain       -workunitid=ants-wu56200010637202290 -testid=\"com.android.FooTest#testBar\" -resultid=0\n" +
				"  bugreport-oriole-123.zip  BUGREPORTZ  18.4 MiB   application/zip  -workunitid=ants-wu56200010637202289\n" +
				"\n" +
				"To fetch an artifact:\n" +
				"  luci artifact <get|head|tail> -invocationid=ants-i05300010637202289 -artifactid=<ARTIFACT_ID> [FLAGS]\n\n"
			assert.Loosely(t, buf.String(), should.Equal(expected))
		})
	})
}
