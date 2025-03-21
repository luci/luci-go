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

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/validate"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

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
	ftt.Run(`ValidateTestResult`, t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC

		validateToScheme := func(id BaseTestIdentifier) error {
			return nil
		}
		validateTR := func(result *pb.TestResult) error {
			return ValidateTestResult(now, validateToScheme, result)
		}

		msg := validTestResult(now)
		assert.Loosely(t, validateTR(msg), should.BeNil)

		t.Run("structured test identifier", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				msg.TestIdStructured = nil
				assert.Loosely(t, validateTR(msg), should.ErrLike("test_id_structured: unspecified"))
			})
			t.Run("structure", func(t *ftt.Test) {
				// ParseAndValidateTestID has its own extensive test cases, these do not need to be repeated here.
				t.Run("case name invalid", func(t *ftt.Test) {
					msg.TestIdStructured.CaseName = "case name \x00"
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_id_structured: case_name: non-printable rune '\\x00' at byte index 10"))
				})
				t.Run("variant invalid", func(t *ftt.Test) {
					msg.TestIdStructured.ModuleVariant = Variant("key\x00", "case name")
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_id_structured: module_variant: \"key\\x00\":\"case name\": key: does not match pattern"))
				})
			})
			t.Run("scheme", func(t *ftt.Test) {
				t.Run("nil validation function", func(t *ftt.Test) {
					// Succeeds.
					validateToScheme = nil
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("passes validation function", func(t *ftt.Test) {
					// Only test to make sure validateToScheme is correctly invoked.
					var baseTestID BaseTestIdentifier
					var invoked bool
					validateToScheme = func(id BaseTestIdentifier) error {
						// Check only invoked once.
						assert.Loosely(t, invoked, should.Equal(false))
						baseTestID = id
						invoked = true
						return nil
					}

					assert.Loosely(t, validateTR(msg), should.BeNil)
					assert.Loosely(t, invoked, should.Equal(true))
					assert.Loosely(t, baseTestID, should.Equal(BaseTestIdentifier{
						ModuleName:   "//infra/java_tests",
						ModuleScheme: "junit",
						CoarseName:   "org.chromium.go.luci",
						FineName:     "ValidationTests",
						CaseName:     "FooBar",
					}))
				})
				t.Run("fails validation", func(t *ftt.Test) {
					var invoked bool
					validateToScheme = func(id BaseTestIdentifier) error {
						// Check only invoked once.
						assert.Loosely(t, invoked, should.Equal(false))
						invoked = true
						return errors.Reason("coarse_name: some error").Err()
					}

					assert.Loosely(t, validateTR(msg), should.ErrLike("test_id_structured: coarse_name: some error"))
					assert.Loosely(t, invoked, should.Equal(true))
				})
			})
			t.Run("legacy", func(t *ftt.Test) {
				msg.TestIdStructured = nil
				msg.TestId = "this is a test ID"
				msg.Variant = Variant("key", "value")
				t.Run("valid", func(t *ftt.Test) {
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("valid with no variant", func(t *ftt.Test) {
					msg.Variant = nil
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("scheme", func(t *ftt.Test) {
					t.Run("nil validation function", func(t *ftt.Test) {
						// Succeeds.
						validateToScheme = nil
						assert.Loosely(t, validateTR(msg), should.BeNil)
					})
					t.Run("passes validation function", func(t *ftt.Test) {
						// Only test to make sure validateToScheme is correctly invoked.
						var baseTestID BaseTestIdentifier
						var invoked bool
						validateToScheme = func(id BaseTestIdentifier) error {
							// Check only invoked once.
							assert.Loosely(t, invoked, should.Equal(false))
							baseTestID = id
							invoked = true
							return nil
						}

						assert.Loosely(t, validateTR(msg), should.BeNil)
						assert.Loosely(t, invoked, should.Equal(true))
						assert.Loosely(t, baseTestID, should.Equal(BaseTestIdentifier{
							ModuleName:   "legacy",
							ModuleScheme: "legacy",
							CoarseName:   "",
							FineName:     "",
							CaseName:     "this is a test ID",
						}))
					})
					t.Run("fails validation", func(t *ftt.Test) {
						var invoked bool
						validateToScheme = func(id BaseTestIdentifier) error {
							// Check only invoked once.
							assert.Loosely(t, invoked, should.Equal(false))
							invoked = true
							return errors.Reason("module_scheme: scheme %q not defined", id.ModuleScheme).Err()
						}

						assert.Loosely(t, validateTR(msg), should.ErrLike("test_id: module_scheme: scheme \"legacy\" not defined"))
						assert.Loosely(t, invoked, should.Equal(true))
					})
				})
				t.Run("invalid test ID", func(t *ftt.Test) {
					// Uses printable unicode character 'µ'.
					msg.TestId = "this is a test ID\x00"
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_id: non-printable rune '\\x00' at byte index 17"))
				})
				t.Run("invalid Variant", func(t *ftt.Test) {
					badInputs := []*pb.Variant{
						Variant("", ""),
						Variant("", "val"),
					}
					for _, in := range badInputs {
						msg.Variant = in
						assert.Loosely(t, validateTR(msg), should.ErrLike("key: unspecified"))
					}
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
			})
		})
		t.Run("Name", func(t *ftt.Test) {
			// ValidateTestResult should not validate TestResult.Name.
			msg.Name = "this is not a valid name for TestResult.Name"
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("Summary HTML", func(t *ftt.Test) {
			t.Run("with valid summary", func(t *ftt.Test) {
				msg.SummaryHtml = strings.Repeat("1", maxLenSummaryHTML)
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with too big summary", func(t *ftt.Test) {
				msg.SummaryHtml = strings.Repeat("☕", maxLenSummaryHTML)
				assert.Loosely(t, validateTR(msg), should.ErrLike("summary_html: exceeds the maximum size"))
			})
		})

		t.Run("Tags", func(t *ftt.Test) {
			t.Run("with empty tags", func(t *ftt.Test) {
				msg.Tags = nil
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with invalid StringPairs", func(t *ftt.Test) {
				msg.Tags = StringPairs("", "")
				assert.Loosely(t, validateTR(msg), should.ErrLike(`"":"": key: unspecified`))
			})

		})

		t.Run("with nil", func(t *ftt.Test) {
			assert.Loosely(t, validateTR(nil), should.ErrLike(validate.Unspecified()))
		})

		t.Run("Result ID", func(t *ftt.Test) {
			t.Run("with empty ResultID", func(t *ftt.Test) {
				msg.ResultId = ""
				assert.Loosely(t, validateTR(msg), should.ErrLike("result_id: unspecified"))
			})

			t.Run("with invalid ResultID", func(t *ftt.Test) {
				badInputs := []string{
					strings.Repeat("1", 32+1),
					string(rune(7)),
				}
				for _, in := range badInputs {
					msg.ResultId = in
					assert.Loosely(t, validateTR(msg), should.ErrLike("result_id: does not match pattern \"^[a-z0-9\\\\-_.]{1,32}$\""))
				}
			})
		})
		t.Run("Status", func(t *ftt.Test) {
			t.Run("with invalid Status", func(t *ftt.Test) {
				msg.Status = pb.TestStatus(len(pb.TestStatus_name) + 1)
				assert.Loosely(t, validateTR(msg), should.ErrLike("status: invalid value"))
			})
			t.Run("with STATUS_UNSPECIFIED", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
				assert.Loosely(t, validateTR(msg), should.ErrLike("status: cannot be STATUS_UNSPECIFIED"))
			})
		})
		t.Run("Skip Reason", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_SKIP
				msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with skip reason but not skip status", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_ABORT
				msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
				assert.Loosely(t, validateTR(msg), should.ErrLike("skip_reason: value must be zero (UNSPECIFIED) when status is not SKIP"))
			})
		})
		t.Run("StartTime and Duration", func(t *ftt.Test) {
			// Valid cases.
			t.Run("with nil start_time", func(t *ftt.Test) {
				msg.StartTime = nil
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with nil duration", func(t *ftt.Test) {
				msg.Duration = nil
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			// Invalid cases.
			t.Run("because start_time is in the future", func(t *ftt.Test) {
				future := timestamppb.New(now.Add(time.Hour))
				msg.StartTime = future
				assert.Loosely(t, validateTR(msg), should.ErrLike(fmt.Sprintf("start_time: cannot be > (now + %s)", maxClockSkew)))
			})

			t.Run("because duration is < 0", func(t *ftt.Test) {
				msg.Duration = durationpb.New(-1 * time.Minute)
				assert.Loosely(t, validateTR(msg), should.ErrLike("duration: is < 0"))
			})

			t.Run("because (start_time + duration) is in the future", func(t *ftt.Test) {
				st := timestamppb.New(now.Add(-1 * time.Hour))
				msg.StartTime = st
				msg.Duration = durationpb.New(2 * time.Hour)
				expected := fmt.Sprintf("start_time + duration: cannot be > (now + %s)", maxClockSkew)
				assert.Loosely(t, validateTR(msg), should.ErrLike(expected))
			})
		})

		t.Run("Test metadata", func(t *ftt.Test) {
			t.Run("no location and no bug component", func(t *ftt.Test) {
				msg.TestMetadata = &pb.TestMetadata{Name: "name"}
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("Location", func(t *ftt.Test) {
				msg.TestMetadata.Location = &pb.TestLocation{
					Repo:     "https://git.example.com",
					FileName: "//a_test.go",
					Line:     54,
				}
				t.Run("unspecified", func(t *ftt.Test) {
					msg.TestMetadata.Location = nil
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
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
					t.Run("invalid", func(t *ftt.Test) {
						msg.TestMetadata.Location.Repo = "https://chromium.googlesource.com/chromium/src.git"
						assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: repo: must not end with .git"))
					})
					t.Run("unspecified", func(t *ftt.Test) {
						msg.TestMetadata.Location.Repo = ""
						assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: repo: required"))
					})
				})
			})
			t.Run("Bug component", func(t *ftt.Test) {

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
			})
			t.Run("Properties", func(t *ftt.Test) {
				t.Run("with valid properties", func(t *ftt.Test) {
					msg.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("value"),
						},
					}
					assert.Loosely(t, validateTR(msg), should.BeNil)
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
					assert.Loosely(t, validateTR(msg), should.ErrLike("exceeds the maximum size"))
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
			t.Run("Previous Test ID", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						PreviousTestId: "",
					}
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						PreviousTestId: "ninja://old_test_id",
					}
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						PreviousTestId: ":module!scheme#",
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: previous_test_id: got delimiter character '#' at byte 14; expected "))
				})
			})
		})
		t.Run("Failure reason", func(t *ftt.Test) {
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

// validTestResult returns a valid TestResult sample.
func validTestResult(now time.Time) *pb.TestResult {
	st := timestamppb.New(now.Add(-2 * time.Minute))
	return &pb.TestResult{
		ResultId: "result_id1",
		TestIdStructured: &pb.TestIdentifier{
			ModuleName:    "//infra/java_tests",
			ModuleVariant: Variant("a", "b"),
			ModuleScheme:  "junit",
			CoarseName:    "org.chromium.go.luci",
			FineName:      "ValidationTests",
			CaseName:      "FooBar",
		},
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
