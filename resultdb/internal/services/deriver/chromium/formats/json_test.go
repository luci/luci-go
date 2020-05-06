// Copyright 2020 The LUCI Authors.
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

package formats

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestJSONConversions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`From JSON works`, t, func() {
		buf := []byte(`
		{
			"version": 3,
			"interrupted": false,
			"path_delimiter": "::",
			"metadata": {"test_name_prefix": "prefix."},
			"builder_name": "Linux Tests",
			"build_number": "82046",
			"tests": {
				"c1": {
					"c2": {
						"t1.html": {
							"actual": "PASS PASS PASS",
							"expected": "PASS",
							"time": 0.3,
							"times": [0.2, 0.1]
						},
						"t2.html": {
							"actual": "PASS FAIL PASS",
							"expected": "PASS FAIL",
							"times": [0.05]
						}
					}
				},
				"c2": {
					"t3.html": {
						"actual": "FAIL",
						"expected": "PASS",
						"artifacts": {
							"log": ["relative/path/to/log"]
						}
					}
				},
				"c3": {
					"time": {
						"time-t1.html": {
							"actual": "PASS",
							"expected": "PASS",
							"time": 0.4,
							"artifacts": {
								"reason": "inlined string"
							}
						}
					}
				}
			}
		}`)

		Convey(`Works`, func() {
			results := &JSONTestResults{}
			err := results.ConvertFromJSON(ctx, bytes.NewReader(buf))
			So(err, ShouldBeNil)
			So(results, ShouldNotBeNil)
			So(results.Version, ShouldEqual, 3)
			So(results.Interrupted, ShouldEqual, false)
			So(results.PathDelimiter, ShouldEqual, "::")
			So(results.BuildNumber, ShouldEqual, "82046")
			So(results.BuilderName, ShouldEqual, "Linux Tests")
			So(results.Tests, ShouldResemble, map[string]*TestFields{
				"prefix.c1::c2::t1.html": {
					Actual:   "PASS PASS PASS",
					Expected: "PASS",
					Time:     0.3,
					Times:    []float64{0.2, 0.1},
				},
				"prefix.c1::c2::t2.html": {
					Actual:   "PASS FAIL PASS",
					Expected: "PASS FAIL",
					Times:    []float64{0.05},
				},
				"prefix.c2::t3.html": {
					Actual:   "FAIL",
					Expected: "PASS",
					ArtifactsRaw: map[string]json.RawMessage{
						"log": json.RawMessage(`["relative/path/to/log"]`),
					},
					Artifacts: map[string][]string{
						"log": {"relative/path/to/log"},
					},
				},
				"prefix.c3::time::time-t1.html": {
					Actual:   "PASS",
					Expected: "PASS",
					Time:     0.4,
					ArtifactsRaw: map[string]json.RawMessage{
						"reason": json.RawMessage(`"inlined string"`),
					},
				},
			})

			Convey(`with default path delimiter`, func() {
				// Clear the delimiter and already processed and flattened tests.
				results.PathDelimiter = ""
				results.Tests = make(map[string]*TestFields)

				err := results.convertTests("", results.TestsRaw)
				So(err, ShouldBeNil)
				So(results, ShouldNotBeNil)

				paths := make([]string, 0, len(results.Tests))
				for path := range results.Tests {
					paths = append(paths, path)
				}
				sort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })
				So(paths, ShouldResemble, []string{
					"prefix.c1/c2/t1.html",
					"prefix.c1/c2/t2.html",
					"prefix.c2/t3.html",
					"prefix.c3/time/time-t1.html",
				})
			})
		})
	})

	Convey(`ToProtos works`, t, func() {
		results := &JSONTestResults{
			Interrupted: true,
			Metadata: map[string]json.RawMessage{
				"tags": json.RawMessage(`["linux", "ubuntu", "desktop"]`),
			},
			Version: 3,
			Tests: map[string]*TestFields{
				"c1/c2/t1.html": {
					Actual:   "PASS PASS PASS",
					Expected: "PASS",
					Time:     0.3,
					Times:    []float64{0.3, 0.2, 0.1},
					Artifacts: map[string][]string{
						"isolate_object_list": {
							"harness/log.txt",
							"harness/retry_1/log.txt",
							"harness/retry_2/log.txt",
							"harness/retry_wat/log.txt",
						},
						"ref_mismatch": {
							"relative/path/to/retry_2/about:blank",
						},
					},
				},
				"c1/c2/t2.html": {
					Actual:   "PASS FAIL PASS CRASH",
					Expected: "PASS FAIL",
					Times:    []float64{0.05, 0.05, 0.05, 0.05},
				},
				"c2/t3.html": {
					Actual:   "FAIL",
					Expected: "PASS",
					Artifacts: map[string][]string{
						"isolate_object": {"relative/path/to/log.txt"},
						"gold_triage_link": {
							"https://chrome-gpu-gold.skia.org/detail?test=foo&digest=beef",
						},
						"isolate_object_list": {
							"relative/path/to/diff.png",
							"unknown",
						},
					},
				},
				"c2/t4.html": {
					Actual:   "PASS PASS PASS",
					Expected: "PASS",
					Time:     0.3,
				},
			},
		}

		availableArtifacts := stringset.NewFromSlice(
			"harness/log.txt",
			"harness/retry_1/log.txt",
			"harness/retry_2/log.txt",
			"relative/path/to/log.txt",
			"relative/path/to/diff.png",
		)

		inv := &pb.Invocation{}
		testResults, err := results.ToProtos(ctx, "ninja://tests/", inv, availableArtifacts)
		So(err, ShouldBeNil)
		So(inv.Tags, ShouldResembleProto, pbutil.StringPairs(
			"json_format_tag", "desktop",
			"json_format_tag", "linux",
			"json_format_tag", "ubuntu",
			OriginalFormatTagKey, FormatJTR,
		))

		assertTestResultsResemble(testResults, []*TestResult{
			// Test 1.
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c1/c2/t1.html",
					Status:   pb.TestStatus_PASS,
					Expected: true,
					Duration: &duration.Duration{Nanos: 3e8},
					Tags:     pbutil.StringPairs("json_format_status", "PASS"),
				},
				Artifacts: map[string]string{"isolate_object_list": "harness/log.txt"},
			},
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c1/c2/t1.html",
					Status:   pb.TestStatus_PASS,
					Expected: true,
					Duration: &duration.Duration{Nanos: 2e8},
					Tags:     pbutil.StringPairs("json_format_status", "PASS"),
				},
				Artifacts: map[string]string{"isolate_object_list": "harness/retry_1/log.txt"},
			},
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c1/c2/t1.html",
					Status:   pb.TestStatus_PASS,
					Expected: true,
					Duration: &duration.Duration{Nanos: 1e8},
					Tags:     pbutil.StringPairs("json_format_status", "PASS"),
				},
				Artifacts: map[string]string{"isolate_object_list": "harness/retry_2/log.txt"},
			},

			// Test 2.
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c1/c2/t2.html",
					Status:   pb.TestStatus_PASS,
					Expected: true,
					Duration: &duration.Duration{Nanos: 5e7},
					Tags:     pbutil.StringPairs("json_format_status", "PASS"),
				},
			},
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c1/c2/t2.html",
					Status:   pb.TestStatus_FAIL,
					Expected: true,
					Duration: &duration.Duration{Nanos: 5e7},
					Tags:     pbutil.StringPairs("json_format_status", "FAIL"),
				},
			},
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c1/c2/t2.html",
					Status:   pb.TestStatus_PASS,
					Expected: true,
					Duration: &duration.Duration{Nanos: 5e7},
					Tags:     pbutil.StringPairs("json_format_status", "PASS"),
				},
			},
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c1/c2/t2.html",
					Status:   pb.TestStatus_CRASH,
					Expected: false,
					Duration: &duration.Duration{Nanos: 5e7},
					Tags:     pbutil.StringPairs("json_format_status", "CRASH"),
				},
			},

			// Test 3
			{
				TestResult: &pb.TestResult{
					TestId:      "ninja://tests/c2/t3.html",
					Status:      pb.TestStatus_FAIL,
					Expected:    false,
					Tags:        pbutil.StringPairs("json_format_status", "FAIL"),
					SummaryHtml: `<ul><li><a href="https://chrome-gpu-gold.skia.org/detail?test=foo&amp;digest=beef">gold_triage_link</a></li></ul>`,
				},
				Artifacts: map[string]string{
					"isolate_object_list": "relative/path/to/diff.png",
					"isolate_object":      "relative/path/to/log.txt",
				},
			},

			// Test 4
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c2/t4.html",
					Status:   pb.TestStatus_PASS,
					Expected: true,
					Duration: &duration.Duration{Nanos: 3e8},
					Tags:     pbutil.StringPairs("json_format_status", "PASS"),
				},
			},
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c2/t4.html",
					Status:   pb.TestStatus_PASS,
					Expected: true,
					Tags:     pbutil.StringPairs("json_format_status", "PASS"),
				},
			},
			{
				TestResult: &pb.TestResult{
					TestId:   "ninja://tests/c2/t4.html",
					Status:   pb.TestStatus_PASS,
					Expected: true,
					Tags:     pbutil.StringPairs("json_format_status", "PASS"),
				},
			},
		})
	})
}

