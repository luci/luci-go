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

	resultspb "go.chromium.org/luci/results/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGTestConversions(t *testing.T) {
	ctx := context.Background()

	Convey(`From JSON works`, t, func() {
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
							"losless_snippet": true,
							"output_snippet": "[ RUN      ] FooTest.TestDoBar",
							"output_snippet_base64": "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmFy",
							"status": "CRASH"
						},
						{
							"elapsed_time_ms": 1856,
							"losless_snippet": false,
							"output_snippet_base64": "c29tZSBkYXRhIHdpdGggACBhbmQg77u/",
							"status": "FAIL"
						}
					],
					"FooTest.TestDoBaz": [
						{
							"elapsed_time_ms": 837,
							"losless_snippet": true,
							"output_snippet": "[ RUN      ] FooTest.TestDoBaz",
							"output_snippet_base64": "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmF6",
							"status": "SUCCESS"
						},
						{
							"elapsed_time_ms": 856,
							"losless_snippet": false,
							"output_snippet_base64": "c29tZSBkYXRhIHdpdGggACBhbmQg77u/",
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

		results := &GTestResults{}
		err := results.ConvertFromJSON(ctx, bytes.NewReader(buf))
		So(err, ShouldBeNil)
		So(results.AllTests, ShouldResemble, []string{"FooTest.TestDoBar", "FooTest.TestDoBaz"})
		So(results.GlobalTags, ShouldResemble, []string{"CPU_64_BITS", "MODE_RELEASE", "OS_WIN"})
		So(results.PerIterationData, ShouldResemble, []map[string][]*GTestRunResult{
			{
				"FooTest.TestDoBar": {
					{
						Status:              "CRASH",
						ElapsedTimeMs:       1837,
						LosslessSnippet:     true,
						OutputSnippetBase64: "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmFy",
					},
					{
						Status:              "FAIL",
						ElapsedTimeMs:       1856,
						OutputSnippetBase64: "c29tZSBkYXRhIHdpdGggACBhbmQg77u/",
					},
				},
				"FooTest.TestDoBaz": {
					{
						Status:              "SUCCESS",
						ElapsedTimeMs:       837,
						LosslessSnippet:     true,
						OutputSnippetBase64: "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmF6",
					},
					{
						Status:              "SUCCESS",
						ElapsedTimeMs:       856,
						OutputSnippetBase64: "c29tZSBkYXRhIHdpdGggACBhbmQg77u/",
					},
				},
			},
		})
		So(results.TestLocations, ShouldResemble, map[string]*Location{
			"FooTest.TestDoBar": {File: "../../chrome/browser/foo/test.cc", Line: 287},
			"FooTest.TestDoBaz": {File: "../../chrome/browser/foo/test.cc", Line: 293},
		})
	})

	Convey(`To Invocation works`, t, func() {
		results := &GTestResults{
			AllTests:   []string{"test1", "test2", "test3"},
			GlobalTags: []string{"tag1", "tag2"},
			PerIterationData: []map[string][]*GTestRunResult{
				{
					"test1": {
						{
							Status:              "SUCCESS",
							ElapsedTimeMs:       10,
							LosslessSnippet:     true,
							OutputSnippetBase64: "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmFyCigxMCBtcyk=",
						},
						{
							Status:              "SUCCESS",
							ElapsedTimeMs:       12,
							LosslessSnippet:     true,
							OutputSnippetBase64: "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmFyCigxMiBtcyk=",
						},
					},
					"test2": {
						{
							Status:              "FAILURE",
							ElapsedTimeMs:       20,
							OutputSnippetBase64: "c29tZSBkYXRhIHdpdGggACBhbmQg77u/",
						},
						{
							Status:              "FAILURE",
							ElapsedTimeMs:       22,
							OutputSnippetBase64: "c29tZSBkYXRhIHdpdGggACBhbmQg77u/",
						},
					},
					"test3": {
						{Status: "FAILURE", ElapsedTimeMs: 30},
						{Status: "SUCCESS", ElapsedTimeMs: 32},
					},
				},
				{
					"test1": {
						{
							Status:              "SUCCESS",
							ElapsedTimeMs:       14,
							LosslessSnippet:     true,
							OutputSnippetBase64: "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmFyCigxNCBtcyk=",
						},
						{
							Status:              "SUCCESS",
							ElapsedTimeMs:       16,
							LosslessSnippet:     true,
							OutputSnippetBase64: "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmFyCigxNiBtcyk=",
						},
					},
					"test2": {
						{
							Status:              "FAILURE",
							ElapsedTimeMs:       24,
							OutputSnippetBase64: "c29tZSBkYXRhIHdpdGggACBhbmQg77u/",
						},
						{
							Status:              "FAILURE",
							ElapsedTimeMs:       26,
							OutputSnippetBase64: "c29tZSBkYXRhIHdpdGggACBhbmQg77u/",
						},
					},
					"test3": {
						{Status: "CRASH", ElapsedTimeMs: 34},
						{Status: "NOTRUN", ElapsedTimeMs: 36},
					},
				},
			},
			TestLocations: map[string]*Location{
				"test1": {File: "fileA.cc", Line: 12},
				"test2": {File: "fileA.cc", Line: 24},
				"test3": {File: "fileB.cc", Line: 36},
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

		// Check tests. They all have the same variant so far.
		expectedTestStatuses := map[string][]resultspb.Status{
			"prefix/test1": {
				resultspb.Status_PASS, resultspb.Status_PASS, resultspb.Status_PASS, resultspb.Status_PASS,
			},
			"prefix/test2": {
				resultspb.Status_FAIL, resultspb.Status_FAIL, resultspb.Status_FAIL, resultspb.Status_FAIL,
			},
			"prefix/test3": {
				resultspb.Status_FAIL, resultspb.Status_PASS, resultspb.Status_CRASH, resultspb.Status_SKIP,
			},
		}
		expectedTestDurationsNs := map[string][]int32{
			"prefix/test1": {10000, 12000, 14000, 16000},
			"prefix/test2": {20000, 22000, 24000, 26000},
			"prefix/test3": {30000, 32000, 34000, 36000},
		}
		expectedTestSummaries := map[string][]string{
			"prefix/test1": {
				"```\n[ RUN      ] FooTest.TestDoBar\\n(10 ms)\n```",
				"```\n[ RUN      ] FooTest.TestDoBar\\n(12 ms)\n```",
				"```\n[ RUN      ] FooTest.TestDoBar\\n(14 ms)\n```",
				"```\n[ RUN      ] FooTest.TestDoBar\\n(16 ms)\n```",
			},
			"prefix/test2": {
				"```\nsome data with \\x00 and \\ufeff\n```",
				"```\nsome data with \\x00 and \\ufeff\n```",
				"```\nsome data with \\x00 and \\ufeff\n```",
				"```\nsome data with \\x00 and \\ufeff\n```",
			},
		}
		expectedTestTags := map[string][]*resultspb.StringPair{
			"prefix/test1": {
				{Key: "lossless_snippet", Value: "true"},
				{Key: "file", Value: "fileA.cc"},
				{Key: "line", Value: "12"},
			},
			"prefix/test2": {
				{Key: "lossless_snippet", Value: "false"},
				{Key: "file", Value: "fileA.cc"},
				{Key: "line", Value: "24"},
			},
			"prefix/test3": {
				{Key: "lossless_snippet", Value: "false"},
				{Key: "file", Value: "fileB.cc"},
				{Key: "line", Value: "36"},
			},
		}
		for _, tpb := range inv.Tests {
			So(expectedTestStatuses, ShouldContainKey, tpb.Name)
			So(len(tpb.Variants), ShouldEqual, 1)

			for i, res := range tpb.Variants[0].Results {
				So(res.Tags, ShouldResemble, expectedTestTags[tpb.Name])
				So(res.Status, ShouldResemble, expectedTestStatuses[tpb.Name][i])

				So(res.Duration.Seconds, ShouldEqual, 0)
				So(res.Duration.Nanos, ShouldAlmostEqual, expectedTestDurationsNs[tpb.Name][i], 100)

				if expectedSummaries, ok := expectedTestSummaries[tpb.Name]; ok {
					So(res.Summary, ShouldNotBeNil)
					So(res.Summary.Text, ShouldResemble, expectedSummaries[i])
				} else {
					So(res.Summary, ShouldBeNil)
				}
			}
		}

		sort.Slice(inv.Tags, func(i, j int) bool { return inv.Tags[i].Key < inv.Tags[j].Key })
		So(
			inv.Tags,
			ShouldResemble,
			[]*resultspb.StringPair{
				{Key: "global_tag", Value: "tag1"},
				{Key: "global_tag", Value: "tag2"},
				{Key: "test_framework", Value: "gtest"},
			},
		)
	})
}
