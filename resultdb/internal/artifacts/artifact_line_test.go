// Copyright 2024 The LUCI Authors.
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

package artifacts

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSupportedArtifacts(t *testing.T) {
	t.Parallel()

	Convey(`isLogSupportedArtifact`, t, func() {
		Convey(`given a supported extension, then should return true`, func() {
			isSupported := isLogSupportedArtifact("log.txt", "")
			So(isSupported, ShouldBeTrue)
		})

		Convey(`given a supported content type, then should return true`, func() {
			isSupported := isLogSupportedArtifact("log", "text/content")
			So(isSupported, ShouldBeTrue)
		})

		Convey(`given a unsupported content type, then should return false`, func() {
			isSupported := isLogSupportedArtifact("log", "image/png")
			So(isSupported, ShouldBeFalse)
		})

		Convey(`given a unsupported extension and no content-type, then should return false`, func() {
			isSupported := isLogSupportedArtifact("log.jpg", "")
			So(isSupported, ShouldBeFalse)
		})
	})
}

func TestExtractTimestamp(t *testing.T) {
	t.Parallel()

	Convey("extractTimestamp", t, func() {
		Convey("given valid log lines with various timestamp formats", func() {

			testYear := 2024

			// Test cases for various timestamp formats
			testCases := []struct {
				description   string
				line          string
				year          int
				expectedTime  time.Time
				expectSuccess bool
			}{
				// tast and upstart log style
				{
					"Tast/upstart log",
					"2023-12-17T07:33:58.266026Z",
					testYear,
					time.Date(2023, 12, 17, 7, 33, 58, 266026000, time.UTC),
					true,
				},

				// gziped files timestamps
				{
					"Gzipped files",
					"03-12 19:08:48.511",
					testYear,
					time.Date(testYear, 3, 12, 19, 8, 48, 511000000, time.UTC),
					true,
				},

				// tast steam out timestamps
				{
					"Tast steam out",
					"2022-10-18 15:10:19",
					testYear,
					time.Date(2022, 10, 18, 15, 10, 19, 0, zone(PstTimeZone)),
					true,
				},

				// tast log.txt timestamps
				{
					"Tast log.txt",
					"2022/10/18 15:10:19",
					testYear,
					time.Date(2022, 10, 18, 15, 10, 19, 0, zone(PstTimeZone)),
					true,
				},

				// <tast_test>_logs.txt timestamps
				{
					"Tast test logs",
					"Jun  9 22:15:15",
					testYear,
					time.Date(testYear, time.June, 9, 22, 15, 15, 0, zone(PstTimeZone)),
					true,
				},

				// MM/DD HH:MM:SS format
				{
					"MM/DD HH:MM:SS",
					"12/16 22:34:29.742",
					testYear,
					time.Date(testYear, 12, 16, 22, 34, 29, 742000000, zone(PstTimeZone)),
					true,
				},

				// ISO UTC format
				{
					"ISO UTC",
					"2019-09-30T09:49:45.355431+00:00",
					testYear,
					time.Date(2019, 9, 30, 9, 49, 45, 355431000, time.UTC),
					true,
				},

				// chrome log style timestamps
				{
					"Chrome log",
					"1222/064727.250343",
					testYear,
					time.Date(testYear, 12, 22, 6, 47, 27, 250343000, zone(PstTimeZone)),
					true,
				},

				// audit.log style timestamps
				{
					"Audit log",
					"msg=audit(1571729288.142:00)",
					testYear,
					time.Unix(1571729288, 142000000),
					true,
				},

				// journal.log style dates
				{
					"Journal log",
					"Nov 01 02:58:43",
					testYear,
					time.Date(testYear, time.November, 1, 2, 58, 43, 0, zone(PstTimeZone)),
					true,
				},

				// vmlog timestamps
				{
					"Vmlog",
					"0620/065447",
					testYear,
					time.Date(testYear, 6, 20, 6, 54, 47, 0, zone(PstTimeZone)),
					true,
				},

				// labstation timestamps
				{
					"Labstation",
					"Jun19 20:18",
					testYear,
					time.Date(testYear, time.June, 19, 20, 18, 0, 0, zone(PstTimeZone)),
					true,
				},

				// Invalid format
				{
					"Invalid format",
					"This is not a timestamp",
					testYear,
					time.Time{},
					false,
				},
			}

			for _, tc := range testCases {
				tc := tc

				Convey(tc.description, func() {
					actualTimestamp, err := extractTimestamp(tc.line, tc.year)

					if tc.expectSuccess {
						So(err, ShouldBeNil)

						// Use cmp.Equal to get more detailed error output if timestamps don't match
						So(actualTimestamp, ShouldNotBeNil)
						So(cmp.Equal(actualTimestamp.AsTime(), tc.expectedTime), ShouldBeTrue)
					} else {
						So(err, ShouldNotBeNil)
						So(actualTimestamp, ShouldBeNil)
					}
				})
			}
		})
	})
}