func assertTestResultsResemble(actual, expected []*TestResult) {
	So(actual, ShouldHaveLength, len(expected))
	for i := range actual {
		So(actual[i].TestResult, ShouldResembleProto, expected[i].TestResult)
		So(actual[i].Artifacts, ShouldResemble, expected[i].Artifacts)
	}
}

func TestArtifactUtils(t *testing.T) {
	t.Parallel()

	Convey(`Logging`, t, func() {
		arts := map[string][]string{
			"stdout": {"log_0.txt", "log_1.txt"},
			"links":  {"https://linklink.com"},
		}
		So(artifactsToString(arts), ShouldEqual,
			"links\n\thttps://linklink.com\nstdout\n\tlog_0.txt\n\tlog_1.txt\n")
	})

	Convey(`Checking subdirs`, t, func() {
		ctx := testutil.TestingContext()

		available := stringset.NewFromSlice(
			"artifacts/a/stdout.txt",
			"artifacts/b/stderr.txt",
			"layout-test-results/c/stderr.txt",
			"d/stderr.txt",
		)
		f := &TestFields{Artifacts: map[string][]string{
			"a": {"a/stdout.txt"},
			"b": {"b\\stderr.txt"},
			"c": {"c/stderr.txt"},
			"d": {"d/stderr.txt"},
		}}

		artifactsPerRun := f.parseArtifacts(ctx, available, "testID")
		So(artifactsPerRun, ShouldHaveLength, 1)

		arts := artifactsPerRun[0].artifacts
		So(arts, ShouldResemble, map[string]string{
			"a": "artifacts/a/stdout.txt",
			"b": "artifacts/b/stderr.txt",
			"c": "layout-test-results/c/stderr.txt",
			"d": "d/stderr.txt",
		})
	})
}
