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
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	typepb "go.chromium.org/luci/resultdb/proto/type"

	"github.com/golang/protobuf/ptypes"
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
		Size:               1024,
	}
	art2 := &pb.Artifact{
		Name:               "this is artifact 2",
		FetchUrl:           "https://foo/bar/log.png",
		FetchUrlExpiration: et,
		ContentType:        "image/png",
		Size:               1024,
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
		Size:               1024,
	}
	art2 := &pb.Artifact{
		Name:               "this has a bad fetch url",
		FetchUrl:           "isolate://foo/bar/log.png",
		FetchUrlExpiration: et,
		ContentType:        "image/png",
		Size:               1024,
	}
	return art1, art2
}

var now = testclock.TestRecentTimeUTC

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

	Convey("Succeeds", t, func() {
		msg := validTestResult(now)
		So(ValidateTestResult(now, msg), ShouldBeNil)

		Convey("with invalid Name", func() {
			// ValidateTestResult should skip validating TestResult.Name.
			msg.Name = "this is not a valid name for TestResult.Name"
			expected := doesNotMatch(testResultNameRe)
			So(ValidateTestResultName(msg.Name), ShouldErrLike, expected)
			So(ValidateTestResult(now, msg), ShouldBeNil)
		})

		Convey("with no variant", func() {
			msg.Variant = nil
			So(ValidateTestResult(now, msg), ShouldBeNil)
		})

		Convey("with valid summary", func() {
			goodInputs := []string{
				strings.Repeat("1", maxLenSummaryHTML),
				strings.Repeat("☕", maxLenSummaryHTML),
			}
			for _, in := range goodInputs {
				msg.SummaryHtml = in
				So(ValidateSummaryHTML(in), ShouldBeNil)
				So(ValidateTestResult(now, msg), ShouldBeNil)
			}
		})

		Convey("with empty tags", func() {
			msg.Tags = nil
			So(ValidateTestResult(now, msg), ShouldBeNil)
		})

		Convey("with empty artifacts", func() {
			Convey("in InputArtifacts", func() {
				msg.InputArtifacts = nil
				So(ValidateTestResult(now, msg), ShouldBeNil)
			})

			Convey("in OutputArtifacts", func() {
				msg.OutputArtifacts = nil
				So(ValidateTestResult(now, msg), ShouldBeNil)
			})

			// or both
			msg.InputArtifacts = nil
			msg.OutputArtifacts = nil
			So(ValidateTestResult(now, msg), ShouldBeNil)
		})

	})

	Convey("Fails", t, func() {
		msg := validTestResult(now)
		Convey("with nil", func() {
			So(ValidateTestResult(now, nil), ShouldErrLike, unspecified())
		})

		Convey("with empty TestID", func() {
			msg.TestId = ""
			So(ValidateTestID(""), ShouldErrLike, unspecified())
			So(ValidateTestResult(now, msg), ShouldErrLike, "test_id")
		})

		Convey("with invalid TestID", func() {
			badInputs := []string{
				strings.Repeat("1", 256+1),
				// [[:print:]] matches with [ -~] and [[:graph:]]
				string(163),
			}
			expected := doesNotMatch(testIDRe)
			for _, in := range badInputs {
				msg.TestId = in
				So(ValidateTestID(msg.TestId), ShouldErrLike, expected)
				So(ValidateTestResult(now, msg), ShouldErrLike, "test_id")
			}
		})

		Convey("with empty ResultID", func() {
			msg.ResultId = ""
			So(ValidateResultID(""), ShouldErrLike, unspecified())
			So(ValidateTestResult(now, msg), ShouldErrLike, "result_id")
		})

		Convey("with invalid ResultID", func() {
			badInputs := []string{
				strings.Repeat("1", 32+1),
				// [[:ascii:]] matches with a char in [\x00-\x7F]
				string(163),
			}
			expected := doesNotMatch(resultIDRe)
			for _, in := range badInputs {
				msg.ResultId = in
				So(ValidateResultID(msg.ResultId), ShouldErrLike, expected)
				So(ValidateTestResult(now, msg), ShouldErrLike, "result_id")
			}
		})

		Convey("with invalid Variant", func() {
			badInputs := []*typepb.Variant{
				Variant("", ""),
				Variant("", "val"),
			}
			expected := "key: " + unspecified().Error()
			for _, in := range badInputs {
				msg.Variant = in
				So(ValidateVariant(msg.Variant), ShouldErrLike, expected)
				So(ValidateTestResult(now, msg), ShouldErrLike, "variant")
			}
		})

		Convey("with invalid Status", func() {
			msg.Status = pb.TestStatus(len(pb.TestStatus_name) + 1)
			So(ValidateTestResult(now, msg), ShouldErrLike, "status")
		})

		Convey("with valid summary", func() {
			badInputs := []string{
				strings.Repeat("1", maxLenSummaryHTML+1),
				strings.Repeat("☕", maxLenSummaryHTML+1),
			}
			expected := fmt.Sprintf("exceeds the maximum length of %d", maxLenSummaryHTML)
			for _, in := range badInputs {
				msg.SummaryHtml = in
				So(ValidateSummaryHTML(in), ShouldErrLike, expected)
				So(ValidateTestResult(now, msg), ShouldErrLike, "summary_html")
			}
		})

		Convey("with invalid StartTime and Duration", func() {
			Convey("because start_time is in the future", func() {
				future, _ := ptypes.TimestampProto(now.Add(time.Hour))
				msg.StartTime = future
				expected := "cannot be in the future"
				So(ValidateStartTimeWithDuration(now, msg.StartTime, msg.Duration), ShouldErrLike, expected)
				So(ValidateTestResult(now, msg), ShouldErrLike, "start_time")
			})

			Convey("because duration is < 0", func() {
				nd := ptypes.DurationProto(-1 * time.Minute)
				msg.Duration = nd
				expected := "is < 0"
				So(ValidateStartTimeWithDuration(now, msg.StartTime, msg.Duration), ShouldErrLike, expected)
				So(ValidateTestResult(now, msg), ShouldErrLike, "duration")
			})

			Convey("because (start_time + duration) is future", func() {
				st, _ := ptypes.TimestampProto(now.Add(-1 * time.Hour))
				msg.StartTime = st
				msg.Duration = ptypes.DurationProto(2 * time.Hour)
				expected := "is invalid"
				So(ValidateStartTimeWithDuration(now, msg.StartTime, msg.Duration), ShouldErrLike, expected)
				So(ValidateTestResult(now, msg), ShouldErrLike, "duration")
			})
		})

		Convey("with invalid StringPairs", func() {
			msg.Tags = StringPairs("", "")
			So(ValidateTestResult(now, msg), ShouldErrLike, unspecified())
		})

		Convey("with invalid artifacts", func() {
			bart1, bart2 := invalidArtifacts(now)
			expected := doesNotMatch(artifactNameRe)

			Convey("in InputArtifacts", func() {
				msg.InputArtifacts = []*pb.Artifact{bart1}
				So(ValidateArtifacts(msg.InputArtifacts), ShouldErrLike, expected)
				So(ValidateTestResult(now, msg), ShouldErrLike, "input_artifacts")
			})

			Convey("in OutputArtifacts", func() {
				msg.OutputArtifacts = []*pb.Artifact{bart1}
				So(ValidateArtifacts(msg.OutputArtifacts), ShouldErrLike, expected)
				So(ValidateTestResult(now, msg), ShouldErrLike, "output_artifacts")
			})

			// or both
			msg.InputArtifacts = []*pb.Artifact{bart1}
			msg.OutputArtifacts = []*pb.Artifact{bart2}
			So(ValidateTestResult(now, msg), ShouldErrLike, expected)
		})
	})
}

