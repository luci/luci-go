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

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
	typepb "go.chromium.org/luci/resultdb/proto/type"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// validArtifacts returns two valid Artifact samples.
func validArtifacts(now time.Time) (*pb.Artifact, *pb.Artifact) {
	et, _ := ptypes.TimestampProto(now.Add(24 * time.Hour))
	art1 := &pb.Artifact{
		Name:               "this is artifact 1",
		FetchUrl:           "https://foo/bar",
		FetchUrlExpiration: et,
		ContentType:        "text/plain",
		SizeBytes:          1024,
	}
	art2 := &pb.Artifact{
		Name:               "this is artifact 2",
		FetchUrl:           "https://foo/bar/log.png",
		FetchUrlExpiration: et,
		ContentType:        "image/png",
		SizeBytes:          1024,
	}
	return art1, art2
}

// invalidArtifacts returns two invalid Artifact samples.
func invalidArtifacts(now time.Time) (*pb.Artifact, *pb.Artifact) {
	et, _ := ptypes.TimestampProto(now.Add(24 * time.Hour))
	art1 := &pb.Artifact{
		Name:               " this is a bad artifact name.",
		FetchUrl:           "https://foo/bar",
		FetchUrlExpiration: et,
		ContentType:        "text/plain",
		SizeBytes:          1024,
	}
	art2 := &pb.Artifact{
		Name:               "this has a bad fetch url",
		FetchUrl:           "isolate://foo/bar/log.png",
		FetchUrlExpiration: et,
		ContentType:        "image/png",
		SizeBytes:          1024,
	}
	return art1, art2
}

