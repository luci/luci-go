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

	resultspb "go.chromium.org/luci/results/proto/v1"
	"go.chromium.org/luci/results/util"

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

	Convey(`To Invocation works`, t, func() {
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
					Times:    []float64{0.05, 0.07, 0.06, 0.01},
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

		req := &resultspb.DeriveInvocationFromSwarmingRequest{
			Task: &resultspb.DeriveInvocationFromSwarmingRequest_SwarmingTask{
				Hostname: "host-swarming",
				Id:       "123",
			},
			TestPathPrefix: "prefix/",
			BaseTestVariant: &resultspb.VariantDef{Def: map[string]string{
				"bucket":     "bkt",
				"builder":    "blder",
				"test_suite": "foo_unittests",
			}},
		}

		inv, err := results.ToInvocation(ctx, req)
		So(err, ShouldBeNil)
		So(inv.Incomplete, ShouldBeTrue)
		So(inv.Tags, ShouldResembleProto, util.StringPairs(
				"json_format_tag", "desktop",
				"json_format_tag", "linux",
				"json_format_tag", "ubuntu",
				"test_framework", "json",
			),
		)

		So(inv.Tests, ShouldHaveLength, 4)

		Convey(`grouping by test name works`, func() {
			paths := make([]string, len(inv.Tests))
			for i, t := range inv.Tests {
				paths[i] = t.Path
			}
			So(paths, ShouldResemble, []string{
				"prefix/c1/c2/t1.html",
				"prefix/c1/c2/t2.html",
				"prefix/c2/t3.html",
				"prefix/c2/t4.html"})
		})

		Convey(`grouping by variant works`, func() {
			// They all have one and the same variant so far.
			So(inv.Tests[0].Variants, ShouldHaveLength, 1)
			So(inv.Tests[1].Variants, ShouldHaveLength, 1)
			So(inv.Tests[2].Variants, ShouldHaveLength, 1)
			So(inv.Tests[3].Variants, ShouldHaveLength, 1)
		})

		Convey(`collecting runs per tests works`, func() {
			So(inv.Tests[0].Variants[0].Results, ShouldHaveLength, 3)
			So(inv.Tests[1].Variants[0].Results, ShouldHaveLength, 4)
			So(inv.Tests[2].Variants[0].Results, ShouldHaveLength, 1)
			So(inv.Tests[3].Variants[0].Results, ShouldHaveLength, 3)
		})

		Convey(`durations captured correctly`, func() {
			Convey(`with Time and Times both provided`, func() {
				testResults := inv.Tests[0].Variants[0].Results
				expectedDurationsNs := []int32{3e8, 2e8, 1e8}
				So(testResults, ShouldHaveLength, len(expectedDurationsNs))
				for i, r := range testResults {
					So(r.Duration.Seconds, ShouldEqual, 0)
					So(r.Duration.Nanos, ShouldAlmostEqual, expectedDurationsNs[i], 100)
				}
			})

			Convey(`with only Times provided`, func() {
				testResults := inv.Tests[1].Variants[0].Results
				expectedDurationsNs := []int32{5e7, 7e7, 6e7, 1e7}
				So(testResults, ShouldHaveLength, len(expectedDurationsNs))
				for i, r := range testResults {
					So(r.Duration.Seconds, ShouldEqual, 0)
					So(r.Duration.Nanos, ShouldAlmostEqual, expectedDurationsNs[i], 100)
				}
			})

			Convey(`with only Time provided`, func() {
				testResults := inv.Tests[3].Variants[0].Results
				for i, r := range testResults {
					if i == 0 {
						So(r.Duration.Seconds, ShouldEqual, 0)
						So(r.Duration.Nanos, ShouldAlmostEqual, 3e8, 100)
					} else {
						So(r.Duration, ShouldBeNil)
					}
				}
			})

			Convey(`and fails with mismatched runs and durations`, func() {
				results.Tests["c1/c2/t2.html"].Actual = "PASS FAIL PASS"
				_, err := results.ToInvocation(ctx, req)
				So(err, ShouldErrLike, "4 durations populated but has 3 test statuses")
			})
		})

		Convey(`expectations captured correctly`, func() {
			testResults := inv.Tests[1].Variants[0].Results
			expectations := make([]bool, len(testResults))
			for i, r := range testResults {
				expectations[i] = r.Expected.Value
			}
			So(expectations, ShouldResemble, []bool{true, true, true, false})
		})
	})
}