func TestValidateArtifacts(t *testing.T) {
	t.Parallel()
	art1, art2 := validArtifacts(now)
	bart1, bart2 := invalidArtifacts(now)

	Convey("Succeeds", t, func() {
		Convey("with no artifact", func() {
			in := []*pb.Artifact{}
			So(ValidateArtifacts(in), ShouldBeNil)
		})

		Convey("with an artifact", func() {
			in := []*pb.Artifact{art1}
			So(ValidateArtifacts(in), ShouldBeNil)
		})
		Convey("with multiple artifacts", func() {
			in := []*pb.Artifact{art1, art2}
			So(ValidateArtifacts(in), ShouldBeNil)
		})
	})

	Convey("Fails", t, func() {
		expected := doesNotMatch(artifactNameRe)

		Convey("with an invalid artifact", func() {
			in := []*pb.Artifact{bart1}
			So(ValidateArtifacts(in), ShouldErrLike, expected)
		})

		Convey("with multiple invalid artifacts", func() {
			in := []*pb.Artifact{bart1, bart2}
			So(ValidateArtifacts(in), ShouldErrLike, expected)
		})

		Convey("with a mix of valid and invalid artifacts", func() {
			in := []*pb.Artifact{art1, bart1}
			So(ValidateArtifacts(in), ShouldErrLike, expected)
		})
	})
}

