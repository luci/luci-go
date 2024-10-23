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

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/validate"

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
	return fmt.Sprintf("%s: %s", fieldName, validate.Unspecified())
}

// fieldDoesNotMatch returns the string of doesNotMatch error with the field name.
func fieldDoesNotMatch(fieldName string, re *regexp.Regexp) string {
	return fmt.Sprintf("%s: %s", fieldName, validate.DoesNotMatchReErr(re))
}

func TestTestResultName(t *testing.T) {
	t.Parallel()

	ftt.Run("ParseTestResultName", t, func(t *ftt.Test) {
		t.Run("Parse", func(t *ftt.Test) {
			invID, testID, resultID, err := ParseTestResultName(
				"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invID, should.Equal("a"))
			assert.Loosely(t, testID, should.Equal("ninja://chrome/test:foo_tests/BarTest.DoBaz"))
			assert.Loosely(t, resultID, should.Equal("result5"))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			t.Run(`has slashes`, func(t *ftt.Test) {
				_, _, _, err := ParseTestResultName(
					"invocations/inv/tests/ninja://test/results/result1")
				assert.Loosely(t, err, should.ErrLike("does not match pattern"))
			})

			t.Run(`bad unescape`, func(t *ftt.Test) {
				_, _, _, err := ParseTestResultName(
					"invocations/a/tests/bad_hex_%gg/results/result1")
				assert.Loosely(t, err, should.ErrLike("test id"))
			})

			t.Run(`unescaped unprintable`, func(t *ftt.Test) {
				_, _, _, err := ParseTestResultName(
					"invocations/a/tests/unprintable_%07/results/result1")
				assert.Loosely(t, err, should.ErrLike("non-printable rune"))
			})
		})

		t.Run("Format", func(t *ftt.Test) {
			assert.Loosely(t, TestResultName("a", "ninja://chrome/test:foo_tests/BarTest.DoBaz", "result5"),
				should.Equal(
					"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5"))
		})
	})
}

func TestValidateTestResult(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	validateTR := func(result *pb.TestResult) error {
		return ValidateTestResult(now, result)
	}

	ftt.Run("Succeeds", t, func(t *ftt.Test) {
		msg := validTestResult(now)
		assert.Loosely(t, validateTR(msg), should.BeNil)

		t.Run("with unicode TestID", func(t *ftt.Test) {
			// Uses printable unicode character 'µ'.
			msg.TestId = "TestVariousDeadlines/5µs"
			assert.Loosely(t, ValidateTestID(msg.TestId), should.ErrLike(nil))
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("with invalid Name", func(t *ftt.Test) {
			// ValidateTestResult should skip validating TestResult.Name.
			msg.Name = "this is not a valid name for TestResult.Name"
			assert.Loosely(t, ValidateTestResultName(msg.Name), should.ErrLike("does not match pattern"))
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("with no variant", func(t *ftt.Test) {
			msg.Variant = nil
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("with valid summary", func(t *ftt.Test) {
			msg.SummaryHtml = strings.Repeat("1", maxLenSummaryHTML)
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("with empty tags", func(t *ftt.Test) {
			msg.Tags = nil
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("with nil start_time", func(t *ftt.Test) {
			msg.StartTime = nil
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("with nil duration", func(t *ftt.Test) {
			msg.Duration = nil
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("with valid properties", func(t *ftt.Test) {
			msg.Properties = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue("value"),
				},
			}
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("with skip reason", func(t *ftt.Test) {
			msg.Status = pb.TestStatus_SKIP
			msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})
	})

	ftt.Run("Fails", t, func(t *ftt.Test) {
		msg := validTestResult(now)
		t.Run("with nil", func(t *ftt.Test) {
			assert.Loosely(t, validateTR(nil), should.ErrLike(validate.Unspecified()))
		})

		t.Run("with empty TestID", func(t *ftt.Test) {
			msg.TestId = ""
			assert.Loosely(t, validateTR(msg), should.ErrLike(fieldUnspecified("test_id")))
		})

		t.Run("with invalid TestID", func(t *ftt.Test) {
			badInputs := []struct {
				badID  string
				errStr string
			}{
				// TestID is too long
				{strings.Repeat("1", 512+1), "longer than 512 bytes"},
				// [[:print:]] matches with [ -~] and [[:graph:]]
				{string(rune(7)), "non-printable"},
				// UTF8 text that is not in normalization form C.
				{string("cafe\u0301"), "not in unicode normalized form C"},
			}
			for _, tc := range badInputs {
				msg.TestId = tc.badID
				check.Loosely(t, validateTR(msg), should.ErrLike(tc.errStr))
			}
		})

		t.Run("with empty ResultID", func(t *ftt.Test) {
			msg.ResultId = ""
			assert.Loosely(t, validateTR(msg), should.ErrLike(fieldUnspecified("result_id")))
		})

		t.Run("with invalid ResultID", func(t *ftt.Test) {
			badInputs := []string{
				strings.Repeat("1", 32+1),
				string(rune(7)),
			}
			for _, in := range badInputs {
				msg.ResultId = in
				assert.Loosely(t, validateTR(msg), should.ErrLike(fieldDoesNotMatch("result_id", resultIDRe)))
			}
		})

		t.Run("with invalid Variant", func(t *ftt.Test) {
			badInputs := []*pb.Variant{
				Variant("", ""),
				Variant("", "val"),
			}
			for _, in := range badInputs {
				msg.Variant = in
				assert.Loosely(t, validateTR(msg), should.ErrLike(fieldUnspecified("key")))
			}
		})

		t.Run("with invalid Status", func(t *ftt.Test) {
			msg.Status = pb.TestStatus(len(pb.TestStatus_name) + 1)
			assert.Loosely(t, validateTR(msg), should.ErrLike("status: invalid value"))
		})

		t.Run("with STATUS_UNSPECIFIED", func(t *ftt.Test) {
			msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
			assert.Loosely(t, validateTR(msg), should.ErrLike("status: cannot be STATUS_UNSPECIFIED"))
		})

		t.Run("with skip reason but not skip status", func(t *ftt.Test) {
			msg.Status = pb.TestStatus_ABORT
			msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
			assert.Loosely(t, validateTR(msg), should.ErrLike("skip_reason: value must be zero (UNSPECIFIED) when status is not SKIP"))
		})

		t.Run("with too big summary", func(t *ftt.Test) {
			msg.SummaryHtml = strings.Repeat("☕", maxLenSummaryHTML)
			assert.Loosely(t, validateTR(msg), should.ErrLike("summary_html: exceeds the maximum size"))
		})

		t.Run("with invalid StartTime and Duration", func(t *ftt.Test) {
			t.Run("because start_time is in the future", func(t *ftt.Test) {
				future := timestamppb.New(now.Add(time.Hour))
				msg.StartTime = future
				assert.Loosely(t, validateTR(msg), should.ErrLike(fmt.Sprintf("start_time: cannot be > (now + %s)", clockSkew)))
			})

			t.Run("because duration is < 0", func(t *ftt.Test) {
				msg.Duration = durationpb.New(-1 * time.Minute)
				assert.Loosely(t, validateTR(msg), should.ErrLike("duration: is < 0"))
			})

			t.Run("because (start_time + duration) is in the future", func(t *ftt.Test) {
				st := timestamppb.New(now.Add(-1 * time.Hour))
				msg.StartTime = st
				msg.Duration = durationpb.New(2 * time.Hour)
				expected := fmt.Sprintf("start_time + duration: cannot be > (now + %s)", clockSkew)
				assert.Loosely(t, validateTR(msg), should.ErrLike(expected))
			})
		})

		t.Run("with invalid StringPairs", func(t *ftt.Test) {
			msg.Tags = StringPairs("", "")
			assert.Loosely(t, validateTR(msg), should.ErrLike(`"":"": key: unspecified`))
		})

		t.Run("Test metadata", func(t *ftt.Test) {
			t.Run("filename", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					msg.TestMetadata.Location.FileName = ""
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: unspecified"))
				})
				t.Run("too long", func(t *ftt.Test) {
					msg.TestMetadata.Location.FileName = "//" + strings.Repeat("super long", 100)
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: length exceeds 512"))
				})
				t.Run("no double slashes", func(t *ftt.Test) {
					msg.TestMetadata.Location.FileName = "file_name"
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: doesn't start with //"))
				})
				t.Run("back slash", func(t *ftt.Test) {
					msg.TestMetadata.Location.FileName = "//dir\\file"
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: has \\"))
				})
				t.Run("trailing slash", func(t *ftt.Test) {
					msg.TestMetadata.Location.FileName = "//file_name/"
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: ends with /"))
				})
			})
			t.Run("line", func(t *ftt.Test) {
				msg.TestMetadata.Location.Line = -1
				assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: line: must not be negative"))
			})
			t.Run("repo", func(t *ftt.Test) {
				msg.TestMetadata.Location.Repo = "https://chromium.googlesource.com/chromium/src.git"
				assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: repo: must not end with .git"))
			})

			t.Run("no location and no bug component", func(t *ftt.Test) {
				msg.TestMetadata = &pb.TestMetadata{Name: "name"}
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("location no repo", func(t *ftt.Test) {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					Location: &pb.TestLocation{
						FileName: "//file_name",
					},
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: repo: required"))
			})

			t.Run("nil bug system in bug component", func(t *ftt.Test) {
				msg.TestMetadata = &pb.TestMetadata{
					Name: "name",
					BugComponent: &pb.BugComponent{
						System: nil,
					},
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike("bug system is required for bug components"))
			})
			t.Run("valid monorail bug component", func(t *ftt.Test) {
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
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("wrong size monorail bug component value", func(t *ftt.Test) {
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
				assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.value: is invalid"))
			})
			t.Run("invalid monorail bug component value", func(t *ftt.Test) {
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
				assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.value: is invalid"))
			})
			t.Run("wrong size monorail bug component project", func(t *ftt.Test) {
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
				assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.project: is invalid"))
			})
			t.Run("using invalid characters in monorail bug component project", func(t *ftt.Test) {
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
				assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.project: is invalid"))
			})
			t.Run("using only numbers in monorail bug component project", func(t *ftt.Test) {
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
				assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.project: is invalid"))
			})
			t.Run("valid buganizer component", func(t *ftt.Test) {
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
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("invalid buganizer component id", func(t *ftt.Test) {
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
				assert.Loosely(t, validateTR(msg), should.ErrLike("issue_tracker.component_id: is invalid"))
			})
			t.Run("with too big properties", func(t *ftt.Test) {
				msg.TestMetadata = &pb.TestMetadata{
					PropertiesSchema: "package.message",
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue(strings.Repeat("1", MaxSizeTestMetadataProperties)),
						},
					},
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike("properties: exceeds the maximum size"))
			})
			t.Run("no properties_schema with non-empty properties", func(t *ftt.Test) {
				msg.TestMetadata = &pb.TestMetadata{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("1"),
						},
					},
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike("properties_schema must be specified with non-empty properties"))
			})
			t.Run("invalid properties_schema", func(t *ftt.Test) {
				msg.TestMetadata = &pb.TestMetadata{
					PropertiesSchema: "package",
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike("properties_schema: does not match"))
			})
			t.Run("valid properties_schema and non-empty properties", func(t *ftt.Test) {
				msg.TestMetadata = &pb.TestMetadata{
					PropertiesSchema: "package.message",
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("1"),
						},
					},
				}
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
		})

		t.Run("with too big properties", func(t *ftt.Test) {
			msg.Properties = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue(strings.Repeat("1", MaxSizeTestResultProperties)),
				},
			}
			assert.Loosely(t, validateTR(msg), should.ErrLike("properties: exceeds the maximum size"))
		})

		t.Run("Validate failure reason", func(t *ftt.Test) {
			errorMessage1 := "error1"
			errorMessage2 := "error2"
			longErrorMessage := strings.Repeat("a very long error message", 100)
			t.Run("valid failure reason", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: errorMessage2},
					},
					TruncatedErrorsCount: 0,
				}
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})

			t.Run("primary_error_message exceeds the maximum limit", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: longErrorMessage,
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike("primary_error_message: "+
					"exceeds the maximum"))
			})

			t.Run("one of the error messages exceeds the maximum limit", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: longErrorMessage},
					},
					TruncatedErrorsCount: 0,
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike(
					"errors[1]: message: exceeds the maximum size of 1024 "+
						"bytes"))
			})

			t.Run("the first error doesn't match primary_error_message", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage2},
					},
					TruncatedErrorsCount: 0,
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike(
					"errors[0]: message: must match primary_error_message"))
			})

			t.Run("the total size of the errors list exceeds the limit", func(t *ftt.Test) {
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
				assert.Loosely(t, validateTR(msg), should.ErrLike(
					"errors: exceeds the maximum total size of 3172 bytes"))
			})

			t.Run("invalid truncated error count", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: errorMessage2},
					},
					TruncatedErrorsCount: -1,
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike("truncated_errors_count: "+
					"must be non-negative"))
			})
		})
	})
}
