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
				},
				"c1/c2/t2.html": {
					Actual:   "PASS FAIL PASS CRASH",
					Expected: "PASS FAIL",
					Times:    []float64{0.05, 0.05, 0.05, 0.05},
				},
				"c2/t3.html": {
					Actual:   "FAIL",
					Expected: "PASS",
				},
				"c2/t4.html": {
					Actual:   "PASS PASS PASS",
					Expected: "PASS",
					Time:     0.3,
				},
			},
		}

		req := &pb.DeriveInvocationRequest{
			SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
				Hostname: "host-swarming",
				Id:       "123",
			},
			TestPathPrefix: "prefix/",
			BaseTestVariant: &pb.VariantDef{Def: map[string]string{
				"bucket":     "bkt",
				"builder":    "blder",
				"test_suite": "foo_unittests",
			}},
		}

		inv := &pb.Invocation{}
		testResults, err := results.ToProtos(ctx, req, inv)
		So(err, ShouldBeNil)
		So(inv.State, ShouldEqual, pb.Invocation_INTERRUPTED)
		So(inv.Tags, ShouldResembleProto, pbutil.StringPairs(
			"json_format_tag", "desktop",
			"json_format_tag", "linux",
			"json_format_tag", "ubuntu",
			"test_framework", "json",
		))

		So(testResults, ShouldResembleProto, []*pb.TestResult{
			// Test 1.
			{
				TestPath: "prefix/c1/c2/t1.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Duration: &duration.Duration{Nanos: 3e8},
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "prefix/c1/c2/t1.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Duration: &duration.Duration{Nanos: 2e8},
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "prefix/c1/c2/t1.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Duration: &duration.Duration{Nanos: 1e8},
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},

			// Test 2.
			{
				TestPath: "prefix/c1/c2/t2.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Duration: &duration.Duration{Nanos: 5e7},
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "prefix/c1/c2/t2.html",
				Status:   pb.TestStatus_FAIL,
				Expected: true,
				Duration: &duration.Duration{Nanos: 5e7},
				Tags:     pbutil.StringPairs("json_format_status", "FAIL"),
			},
			{
				TestPath: "prefix/c1/c2/t2.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Duration: &duration.Duration{Nanos: 5e7},
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "prefix/c1/c2/t2.html",
				Status:   pb.TestStatus_CRASH,
				Expected: false,
				Duration: &duration.Duration{Nanos: 5e7},
				Tags:     pbutil.StringPairs("json_format_status", "CRASH"),
			},

			// Test 3
			{
				TestPath: "prefix/c2/t3.html",
				Status:   pb.TestStatus_FAIL,
				Expected: false,
				Tags:     pbutil.StringPairs("json_format_status", "FAIL"),
			},

			// Test 4
			{
				TestPath: "prefix/c2/t4.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Duration: &duration.Duration{Nanos: 3e8},
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "prefix/c2/t4.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
			{
				TestPath: "prefix/c2/t4.html",
				Status:   pb.TestStatus_PASS,
				Expected: true,
				Tags:     pbutil.StringPairs("json_format_status", "PASS"),
			},
		})
	})
}
