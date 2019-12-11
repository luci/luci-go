// Copyright 2019 The LUCI Authors.
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

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestJSONConversions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`From JSON works`, t, func() {
		buf := []byte(
			`{
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
									"times": [ 0.2, 0.1 ]
								},
								"t2.html": {
									"actual": "PASS FAIL PASS",
									"expected": "PASS FAIL",
									"times": [ 0.05 ]
								}
							}
						},
						"c2": {
							"t3.html": {
								"actual": "FAIL",
								"expected": "PASS"
							}
						},
						"c3": {
							"time": {
								"time-t1.html": {
									"actual": "PASS",
									"expected": "PASS",
									"time": 0.4
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
				},
				"prefix.c3::time::time-t1.html": {
					Actual:   "PASS",
					Expected: "PASS",
					Time:     0.4,
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

		isolatedOutputs := map[string]*pb.Artifact{
			"harness/log.txt":         {Name: "log_0.txt"},
			"harness/retry_1/log.txt": {Name: "log_1.txt"},
			"harness/retry_2/log.txt": {Name: "log_2.txt"},
			"relative/path/to/log.txt": {
				Name:        "relative/path/to/log.txt",
				FetchUrl:    "isolate://isosrv/a104",
				ContentType: "plain/text",
				Size:        32,
			},
			"relative/path/to/diff.png": {
				Name:        "relative/path/to/diff.png",
				FetchUrl:    "isolate://isosrv/ad1ff",
				ContentType: "image/png",
				Size:        8192,
			},
		}

		inv := &pb.Invocation{}
		testResults, err := results.ToProtos(ctx, "ninja://tests/", inv, isolatedOutputs)
		So(err, ShouldBeNil)
		So(inv.State, ShouldEqual, pb.Invocation_INTERRUPTED)
		So(inv.Tags, ShouldResembleProto, pbutil.StringPairs(
			"json_format_tag", "desktop",
			"json_format_tag", "linux",
			"json_format_tag", "ubuntu",
			OriginalFormatTagKey, FormatJTR,
		))

		So(testResults, ShouldResembleProto, []*pb.TestResult{
			// Test 1.
			{
				TestPath:        "ninja://tests/c1/c2/t1.html",
				Status:          pb.TestStatus_PASS,
				Expected:        true,
				Duration:        &duration.Duration{Nanos: 3e8},
				Tags:            pbutil.StringPairs("json_format_status", "PASS"),
				OutputArtifacts: []*pb.Artifact{{Name: "log_0.txt"}},
			},
			{
				TestPath:        "ninja://tests/c1/c2/t1.html",
				Status:          pb.TestStatus_PASS,
				Expected:        true,
				Duration:        &duration.Duration{Nanos: 2e8},
				Tags:            pbutil.StringPairs("json_format_status", "PASS"),
				OutputArtifacts: []*pb.Artifact{{Name: "log_1.txt"}},
			},
			{
				TestPath:        "ninja://tests/c1/c2/t1.html",
				Status:          pb.TestStatus_PASS,
				Expected:        true,
				Duration:        &duration.Duration{Nanos: 1e8},
				Tags:            pbutil.StringPairs("json_format_status", "PASS"),
				OutputArtifacts: []*pb.Artifact{{Name: "log_2.txt"}},
			},

			// Test 2.
			{
				TestPath: "ninja://tests/c1/c2/t2.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Duration: &duration.Duration{Nanos: 5e7},
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "ninja://tests/c1/c2/t2.html",
				Status:   pb.TestStatus_FAIL,
				Expected: true,
				Duration: &duration.Duration{Nanos: 5e7},
				Tags:     pbutil.StringPairs("json_format_status", "FAIL"),
			},
			{
				TestPath: "ninja://tests/c1/c2/t2.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Duration: &duration.Duration{Nanos: 5e7},
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "ninja://tests/c1/c2/t2.html",
				Status:   pb.TestStatus_CRASH,
				Expected: false,
				Duration: &duration.Duration{Nanos: 5e7},
				Tags:     pbutil.StringPairs("json_format_status", "CRASH"),
			},

			// Test 3
			{
				TestPath: "ninja://tests/c2/t3.html",
				Status:   pb.TestStatus_FAIL,
				Expected: false,
				Tags:     pbutil.StringPairs("json_format_status", "FAIL"),
				OutputArtifacts: []*pb.Artifact{
					{
						Name:    "gold_triage_link",
						ViewUrl: "https://chrome-gpu-gold.skia.org/detail?test=foo&digest=beef",
					},
					{
						Name:        "relative/path/to/diff.png",
						FetchUrl:    "isolate://isosrv/ad1ff",
						ContentType: "image/png",
						Size:        8192,
					},
					{
						Name:        "relative/path/to/log.txt",
						FetchUrl:    "isolate://isosrv/a104",
						ContentType: "plain/text",
						Size:        32,
					},
				},
			},

			// Test 4
			{
				TestPath: "ninja://tests/c2/t4.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Duration: &duration.Duration{Nanos: 3e8},
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "ninja://tests/c2/t4.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "ninja://tests/c2/t4.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
		})
	})
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
		outputsToProcess := map[string]*pb.Artifact{
			"artifacts/a/stdout.txt":           {Name: "artifacts/a/stdout.txt"},
			"artifacts/a/stderr.txt":           {Name: "artifacts/a/stderr.txt"},
			"layout-test-results/b/stderr.txt": {Name: "layout-test-results/b/stderr.txt"},
			"c/stderr.txt":                     {Name: "c/stderr.txt"},
		}
		f := &TestFields{Artifacts: map[string][]string{
			"a": {"a/stdout.txt", "a\\stderr.txt"},
			"b": {"b/stderr.txt"},
			"c": {"c/stderr.txt"},
		}}

		artifactsPerRun, unresolved := f.getArtifacts(outputsToProcess)
		So(artifactsPerRun, ShouldHaveLength, 1)

		pbutil.NormalizeArtifactSlice(artifactsPerRun[0])
		So(artifactsPerRun[0], ShouldResemble, []*pb.Artifact{
			{Name: "artifacts/a/stderr.txt"},
			{Name: "artifacts/a/stdout.txt"},
			{Name: "c/stderr.txt"},
			{Name: "layout-test-results/b/stderr.txt"},
		})
		So(unresolved, ShouldBeEmpty)
	})
}
