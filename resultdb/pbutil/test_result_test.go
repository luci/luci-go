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

package pbutil

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validTestResult returns a valid TestResult sample.
func validTestResult(now time.Time) *pb.TestResult {
	st := timestamppb.New(now.Add(-2 * time.Minute))
	return &pb.TestResult{
		Name:        "invocations/a/tests/invocation_id1/results/result_id1",
		TestId:      "this is testID",
		ResultId:    "result_id1",
		Variant:     Variant("a", "b"),
		Expected:    true,
		Status:      pb.TestStatus_PASS,
		SummaryHtml: "HTML summary",
		StartTime:   st,
		Duration:    durationpb.New(time.Minute),
		TestMetadata: &pb.TestMetadata{
			Location: &pb.TestLocation{
				Repo:     "https://git.example.com",
				FileName: "//a_test.go",
				Line:     54,
			},
			BugComponent: &pb.BugComponent{
				System: &pb.BugComponent_Monorail{
					Monorail: &pb.MonorailComponent{
						Project: "chromium",
						Value:   "Component>Value",
					},
				},
			},
		},
		Tags: StringPairs("k1", "v1"),
	}
}

// fieldDoesNotMatch returns the string of unspecified error with the field name.
func fieldUnspecified(fieldName string) string {
	return fmt.Sprintf("%s: %s", fieldName, unspecified())
}

// fieldDoesNotMatch returns the string of doesNotMatch error with the field name.
func fieldDoesNotMatch(fieldName string, re *regexp.Regexp) string {
	return fmt.Sprintf("%s: %s", fieldName, doesNotMatch(re))
}

func TestTestResultName(t *testing.T) {
	t.Parallel()

	Convey("ParseTestResultName", t, func() {
		Convey("Parse", func() {
			invID, testID, resultID, err := ParseTestResultName(
				"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5")
			So(err, ShouldBeNil)
			So(invID, ShouldEqual, "a")
			So(testID, ShouldEqual, "ninja://chrome/test:foo_tests/BarTest.DoBaz")
			So(resultID, ShouldEqual, "result5")
		})

		Convey("Invalid", func() {
			Convey(`has slashes`, func() {
				_, _, _, err := ParseTestResultName(
					"invocations/inv/tests/ninja://test/results/result1")
				So(err, ShouldErrLike, doesNotMatch(testResultNameRe))
			})

			Convey(`bad unescape`, func() {
				_, _, _, err := ParseTestResultName(
					"invocations/a/tests/bad_hex_%gg/results/result1")
				So(err, ShouldErrLike, "test id")
			})

			Convey(`unescaped unprintable`, func() {
				_, _, _, err := ParseTestResultName(
					"invocations/a/tests/unprintable_%07/results/result1")
				So(err, ShouldErrLike, "non-printable rune")
			})
		})

		Convey("Format", func() {
			So(TestResultName("a", "ninja://chrome/test:foo_tests/BarTest.DoBaz", "result5"),
				ShouldEqual,
				"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5")
		})
	})
}

