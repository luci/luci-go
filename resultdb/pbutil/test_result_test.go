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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/validate"

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
						return errors.New("coarse_name: some error")
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
							return errors.Fmt("module_scheme: scheme %q not defined", id.ModuleScheme)
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
			msg.Status = pb.TestStatus_PASS
			msg.Expected = true
			msg.StatusV2 = pb.TestResult_STATUS_UNSPECIFIED

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with invalid Status", func(t *ftt.Test) {
				msg.Status = pb.TestStatus(len(pb.TestStatus_name) + 1)
				assert.Loosely(t, validateTR(msg), should.ErrLike("status: invalid value"))
			})
			t.Run("unspecified", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
				// Error message is on status_v2 rather than status as neither was specified,
				// and status is deprecated.
				assert.Loosely(t, validateTR(msg), should.ErrLike("status_v2: cannot be STATUS_UNSPECIFIED"))
			})
		})
		t.Run("Status V2", func(t *ftt.Test) {
			msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
			msg.Expected = false
			msg.StatusV2 = pb.TestResult_PASSED

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with invalid Status", func(t *ftt.Test) {
				msg.StatusV2 = pb.TestResult_Status(len(pb.TestResult_Status_name) + 1)
				assert.Loosely(t, validateTR(msg), should.ErrLike("status_v2: invalid value"))
			})
			t.Run("unspecified", func(t *ftt.Test) {
				msg.StatusV2 = pb.TestResult_STATUS_UNSPECIFIED
				assert.Loosely(t, validateTR(msg), should.ErrLike("status_v2: cannot be STATUS_UNSPECIFIED"))
			})
			t.Run("both status v1 and v2 specified", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_PASS
				assert.Loosely(t, validateTR(msg), should.ErrLike("status: must not specify at same time as status_v2; specify status_v2 only"))
			})
			t.Run("both expected and v2 specified", func(t *ftt.Test) {
				msg.Expected = true
				assert.Loosely(t, validateTR(msg), should.ErrLike("expected: must not specify at same time as status_v2; specify status_v2 only"))
			})
		})
		t.Run("Skip Reason", func(t *ftt.Test) {
			t.Run("With legacy status", func(t *ftt.Test) {
				msg.StatusV2 = pb.TestResult_STATUS_UNSPECIFIED
				msg.Status = pb.TestStatus_SKIP
				msg.Expected = true

				t.Run("with skip reason is valid", func(t *ftt.Test) {
					msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("without skip reason is valid", func(t *ftt.Test) {
					msg.SkipReason = pb.SkipReason_SKIP_REASON_UNSPECIFIED
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("with skip reason but not skip status", func(t *ftt.Test) {
					msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
					msg.Status = pb.TestStatus_ABORT
					assert.Loosely(t, validateTR(msg), should.ErrLike("skip_reason: value must be zero (UNSPECIFIED) when status is not SKIP"))
				})
			})
			t.Run("With status_v2", func(t *ftt.Test) {
				msg.StatusV2 = pb.TestResult_SKIPPED
				msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
				msg.Expected = false

				// Required because we specified a StatusV2 of Skipped.
				msg.SkippedReason = &pb.SkippedReason{
					Kind: pb.SkippedReason_DISABLED_AT_DECLARATION,
				}

				// This field is deprecated in favour of SkippedReason, so we want
				// to discourage further adoption.
				t.Run("no skip reason is valid", func(t *ftt.Test) {
					msg.SkipReason = pb.SkipReason_SKIP_REASON_UNSPECIFIED
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("with skip reason is invalid", func(t *ftt.Test) {
					msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
					assert.Loosely(t, validateTR(msg), should.ErrLike("skip_reason: must not be set in conjuction with status_v2; set skipped_reason.kind instead"))
				})
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
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"@type": structpb.NewStringValue("luci.chromium.org/package.message"),
								"key":   structpb.NewStringValue(strings.Repeat("1", MaxSizeTestMetadataProperties)),
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("exceeds the maximum size"))
				})
				t.Run("no @type with non-empty properties", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike(`properties: must have a field "@type"`))
				})
				t.Run("non-empty properties_schema", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						PropertiesSchema: "package",
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("properties_schema: may not be set"))
				})
				t.Run("invalid @type", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"@type": structpb.NewStringValue("package.message"),
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike(`properties: "@type" value "package.message" must contain at least one "/" character`))
				})
				t.Run("valid properties_schema and non-empty properties", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"@type": structpb.NewStringValue("luci.chromium.org/package.message"),
								"key":   structpb.NewStringValue("1"),
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
			msg.StatusV2 = pb.TestResult_FAILED // Must be failed.
			msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
			msg.Expected = false

			errorMessage1 := "error1"
			errorTrace1 := "trace1"
			errorMessage2 := "error2"
			errorTrace2 := "trace2"
			longErrorMessage := strings.Repeat("a very long error message", 100)
			longErrorTrace := strings.Repeat("a very long error trace", 400)
			msg.FailureReason = &pb.FailureReason{
				Kind: pb.FailureReason_ORDINARY,
				Errors: []*pb.FailureReason_Error{
					{Message: errorMessage1, Trace: errorTrace1},
					{Message: errorMessage2, Trace: errorTrace2},
				},
				TruncatedErrorsCount: 0,
			}

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("must be empty for non-failed results", func(t *ftt.Test) {
				msg.StatusV2 = pb.TestResult_EXECUTION_ERRORED
				assert.Loosely(t, validateTR(msg), should.ErrLike("failure_reason: must not be set when status_v2 is not FAILED"))

				msg.FailureReason = nil
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("must not be empty for failed results", func(t *ftt.Test) {
				msg.FailureReason = nil
				assert.Loosely(t, validateTR(msg), should.ErrLike("failure_reason: must be set when status_v2 is FAILED"))
			})
			t.Run("primary_error_message must not be set", func(t *ftt.Test) {
				// For results using status_v2, this is an OUTPUT_ONLY field.
				msg.FailureReason.PrimaryErrorMessage = errorMessage1
				assert.Loosely(t, validateTR(msg), should.ErrLike("primary_error_message: must not be set when status_v2 is set; set errors instead"))
			})

			t.Run("with legacy status", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_SKIP // Can be anything
				msg.Expected = true             // Can be anything
				msg.StatusV2 = pb.TestResult_STATUS_UNSPECIFIED
				msg.FailureReason.Kind = pb.FailureReason_KIND_UNSPECIFIED

				t.Run("valid", func(t *ftt.Test) {
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})

				t.Run("kind specified is invalid", func(t *ftt.Test) {
					msg.FailureReason.Kind = pb.FailureReason_ORDINARY
					assert.Loosely(t, validateTR(msg), should.ErrLike("failure_reason: kind: please migrate to using status_v2 if you want to set this field"))
				})

				// Primary error message is normally OUTPUT_ONLY and is only
				// allowed for input on legacy results.
				t.Run("only primary error message set", func(t *ftt.Test) {
					msg.FailureReason = &pb.FailureReason{
						PrimaryErrorMessage:  errorMessage1,
						TruncatedErrorsCount: 0,
					}
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("errors and primary error message set, matching", func(t *ftt.Test) {
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
				t.Run("primary_error_message exceeds the maximum limit", func(t *ftt.Test) {
					msg.FailureReason = &pb.FailureReason{
						PrimaryErrorMessage: longErrorMessage,
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("primary_error_message: "+
						"exceeds the maximum"))
				})
			})
			t.Run("kind", func(t *ftt.Test) {
				t.Run("kind unspecified is invalid", func(t *ftt.Test) {
					msg.FailureReason.Kind = pb.FailureReason_KIND_UNSPECIFIED
					assert.Loosely(t, validateTR(msg), should.ErrLike("failure_reason: kind: unspecified"))
				})
				t.Run("unrecognised value is invalid", func(t *ftt.Test) {
					msg.FailureReason.Kind = pb.FailureReason_Kind(len(pb.FailureReason_Kind_name))
					assert.Loosely(t, validateTR(msg), should.ErrLike("failure_reason: kind: invalid value 4"))
				})
			})
			t.Run("errors", func(t *ftt.Test) {
				t.Run("one of the error messages exceeds the maximum limit", func(t *ftt.Test) {
					msg.FailureReason.Errors = []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: longErrorMessage},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike(
						"errors[1]: message: exceeds the maximum size of 1024 "+
							"bytes"))
				})
				t.Run("one of the error traces exceeds the maximum limit", func(t *ftt.Test) {
					msg.FailureReason.Errors = []*pb.FailureReason_Error{
						{Message: errorMessage1, Trace: errorTrace1},
						{Message: errorMessage2, Trace: longErrorTrace},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike(
						"errors[1]: trace: exceeds the maximum size of 4096 "+
							"bytes"))
				})
				t.Run("the total size of the errors list exceeds the limit", func(t *ftt.Test) {
					maxErrorMessage := strings.Repeat(".", 1024)
					maxErrorTrace := strings.Repeat(".", 4096)
					msg.FailureReason.Errors = []*pb.FailureReason_Error{
						{Message: maxErrorMessage, Trace: maxErrorTrace},
						{Message: maxErrorMessage, Trace: maxErrorTrace},
						{Message: maxErrorMessage, Trace: maxErrorTrace},
						{Message: maxErrorMessage, Trace: maxErrorTrace},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike(
						"errors: exceeds the maximum total size of 16384 bytes"))
				})
				t.Run("invalid UTF-8 error message", func(t *ftt.Test) {
					msg.FailureReason.Errors = []*pb.FailureReason_Error{
						{Message: "some test.\xFF"},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("errors[0]: message: is not valid UTF-8"))
				})
				t.Run("invalid UTF-8 error trace", func(t *ftt.Test) {
					msg.FailureReason.Errors = []*pb.FailureReason_Error{
						{Message: errorMessage1, Trace: "some test.\xFF"},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("errors[0]: trace: is not valid UTF-8"))
				})

				t.Run("empty error message", func(t *ftt.Test) {
					msg.FailureReason.Errors = []*pb.FailureReason_Error{
						{Message: ""},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("errors[0]: message: unspecified"))
				})
				t.Run("empty error trace is valid", func(t *ftt.Test) {
					msg.FailureReason.Errors = []*pb.FailureReason_Error{
						{Message: errorMessage1, Trace: ""},
					}
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
			})

			t.Run("invalid truncated error count", func(t *ftt.Test) {
				msg.FailureReason.TruncatedErrorsCount = -1
				assert.Loosely(t, validateTR(msg), should.ErrLike("truncated_errors_count: "+
					"must be non-negative"))
			})
		})
		t.Run("Skipped reason", func(t *ftt.Test) {
			msg.SkippedReason = &pb.SkippedReason{
				Kind:          pb.SkippedReason_SKIPPED_BY_TEST_BODY,
				ReasonMessage: "sid_unittest.cc(183): Platform does not support DeriveCapabilitySidsFromName function.",
			}
			// Skipped reason requires status to be set via Status V2.
			msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
			msg.Expected = false
			msg.StatusV2 = pb.TestResult_SKIPPED

			t.Run("must be set for skipped status", func(t *ftt.Test) {
				msg.SkippedReason = nil

				assert.Loosely(t, validateTR(msg), should.ErrLike("skipped_reason: must be set when status_v2 is SKIPPED"))
			})
			t.Run("may not be set with status v1", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_SKIP
				msg.Expected = true
				msg.StatusV2 = pb.TestResult_STATUS_UNSPECIFIED

				assert.Loosely(t, validateTR(msg), should.ErrLike("skipped_reason: please migrate to using status_v2 if you want to set this field"))
			})

			t.Run("kind", func(t *ftt.Test) {
				t.Run("may not be unspecified", func(t *ftt.Test) {
					msg.SkippedReason.Kind = pb.SkippedReason_KIND_UNSPECIFIED
					assert.Loosely(t, validateTR(msg), should.ErrLike("skipped_reason: kind: cannot be KIND_UNSPECIFIED"))
				})
				t.Run("unrecognised value is invalid", func(t *ftt.Test) {
					msg.SkippedReason.Kind = pb.SkippedReason_Kind(len(pb.SkippedReason_Kind_name))
					assert.Loosely(t, validateTR(msg), should.ErrLike("skipped_reason: kind: invalid value 5"))
				})
			})
			t.Run("reason message", func(t *ftt.Test) {
				t.Run("valid", func(t *ftt.Test) {
					msg.SkippedReason.ReasonMessage = "sid_unittest.cc(183): Platform does not support DeriveCapabilitySidsFromName function."
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("may be empty for some statuses", func(t *ftt.Test) {
					msg.SkippedReason.Kind = pb.SkippedReason_DISABLED_AT_DECLARATION
					msg.SkippedReason.ReasonMessage = ""
				})
				t.Run("invalid UTF-8", func(t *ftt.Test) {
					msg.SkippedReason.ReasonMessage = "some test.\x00"
					assert.Loosely(t, validateTR(msg), should.ErrLike("skipped_reason: reason_message: non-printable rune '\\x00' at byte index 10"))
				})
				t.Run("too long", func(t *ftt.Test) {
					msg.SkippedReason.ReasonMessage = strings.Repeat("a", 1025)
					assert.Loosely(t, validateTR(msg), should.ErrLike("skipped_reason: reason_message: longer than 1024 bytes"))
				})
				t.Run("required for OTHER", func(t *ftt.Test) {
					msg.SkippedReason.Kind = pb.SkippedReason_OTHER
					msg.SkippedReason.ReasonMessage = ""
					assert.Loosely(t, validateTR(msg), should.ErrLike("skipped_reason: reason_message: must be set when skipped reason kind is OTHER"))
				})
				t.Run("required for DEMOTED", func(t *ftt.Test) {
					msg.SkippedReason.Kind = pb.SkippedReason_DEMOTED
					msg.SkippedReason.ReasonMessage = ""
					assert.Loosely(t, validateTR(msg), should.ErrLike("skipped_reason: reason_message: must be set when skipped reason kind is DEMOTED"))
				})
			})
		})
		t.Run("Framework extensions", func(t *ftt.Test) {
			msg.FrameworkExtensions = &pb.FrameworkExtensions{}
			msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
			msg.Expected = false
			msg.StatusV2 = pb.TestResult_PASSED

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("may not be set with status v1", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_PASS
				msg.Expected = true
				msg.StatusV2 = pb.TestResult_STATUS_UNSPECIFIED

				assert.Loosely(t, validateTR(msg), should.ErrLike("framework_extensions: please migrate to using status_v2 if you want to set this field"))
			})
			t.Run("web test", func(t *ftt.Test) {
				msg.FrameworkExtensions.WebTest = &pb.WebTest{
					IsExpected: true,
					Status:     pb.WebTest_PASS,
				}

				t.Run("valid", func(t *ftt.Test) {
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("inconsistent with top-level status_v2", func(t *ftt.Test) {
					t.Run("passed", func(t *ftt.Test) {
						msg.StatusV2 = pb.TestResult_PASSED
						msg.FrameworkExtensions.WebTest = &pb.WebTest{
							IsExpected: true,
							Status:     pb.WebTest_FAIL,
						}
						assert.Loosely(t, validateTR(msg), should.BeNil)

						t.Run("wrong expectation", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.IsExpected = false
							assert.Loosely(t, validateTR(msg), should.ErrLike(
								"framework_extensions: web_test: is_expected: a result with a top-level status_v2 of PASSED must be marked expected"))
						})
						t.Run("wrong status", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.Status = pb.WebTest_SKIP
							assert.Loosely(t, validateTR(msg), should.ErrLike("framework_extensions: web_test: status: a result with a top-level status_v2 of PASSED must not be marked a web test skip"))
						})
					})
					t.Run("failed", func(t *ftt.Test) {
						msg.StatusV2 = pb.TestResult_FAILED
						msg.FailureReason = &pb.FailureReason{
							Kind: pb.FailureReason_ORDINARY,
						}
						msg.FrameworkExtensions.WebTest = &pb.WebTest{
							IsExpected: false,
							Status:     pb.WebTest_FAIL,
						}
						assert.Loosely(t, validateTR(msg), should.BeNil)

						t.Run("wrong expectation", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.IsExpected = true
							assert.Loosely(t, validateTR(msg), should.ErrLike(
								"framework_extensions: web_test: is_expected: a result with a top-level status_v2 of FAILED must be marked unexpected"))
						})
						t.Run("wrong status", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.Status = pb.WebTest_SKIP
							assert.Loosely(t, validateTR(msg), should.ErrLike("framework_extensions: web_test: status: a result with a top-level status_v2 of FAILED must not be marked a web test skip"))
						})
					})
					t.Run("skipped", func(t *ftt.Test) {
						msg.StatusV2 = pb.TestResult_SKIPPED
						msg.SkippedReason = &pb.SkippedReason{
							Kind: pb.SkippedReason_SKIPPED_BY_TEST_BODY,
						}
						msg.FrameworkExtensions.WebTest = &pb.WebTest{
							IsExpected: true,
							Status:     pb.WebTest_SKIP,
						}
						assert.Loosely(t, validateTR(msg), should.BeNil)

						t.Run("wrong expectation", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.IsExpected = false
							assert.Loosely(t, validateTR(msg), should.ErrLike(
								"framework_extensions: web_test: is_expected: a result with a top-level status_v2 of SKIPPED must be marked expected"))
						})
						t.Run("wrong status", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.Status = pb.WebTest_PASS
							assert.Loosely(t, validateTR(msg), should.ErrLike(
								"framework_extensions: web_test: status: a result with a top-level status_v2 of SKIPPED may not be used in conjunction with with a web test fail, pass, crash or timeout"))
						})
					})
					t.Run("execution errored", func(t *ftt.Test) {
						msg.StatusV2 = pb.TestResult_EXECUTION_ERRORED
						msg.FrameworkExtensions.WebTest = &pb.WebTest{
							IsExpected: false,
							Status:     pb.WebTest_SKIP,
						}
						assert.Loosely(t, validateTR(msg), should.BeNil)

						t.Run("wrong expectation", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.IsExpected = true
							assert.Loosely(t, validateTR(msg), should.ErrLike(
								"framework_extensions: web_test: is_expected: a result with a top-level status_v2 of EXECUTION_ERRORED must be marked unexpected"))
						})
						t.Run("wrong status", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.Status = pb.WebTest_FAIL
							assert.Loosely(t, validateTR(msg), should.ErrLike(
								"framework_extensions: web_test: status: a result with a top-level status_v2 of EXECUTION_ERRORED may not be used in conjunction with with a web test fail, pass, crash or timeout"))
						})
					})
					t.Run("precluded", func(t *ftt.Test) {
						msg.StatusV2 = pb.TestResult_PRECLUDED
						msg.FrameworkExtensions.WebTest = &pb.WebTest{
							IsExpected: false,
							Status:     pb.WebTest_SKIP,
						}
						assert.Loosely(t, validateTR(msg), should.BeNil)

						t.Run("wrong expectation", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.IsExpected = true
							assert.Loosely(t, validateTR(msg), should.ErrLike(
								"framework_extensions: web_test: is_expected: a result with a top-level status_v2 of PRECLUDED must be marked unexpected"))
						})
						t.Run("wrong status", func(t *ftt.Test) {
							msg.FrameworkExtensions.WebTest.Status = pb.WebTest_CRASH
							assert.Loosely(t, validateTR(msg), should.ErrLike(
								"framework_extensions: web_test: status: a result with a top-level status_v2 of PRECLUDED may not be used in conjunction with with a web test fail, pass, crash or timeout"))
						})
					})
				})
				t.Run("status", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						msg.FrameworkExtensions.WebTest.Status = pb.WebTest_STATUS_UNSPECIFIED
						assert.Loosely(t, validateTR(msg), should.ErrLike("framework_extensions: web_test: status: cannot be STATUS_UNSPECIFIED"))
					})
					t.Run("invalid", func(t *ftt.Test) {
						msg.FrameworkExtensions.WebTest.Status = pb.WebTest_Status(len(pb.WebTest_Status_name))
						assert.Loosely(t, validateTR(msg), should.ErrLike("framework_extensions: web_test: status: invalid value"))
					})
				})
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
		StatusV2:    pb.TestResult_PASSED,
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
