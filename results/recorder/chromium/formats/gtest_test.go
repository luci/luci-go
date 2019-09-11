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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGTestJsonConversion(t *testing.T) {
	ctx := context.Background()
	buf := []byte(
		`{
			"all_tests": [
				"FooTest.TestDoBar",
				"FooTest.TestDoBaz"
			],
      "global_tags": ["CPU_64_BITS","MODE_RELEASE","OS_WIN"],
			"per_iteration_data": [{
				"FooTest.TestDoBar": [
					{
						"elapsed_time_ms": 1837,
						"losless_snippet": false,
						"output_snippet": "[ RUN      ] FooTest.TestDoBar",
						"status": "CRASH"
					},
					{
						"elapsed_time_ms": 1856,
						"losless_snippet": false,
						"output_snippet": "[ RUN      ] FooTest.TestDoBar",
						"status": "FAIL"
					}
				],
				"FooTest.TestDoBaz": [
					{
						"elapsed_time_ms": 837,
						"losless_snippet": false,
						"output_snippet": "[ RUN      ] FooTest.TestDoBaz",
						"status": "SUCCESS"
					},
					{
						"elapsed_time_ms": 856,
						"losless_snippet": false,
						"output_snippet": "[ RUN      ] FooTest.TestDoBaz",
						"status": "SUCCESS"
					}
				]
			}],
			"test_locations": {
				"FooTest.TestDoBar": {
					"file": "../../chrome/browser/foo/test.cc",
					"line": 287
				},
				"FooTest.TestDoBaz": {
					"file": "../../chrome/browser/foo/test.cc",
					"line": 293
				}
			}
		}`)

	Convey(`Works`, t, func() {
		results := &GTestResults{}
		err := results.ConvertFromJSON(ctx, bytes.NewReader(buf))
		So(err, ShouldBeNil)
		So(results.AllTests, ShouldResemble, []string{"FooTest.TestDoBar", "FooTest.TestDoBaz"})
		So(results.GlobalTags, ShouldResemble, []string{"CPU_64_BITS", "MODE_RELEASE", "OS_WIN"})
		So(results.PerIterationData, ShouldResemble, []map[string][]*GTestRunResult{
			{
				"FooTest.TestDoBar": {
					{
						Status:        "CRASH",
						ElapsedTimeMs: 1837,
						OutputSnippet: "[ RUN      ] FooTest.TestDoBar",
					},
					{
						Status:        "FAIL",
						ElapsedTimeMs: 1856,
						OutputSnippet: "[ RUN      ] FooTest.TestDoBar",
					},
				},
				"FooTest.TestDoBaz": {
					{
						Status:        "SUCCESS",
						ElapsedTimeMs: 837,
						OutputSnippet: "[ RUN      ] FooTest.TestDoBaz",
					},
					{
						Status:        "SUCCESS",
						ElapsedTimeMs: 856,
						OutputSnippet: "[ RUN      ] FooTest.TestDoBaz",
					},
				},
			},
		})
		So(results.TestLocations, ShouldResemble, map[string]*Location{
			"FooTest.TestDoBar": {File: "../../chrome/browser/foo/test.cc", Line: 287},
			"FooTest.TestDoBaz": {File: "../../chrome/browser/foo/test.cc", Line: 293},
		})
	})
}