func TestExtractSeverity(t *testing.T) {
	t.Parallel()

	Convey(`extractSeverity`, t, func() {
		Convey("Given various log lines with severity levels", func() {

			testCases := []struct {
				description string
				logLine     string
				expected    pb.ArtifactLine_Severity
			}{
				// FATAL
				{
					description: "FATAL with whitespace",
					logLine:     "  FATAL : This is a fatal error",
					expected:    pb.ArtifactLine_FATAL,
				},
				{
					description: "FATAL with pipe",
					logLine:     "FATAL | Some critical failure",
					expected:    pb.ArtifactLine_FATAL,
				},
				{
					description: "FATAL abbreviated",
					logLine:     "Something F | Important message",
					expected:    pb.ArtifactLine_FATAL,
				},
				// ERROR
				{
					description: "ERROR",
					logLine:     "ERROR: An error occurred",
					expected:    pb.ArtifactLine_ERROR,
				},
				{
					description: "ERROR abbreviated",
					logLine:     "ERR | Another error message",
					expected:    pb.ArtifactLine_ERROR,
				},
				{
					description: "ERROR single",
					logLine:     "Something E Yet another error",
					expected:    pb.ArtifactLine_ERROR,
				},
				// WARNING
				{
					description: "WARNING",
					logLine:     "WARNING: A warning message",
					expected:    pb.ArtifactLine_WARNING,
				},
				{
					description: "WARNING variations",
					logLine:     "WARNIN: Be careful!",
					expected:    pb.ArtifactLine_WARNING,
				},
				{
					description: "WARN abbreviated",
					logLine:     "Something W | Just a note",
					expected:    pb.ArtifactLine_WARNING,
				},
				// NOTICE
				{
					description: "NOTICE",
					logLine:     "NOTICE: Something to note",
					expected:    pb.ArtifactLine_NOTICE,
				},
				{
					description: "NOTICE abbreviated",
					logLine:     "NOTIC: Pay attention",
					expected:    pb.ArtifactLine_NOTICE,
				},
				{
					description: "N abbreviated",
					logLine:     "Something N | FYI",
					expected:    pb.ArtifactLine_NOTICE,
				},
				// INFO
				{
					description: "INFO",
					logLine:     "INFO: Information message",
					expected:    pb.ArtifactLine_INFO,
				},
				{
					description: "INFO abbreviated",
					logLine:     "Something I | More info",
					expected:    pb.ArtifactLine_INFO,
				},
				// DEBUG
				{
					description: "DEBUG",
					logLine:     "DEBUG: Debugging information",
					expected:    pb.ArtifactLine_DEBUG,
				},
				{
					description: "DEBUG abbreviated",
					logLine:     "Something D | Debug output",
					expected:    pb.ArtifactLine_DEBUG,
				},
				// VERBOSE
				{
					description: "VERBOSE",
					logLine:     "VERBOSE: Detailed log",
					expected:    pb.ArtifactLine_VERBOSE,
				},
				{
					description: "VERBOSE1",
					logLine:     "VERBOSE1 | Super detailed log",
					expected:    pb.ArtifactLine_VERBOSE,
				},
				{
					description: "V abbreviated",
					logLine:     "Something V : Very verbose",
					expected:    pb.ArtifactLine_VERBOSE,
				},
				// UNSPECIFIED (no match or empty line)
				{
					description: "No severity",
					logLine:     "This line has no severity level",
					expected:    pb.ArtifactLine_SEVERITY_UNSPECIFIED,
				},
				{
					description: "Empty line",
					logLine:     "",
					expected:    pb.ArtifactLine_SEVERITY_UNSPECIFIED,
				},
				{
					description: "Invalid format",
					logLine:     "No log here.",
					expected:    pb.ArtifactLine_SEVERITY_UNSPECIFIED, // Shouldn't match
				},
			}

			for _, tc := range testCases {
				tc := tc

				Convey(tc.description, func() {
					actual := extractSeverity(tc.logLine)
					So(actual, ShouldEqual, tc.expected)
				})
			}
		})
	})
}