func TestValidateArtifact(t *testing.T) {
	t.Parallel()

	Convey("Succeeds", t, func() {
		msg, _ := validArtifacts(now)
		So(ValidateArtifact(msg), ShouldBeNil)

		Convey("with no FetchUrlExpiration", func() {
			msg.FetchUrlExpiration = nil
			So(ValidateArtifact(msg), ShouldBeNil)
		})

		Convey("with empty ContentType", func() {
			msg.ContentType = ""
			So(ValidateArtifact(msg), ShouldBeNil)
		})

		Convey("with 0 in Size", func() {
			msg.Size = 0
			So(ValidateArtifact(msg), ShouldBeNil)
		})
	})

	Convey("Fails", t, func() {
		msg, _ := validArtifacts(now)

		Convey("with nil", func() {
			So(ValidateArtifact(nil), ShouldErrLike, unspecified())
		})

		Convey("with empty Name", func() {
			msg.Name = ""
			So(ValidateArtifactName(""), ShouldErrLike, unspecified())
			So(ValidateArtifact(msg), ShouldErrLike, "name")
		})

		Convey("with invalid Name", func() {
			badInputs := []string{
				" name", "name ", "name ##", "name ?", "name 1@",
				strings.Repeat("n", 256+1),
			}
			expected := doesNotMatch(artifactNameRe)
			for _, in := range badInputs {
				msg.Name = in
				So(ValidateArtifactName(in), ShouldErrLike, expected)
				So(ValidateArtifact(msg), ShouldErrLike, "name")
			}
		})

		Convey("with empty FetchUrl", func() {
			msg.FetchUrl = ""
			So(ValidateArtifactFetchURL(""), ShouldErrLike, "empty")
			So(ValidateArtifact(msg), ShouldErrLike, "fetch_url")
		})

		Convey("with invalid URI", func() {
			msg.FetchUrl = "foo/bar"
			expected := "invalid URI for request"
			So(ValidateArtifactFetchURL("foo/bar"), ShouldErrLike, expected)
			So(ValidateArtifact(msg), ShouldErrLike, "fetch_url")

		})

		Convey("with unsupported Scheme", func() {
			badInputs := []string{
				"/my_test", "http://host/page", "isolate://foo/bar",
			}
			expected := "the URL scheme is not HTTPS"
			for _, in := range badInputs {
				msg.FetchUrl = in
				So(ValidateArtifactFetchURL(in), ShouldErrLike, expected)
				So(ValidateArtifact(msg), ShouldErrLike, "fetch_url")
			}
		})

		Convey("without host", func() {
			msg.FetchUrl = "https://"
			So(ValidateArtifactFetchURL("https://"), ShouldErrLike, "missing host")
			So(ValidateArtifact(msg), ShouldErrLike, "fetch_url")
		})
	})
}
