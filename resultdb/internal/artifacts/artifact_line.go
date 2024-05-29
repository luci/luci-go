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
	"bytes"
	"fmt"
	"math"
	"mime"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ToLogLines retrieves and processes an artifact and returns
// its content as a set of log lines.
// It executes best effort extraction of the timestamp and the severity
// of each log line.
// The `year` is used as a fallback if the year was absent from
// the log line's timestamp.
// The max specified the maximum number of results to return,
// if the max <= 0 it will return all lines.
func ToLogLines(artifactID string, contentType string, content []byte, year, maxLines, maxBytes int) ([]*pb.ArtifactLine, error) {
	isSupported := isLogSupportedArtifact(artifactID, contentType)

	if !isSupported {
		return nil, errors.New("unsupported file type")
	}

	linesContent := bytes.Split(content, []byte("\n"))

	limit := len(linesContent)
	if maxLines < limit && maxLines > 0 {
		limit = maxLines
	}

	ret := make([]*pb.ArtifactLine, 0, limit)

	var totalSize int

	for index, lineContent := range linesContent[:limit] {
		lineContentStr := string(lineContent)
		timestamp, _ := extractTimestamp(lineContentStr, year)
		severity := extractSeverity(lineContentStr)
		line := &pb.ArtifactLine{
			Number:    int64(index) + 1,
			Timestamp: timestamp,
			Severity:  severity,
			Content:   lineContent,
		}
		totalSize += proto.Size(line)

		if totalSize > maxBytes {
			break
		}

		ret = append(ret, line)
	}

	// The file contains lines but no single line fits the max size.
	if len(linesContent) > 0 && len(ret) == 0 {
		return nil, errors.New(fmt.Sprintf("first file line content exceeds maximum size limit: %d bytes", maxBytes))
	}
	return ret, nil
}

var SupportedContentTypes = map[string]bool{
	"application/octet-stream": true,
	"application/x-gzip":       true,
}

func isLogSupportedArtifact(artifactID string, contentType string) bool {
	return isSupportedContentType(contentType) && !isNonTextFile(artifactID)
}

func isSupportedContentType(contentType string) bool {
	// If the content type is not available then
	// we allow other checks to validate if the file is supported.
	if contentType == "" {
		return true
	}
	_, ok := SupportedContentTypes[contentType]
	return strings.HasPrefix(contentType, "text") || ok
}

func isNonTextFile(filePath string) bool {
	ext := filepath.Ext(filePath)
	mimeType := mime.TypeByExtension(ext)
	fmt.Println(filePath)
	if ext != "" {
		return mimeType != "" && !strings.HasPrefix(mimeType, "text/")
	} else {
		return false
	}
}

type logTimestamp struct {
	// The regexp for the supported log timestamp
	Regexp *regexp.Regexp
	// The time layout defining the parser format used by time parser lib
	Layout string
	// Specific time zone, e.g. America/Los_Angeles
	Zone string
	// Specifies whether the timestamp is in sec.millisec epoch format
	IsEpoch bool
	// Specifies whether the year is missing from the timestamp
	YearMissing bool
}

const (
	RFC3339FullDate = "2006-01-02"
	PstTimeZone     = "America/Los_Angeles"
	UtcTimeZone     = "UTC"
)