// validTestResult returns a valid TestResult sample.
func validTestResult(now time.Time) *pb.TestResult {
	st, _ := ptypes.TimestampProto(now.Add(-2 * time.Minute))
	art1, art2 := validArtifacts(now)
	return &pb.TestResult{
		Name:            "invocations/a/tests/invocation_id1/results/result_id1",
		TestId:          "this is testID",
		ResultId:        "result_id1",
		Variant:         Variant("a", "b"),
		Expected:        true,
		Status:          pb.TestStatus_PASS,
		SummaryHtml:     "HTML summary",
		StartTime:       st,
		Duration:        ptypes.DurationProto(time.Minute),
		Tags:            StringPairs("k1", "v1"),
		InputArtifacts:  []*pb.Artifact{art1},
		OutputArtifacts: []*pb.Artifact{art2},
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

		Convey("with empty artifacts", func() {
			Convey("in InputArtifacts", func() {
				msg.InputArtifacts = nil
				So(validate(msg), ShouldBeNil)
			})

			Convey("in OutputArtifacts", func() {
				msg.OutputArtifacts = nil
				So(validate(msg), ShouldBeNil)
			})

			// or both
			msg.InputArtifacts = nil
			msg.OutputArtifacts = nil
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
				strings.Repeat("1", 256+1),
				// [[:print:]] matches with [ -~] and [[:graph:]]
				string(163),
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
				string(163),
			}
			for _, in := range badInputs {
				msg.ResultId = in
				So(validate(msg), ShouldErrLike, fieldDoesNotMatch("result_id", resultIDRe))
			}
		})

		Convey("with invalid Variant", func() {
			badInputs := []*typepb.Variant{
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

		Convey("with invalid artifacts", func() {
			bart1, bart2 := invalidArtifacts(now)
			nameErr := fieldDoesNotMatch("name", artifactNameRe)

			Convey("in InputArtifacts", func() {
				msg.InputArtifacts = []*pb.Artifact{bart1}
				expected := fmt.Sprintf("input_artifacts: 0: %s", nameErr)
				So(validate(msg), ShouldErrLike, expected)
			})

			Convey("in OutputArtifacts", func() {
				msg.OutputArtifacts = []*pb.Artifact{bart1}
				expected := fmt.Sprintf("output_artifacts: 0: %s", nameErr)
				So(validate(msg), ShouldErrLike, expected)
			})

			// or both
			msg.InputArtifacts = []*pb.Artifact{bart1}
			msg.OutputArtifacts = []*pb.Artifact{bart2}
			So(validate(msg), ShouldErrLike, fmt.Sprintf("input_artifacts: 0: %s", nameErr))
		})
	})
}

func TestValidateArtifacts(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	art1, art2 := validArtifacts(now)
	bart1, bart2 := invalidArtifacts(now)
	validate := func(arts ...*pb.Artifact) error {
		return ValidateArtifacts(arts)
	}

	Convey("Succeeds", t, func() {
		Convey("with no artifact", func() {
			So(validate(), ShouldBeNil)
		})

		Convey("with an artifact", func() {
			So(validate(art1), ShouldBeNil)
		})
		Convey("with multiple artifacts", func() {
			So(validate(art1, art2), ShouldBeNil)
		})
	})

	Convey("Fails", t, func() {
		expected := fieldDoesNotMatch("name", artifactNameRe)

		Convey("with duplicate names", func() {
			art2.Name = art1.Name
			So(validate(art1, art2), ShouldErrLike, "duplicate name")
		})

		Convey("with an invalid artifact", func() {
			So(validate(bart1), ShouldErrLike, expected)
		})

		Convey("with multiple invalid artifacts", func() {
			So(validate(bart1, bart2), ShouldErrLike, expected)
		})

		Convey("with a mix of valid and invalid artifacts", func() {
			So(validate(art1, bart1), ShouldErrLike, expected)
		})
	})
}

func TestValidateSinkArtifacts(t *testing.T) {
	t.Parallel()
	// valid artifacts
	validArts := map[string]*sinkpb.Artifact{
		"art1": {
			Body:        &sinkpb.Artifact_FilePath{"/tmp/foo"},
			ContentType: "text/plain",
		},
		"art2": {
			Body:        &sinkpb.Artifact_Contents{[]byte("contents")},
			ContentType: "text/plain",
		},
	}
	// invalid artifacts
	invalidArts := map[string]*sinkpb.Artifact{
		"art1": {ContentType: "text/plain"},
	}

	Convey("Succeeds", t, func() {
		Convey("with no artifact", func() {
			So(ValidateSinkArtifacts(nil), ShouldBeNil)
		})

		Convey("with valid artifacts", func() {
			So(ValidateSinkArtifacts(validArts), ShouldBeNil)
		})
	})

	Convey("Fails", t, func() {
		expected := "body: either file_path or contents must be provided"

		Convey("with invalid artifacts", func() {
			So(ValidateSinkArtifacts(invalidArts), ShouldErrLike, expected)
		})

		Convey("with a mix of valid and invalid artifacts", func() {
			invalidArts["art2"] = validArts["art2"]
			So(ValidateSinkArtifacts(invalidArts), ShouldErrLike, expected)
		})
	})
}

func TestValidateArtifact(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	validate := func(art *pb.Artifact) error {
		return ValidateArtifact(art)
	}

	Convey("Succeeds", t, func() {
		art, _ := validArtifacts(now)
		So(validate(art), ShouldBeNil)

		Convey("with no FetchUrlExpiration", func() {
			art.FetchUrlExpiration = nil
			So(validate(art), ShouldBeNil)
		})

		Convey("with empty ContentType", func() {
			art.ContentType = ""
			So(validate(art), ShouldBeNil)
		})

		Convey("with 0 in Size", func() {
			art.SizeBytes = 0
			So(validate(art), ShouldBeNil)
		})
	})

	Convey("Fails", t, func() {
		art, _ := validArtifacts(now)

		Convey("with nil", func() {
			So(validate(nil), ShouldErrLike, unspecified())
		})

		Convey("with empty Name", func() {
			art.Name = ""
			So(validate(art), ShouldErrLike, fieldUnspecified("name"))
		})

		Convey("with invalid Name", func() {
			badInputs := []string{
				" name", "name ", "name ##", "name ?", "name 1@",
				strings.Repeat("n", 256+1),
			}
			for _, in := range badInputs {
				art.Name = in
				So(validate(art), ShouldErrLike, fieldDoesNotMatch("name", artifactNameRe))
			}
		})

		Convey("with empty FetchUrl", func() {
			art.FetchUrl = ""
			So(validate(art), ShouldErrLike, "empty url")
		})

		Convey("with invalid URI", func() {
			art.FetchUrl = "a/b"
			So(validate(art), ShouldErrLike, "invalid URI for request")
		})

		Convey("with unsupported Scheme", func() {
			badInputs := []string{
				"/my_test", "http://host/page", "isolate://a/b",
			}
			for _, in := range badInputs {
				art.FetchUrl = in
				So(validate(art), ShouldErrLike, "the URL scheme is not HTTPS")
			}
		})

		Convey("without host", func() {
			art.FetchUrl = "https://"
			So(validate(art), ShouldErrLike, "missing host")
		})
	})
}