func TestValidateTestResult(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	validate := func(result *pb.TestResult) error {
		return ValidateTestResult(now, result)
	}

	Convey("Succeeds", t, func() {
		msg := validTestResult(now)
		So(validate(msg), ShouldBeNil)

		Convey("with unicode TestID", func() {
			// Uses printable unicode character 'µ'.
			msg.TestId = "TestVariousDeadlines/5µs"
			So(ValidateTestID(msg.TestId), ShouldErrLike, nil)
			So(validate(msg), ShouldBeNil)
		})

		Convey("with invalid Name", func() {
			// ValidateTestResult should skip validating TestResult.Name.
			msg.Name = "this is not a valid name for TestResult.Name"
			So(ValidateTestResultName(msg.Name), ShouldErrLike, doesNotMatch(testResultNameRe))
			So(validate(msg), ShouldBeNil)
		})

		Convey("with no variant", func() {
			msg.Variant = nil
			So(validate(msg), ShouldBeNil)
		})

		Convey("with valid summary", func() {
			msg.SummaryHtml = strings.Repeat("1", maxLenSummaryHTML)
			So(validate(msg), ShouldBeNil)
		})

		Convey("with empty tags", func() {
			msg.Tags = nil
			So(validate(msg), ShouldBeNil)
		})

		Convey("with nil start_time", func() {
			msg.StartTime = nil
			So(validate(msg), ShouldBeNil)
		})

		Convey("with nil duration", func() {
			msg.Duration = nil
			So(validate(msg), ShouldBeNil)
		})

		Convey("with valid properties", func() {
			msg.Properties = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue("value"),
				},
			}
			So(validate(msg), ShouldBeNil)
		})

		Convey("with skip reason", func() {
			msg.Status = pb.TestStatus_SKIP
			msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
			So(validate(msg), ShouldBeNil)
		})
	})

	Convey("Fails", t, func() {
		msg := validTestResult(now)
		Convey("with nil", func() {
			So(validate(nil), ShouldErrLike, unspecified())
		})

		Convey("with empty TestID", func() {
			msg.TestId = ""
			So(validate(msg), ShouldErrLike, fieldUnspecified("test_id"))
		})

		Convey("with invalid TestID", func() {
			badInputs := []string{
				strings.Repeat("1", 512+1),
				// [[:print:]] matches with [ -~] and [[:graph:]]
				string(rune(7)),
				string("cafe\u0301"), // UTF8 text that is not in normalization form C.
			}
			for _, in := range badInputs {
				msg.TestId = in
				So(validate(msg), ShouldErrLike, "")
			}
		})

		Convey("with empty ResultID", func() {
			msg.ResultId = ""
			So(validate(msg), ShouldErrLike, fieldUnspecified("result_id"))
		})

		Convey("with invalid ResultID", func() {
			badInputs := []string{
				strings.Repeat("1", 32+1),
				string(rune(7)),
			}
			for _, in := range badInputs {
				msg.ResultId = in
				So(validate(msg), ShouldErrLike, fieldDoesNotMatch("result_id", resultIDRe))
			}
		})

		Convey("with invalid Variant", func() {
			badInputs := []*pb.Variant{
				Variant("", ""),
				Variant("", "val"),
			}
			for _, in := range badInputs {
				msg.Variant = in
				So(validate(msg), ShouldErrLike, fieldUnspecified("key"))
			}
		})

		Convey("with invalid Status", func() {
			msg.Status = pb.TestStatus(len(pb.TestStatus_name) + 1)
			So(validate(msg), ShouldErrLike, "status: invalid value")
		})

		Convey("with STATUS_UNSPECIFIED", func() {
			msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
			So(validate(msg), ShouldErrLike, "status: cannot be STATUS_UNSPECIFIED")
		})

		Convey("with skip reason but not skip status", func() {
			msg.Status = pb.TestStatus_ABORT
			msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
			So(validate(msg), ShouldErrLike, "skip_reason: value must be zero (UNSPECIFIED) when status is not SKIP")
		})

		Convey("with too big summary", func() {
			msg.SummaryHtml = strings.Repeat("☕", maxLenSummaryHTML)
			So(validate(msg), ShouldErrLike, "summary_html: exceeds the maximum size")
		})

		Convey("with invalid StartTime and Duration", func() {
			Convey("because start_time is in the future", func() {
				future := timestamppb.New(now.Add(time.Hour))
				msg.StartTime = future
				So(validate(msg), ShouldErrLike, fmt.Sprintf("start_time: cannot be > (now + %s)", clockSkew))
			})

			Convey("because duration is < 0", func() {
				msg.Duration = durationpb.New(-1 * time.Minute)
				So(validate(msg), ShouldErrLike, "duration: is < 0")
			})

			Convey("because (start_time + duration) is in the future", func() {
				st := timestamppb.New(now.Add(-1 * time.Hour))
				msg.StartTime = st
				msg.Duration = durationpb.New(2 * time.Hour)
				expected := fmt.Sprintf("start_time + duration: cannot be > (now + %s)", clockSkew)
				So(validate(msg), ShouldErrLike, expected)
			})
		})

		Convey("with invalid StringPairs", func() {
			msg.Tags = StringPairs("", "")
			So(validate(msg), ShouldErrLike, `"":"": key: unspecified`)
		})

		Convey("Test metadata", func() {
			Convey("filename", func() {
				Convey("unspecified", func() {
					msg.TestMetadata.Location.FileName = ""
					So(validate(msg), ShouldErrLike, "test_metadata: location: file_name: unspecified")
				})
				Convey("too long", func() {
					msg.TestMetadata.Location.FileName = "//" + strings.Repeat("super long", 100)
					So(validate(msg), ShouldErrLike, "test_metadata: location: file_name: length exceeds 512")
				})
				Convey("no double slashes", func() {
					msg.TestMetadata.Location.FileName = "file_name"
					So(validate(msg), ShouldErrLike, "test_metadata: location: file_name: doesn't start with //")
				})
				Convey("back slash", func() {
					msg.TestMetadata.Location.FileName = "//dir\\file"
					So(validate(msg), ShouldErrLike, "test_metadata: location: file_name: has \\")
				})
				Convey("trailing slash", func() {
					msg.TestMetadata.Location.FileName = "//file_name/"
					So(validate(msg), ShouldErrLike, "test_metadata: location: file_name: ends with /")
				})
			})
			Convey("line", func() {
				msg.TestMetadata.Location.Line = -1
				So(validate(msg), ShouldErrLike, "test_metadata: location: line: must not be negative")
			})
			Convey("repo", func() {
				msg.TestMetadata.Location.Repo = "https://chromium.googlesource.com/chromium/src.git"
				So(validate(msg), ShouldErrLike, "test_metadata: location: repo: must not end with .git")
			})

			Convey("no location and no bug component", func() {
				msg.TestMetadata = &pb.TestMetadata{Name: "name"}
				So(validate(msg), ShouldBeNil)
			})
			Convey("location no repo", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					Location: &pb.TestLocation{
						FileName: "//file_name",
					},
				}
				So(validate(msg), ShouldErrLike, "test_metadata: location: repo: required")
			})

			Convey("nil bug system in bug component", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: nil,
					},
				}
				So(validate(msg), ShouldErrLike, "bug system is required for bug components")
			})
			Convey("valid monorail bug component", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: &pb.BugComponent_Monorail{
							Monorail: &pb.MonorailComponent{
								Project: "1chromium1",
								Value:   "Component>Value",
							},
						},
					},
				}
				So(validate(msg), ShouldBeNil)
			})
			Convey("wrong size monorail bug component value", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: &pb.BugComponent_Monorail{
							Monorail: &pb.MonorailComponent{
								Project: "chromium",
								Value:   strings.Repeat("a", 601),
							},
						},
					},
				}
				So(validate(msg), ShouldErrLike, "monorail.value: is invalid")
			})
			Convey("invalid monorail bug component value", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: &pb.BugComponent_Monorail{
							Monorail: &pb.MonorailComponent{
								Project: "chromium",
								Value:   "Component<><>Value",
							},
						},
					},
				}
				So(validate(msg), ShouldErrLike, "monorail.value: is invalid")
			})
			Convey("wrong size monorail bug component project", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: &pb.BugComponent_Monorail{
							Monorail: &pb.MonorailComponent{
								Project: strings.Repeat("a", 64),
								Value:   "Component>Value",
							},
						},
					},
				}
				So(validate(msg), ShouldErrLike, "monorail.project: is invalid")
			})
			Convey("using invalid characters in monorail bug component project", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: &pb.BugComponent_Monorail{
							Monorail: &pb.MonorailComponent{
								Project: "$%^ $$^%",
								Value:   "Component>Value",
							},
						},
					},
				}
				So(validate(msg), ShouldErrLike, "monorail.project: is invalid")
			})
			Convey("using only numbers in monorail bug component project", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: &pb.BugComponent_Monorail{
							Monorail: &pb.MonorailComponent{
								Project: "11111",
								Value:   "Component>Value",
							},
						},
					},
				}
				So(validate(msg), ShouldErrLike, "monorail.project: is invalid")
			})
			Convey("valid buganizer component", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: &pb.BugComponent_IssueTracker{
							IssueTracker: &pb.IssueTrackerComponent{
								ComponentId: 1234,
							},
						},
					},
				}
				So(validate(msg), ShouldBeNil)
			})
			Convey("invalid buganizer component id", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: &pb.BugComponent_IssueTracker{
							IssueTracker: &pb.IssueTrackerComponent{
								ComponentId: -1,
							},
						},
					},
				}
				So(validate(msg), ShouldErrLike, "issue_tracker.component_id: is invalid")
			})
			Convey("with too big properties", func() {
				msg.TestMetadata = &pb.TestMetadata{
					PropertiesSchema: "package.message",
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue(strings.Repeat("1", MaxSizeProperties)),
						},
					},
				}
				So(validate(msg), ShouldErrLike, "properties: exceeds the maximum size")
			})
			Convey("no properties_schema with non-empty properties", func() {
				msg.TestMetadata = &pb.TestMetadata{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("1"),
						},
					},
				}
				So(validate(msg), ShouldErrLike, "properties_schema must be specified with non-empty properties")
			})
			Convey("invalid properties_schema", func() {
				msg.TestMetadata = &pb.TestMetadata{
					PropertiesSchema: "package",
				}
				So(validate(msg), ShouldErrLike, "properties_schema: does not match")
			})
			Convey("valid properties_schema and non-empty properties", func() {
				msg.TestMetadata = &pb.TestMetadata{
					PropertiesSchema: "package.message",
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("1"),
						},
					},
				}
				So(validate(msg), ShouldBeNil)
			})
		})

		Convey("with too big properties", func() {
			msg.Properties = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue(strings.Repeat("1", MaxSizeProperties)),
				},
			}
			So(validate(msg), ShouldErrLike, "properties: exceeds the maximum size")
		})

		Convey("Validate failure reason", func() {
			errorMessage1 := "error1"
			errorMessage2 := "error2"
			longErrorMessage := strings.Repeat("a very long error message", 100)
			Convey("valid failure reason", func() {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: errorMessage2},
					},
					TruncatedErrorsCount: 0,
				}
				So(validate(msg), ShouldBeNil)
			})

			Convey("primary_error_message exceeds the maximum limit", func() {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: longErrorMessage,
				}
				So(validate(msg), ShouldErrLike, "primary_error_message: "+
					"exceeds the maximum")
			})

			Convey("one of the error messages exceeds the maximum limit", func() {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: longErrorMessage},
					},
					TruncatedErrorsCount: 0,
				}
				So(validate(msg), ShouldErrLike,
					"errors[1]: message: exceeds the maximum size of 1024 "+
						"bytes")
			})

			Convey("the first error doesn't match primary_error_message", func() {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage2},
					},
					TruncatedErrorsCount: 0,
				}
				So(validate(msg), ShouldErrLike,
					"errors[0]: message: must match primary_error_message")
			})

			Convey("the total size of the errors list exceeds the limit", func() {
				maxErrorMessage := strings.Repeat(".", 1024)
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: maxErrorMessage,
					Errors: []*pb.FailureReason_Error{
						{Message: maxErrorMessage},
						{Message: maxErrorMessage},
						{Message: maxErrorMessage},
						{Message: maxErrorMessage},
					},
					TruncatedErrorsCount: 1,
				}
				So(validate(msg), ShouldErrLike,
					"errors: exceeds the maximum total size of 3172 bytes")
			})

			Convey("invalid truncated error count", func() {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: errorMessage2},
					},
					TruncatedErrorsCount: -1,
				}
				So(validate(msg), ShouldErrLike, "truncated_errors_count: "+
					"must be non-negative")
			})
		})
	})
}
