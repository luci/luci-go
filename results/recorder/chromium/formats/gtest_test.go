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
	"fmt"
	"testing"
	"unicode/utf8"

	resultspb "go.chromium.org/luci/results/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGTestConversions(t *testing.T) {
	t.Parallel()
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
			GlobalTags: []string{"tag2", "tag1"},
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
							Status:              "EXCESSIVE_OUTPUT",
							OutputSnippetBase64: "Qnl0ZXM6IDB4QzAgMHg4QSAweDNEIDB4MjcKPGEgYsCKPSdjJy8+",
						},
						{
							Status:              "FAILURE_ON_EXIT",
							OutputSnippetBase64: "Qnl0ZXM6IDB4QzAgMHg4QSAweDNEIDB4MjcKPGEgYsCKPSdjJy8+",
						},
					},
					"test3": {
						{Status: "FAILURE"},
						{Status: "SUCCESS"},
						{Status: "FAILURE_ON_EXIT"},
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
							OutputSnippetBase64: "Qnl0ZXM6IDB4QzAgMHg4QSAweDNEIDB4MjcKPGEgYsCKPSdjJy8+",
						},
						{
							Status:              "FAILURE_ON_EXIT",
							OutputSnippetBase64: "Qnl0ZXM6IDB4QzAgMHg4QSAweDNEIDB4MjcKPGEgYsCKPSdjJy8+",
						},
					},
					"test3": {
						{Status: "CRASH"},
						{Status: "NOTRUN"},
						{Status: "EXCESSIVE_OUTPUT"},
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
		So(
			inv.Tags,
			ShouldResemble,
			[]*resultspb.StringPair{
				{Key: "gtest_global_tag", Value: "tag1"},
				{Key: "gtest_global_tag", Value: "tag2"},
				{Key: "test_framework", Value: "gtest"},
			},
		)

		// Check tests.
		So(inv.Tests, ShouldHaveLength, 3)

		Convey(`grouping by test name works`, func() {
			So(inv.Tests[0].Path, ShouldEqual, "prefix/test1")
			So(inv.Tests[1].Path, ShouldEqual, "prefix/test2")
			So(inv.Tests[2].Path, ShouldEqual, "prefix/test3")
		})

		Convey(`grouping by variant works`, func() {
			// They all have one and the same variant so far.
			So(inv.Tests[0].Variants, ShouldHaveLength, 1)
			So(inv.Tests[1].Variants, ShouldHaveLength, 1)
			So(inv.Tests[2].Variants, ShouldHaveLength, 1)
		})

		Convey(`collecting runs per tests works`, func() {
			So(inv.Tests[0].Variants[0].Results, ShouldHaveLength, 4)
			So(inv.Tests[1].Variants[0].Results, ShouldHaveLength, 4)
			So(inv.Tests[2].Variants[0].Results, ShouldHaveLength, 6)
		})

		Convey(`statuses mapped correctly`, func() {
			// Sanity check.
			testResults := inv.Tests[2].Variants[0].Results
			statuses := make([]resultspb.Status, len(testResults))
			for i, r := range testResults {
				statuses[i] = r.Status
			}
			So(statuses, ShouldResemble, []resultspb.Status{
				resultspb.Status_FAIL,
				resultspb.Status_PASS,
				resultspb.Status_FAIL,
				resultspb.Status_CRASH,
				resultspb.Status_SKIP,
				resultspb.Status_FAIL,
			})
		})

		Convey(`durations captured correctly`, func() {
			testResults := inv.Tests[0].Variants[0].Results
			expectedDurationsNs := []int32{10000, 12000, 14000, 16000}
			So(testResults, ShouldHaveLength, len(expectedDurationsNs))
			for i, r := range testResults {
				So(r.Duration.Seconds, ShouldEqual, 0)
				So(r.Duration.Nanos, ShouldAlmostEqual, expectedDurationsNs[i], 100)
			}
		})

		Convey(`summaries captured correctly`, func() {
			Convey(`for lossless snippets`, func() {
				testResults := inv.Tests[0].Variants[0].Results
				summaries := make([]string, len(testResults))
				for i, r := range testResults {
					summaries[i] = r.SummaryMarkdown
				}
				So(summaries, ShouldResemble, []string{
					"[ RUN      ] FooTest.TestDoBar\n(10 ms)",
					"[ RUN      ] FooTest.TestDoBar\n(12 ms)",
					"[ RUN      ] FooTest.TestDoBar\n(14 ms)",
					"[ RUN      ] FooTest.TestDoBar\n(16 ms)",
				})
			})

			Convey(`for lossy snippets`, func() {
				testResults := inv.Tests[1].Variants[0].Results
				expectedText := fmt.Sprintf("Bytes: 0xC0 0x8A 0x3D 0x27\n<a b%c='c'/>", utf8.RuneError)
				for _, r := range testResults {
					So(r.SummaryMarkdown, ShouldEqual, expectedText)
				}
			})

			Convey(`for missing snippets`, func() {
				So(inv.Tests[2].Variants[0].Results[0].SummaryMarkdown, ShouldEqual, "")
			})
		})

		Convey(`test tags work`, func() {
			testResults := inv.Tests[0].Variants[0].Results
			expectedTags := []*resultspb.StringPair{
				{Key: "file", Value: "fileA.cc"},
				{Key: "gtest_status", Value: "SUCCESS"},
				{Key: "line", Value: "12"},
				{Key: "lossless_snippet", Value: "true"},
			}
			for _, r := range testResults {
				So(r.Tags, ShouldResemble, expectedTags)
			}

			testResults = inv.Tests[1].Variants[0].Results
			So(testResults[0].Tags, ShouldResemble, []*resultspb.StringPair{
				{Key: "file", Value: "fileA.cc"},
				{Key: "gtest_status", Value: "EXCESSIVE_OUTPUT"},
				{Key: "line", Value: "24"},
				{Key: "lossless_snippet", Value: "false"},
			})
		})
	})
}
