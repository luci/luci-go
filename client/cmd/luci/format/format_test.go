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
	pb "go.chromium.org/luci/resultdb/proto/v1"
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
			v := &pb.Variant{Def: map[string]string{}}
			assert.Loosely(t, FormatVariant(v), should.Equal(""))
		})

		t.Run(`populated variant`, func(t *ftt.Test) {
			v := &pb.Variant{
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
	pb.ResultDBClient
	getArtifactFunc func(ctx context.Context, in *pb.GetArtifactRequest, opts ...grpc.CallOption) (*pb.Artifact, error)
}

func (f *fakeRDBClient) GetArtifact(ctx context.Context, in *pb.GetArtifactRequest, opts ...grpc.CallOption) (*pb.Artifact, error) {
	if f.getArtifactFunc != nil {
		return f.getArtifactFunc(ctx, in, opts...)
	}
	return nil, nil
}

func TestFormatSummaryHTML(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatSummaryHTML`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`without showArtifacts`, func(t *ftt.Test) {
			html := `<p>Test failed</p><br><text-artifact artifact-id="log.txt">`
			res := FormatSummaryHTML(ctx, nil, nil, "invocations/inv1/tests/t1/results/r1", html, false)
			assert.Loosely(t, res, should.Equal("Test failed\n\n[Embedded Artifact: log.txt (pass --show-artifacts to view)]"))
		})

		t.Run(`with showArtifacts`, func(t *ftt.Test) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Error log content from artifact"))
			}))
			defer server.Close()

			fakeClient := &fakeRDBClient{
				getArtifactFunc: func(ctx context.Context, in *pb.GetArtifactRequest, opts ...grpc.CallOption) (*pb.Artifact, error) {
					return &pb.Artifact{
						Name:       in.Name,
						ArtifactId: "log.txt",
						FetchUrl:   server.URL,
					}, nil
				},
			}

			html := `<p>Test failed</p><text-artifact artifact-id="log.txt">`
			res := FormatSummaryHTML(ctx, fakeClient, server.Client(), "invocations/inv1/tests/t1/results/r1", html, true)
			assert.Loosely(t, res, should.Equal("Test failed\n\n--- Embedded Artifact: log.txt ---\nError log content from artifact\n--- End Artifact ---"))
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

	ftt.Run(`FormatSize`, t, func(t *ftt.Test) {
		t.Run(`zero bytes`, func(t *ftt.Test) {
			assert.Loosely(t, FormatSize(0), should.Equal("0 B"))
		})
		t.Run(`bytes under 1024`, func(t *ftt.Test) {
			assert.Loosely(t, FormatSize(512), should.Equal("512 B"))
		})
		t.Run(`KiB`, func(t *ftt.Test) {
			assert.Loosely(t, FormatSize(1536), should.Equal("1.5 KiB (1536 bytes)"))
		})
		t.Run(`MiB`, func(t *ftt.Test) {
			assert.Loosely(t, FormatSize(2621440), should.Equal("2.5 MiB (2621440 bytes)"))
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
			fr := &pb.FailureReason{PrimaryErrorMessage: "Something went wrong"}
			PrintFailureReason(fr, "")
		})

		t.Run(`single error with message and trace`, func(t *ftt.Test) {
			fr := &pb.FailureReason{
				Errors: []*pb.FailureReason_Error{
					{
						Message: "NullPointerException: object was null",
						Trace:   "java.lang.NullPointerException\n\tat com.example.MyClass.run(MyClass.java:42)",
					},
				},
			}
			PrintFailureReason(fr, "")
		})

		t.Run(`multiple errors`, func(t *ftt.Test) {
			fr := &pb.FailureReason{
				Errors: []*pb.FailureReason_Error{
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
			fr := &pb.FailureReason{
				PrimaryErrorMessage: "Primary error message here",
				Errors: []*pb.FailureReason_Error{
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
			assert.Loosely(t, TruncateFirstLine("hello\nworld", 0), should.Equal("hello"))
			assert.Loosely(t, TruncateFirstLine("hello world this is a long line", 10), should.Equal("hello worl..."))
			assert.Loosely(t, TruncateFirstLine("你好世界，这是一个测试", 4), should.Equal("你好世界..."))
			assert.Loosely(t, TruncateFirstLine("", 10), should.Equal(""))
		})

		t.Run(`FormatFailureReasonFirstLine`, func(t *ftt.Test) {
			assert.Loosely(t, FormatFailureReasonFirstLine(nil, 50), should.Equal(""))
			fr := &pb.FailureReason{
				Errors: []*pb.FailureReason_Error{
					{
						Message: "AssertionError: expected true but was false\nextra details here",
					},
				},
			}
			assert.Loosely(t, FormatFailureReasonFirstLine(fr, 100), should.Equal("AssertionError: expected true but was false"))

			frFallback := &pb.FailureReason{
				PrimaryErrorMessage: "Fallback primary error",
				Errors: []*pb.FailureReason_Error{
					{
						Message: "",
					},
				},
			}
			assert.Loosely(t, FormatFailureReasonFirstLine(frFallback, 100), should.Equal("Fallback primary error"))
		})
	})
}