func TestToLogLines(t *testing.T) {

	verifyArtifactLine := func(line *pb.ArtifactLine, timestamp string, severity pb.ArtifactLine_Severity) {
		if timestamp != "" {
			t, err := time.Parse(time.RFC3339Nano, timestamp)
			So(err, ShouldBeNil)
			So(line.Timestamp, ShouldResembleProto, timestamppb.New(t))
		}
		So(line.Severity, ShouldEqual, severity)
	}

	contentString := `2024-05-06T05:58:57.490076Z ERROR test[9617:9617]: log line 1
2024-05-06T05:58:57.491037Z VERBOSE1 test[9617:9617]: [file.cc(845)] log line 2
2024-05-06T05:58:57.577095Z WARNING test[9617:9617]: [file.cc(89)] log line 3.
2024-05-06T05:58:57.577324Z INFO test[9617:9617]: [file.cc(140)] log line 4 {
	log line no timestamp
}`

	Convey(`ToLogLines`, t, func() {
		Convey(`given a list of lines, should return valid line entries`, func() {
			contentBytes := []byte(contentString)
			lines, err := ToLogLines("log.text", "text/log", contentBytes, 2024, -1, 1000000)
			So(err, ShouldBeNil)
			So(len(lines), ShouldEqual, 6)
			So(lines[0].Content, ShouldEqual, []byte("2024-05-06T05:58:57.490076Z ERROR test[9617:9617]: log line 1"))
			verifyArtifactLine(lines[0], "2024-05-06T05:58:57.490076Z", pb.ArtifactLine_ERROR)
			So(lines[1].Content, ShouldEqual, []byte("2024-05-06T05:58:57.491037Z VERBOSE1 test[9617:9617]: [file.cc(845)] log line 2"))
			verifyArtifactLine(lines[1], "2024-05-06T05:58:57.491037Z", pb.ArtifactLine_VERBOSE)
			So(lines[2].Content, ShouldEqual, []byte("2024-05-06T05:58:57.577095Z WARNING test[9617:9617]: [file.cc(89)] log line 3."))
			verifyArtifactLine(lines[2], "2024-05-06T05:58:57.577095Z", pb.ArtifactLine_WARNING)
			So(lines[3].Content, ShouldEqual, []byte("2024-05-06T05:58:57.577324Z INFO test[9617:9617]: [file.cc(140)] log line 4 {"))
			verifyArtifactLine(lines[3], "2024-05-06T05:58:57.577324Z", pb.ArtifactLine_INFO)
			So(lines[4].Content, ShouldEqual, []byte("	log line no timestamp"))
			verifyArtifactLine(lines[4], "", pb.ArtifactLine_SEVERITY_UNSPECIFIED)
		})

		Convey(`given a maxLines, should return lines length equal to maxLines`, func() {
			contentBytes := []byte(contentString)
			lines, err := ToLogLines("log.text", "text/log", contentBytes, 2024, 3, 1000000)
			So(err, ShouldBeNil)
			So(len(lines), ShouldEqual, 3)
		})

		Convey(`given a maxBytes, then total size should be less than or equal to the maxBytes`, func() {
			contentBytes := []byte(contentString)
			lines, err := ToLogLines("log.text", "text/log", contentBytes, 2024, -1, 100)
			So(err, ShouldBeNil)
			So(len(lines), ShouldBeGreaterThan, 0)
			total := 0
			for _, line := range lines {
				total += proto.Size(line)
			}
			So(total, ShouldBeLessThanOrEqualTo, 100)
		})

		Convey(`given initial line exceeding max bytes, then should return error`, func() {
			contentBytes := []byte(contentString)
			lines, err := ToLogLines("log.text", "text/log", contentBytes, 2024, -1, 5)
			So(lines, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err, ShouldErrLike, `first file line content exceeds maximum size limit: 5 bytes`)
		})
	})
}