// List of supported log timestamps.
var supportedLogTimestamps = []logTimestamp{
	// tast and upstart log style: 2021-12-17T07:33:58.266026Z
	{
		Regexp: regexp.MustCompile(
			`(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2}).(\d{3,6})[A-Z]`),
		Layout: "2006-01-02T15:04:05.999999Z07:00",
		Zone:   UtcTimeZone,
	},
	// gziped files timestamps: 03-12 19:08:48.511
	{
		Regexp: regexp.MustCompile(
			`^(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2}).(\d{3})`),
		Layout:      "2006/01-02 15:04:05.000",
		Zone:        UtcTimeZone,
		YearMissing: true,
	},
	// tast steam out timestamps: 2022-10-18 15:10:19
	{
		Regexp:      regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})`),
		Layout:      "2006-01-02 15:04:05",
		Zone:        PstTimeZone,
		YearMissing: false,
	},
	// tast log.txt timestamps: 2022/10/18 15:10:19
	{
		Regexp:      regexp.MustCompile(`(\d{4})\/(\d{2})\/(\d{2}) (\d{2}):(\d{2}):(\d{2})`),
		Layout:      "2006/01/02 15:04:05",
		Zone:        PstTimeZone,
		YearMissing: false,
	},
	// <tast_test>_logs.txt timestamps: Jun  9 22:15:15
	{
		Regexp:      regexp.MustCompile(`(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)  (\d{1,}) (\d{2}):(\d{2}):(\d{2})`),
		Layout:      "2006/Jan  2 15:04:05",
		Zone:        PstTimeZone,
		YearMissing: true,
	},
	// Format for timestamps like: 12/16 22:34:29.742
	{
		Regexp:      regexp.MustCompile(`(\d{2})\/(\d{2}) (\d{2}):(\d{2}):(\d{2})(?:.(\d+))?`),
		Layout:      "2006/01/02 15:04:05.000",
		Zone:        PstTimeZone,
		YearMissing: true,
	},
	// ISO UTC format: 2019-09-30T09:49:45.355431+00:00
	{
		Regexp: regexp.MustCompile(
			`(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2}).(\d{3})\d+[\+\-]\d{2}:\d{2}`),
		Layout: "2006-01-02T15:04:05.000000-07:00",
		Zone:   UtcTimeZone,
	},
	// chrome log style timestamps, eg: 1119/064727.250343
	{
		Regexp:      regexp.MustCompile(`(\d{2})(\d{2})\/(\d{2})(\d{2})(\d{2})\.(\d{6})`),
		Layout:      "2006/0102/150405.000000",
		Zone:        PstTimeZone,
		YearMissing: true,
	},
	// audit.log style timestamps: msg=audit(1571729288.142:53)
	{
		Regexp:      regexp.MustCompile(`msg=audit\((\d+)\.(\d+):(\d+)`),
		IsEpoch:     true,
		Zone:        UtcTimeZone,
		YearMissing: true,
	},
	// journal.log style dates: Nov 01 02:58:43
	{
		Regexp: regexp.MustCompile(
			`(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) ([0-9]{1,}) ([0-9]{2}):([0-9]{2}):([0-9]{2})`),
		Layout:      "2006/Jan 02 15:04:05",
		Zone:        PstTimeZone,
		YearMissing: true,
	},
	// vmlog timestamps: 0620/065447
	{
		Regexp:      regexp.MustCompile(`(\d{2})(\d{2})/(\d{2})(\d{2})(\d{2})`),
		Layout:      "2006/0102/150405",
		Zone:        PstTimeZone,
		YearMissing: true,
	},
	// labstation timestamps: Jun19 20:18
	{
		Regexp: regexp.MustCompile(
			`(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)([0-9]{2}) ([0-9]{2}):([0-9]{2})`),
		Layout:      "2006/Jan02 15:04",
		Zone:        PstTimeZone,
		YearMissing: true,
	},
}

func extractTimestamp(line string, year int) (*timestamppb.Timestamp, error) {
	for _, l := range supportedLogTimestamps {
		m := l.Regexp.FindStringSubmatch(line)

		// If timestamp is already converted to millisec epoc, skip the time
		// parser
		if l.IsEpoch {
			if len(m) > 3 {
				// Convert string to int base 10 with 64 bit size
				sec, err := strconv.ParseInt(m[1], 10, 64)
				if err != nil {
					break
				}

				milliSec, err := strconv.ParseInt(m[2], 10, 64)
				if err != nil {
					break
				}

				nanoSec, err := strconv.ParseInt(m[3], 10, 64)
				if err != nil {
					break
				}

				nanoSec += milliSec * int64(math.Pow10(6))
				return timestamppb.New(time.Unix(sec, nanoSec)), nil
			}
		} else {
			if len(m) > 1 {
				ts := m[0]
				if l.YearMissing {
					ts = fmt.Sprintf("%d/%s", year, ts)
				}
				t, err := time.ParseInLocation(l.Layout, ts, zone(l.Zone))
				if err != nil {
					return nil, err
				}
				return timestamppb.New(t), nil
			}
		}
	}
	return nil, errors.New("invalid timestamp format provided.")
}

// zone gets the timestamp location from a specific zone name.
// If an invalid zone is provided it defaults to UTC.
func zone(name string) *time.Location {
	zone, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return zone
}

// severityRegexes are the recognized log severity patterns found in
// most of the artifacts that we process, severity extraction is best effort
// and more severities can be added in the future.
//
// Matches the following surrounded by whitespace,
// the full wrods can also be surrounded by colon `:` character:
// * FATAL = FATAL or F.
// * ERROR = ERROR or ERR or E.
// * WARNING = WARNING or WARNIN or WARNI or WARN OR W.
// * NOTICE = NOTICE or NOTIC or N.
// * INFO = INFO or I.
// * DEBUG = DEBUG or D.
// * VERBOSE = VERBOSE or VERBOSE1 or V.
var severityRegexes = []regexp.Regexp{
	*regexp.MustCompile(`\s*\:*\(?(FATAL)\)?\:*[\s\|]+|.+\s(F)\s.+`),
	*regexp.MustCompile(`\s*\:*\(?(ERROR|ERR)\)?\:*[\s\|]+|.+\s(E)\s.+`),
	*regexp.MustCompile(`\s*\:*\(?(WARNING|WARNIN|WARNI)\)?\:*[\s\|]+|.+\s(W)\s.+`),
	*regexp.MustCompile(`\s*\:*\(?(NOTICE|NOTIC)\)?\:*[\s\|]+|.+\s(N)\s.+`),
	*regexp.MustCompile(`\s*\:*\(?(INFO)\)?\:*[\s\|]+|.+\s(I)\s.+`),
	*regexp.MustCompile(`\s*\:*\(?(DEBUG)\)?\:*[\s|]+|.+\s(D)\s.+`),
	*regexp.MustCompile(`\s*\:*\(?(VERBOSE|VERBOSE1)\)?\:*[\s|]+|.+\s(V)\s.+`),
}

// Function to extract the log level
func extractSeverity(logLine string) pb.ArtifactLine_Severity {
	var severity string
	for _, severityRegex := range severityRegexes {
		match := severityRegex.FindStringSubmatch(logLine)
		if len(match) > 1 {
			// Find the non-empty capturing group
			for _, group := range match[1:] {
				if len(group) > 0 {
					// Break on the first one as the severity
					// is generally found at the beginning of the line.
					severity = string(group)
					break
				}
			}
		}
	}

	if severity != "" {
		switch {
		case strings.HasPrefix(severity, "F"):
			return pb.ArtifactLine_FATAL
		case strings.HasPrefix(severity, "E"):
			return pb.ArtifactLine_ERROR
		case strings.HasPrefix(severity, "W"):
			return pb.ArtifactLine_WARNING
		case strings.HasPrefix(severity, "N"):
			return pb.ArtifactLine_NOTICE
		case strings.HasPrefix(severity, "I"):
			return pb.ArtifactLine_INFO
		case strings.HasPrefix(severity, "D"):
			return pb.ArtifactLine_DEBUG
		case strings.HasPrefix(severity, "V"):
			return pb.ArtifactLine_VERBOSE
		}
	}
	return pb.ArtifactLine_SEVERITY_UNSPECIFIED
}
