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

	resultspb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/util"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey("convertTestResult", t, func() {
		Convey("EXCESSIVE_OUTPUT", func() {
			tr, err := (&GTestResults{}).convertTestResult(ctx, "testPath", "TestName", &GTestRunResult{
				Status: "EXCESSIVE_OUTPUT",
			})
			So(err, ShouldBeNil)
			So(tr.Status, ShouldEqual, resultspb.TestStatus_FAIL)
			So(util.StringPairsContain(tr.Tags, util.StringPair("gtest_status", "EXCESSIVE_OUTPUT")), ShouldBeTrue)
		})

		Convey("NOTRUN", func() {
			tr, err := (&GTestResults{}).convertTestResult(ctx, "testPath", "TestName", &GTestRunResult{
				Status: "NOTRUN",
			})
			So(err, ShouldBeNil)
			So(tr.Status, ShouldEqual, resultspb.TestStatus_SKIP)
			So(util.StringPairsContain(tr.Tags, util.StringPair("gtest_status", "NOTRUN")), ShouldBeTrue)
		})

		Convey("Duration", func() {
			tr, err := (&GTestResults{}).convertTestResult(ctx, "testPath", "TestName", &GTestRunResult{
				Status:        "SUCCESS",
				ElapsedTimeMs: 1e6,
			})
			So(err, ShouldBeNil)
			So(tr.Duration.GetSeconds(), ShouldEqual, 1)
			So(tr.Duration.GetNanos(), ShouldEqual, 0)
		})

		Convey("snippet", func() {
			Convey("valid", func() {
				tr, err := (&GTestResults{}).convertTestResult(ctx, "testPath", "TestName", &GTestRunResult{
					Status:              "SUCCESS",
					LosslessSnippet:     true,
					OutputSnippetBase64: "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmFyCigxMCBtcyk=",
				})
				So(err, ShouldBeNil)
				So(tr.SummaryMarkdown, ShouldEqual, "[ RUN      ] FooTest.TestDoBar\n(10 ms)")
			})

			Convey("invalid does not cause a fatal error", func() {
				tr, err := (&GTestResults{}).convertTestResult(ctx, "testPath", "TestName", &GTestRunResult{
					Status:              "SUCCESS",
					LosslessSnippet:     true,
					OutputSnippetBase64: "invalid base64",
				})
				So(err, ShouldBeNil)
				So(tr.SummaryMarkdown, ShouldEqual, "")
			})
		})

		Convey("testLocations", func() {
			results := &GTestResults{
				TestLocations: map[string]*Location{
					"TestName": {
						File: "TestFile",
						Line: 54,
					},
				},
			}
			tr, err := results.convertTestResult(ctx, "testPath", "TestName", &GTestRunResult{
				Status: "SUCCESS",
			})
			So(err, ShouldBeNil)
			So(util.StringPairsContain(tr.Tags, util.StringPair("gtest_file", "TestFile")), ShouldBeTrue)
			So(util.StringPairsContain(tr.Tags, util.StringPair("gtest_line", "54")), ShouldBeTrue)
		})
	})

	Convey(`ToProtos`, t, func() {
		req := &resultspb.DeriveInvocationRequest{
			SwarmingTask: &resultspb.DeriveInvocationRequest_SwarmingTask{
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

		inv := &resultspb.Invocation{}

		Convey("Works", func() {
			results := &GTestResults{
				PerIterationData: []map[string][]*GTestRunResult{
					{
						"test1": {
							{
								Status: "SUCCESS",
							},
							{
								Status: "FAILURE",
							},
						},
						"test2": {
							{
								Status: "EXCESSIVE_OUTPUT",
							},
							{
								Status: "FAILURE_ON_EXIT",
							},
						},
					},
					{
						"test1": {
							{
								Status: "SUCCESS",
							},
							{
								Status: "SUCCESS",
							},
						},
						"test2": {
							{
								Status: "FAILURE",
							},
							{
								Status: "FAILURE_ON_EXIT",
							},
						},
					},
				},
			}

			testResults, err := results.ToProtos(ctx, req, inv)
			So(err, ShouldBeNil)
			So(util.StringPairsContain(inv.Tags, util.StringPair("test_framework", "gtest")), ShouldBeTrue)
			So(testResults, ShouldResembleProto, []*resultspb.TestResult{
				// Iteration 1.
				{
					TestPath: "prefix/test1",
					Status:   resultspb.TestStatus_PASS,
					Tags: util.StringPairs(
						"gtest_status", "SUCCESS",
						"lossless_snippet", "false",
					),
				},
				{
					TestPath: "prefix/test1",
					Status:   resultspb.TestStatus_FAIL,
					Tags: util.StringPairs(
						"gtest_status", "FAILURE",
						"lossless_snippet", "false",
					),
				},
				{
					TestPath: "prefix/test2",
					Status:   resultspb.TestStatus_FAIL,
					Tags: util.StringPairs(
						"gtest_status", "EXCESSIVE_OUTPUT",
						"lossless_snippet", "false",
					),
				},
				{
					TestPath: "prefix/test2",
					Status:   resultspb.TestStatus_FAIL,
					Tags: util.StringPairs(
						"gtest_status", "FAILURE_ON_EXIT",
						"lossless_snippet", "false",
					),
				},

				// Iteration 2.
				{
					TestPath: "prefix/test1",
					Status:   resultspb.TestStatus_PASS,
					Tags: util.StringPairs(
						"gtest_status", "SUCCESS",
						"lossless_snippet", "false",
					),
				},
				{
					TestPath: "prefix/test1",
					Status:   resultspb.TestStatus_PASS,
					Tags: util.StringPairs(
						"gtest_status", "SUCCESS",
						"lossless_snippet", "false",
					),
				},
				{
					TestPath: "prefix/test2",
					Status:   resultspb.TestStatus_FAIL,
					Tags: util.StringPairs(
						"gtest_status", "FAILURE",
						"lossless_snippet", "false",
					),
				},
				{
					TestPath: "prefix/test2",
					Status:   resultspb.TestStatus_FAIL,
					Tags: util.StringPairs(
						"gtest_status", "FAILURE_ON_EXIT",
						"lossless_snippet", "false",
					),
				},
			})
		})

		Convey("GlobalTags", func() {
			results := &GTestResults{
				GlobalTags: []string{"tag2", "tag1"},
				PerIterationData: []map[string][]*GTestRunResult{
					{
						"test1": {
							{Status: "SUCCESS"},
						},
					},
				},
			}

			_, err := results.ToProtos(ctx, req, inv)
			So(err, ShouldBeNil)
			So(util.StringPairsContain(inv.Tags, util.StringPair("gtest_global_tag", "tag1")), ShouldBeTrue)
			So(util.StringPairsContain(inv.Tags, util.StringPair("gtest_global_tag", "tag2")), ShouldBeTrue)
		})

		Convey("Interrupted", func() {
			results := &GTestResults{
				PerIterationData: []map[string][]*GTestRunResult{
					{
						"test1": {{Status: "NOTRUN"}},
					},
				},
			}

			_, err := results.ToProtos(ctx, req, inv)
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, resultspb.Invocation_INTERRUPTED)
		})
	})
}
