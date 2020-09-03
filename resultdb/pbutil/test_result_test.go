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

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/clock/testclock"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// validTestResult returns a valid TestResult sample.
func validTestResult(now time.Time) *pb.TestResult {
	st, _ := ptypes.TimestampProto(now.Add(-2 * time.Minute))
	return &pb.TestResult{
		Name:        "invocations/a/tests/invocation_id1/results/result_id1",
		TestId:      "this is testID",
		ResultId:    "result_id1",
		Variant:     Variant("a", "b"),
		Expected:    true,
		Status:      pb.TestStatus_PASS,
		SummaryHtml: "HTML summary",
		StartTime:   st,
		Duration:    ptypes.DurationProto(time.Minute),
		TestLocation: &pb.TestLocation{
			FileName: "//a_test.go",
			Line:     54,
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
				So(err, ShouldErrLike, doesNotMatch(testIDRe))
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
				string(rune(163)),
			}
			for _, in := range badInputs {
				msg.TestId = in
				So(validate(msg), ShouldErrLike, fieldDoesNotMatch("test_id", testIDRe))
			}
		})

		Convey("with empty ResultID", func() {
			msg.ResultId = ""
			So(validate(msg), ShouldErrLike, fieldUnspecified("result_id"))
		})

		Convey("with invalid ResultID", func() {
			badInputs := []string{
				strings.Repeat("1", 32+1),
				// [[:ascii:]] matches with a char in [\x00-\x7F]
				string(rune(163)),
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

		Convey("with too big summary", func() {
			msg.SummaryHtml = strings.Repeat("☕", maxLenSummaryHTML)
			So(validate(msg), ShouldErrLike, "summary_html: exceeds the maximum size")
		})

		Convey("with invalid StartTime and Duration", func() {
			Convey("because start_time is in the future", func() {
				future, _ := ptypes.TimestampProto(now.Add(time.Hour))
				msg.StartTime = future
				So(validate(msg), ShouldErrLike, "start_time: cannot be in the future")
			})

			Convey("because duration is < 0", func() {
				msg.Duration = ptypes.DurationProto(-1 * time.Minute)
				So(validate(msg), ShouldErrLike, "duration: is < 0")
			})

			Convey("because (start_time + duration) is in the future", func() {
				st, _ := ptypes.TimestampProto(now.Add(-1 * time.Hour))
				msg.StartTime = st
				msg.Duration = ptypes.DurationProto(2 * time.Hour)
				expected := "start_time + duration: cannot be in the future"
				So(validate(msg), ShouldErrLike, expected)
			})
		})

		Convey("with invalid StringPairs", func() {
			msg.Tags = StringPairs("", "")
			So(validate(msg), ShouldErrLike, `"":"": key: unspecified`)
		})

		Convey("Test location", func() {
			Convey("filename", func() {
				Convey("unspecified", func() {
					msg.TestLocation.FileName = ""
					So(validate(msg), ShouldErrLike, "test_location: file_name: unspecified")
				})
				Convey("too long", func() {
					msg.TestLocation.FileName = strings.Repeat("super long", 100)
					So(validate(msg), ShouldErrLike, "test_location: file_name: length exceeds 512")
				})
			})
			Convey("line", func() {
				msg.TestLocation.Line = -1
				So(validate(msg), ShouldErrLike, "test_location: line: must not be negative")
			})
		})
	})
}
