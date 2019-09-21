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
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConversion(t *testing.T) {
	ctx := context.Background()
	buf := []byte(
		`{
				"version": 3,
				"interrupted": false,
				"path_delimiter": "::",
				"seconds_since_epoch": 1565504423,
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
					}
				}
			}`)

	Convey(`Works`, t, func() {
		results, err := ConvertFromJSON(ctx, bytes.NewReader(buf))
		So(err, ShouldBeNil)
		So(results, ShouldNotBeNil)
		So(results.Version, ShouldEqual, 3)
		So(results.Interrupted, ShouldEqual, false)
		So(results.PathDelimiter, ShouldEqual, "::")
		So(results.SecondsSinceEpoch, ShouldEqual, 1565504423)
		So(results.Tests, ShouldResemble, map[string]*TestFields{
			"c1::c2::t1.html": {
				ActualRaw:    "PASS PASS PASS",
				Actual:       []string{"PASS", "PASS", "PASS"},
				ExpectedRaw:  "PASS",
				ExpectedSet:  []string{"PASS"},
				Time:         0.3,
				Times:        []float64{0.2, 0.1},
				AllDurations: []float64{0.3, 0.2, 0.1},
			},
			"c1::c2::t2.html": {
				ActualRaw:    "PASS FAIL PASS",
				Actual:       []string{"PASS", "FAIL", "PASS"},
				ExpectedRaw:  "PASS FAIL",
				ExpectedSet:  []string{"PASS", "FAIL"},
				Times:        []float64{0.05},
				AllDurations: []float64{0.05},
			},
			"c2::t3.html": {
				ActualRaw:   "FAIL",
				Actual:      []string{"FAIL"},
				ExpectedRaw: "PASS",
				ExpectedSet: []string{"PASS"},
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
				"c1/c2/t1.html",
				"c1/c2/t2.html",
				"c2/t3.html",
			})
		})
	})
}
