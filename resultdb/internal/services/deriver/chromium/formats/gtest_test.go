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
	"encoding/base64"
	"strings"
	"testing"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

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
							"status": "FAILURE"
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
							"status": "SUCCESS",
							"links": {
								"logcat": "https://luci-logdog.appspot.com/v/?s=logcat"
							}
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
						Status:              "FAILURE",
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
						Links: map[string]string{
							"logcat": "https://luci-logdog.appspot.com/v/?s=logcat",
						},
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
		convert := func(result *GTestRunResult) *pb.TestResult {
			tr, err := (&GTestResults{}).convertTestResult(ctx, "testId", "TestName", result, &strings.Builder{})
			So(err, ShouldBeNil)
			return tr
		}
		Convey("EXCESSIVE_OUTPUT", func() {
			tr := convert(&GTestRunResult{Status: "EXCESSIVE_OUTPUT"})
			So(tr.Status, ShouldEqual, pb.TestStatus_FAIL)
			So(pbutil.StringPairsContain(tr.Tags, pbutil.StringPair("gtest_status", "EXCESSIVE_OUTPUT")), ShouldBeTrue)
		})

		Convey("NOTRUN", func() {
			tr := convert(&GTestRunResult{Status: "NOTRUN"})
			So(tr.Status, ShouldEqual, pb.TestStatus_SKIP)
			So(tr.Expected, ShouldBeFalse)
			So(pbutil.StringPairsContain(tr.Tags, pbutil.StringPair("gtest_status", "NOTRUN")), ShouldBeTrue)
		})

		Convey("SKIPPED", func() {
			tr := convert(&GTestRunResult{Status: "SKIPPED"})
			So(tr.Status, ShouldEqual, pb.TestStatus_SKIP)
			So(tr.Expected, ShouldBeTrue)
			So(pbutil.StringPairsContain(tr.Tags, pbutil.StringPair("gtest_status", "SKIPPED")), ShouldBeTrue)
		})

		Convey("Duration", func() {
			tr := convert(&GTestRunResult{
				Status:        "SUCCESS",
				ElapsedTimeMs: 1e3,
			})
			So(tr.Duration.GetSeconds(), ShouldEqual, 1)
			So(tr.Duration.GetNanos(), ShouldEqual, 0)
		})

		Convey("snippet", func() {
			Convey("valid", func() {
				tr := convert(&GTestRunResult{
					Status:              "SUCCESS",
					LosslessSnippet:     true,
					OutputSnippetBase64: "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmFyCigxMCBtcyk=",
				})
				So(tr.SummaryHtml, ShouldEqual, "<div><pre>[ RUN      ] FooTest.TestDoBar\n(10 ms)</pre></div>")
			})

			Convey("invalid does not cause a fatal error", func() {
				tr := convert(&GTestRunResult{
					Status:              "SUCCESS",
					LosslessSnippet:     true,
					OutputSnippetBase64: "invalid base64",
				})
				So(tr.SummaryHtml, ShouldEqual, "")
			})

			Convey("invalid utf8 is fixed", func() {
				tr := convert(&GTestRunResult{
					Status:              "SUCCESS",
					LosslessSnippet:     true,
					OutputSnippetBase64: base64.StdEncoding.EncodeToString([]byte("[\x00]")),
				})
				So(tr.SummaryHtml, ShouldContainSubstring, "[�]")
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
			tr, err := results.convertTestResult(ctx, "testId", "TestName", &GTestRunResult{Status: "SUCCESS"}, &strings.Builder{})
			So(err, ShouldBeNil)
			So(pbutil.StringPairsContain(tr.Tags, pbutil.StringPair("test_location", "TestFile:54")), ShouldBeTrue)
		})

		Convey("links", func() {
			tr := convert(&GTestRunResult{
				Status:              "SUCCESS",
				LosslessSnippet:     true,
				OutputSnippetBase64: "invalid base64",
				Links: map[string]string{
					"logcat": "https://luci-logdog.appspot.com/v/?s=logcat",
				},
			})
			pbutil.NormalizeTestResult(tr)
			So(tr.SummaryHtml, ShouldContainSubstring, `<a href="https://luci-logdog.appspot.com/v/?s=logcat">logcat</a>`)
		})
	})

	Convey(`extractGTestParameters`, t, func() {
		Convey(`type parametrized`, func() {
			Convey(`with instantiation`, func() {
				baseID, err := extractGTestParameters("MyInstantiation/FooTest/1.DoesBar")
				So(err, ShouldBeNil)
				So(baseID, ShouldEqual, "FooTest.DoesBar/MyInstantiation.1")
			})

			Convey(`without instantiation`, func() {
				baseID, err := extractGTestParameters("FooTest/1.DoesBar")
				So(err, ShouldBeNil)
				So(baseID, ShouldEqual, "FooTest.DoesBar/1")
			})
		})

		Convey(`value parametrized`, func() {
			Convey(`with instantiation`, func() {
				baseID, err := extractGTestParameters("MyInstantiation/FooTest.DoesBar/1")
				So(err, ShouldBeNil)
				So(baseID, ShouldEqual, "FooTest.DoesBar/MyInstantiation.1")
			})

			Convey(`without instantiation`, func() {
				baseID, err := extractGTestParameters("FooTest.DoesBar/1")
				So(err, ShouldBeNil)
				So(baseID, ShouldEqual, "FooTest.DoesBar/1")
			})
		})

		Convey(`not parametrized`, func() {
			baseID, err := extractGTestParameters("FooTest.DoesBar")
			So(err, ShouldBeNil)
			So(baseID, ShouldEqual, "FooTest.DoesBar")
		})

		Convey(`with magic prefixes`, func() {
			baseID, err := extractGTestParameters("FooTest.PRE_PRE_MANUAL_DoesBar")
			So(err, ShouldBeNil)
			So(baseID, ShouldEqual, "FooTest.DoesBar")
		})

		Convey(`with JUnit tests`, func() {
			baseID, err := extractGTestParameters("org.chromium.tests#testFoo_sub__param=val")
			So(err, ShouldBeNil)
			So(baseID, ShouldEqual, "org.chromium.tests#testFoo_sub__param=val")
		})

		Convey(`synthetic parameterized test`, func() {
			_, err := extractGTestParameters("GoogleTestVerification.UninstantiatedParamaterizedTestSuite<Suite>")
			So(err, ShouldErrLike, "not a real test")
			So(syntheticTestTag.In(err), ShouldBeTrue)
		})

		Convey(`synthetic type parameterized test`, func() {
			_, err := extractGTestParameters("GoogleTestVerification.UninstantiatedTypeParamaterizedTestSuite<Suite>")
			So(err, ShouldErrLike, "not a real test")
			So(syntheticTestTag.In(err), ShouldBeTrue)
		})

		Convey(`with unrecognized format`, func() {
			_, err := extractGTestParameters("not_gtest_test")
			So(err, ShouldErrLike, "test id of unknown format")
		})
	})

	Convey(`ToProtos`, t, func() {
		inv := &pb.Invocation{}

		Convey("Works", func() {
			results := &GTestResults{
				PerIterationData: []map[string][]*GTestRunResult{
					{
						"BazTest.DoesQux": {
							{
								Status: "SUCCESS",
							},
							{
								Status: "FAILURE",
							},
						},
						"GoogleTestVerification.UninstantiatedTypeParamaterizedTestSuite<Suite>": {
							{
								Status: "SUCCESS",
							},
						},
						"FooTest.DoesBar": {
							{
								Status: "EXCESSIVE_OUTPUT",
							},
							{
								Status: "FAILURE_ON_EXIT",
							},
						},
					},
					{
						"BazTest.DoesQux": {
							{
								Status: "SUCCESS",
							},
							{
								Status: "SUCCESS",
							},
						},
						"FooTest.DoesBar": {
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

			testResults, err := results.ToProtos(ctx, "ninja://tests/", inv)
			So(err, ShouldBeNil)
			So(pbutil.StringPairsContain(inv.Tags, pbutil.StringPair(OriginalFormatTagKey, FormatGTest)), ShouldBeTrue)
			assertTestResultsResemble(testResults, []*TestResult{
				// Iteration 1.
				{
					TestResult: &pb.TestResult{
						TestId:   "ninja://tests/BazTest.DoesQux",
						Expected: true,
						Status:   pb.TestStatus_PASS,
						Tags: pbutil.StringPairs(
							"test_name", "BazTest.DoesQux",
							"gtest_status", "SUCCESS",
							"lossless_snippet", "false",
						),
					},
				},
				{
					TestResult: &pb.TestResult{
						TestId: "ninja://tests/BazTest.DoesQux",
						Status: pb.TestStatus_FAIL,
						Tags: pbutil.StringPairs(
							"test_name", "BazTest.DoesQux",
							"gtest_status", "FAILURE",
							"lossless_snippet", "false",
						),
					},
				},
				{
					TestResult: &pb.TestResult{
						TestId: "ninja://tests/FooTest.DoesBar",
						Status: pb.TestStatus_FAIL,
						Tags: pbutil.StringPairs(
							"test_name", "FooTest.DoesBar",
							"gtest_status", "EXCESSIVE_OUTPUT",
							"lossless_snippet", "false",
						),
					},
				},
				{
					TestResult: &pb.TestResult{
						TestId: "ninja://tests/FooTest.DoesBar",
						Status: pb.TestStatus_FAIL,
						Tags: pbutil.StringPairs(
							"test_name", "FooTest.DoesBar",
							"gtest_status", "FAILURE_ON_EXIT",
							"lossless_snippet", "false",
						),
					},
				},

				// Iteration 2.
				{
					TestResult: &pb.TestResult{
						TestId:   "ninja://tests/BazTest.DoesQux",
						Expected: true,
						Status:   pb.TestStatus_PASS,
						Tags: pbutil.StringPairs(
							"test_name", "BazTest.DoesQux",
							"gtest_status", "SUCCESS",
							"lossless_snippet", "false",
						),
					},
				},
				{
					TestResult: &pb.TestResult{
						TestId:   "ninja://tests/BazTest.DoesQux",
						Expected: true,
						Status:   pb.TestStatus_PASS,
						Tags: pbutil.StringPairs(
							"test_name", "BazTest.DoesQux",
							"gtest_status", "SUCCESS",
							"lossless_snippet", "false",
						),
					},
				},
				{
					TestResult: &pb.TestResult{
						TestId: "ninja://tests/FooTest.DoesBar",
						Status: pb.TestStatus_FAIL,
						Tags: pbutil.StringPairs(
							"test_name", "FooTest.DoesBar",
							"gtest_status", "FAILURE",
							"lossless_snippet", "false",
						),
					},
				},
				{
					TestResult: &pb.TestResult{
						TestId: "ninja://tests/FooTest.DoesBar",
						Status: pb.TestStatus_FAIL,
						Tags: pbutil.StringPairs(
							"test_name", "FooTest.DoesBar",
							"gtest_status", "FAILURE_ON_EXIT",
							"lossless_snippet", "false",
						),
					},
				},
			})
		})

		Convey("GlobalTags", func() {
			results := &GTestResults{
				GlobalTags: []string{"tag2", "tag1"},
				PerIterationData: []map[string][]*GTestRunResult{
					{
						"BazTest.DoesQux": {
							{Status: "SUCCESS"},
						},
					},
				},
			}

			_, err := results.ToProtos(ctx, "ninja://tests/", inv)
			So(err, ShouldBeNil)
			So(pbutil.StringPairsContain(inv.Tags, pbutil.StringPair("gtest_global_tag", "tag1")), ShouldBeTrue)
			So(pbutil.StringPairsContain(inv.Tags, pbutil.StringPair("gtest_global_tag", "tag2")), ShouldBeTrue)
		})
	})
}
